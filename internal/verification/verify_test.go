package verification

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"
)

func digest(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
func validJob() Job {
	data := []byte(`{"sources":[{"source_id":"one","source_url":"https://example.org/"}]}`)
	manifest := []byte(`{"title":"Synthetic","schema_family":"source_catalog_v1","template_id":"fixture","license_id":"CC0-1.0","attribution":[{"principal_id":"principal-fixture","text":"Original"}],"inputs":[],"source_declaration":"Original synthetic data"}`)
	return Job{Data: data, Manifest: manifest, InputSHA256: digest(data), ManifestSHA256: digest(manifest)}
}

func TestVerification(t *testing.T) {
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	job := validJob()
	got := Verify(job, now, "image-fixture")
	if got.Result != "passed" || got.InputSHA256 != digest(job.Data) || got.ManifestSHA256 != digest(job.Manifest) || got.SpecSHA256 == "" {
		t.Fatalf("%+v", got)
	}
	if fmt.Sprint(got) != fmt.Sprint(Verify(job, now, "image-fixture")) {
		t.Fatal("not deterministic")
	}
	cases := map[string]string{
		"wrong case":     `{"SOURCES":[{"source_id":"a","source_url":"https://example.org"}]}`,
		"null timestamp": `{"sources":[{"source_id":"a","source_url":"https://example.org","observed_at":null}]}`,
		"duplicate IDs":  `{"sources":[{"source_id":"a","source_url":"https://example.org"},{"source_id":"a","source_url":"https://example.org"}]}`,
		"duplicate keys": `{"sources":[],"sources":[{"source_id":"a","source_url":"https://example.org"}]}`,
		"missing source": `{"sources":[{"source_id":"a"}]}`,
		"unsafe URL":     `{"sources":[{"source_id":"a","source_url":"file:///etc/passwd"}]}`,
		"bad timestamp":  `{"sources":[{"source_id":"a","source_url":"https://example.org","observed_at":"yesterday"}]}`,
		"remote schema":  `{"$ref":"https://attacker.invalid/schema","sources":[]}`,
		"deep":           strings.Repeat("[", 40) + strings.Repeat("]", 40),
		"trailing":       `{"sources":[]} {}`,
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			j := validJob()
			j.Data = []byte(data)
			j.InputSHA256 = digest(j.Data)
			if e := Verify(j, now, "image"); e.Result != "failed" {
				t.Fatalf("%+v", e)
			}
		})
	}
	job.InputSHA256 = strings.Repeat("0", 64)
	if e := Verify(job, now, "image"); e.Result != "error" {
		t.Fatalf("hash mismatch: %+v", e)
	}
	job = validJob()
	job.Data = make([]byte, MaxDataBytes+1)
	job.InputSHA256 = digest(job.Data)
	if e := Verify(job, now, "image"); e.Result != "error" {
		t.Fatalf("limit: %+v", e)
	}
	job = validJob()
	job.Manifest = []byte(strings.Replace(string(job.Manifest), "source_catalog_v1", "executable_v1", 1))
	job.ManifestSHA256 = digest(job.Manifest)
	if e := Verify(job, now, "image"); e.Result != "unsupported" {
		t.Fatalf("schema: %+v", e)
	}
}
