package network

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

type capabilityOutput struct {
	Kind         string `json:"kind,omitempty"`
	SchemaFamily string `json:"schema_family,omitempty"`
}
type capabilityClaim struct {
	Version        string            `json:"version"`
	Output         *capabilityOutput `json:"output,omitempty"`
	ProvidedFields *[]string         `json:"provided_fields,omitempty"`
	FieldsComplete bool              `json:"fields_complete,omitempty"`
	LicenseIDs     []string          `json:"license_ids,omitempty"`
}

func parseCapabilityClaim(raw []byte) (*capabilityClaim, error) {
	// Bound decoding separately from the normalized declaration: PostgreSQL
	// JSONB spacing must not make the same permitted claim become invalid.
	if len(raw) > 16384 {
		return nil, errors.New("capability claim encoding exceeds budget")
	}
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var claim capabilityClaim
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claim); err != nil {
		return nil, errors.New("invalid capability claim")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid capability claim suffix")
	}
	if claim.Version != "capability-claim-v1" || claim.FieldsComplete && claim.ProvidedFields == nil {
		return nil, errors.New("unsupported or incomplete capability claim")
	}
	valid := func(value string, max int) bool {
		return utf8.ValidString(value) && len(value) <= max && !strings.ContainsRune(value, '\x00') && strings.TrimSpace(value) == value
	}
	if claim.Output != nil && (!valid(claim.Output.Kind, 64) || !valid(claim.Output.SchemaFamily, 128)) {
		return nil, errors.New("invalid declared output")
	}
	list := func(values []string, max int) bool {
		if len(values) > max {
			return false
		}
		seen := map[string]bool{}
		for _, value := range values {
			if value == "" || !valid(value, 128) || seen[value] {
				return false
			}
			seen[value] = true
		}
		return true
	}
	if claim.ProvidedFields != nil && !list(*claim.ProvidedFields, 100) || !list(claim.LicenseIDs, 20) {
		return nil, errors.New("invalid declared field or license set")
	}
	if len(canonical(claim)) > 8192 {
		return nil, errors.New("normalized capability claim exceeds budget")
	}
	return &claim, nil
}
