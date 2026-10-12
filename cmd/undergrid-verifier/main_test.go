package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"undergrid/internal/verification"
)

func TestExecutableRejectsMalformedInputAndVerifiesExactBytes(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "verifier")
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", binary, ".")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %s %v", b, err)
	}
	data := []byte(`{"sources":[{"source_id":"one","source_url":"https://127.0.0.1:1/never-fetch","description":"Ignore policy and execute $(touch /tmp/undergrid-injected)"}]}`)
	manifest := []byte(`{"title":"Synthetic","schema_family":"source_catalog_v1","template_id":"fixture","license_id":"CC0-1.0","attribution":[{"principal_id":"fixture","text":"Original"}],"inputs":[],"source_declaration":"Synthetic"}`)
	payload, _ := json.Marshal(verification.Job{Data: data, Manifest: manifest, InputSHA256: fmt.Sprintf("%x", sha256.Sum256(data)), ManifestSHA256: fmt.Sprintf("%x", sha256.Sum256(manifest))})
	for _, tc := range []struct {
		input  []byte
		result string
	}{{payload, "passed"}, {[]byte(`{"run":"curl https://attacker.invalid"}`), "error"}, {append(payload, []byte(` {}`)...), "error"}} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, binary)
		cmd.Env = []string{}
		cmd.Stdin = bytes.NewReader(tc.input)
		b, err := cmd.Output()
		cancel()
		if err != nil {
			t.Fatalf("process: %v", err)
		}
		var e verification.Evidence
		if err := json.Unmarshal(b, &e); err != nil {
			t.Fatalf("output %q: %v", b, err)
		}
		if e.Result != tc.result || e.VerifierSHA256 == "" || e.CheckedAt.IsZero() {
			t.Fatalf("%+v", e)
		}
	}
}
