// Package verification implements fixed checks over untrusted bytes. It neither
// resolves references nor establishes ownership, factual truth or publication rights.
package verification

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxDataBytes = 10 << 20
const MaxManifestBytes = 64 << 10
const MaxEnvelopeBytes = 15 << 20

// Spec is the complete versioned supported check contract, not a user-supplied schema.
const Spec = `source_catalog_v1/json_schema_v1; utf8; depth<=32; tokens<=500000; sources=1..50000; unique nonempty source_id<=256; absolute https source_url<=4096 without userinfo; optional observed_at/updated_at RFC3339; additional data fields inert; manifest<=64KiB; inputs<=50; exact-byte SHA256`

type Job struct {
	Data           []byte `json:"data"`
	Manifest       []byte `json:"manifest"`
	InputSHA256    string `json:"input_sha256"`
	ManifestSHA256 string `json:"manifest_sha256"`
}
type Evidence struct {
	Result         string    `json:"result"`
	Code           string    `json:"code"`
	InputSHA256    string    `json:"input_sha256"`
	ManifestSHA256 string    `json:"manifest_sha256"`
	SpecSHA256     string    `json:"spec_sha256"`
	ImageSHA256    string    `json:"image_sha256"`
	VerifierSHA256 string    `json:"verifier_sha256"`
	CheckedAt      time.Time `json:"checked_at"`
	SourceCount    int       `json:"source_count"`
}
type Manifest struct {
	Title        string `json:"title"`
	SchemaFamily string `json:"schema_family"`
	TemplateID   string `json:"template_id"`
	LicenseID    string `json:"license_id"`
	Attribution  []struct {
		PrincipalID string `json:"principal_id"`
		Text        string `json:"text"`
	} `json:"attribution"`
	Inputs []struct {
		ArtifactID string `json:"artifact_id"`
		SHA256     string `json:"sha256"`
		LicenseID  string `json:"license_id"`
	} `json:"inputs"`
	SourceDeclaration string `json:"source_declaration"`
}

func Hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func validHash(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && strings.ToLower(s) == s
}

func Verify(j Job, now time.Time, verifierHash string) Evidence {
	e := Evidence{Result: "error", CheckedAt: now.UTC(), VerifierSHA256: verifierHash, SpecSHA256: Hash([]byte(Spec))}
	fail := func(result, code string) Evidence { e.Result = result; e.Code = code; return e }
	if len(j.Data) > MaxDataBytes || len(j.Manifest) > MaxManifestBytes {
		return fail("error", "input_limit")
	}
	e.InputSHA256 = Hash(j.Data)
	e.ManifestSHA256 = Hash(j.Manifest)
	if !validHash(j.InputSHA256) || !validHash(j.ManifestSHA256) || e.InputSHA256 != j.InputSHA256 || e.ManifestSHA256 != j.ManifestSHA256 {
		return fail("error", "hash_mismatch")
	}
	if boundedJSON(j.Manifest) != nil {
		return fail("failed", "invalid_manifest_json")
	}
	var m Manifest
	d := json.NewDecoder(bytes.NewReader(j.Manifest))
	d.DisallowUnknownFields()
	if d.Decode(&m) != nil {
		return fail("unsupported", "manifest_contract")
	}
	if m.SchemaFamily != "source_catalog_v1" {
		return fail("unsupported", "schema_family")
	}
	if m.Title == "" || m.TemplateID == "" || m.LicenseID == "" || m.SourceDeclaration == "" || len(m.Attribution) == 0 || len(m.Attribution) > 50 || len(m.Inputs) > 50 {
		return fail("failed", "manifest_required_fields")
	}
	for _, a := range m.Attribution {
		if a.PrincipalID == "" || a.Text == "" {
			return fail("failed", "attribution_incomplete")
		}
	}
	seenInputs := map[string]bool{}
	for _, in := range m.Inputs {
		if in.ArtifactID == "" || !validHash(in.SHA256) || in.LicenseID == "" || seenInputs[in.ArtifactID] {
			return fail("failed", "input_provenance_incomplete")
		}
		seenInputs[in.ArtifactID] = true
	}
	if boundedJSON(j.Data) != nil {
		return fail("failed", "invalid_data_json")
	}
	var root map[string]json.RawMessage
	var sources []map[string]json.RawMessage
	if json.Unmarshal(j.Data, &root) != nil || json.Unmarshal(root["sources"], &sources) != nil || len(sources) == 0 || len(sources) > 50000 {
		return fail("failed", "source_schema")
	}
	seen := map[string]bool{}
	for _, row := range sources {
		var id, reference string
		if json.Unmarshal(row["source_id"], &id) != nil || strings.TrimSpace(id) == "" || len(id) > 256 || seen[id] {
			return fail("failed", "source_id_not_unique_or_missing")
		}
		seen[id] = true
		if json.Unmarshal(row["source_url"], &reference) != nil {
			return fail("failed", "source_reference")
		}
		u, err := url.Parse(reference)
		if err != nil || len(reference) > 4096 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" {
			return fail("failed", "source_reference")
		}
		for _, key := range []string{"observed_at", "updated_at"} {
			if raw, present := row[key]; present {
				var stamp string
				if json.Unmarshal(raw, &stamp) != nil {
					return fail("failed", "declared_timestamp")
				}
				if _, err := time.Parse(time.RFC3339, stamp); err != nil {
					return fail("failed", "declared_timestamp")
				}
			}
		}
	}
	e.SourceCount = len(sources)
	return fail("passed", "fixed_checks_passed")
}

// The token walk detects duplicate keys before ordinary decoding loses them.
// Bounded depth and token count cover ignored extension fields as well.
func boundedJSON(b []byte) error {
	if !utf8.Valid(b) {
		return errors.New("invalid UTF8")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	tokens := 0
	var value func(int) error
	value = func(depth int) error {
		tokens++
		if depth > 32 || tokens > 500000 {
			return errors.New("JSON resource limit")
		}
		t, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		if delim != '{' && delim != '[' {
			return errors.New("unexpected delimiter")
		}
		keys := map[string]bool{}
		for d.More() {
			if delim == '{' {
				k, err := d.Token()
				if err != nil {
					return err
				}
				key, ok := k.(string)
				if !ok || keys[key] {
					return errors.New("duplicate or invalid key")
				}
				keys[key] = true
				tokens++
			}
			if err := value(depth + 1); err != nil {
				return err
			}
		}
		_, err = d.Token()
		return err
	}
	if err := value(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

// ReadJob admits exactly one bounded envelope. Byte fields use JSON base64,
// preserving exact upload bytes instead of reserializing agent JSON.
func ReadJob(r io.Reader) (Job, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxEnvelopeBytes+1))
	if err != nil {
		return Job{}, err
	}
	if len(b) > MaxEnvelopeBytes {
		return Job{}, errors.New("envelope limit")
	}
	if err := boundedJSON(b); err != nil {
		return Job{}, err
	}
	var j Job
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&j); err != nil {
		return Job{}, fmt.Errorf("invalid envelope: %w", err)
	}
	return j, nil
}
