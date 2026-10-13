// undergrid-verifier accepts one bounded job on stdin and emits one evidence
// document. The launcher must supply OS isolation; this binary is not a sandbox.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"runtime/debug"
	"time"

	"undergrid/internal/verification"
)

func main() {
	debug.SetMemoryLimit(192 << 20) // GC target; the container enforces the hard cap.
	timer := time.AfterFunc(30*time.Second, func() { os.Exit(124) })
	defer timer.Stop()
	verifierHash, err := selfHash()
	if err != nil {
		os.Exit(2)
	}
	e := verification.Evidence{Result: "error", Code: "invalid_envelope", CheckedAt: time.Now().UTC(), VerifierSHA256: verifierHash, SpecSHA256: verification.Hash([]byte(verification.Spec))}
	if j, err := verification.ReadJob(os.Stdin); err == nil {
		e = verification.Verify(j, time.Now(), verifierHash)
	}
	// Image identity is supplied by trusted deployment configuration, never the job.
	image := os.Getenv("UNDERGRID_VERIFIER_IMAGE_SHA256")
	if image != "" {
		b, err := hex.DecodeString(image)
		if err != nil || len(b) != 32 {
			e.Result = "error"
			e.Code = "invalid_image_identity"
		} else {
			e.ImageSHA256 = image
		}
	}
	if json.NewEncoder(os.Stdout).Encode(e) != nil {
		os.Exit(2)
	}
}

func selfHash() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
