package network

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"undergrid/internal/verification"
)

// LocalArtifactConfig is an operator-controlled development adapter. It is not
// provider verification or OS isolation qualification. Production rejects it.
type LocalArtifactConfig struct {
	Root           string `json:"root"`
	Origin         string `json:"origin"`
	VerifierPath   string `json:"verifier_path"`
	VerifierSHA256 string `json:"verifier_sha256"`
	ImageSHA256    string `json:"image_sha256"`
}
type localArtifactRuntime struct{ config LocalArtifactConfig }
type artifactContent []byte

func (s *Service) SetLocalArtifactRuntime(c LocalArtifactConfig) error {
	if s.environment != "development" {
		return errors.New("local artifact adapter is development only; qualified provider and isolated verifier required")
	}
	u, err := url.Parse(c.Origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || !filepath.IsAbs(c.Root) || !filepath.IsAbs(c.VerifierPath) || !artifactHash(c.VerifierSHA256) || !artifactHash(c.ImageSHA256) {
		return errors.New("invalid trusted artifact configuration")
	}
	c.Root = filepath.Clean(c.Root)
	c.Origin = strings.TrimSuffix(c.Origin, "/")
	if err = os.MkdirAll(c.Root, 0700); err != nil {
		return err
	}
	st, err := os.Lstat(c.Root)
	if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm()&0077 != 0 {
		return errors.New("private nonsymlink artifact directory required")
	}
	binary, err := os.ReadFile(c.VerifierPath)
	if err != nil || digest(binary) != c.VerifierSHA256 {
		return errors.New("trusted verifier digest mismatch")
	}
	st, err = os.Lstat(c.VerifierPath)
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0022 != 0 {
		return errors.New("trusted verifier must be a regular non-world-writable executable")
	}
	s.artifacts = &localArtifactRuntime{c}
	return nil
}
func artifactHash(v string) bool {
	b, e := hex.DecodeString(v)
	return e == nil && len(b) == 32 && strings.ToLower(v) == v
}
func (s *Service) ConfigureArtifactsFromEnvironment() error {
	path := os.Getenv("UNDERGRID_LOCAL_ARTIFACT_CONFIG_FILE")
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var config LocalArtifactConfig
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err = d.Decode(&config); err != nil {
		return err
	}
	return s.SetLocalArtifactRuntime(config)
}
func (rt *localArtifactRuntime) blobPath(generation string) (string, error) {
	if !validID(generation) {
		return "", errors.New("invalid generation")
	}
	return filepath.Join(rt.config.Root, generation+".json"), nil
}
func (rt *localArtifactRuntime) read(generation string, size int64) ([]byte, error) {
	path, err := rt.blobPath(generation)
	if err != nil {
		return nil, err
	}
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() {
		return nil, fail(409, "artifact_content_unavailable")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fail(409, "artifact_content_unavailable")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, verification.MaxDataBytes+1))
	if err != nil || int64(len(b)) != size {
		return nil, fail(409, "artifact_content_unavailable")
	}
	return b, nil
}
func (rt *localArtifactRuntime) put(generation string, size int64, r io.Reader) error {
	path, err := rt.blobPath(generation)
	if err != nil {
		return err
	}
	b, err := io.ReadAll(io.LimitReader(r, size+1))
	if err != nil {
		return fail(400, "invalid_upload")
	}
	if int64(len(b)) != size {
		return fail(422, "upload_size_mismatch")
	}
	if existing, err := rt.read(generation, size); err == nil {
		if !bytes.Equal(existing, b) {
			return fail(409, "generation_already_uploaded")
		}
		return nil
	}
	f, err := os.CreateTemp(rt.config.Root, ".quarantine-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	defer f.Close()
	if _, err = f.Write(b); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Chmod(0400); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	// Link publishes a complete immutable generation atomically and never replaces.
	if err = os.Link(name, path); err != nil {
		if existing, readErr := rt.read(generation, size); readErr == nil && bytes.Equal(existing, b) {
			return nil
		}
		return fail(409, "generation_already_uploaded")
	}
	directory, err := os.Open(rt.config.Root)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

type boundedEvidenceWriter struct{ bytes.Buffer }

func (w *boundedEvidenceWriter) Write(p []byte) (int, error) {
	if w.Len()+len(p) > 65536 {
		return 0, errors.New("verifier output limit")
	}
	return w.Buffer.Write(p)
}
func (rt *localArtifactRuntime) verify(ctx context.Context, j verification.Job) (verification.Evidence, error) {
	var e verification.Evidence
	binary, err := os.ReadFile(rt.config.VerifierPath)
	if err != nil || digest(binary) != rt.config.VerifierSHA256 {
		return e, errors.New("configured verifier changed")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, rt.config.VerifierPath)
	// Never inherit database credentials, PATH or agent-defined arguments. No shell.
	cmd.Env = []string{"UNDERGRID_VERIFIER_IMAGE_SHA256=" + rt.config.ImageSHA256}
	cmd.Dir = rt.config.Root
	cmd.Stdin = bytes.NewReader(canonical(j))
	var stdout, stderr boundedEvidenceWriter
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err = cmd.Run(); err != nil {
		return e, err
	}
	d := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	d.DisallowUnknownFields()
	if err = d.Decode(&e); err != nil {
		return e, err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return e, errors.New("extra verifier output")
	}
	if e.InputSHA256 != j.InputSHA256 || e.ManifestSHA256 != j.ManifestSHA256 || e.SpecSHA256 != digest([]byte(verification.Spec)) || e.VerifierSHA256 != rt.config.VerifierSHA256 || e.ImageSHA256 != rt.config.ImageSHA256 || !containsResult(e.Result) {
		return e, errors.New("verifier evidence identity mismatch")
	}
	return e, nil
}
func containsResult(v string) bool {
	return v == "passed" || v == "failed" || v == "error" || v == "unsupported"
}
