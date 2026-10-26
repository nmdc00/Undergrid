package network

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSemanticProviderPinsInputAndModelWithoutDisclosureExtras(t *testing.T) {
	cases := []struct {
		name     string
		response func(map[string]any) any
		bad      bool
	}{
		{"valid", func(in map[string]any) any {
			return map[string]any{"model": in["model"], "model_revision": in["model_revision"], "input_sha256": digest([]byte(in["input"].(string))), "embedding": []float64{1, 0, 0}}
		}, false},
		{"wrong_model", func(in map[string]any) any {
			return map[string]any{"model": "other", "model_revision": in["model_revision"], "input_sha256": digest([]byte(in["input"].(string))), "embedding": []float64{1, 0, 0}}
		}, true},
		{"wrong_input", func(in map[string]any) any {
			return map[string]any{"model": in["model"], "model_revision": in["model_revision"], "input_sha256": strings.Repeat("0", 64), "embedding": []float64{1, 0, 0}}
		}, true},
		{"zero", func(in map[string]any) any {
			return map[string]any{"model": in["model"], "model_revision": in["model_revision"], "input_sha256": digest([]byte(in["input"].(string))), "embedding": []float64{0, 0, 0}}
		}, true},
		{"wrong_dimensions", func(in map[string]any) any {
			return map[string]any{"model": in["model"], "model_revision": in["model_revision"], "input_sha256": digest([]byte(in["input"].(string))), "embedding": []float64{1, 0}}
		}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var received map[string]any
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					t.Error(r.Method)
				}
				if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
					t.Error(err)
				}
				if len(received) != 3 || received["input"] != "Approved public projection" {
					t.Errorf("unexpected disclosure: %v", received)
				}
				_ = json.NewEncoder(w).Encode(c.response(received))
			}))
			defer server.Close()
			dir := t.TempDir()
			ca := filepath.Join(dir, "ca.pem")
			if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
				t.Fatal(err)
			}
			config := semanticConfig{Endpoint: server.URL, ProviderID: "fixture", Model: "fixture-model", ModelRevision: "1", Dimensions: 3, PermissionEvidence: "controlled test only", PrivacyPolicySHA256: strings.Repeat("a", 64), AllowPublicProjections: true, TestOnly: true, TLSRootCAFile: ca}
			profile, err := newSemanticProvider(config, "development")
			if err != nil {
				t.Fatal(err)
			}
			vector, err := profile.embed(context.Background(), "Approved public projection")
			if c.bad {
				if err == nil {
					t.Fatal("invalid embedding accepted", vector)
				}
				return
			}
			if err != nil || len(vector) != 3 || math.Abs(float64(vector[0])-1) > 1e-7 {
				t.Fatal(vector, err)
			}
			if profile.hash == "" {
				t.Fatal("missing immutable profile hash")
			}
			if _, err = newSemanticProvider(config, "production"); err == nil {
				t.Fatal("test CA/loopback accepted in production")
			}
		})
	}
}

func TestSemanticProviderRejectsRedirectAndOversizedResponse(t *testing.T) {
	var calls int
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer target.Close()
	for _, redirect := range []bool{true, false} {
		t.Run(map[bool]string{true: "redirect", false: "oversize"}[redirect], func(t *testing.T) {
			source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if redirect {
					http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
				} else {
					_, _ = w.Write([]byte(strings.Repeat("x", 65537)))
				}
			}))
			defer source.Close()
			dir := t.TempDir()
			ca := filepath.Join(dir, "ca.pem")
			_ = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: source.Certificate().Raw}), 0600)
			p, err := newSemanticProvider(semanticConfig{Endpoint: source.URL, ProviderID: "fixture", Model: "fixture", ModelRevision: "1", Dimensions: 3, PermissionEvidence: "fixture", PrivacyPolicySHA256: strings.Repeat("a", 64), AllowPublicProjections: true, TestOnly: true, TLSRootCAFile: ca}, "development")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = p.embed(context.Background(), "Allowed summary"); err == nil {
				t.Fatal("unsafe response accepted")
			}
		})
	}
	if calls != 0 {
		t.Fatal("redirect disclosed input to another endpoint")
	}
}
