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
	"runtime"
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

// IsolatedVerifierConfig pins the Linux sandbox runner and the resource-limit
// utility independently from the fixed verifier. ImageSHA256 remains an
// operator-supplied digest attestation; this process runner does not measure a
// container image.
type IsolatedVerifierConfig struct {
	LocalArtifactConfig
	BubblewrapPath   string `json:"bubblewrap_path"`
	BubblewrapSHA256 string `json:"bubblewrap_sha256"`
	PrlimitPath      string `json:"prlimit_path"`
	PrlimitSHA256    string `json:"prlimit_sha256"`
	CgroupRoot       string `json:"cgroup_root"`
}

type verifierRuntimeMode string

const (
	developmentProcessMode verifierRuntimeMode = "development_process_not_qualified"
	bubblewrapMode         verifierRuntimeMode = "bubblewrap_cgroup_v2_private_namespaces"
)

type localArtifactRuntime struct {
	config  LocalArtifactConfig
	mode    verifierRuntimeMode
	sandbox IsolatedVerifierConfig
}
type artifactContent []byte

func (s *Service) SetLocalArtifactRuntime(c LocalArtifactConfig) error {
	if s.environment != "development" {
		return errors.New("local artifact adapter is development only; qualified provider and isolated verifier required")
	}
	if err := validateLocalArtifactConfig(&c); err != nil {
		return err
	}
	s.artifacts = &localArtifactRuntime{config: c, mode: developmentProcessMode}
	return nil
}

// SetIsolatedVerifierRuntime selects a fail-closed bubblewrap runner for the
// development artifact adapter. It does not qualify the local artifact store
// for production or establish image/provider identity.
func (s *Service) SetIsolatedVerifierRuntime(c IsolatedVerifierConfig) error {
	// A failed qualified configuration must not leave a previously configured
	// development process runner available as an implicit fallback.
	s.artifacts = nil
	if s.environment != "development" {
		return errors.New("local artifact adapter is development only; qualified provider and isolated verifier required")
	}
	if err := validateLocalArtifactConfig(&c.LocalArtifactConfig); err != nil {
		return err
	}
	if runtime.GOOS != "linux" {
		return errors.New("bubblewrap verifier requires Linux")
	}
	if err := validatePinnedExecutable(c.BubblewrapPath, c.BubblewrapSHA256, true); err != nil {
		return errors.New("invalid pinned bubblewrap executable: " + err.Error())
	}
	if err := validatePinnedExecutable(c.PrlimitPath, c.PrlimitSHA256, true); err != nil {
		return errors.New("invalid pinned prlimit executable: " + err.Error())
	}
	if err := staticExecutable(c.VerifierPath); err != nil {
		return errors.New("isolated verifier must be statically linked: " + err.Error())
	}
	if err := validateDelegatedCgroup(c.CgroupRoot); err != nil {
		return err
	}
	s.artifacts = &localArtifactRuntime{config: c.LocalArtifactConfig, mode: bubblewrapMode, sandbox: c}
	return nil
}

func validateLocalArtifactConfig(c *LocalArtifactConfig) error {
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
	if err = validatePinnedExecutable(c.VerifierPath, c.VerifierSHA256, false); err != nil {
		return errors.New("trusted verifier: " + err.Error())
	}
	return nil
}

func artifactHash(v string) bool {
	b, e := hex.DecodeString(v)
	return e == nil && len(b) == 32 && strings.ToLower(v) == v
}
func (s *Service) ConfigureArtifactsFromEnvironment() error {
	isolatedPath := os.Getenv("UNDERGRID_ISOLATED_VERIFIER_CONFIG_FILE")
	if isolatedPath != "" {
		// An invalid isolated configuration must not leave an older local
		// process runner active as an implicit fallback.
		s.artifacts = nil
		b, err := os.ReadFile(isolatedPath)
		if err != nil {
			return err
		}
		var config IsolatedVerifierConfig
		d := json.NewDecoder(bytes.NewReader(b))
		d.DisallowUnknownFields()
		if err = d.Decode(&config); err != nil {
			return err
		}
		var extra any
		if d.Decode(&extra) != io.EOF {
			return errors.New("extra isolated verifier config")
		}
		return s.SetIsolatedVerifierRuntime(config)
	}
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

type boundedEvidenceWriter struct {
	bytes.Buffer
	overflow bool
}

func (w *boundedEvidenceWriter) Write(p []byte) (int, error) {
	remaining := 65536 - w.Len()
	if remaining <= 0 {
		w.overflow = true
		return len(p), nil
	}
	if len(p) > remaining {
		w.overflow = true
		_, _ = w.Buffer.Write(p[:remaining])
		return len(p), nil
	}
	return w.Buffer.Write(p)
}
func (rt *localArtifactRuntime) verify(ctx context.Context, j verification.Job) (verification.Evidence, error) {
	var e verification.Evidence
	var err error
	if rt.mode != bubblewrapMode {
		binary, readErr := os.ReadFile(rt.config.VerifierPath)
		if readErr != nil || digest(binary) != rt.config.VerifierSHA256 {
			return e, errors.New("configured verifier changed")
		}
	}
	var stdout boundedEvidenceWriter
	if rt.mode == bubblewrapMode {
		stdout, _, err = rt.runIsolated(ctx, j)
	} else {
		stdout, _, err = rt.runDevelopment(ctx, j)
	}
	if err != nil {
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
	// Runtime claims are assigned by the trusted parent after the process exits;
	// the configured verifier cannot self-assert its isolation state.
	e.RuntimeIsolation = string(rt.mode)
	e.BubblewrapSHA256 = ""
	e.PrlimitSHA256 = ""
	e.ResourceLimitsEvidence = ""
	e.ImageIdentityEvidence = "operator_configured_digest_attestation_not_runtime_attestation"
	if rt.mode == bubblewrapMode {
		e.BubblewrapSHA256 = rt.sandbox.BubblewrapSHA256
		e.PrlimitSHA256 = rt.sandbox.PrlimitSHA256
		e.ResourceLimitsEvidence = "cgroup_v2_cpu_1_memory_256m_swap_0_pids_32;rlimit_cpu_30s_fsize_64k_nproc_16_nofile_32;tmpfs_16m;stdout_and_stderr_each_64k"
	} else {
		e.ResourceLimitsEvidence = "not_qualified"
	}
	return e, nil
}

func (rt *localArtifactRuntime) runDevelopment(ctx context.Context, j verification.Job) (boundedEvidenceWriter, boundedEvidenceWriter, error) {
	var stdout, stderr boundedEvidenceWriter
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, rt.config.VerifierPath)
	cmd.Env = []string{"UNDERGRID_VERIFIER_IMAGE_SHA256=" + rt.config.ImageSHA256}
	cmd.Dir = rt.config.Root
	cmd.Stdin = bytes.NewReader(canonical(j))
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout, stderr, err
	}
	if stdout.overflow || stderr.overflow {
		return stdout, stderr, errors.New("verifier output limit")
	}
	return stdout, stderr, nil
}

func containsResult(v string) bool {
	return v == "passed" || v == "failed" || v == "error" || v == "unsupported"
}
