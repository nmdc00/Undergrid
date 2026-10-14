package network

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"undergrid/internal/verification"
)

// This compiled probe is a trusted test fixture, not a verifier, provider, or
// image attestation. It asks the actual configured runner what the OS boundary
// permits and reports those observations through verifier evidence.
func TestBubblewrapVerifierIsolationEnforcesFilesystemNetworkAndLimits(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		skipOrFailIsolationQualification(t, "bubblewrap is not installed")
	}
	if _, err := exec.LookPath("prlimit"); err != nil {
		skipOrFailIsolationQualification(t, "prlimit is not installed")
	}
	probe := buildIsolationProbe(t)
	verifierHash := fileHash(t, probe)
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		t.Fatal(err)
	}
	prlimit, err := exec.LookPath("prlimit")
	if err != nil {
		t.Fatal(err)
	}
	config := IsolatedVerifierConfig{
		LocalArtifactConfig: LocalArtifactConfig{
			Root: privateTestDir(t), Origin: "https://artifacts.example.test", VerifierPath: probe,
			VerifierSHA256: verifierHash, ImageSHA256: strings.Repeat("b", 64),
		},
		BubblewrapPath: bwrap, BubblewrapSHA256: fileHash(t, bwrap),
		PrlimitPath: prlimit, PrlimitSHA256: fileHash(t, prlimit),
		CgroupRoot: "/sys/fs/cgroup",
	}
	s := New("", "development", nil)
	if err := s.SetIsolatedVerifierRuntime(config); err != nil {
		if strings.Contains(err.Error(), "cgroup") {
			if os.Getenv("UNDERGRID_REQUIRE_VERIFIER_ISOLATION") == "1" {
				t.Fatalf("required isolated verifier qualification unavailable: %v", err)
			}
			t.Skipf("host does not delegate hard verifier resource controls: %v", err)
		}
		t.Fatalf("configure isolated verifier: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, e := listener.Accept()
		if e == nil {
			_ = conn.Close()
		}
	}()
	data, err := json.Marshal(map[string]any{"probe_target": listener.Addr().String(), "sources": []any{map[string]any{"source_id": "isolation-probe", "source_url": "https://example.org/"}}})
	if err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"title":"fixture","schema_family":"source_catalog_v1","template_id":"fixture","license_id":"CC0-1.0","attribution":[{"principal_id":"fixture","text":"fixture"}],"inputs":[],"source_declaration":"fixture"}`)
	job := verification.Job{Data: data, Manifest: manifest, InputSHA256: verification.Hash(data), ManifestSHA256: verification.Hash(manifest)}
	evidence, err := s.artifacts.verify(context.Background(), job)
	if err != nil {
		t.Fatalf("isolated process: %v", err)
	}
	if evidence.Code != "isolation_probe_passed" {
		t.Fatalf("probe did not observe required boundary: code=%q evidence=%+v", evidence.Code, evidence)
	}
	if evidence.RuntimeIsolation != "bubblewrap_cgroup_v2_private_namespaces" || evidence.BubblewrapSHA256 != config.BubblewrapSHA256 || evidence.PrlimitSHA256 != config.PrlimitSHA256 {
		t.Fatalf("missing runtime-derived isolation evidence: %+v", evidence)
	}
	if evidence.ImageIdentityEvidence != "operator_configured_digest_attestation_not_runtime_attestation" {
		t.Fatalf("operator digest was represented as runtime image proof: %+v", evidence)
	}
	if evidence.ResourceLimitsEvidence == "" {
		t.Fatalf("missing kernel-enforced resource limit evidence: %+v", evidence)
	}
}

func TestBubblewrapProcessBoundaryWithoutQualification(t *testing.T) {
	// This test observes namespaces and filesystem visibility only. Resource
	// qualification is separately failclosed on delegated cgroup availability.
	probe := buildIsolationProbe(t)
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		skipOrFailIsolationQualification(t, err.Error())
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, e := listener.Accept()
		if e == nil {
			_ = conn.Close()
		}
	}()
	data, err := json.Marshal(map[string]any{"skip_limits": true, "probe_target": listener.Addr().String(), "sources": []any{map[string]any{"source_id": "isolation-probe", "source_url": "https://example.org/"}}})
	if err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"title":"fixture","schema_family":"source_catalog_v1","template_id":"fixture","license_id":"CC0-1.0","attribution":[{"principal_id":"fixture","text":"fixture"}],"inputs":[],"source_declaration":"fixture"}`)
	job := verification.Job{Data: data, Manifest: manifest, InputSHA256: verification.Hash(data), ManifestSHA256: verification.Hash(manifest)}
	verifier, err := os.Open(probe)
	if err != nil {
		t.Fatal(err)
	}
	defer verifier.Close()
	cmd := exec.Command(bwrap, "--unshare-all", "--unshare-user", "--die-with-parent", "--new-session", "--disable-userns", "--proc", "/proc", "--dev", "/dev", "--size", "16777216", "--tmpfs", "/tmp", "--size", "16777216", "--tmpfs", "/work", "--chdir", "/work", "--clearenv", "--setenv", "UNDERGRID_VERIFIER_IMAGE_SHA256", strings.Repeat("d", 64), "--setenv", "UNDERGRID_VERIFIER_SHA256", fileHash(t, probe), "--cap-drop", "ALL", "--uid", "65534", "--gid", "65534", "--ro-bind-fd", "3", "/verifier", "--", "/verifier")
	cmd.ExtraFiles = []*os.File{verifier}
	cmd.Stdin = strings.NewReader(string(canonical(job)))
	var output boundedEvidenceWriter
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err = cmd.Run(); err != nil {
		t.Fatalf("bubblewrap namespace probe: %v output=%s", err, output.String())
	}
	var evidence verification.Evidence
	if err = json.Unmarshal(output.Bytes(), &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence.Code != "isolation_probe_passed" {
		t.Fatalf("sandbox boundary observation failed: %+v", evidence)
	}
}

func TestVerifierAndLauncherBytesAreSealedBeforeExecution(t *testing.T) {
	probe := buildIsolationProbe(t)
	want := fileHash(t, probe)
	sealed, err := sealedExecutable(probe, want, false)
	if err != nil {
		t.Fatalf("seal trusted executable: %v", err)
	}
	defer sealed.Close()
	if _, err = sealed.WriteAt([]byte{0}, 0); err == nil {
		t.Fatal("sealed executable remained writable")
	}
	b, err := os.ReadFile("/proc/self/fd/" + strconv.FormatUint(uint64(sealed.Fd()), 10))
	if err != nil || digest(b) != want {
		t.Fatalf("sealed executable bytes changed: %v", err)
	}
}

func TestSealedVerifierExecutesAndReportsItsCopiedBinaryHash(t *testing.T) {
	verifier := filepath.Join(t.TempDir(), "undergrid-verifier")
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", verifier, "../../cmd/undergrid-verifier")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build verifier: %v %s", err, out)
	}
	wantHash := fileHash(t, verifier)
	sealed, err := sealedExecutable(verifier, wantHash, false)
	if err != nil {
		t.Fatal(err)
	}
	defer sealed.Close()
	data := []byte(`{"sources":[{"source_id":"sealed","source_url":"https://example.org/"}]}`)
	manifest := []byte(`{"title":"fixture","schema_family":"source_catalog_v1","template_id":"fixture","license_id":"CC0-1.0","attribution":[{"principal_id":"fixture","text":"fixture"}],"inputs":[],"source_declaration":"fixture"}`)
	job := verification.Job{Data: data, Manifest: manifest, InputSHA256: verification.Hash(data), ManifestSHA256: verification.Hash(manifest)}
	cmd := exec.Command("/proc/self/fd/3")
	cmd.ExtraFiles = []*os.File{sealed}
	cmd.Env = []string{"UNDERGRID_VERIFIER_IMAGE_SHA256:" + strings.Repeat("e", 64)}
	cmd.Stdin = strings.NewReader(string(canonical(job)))
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("execute sealed verifier: %v", err)
	}
	var evidence verification.Evidence
	if err = json.Unmarshal(output, &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence.VerifierSHA256 != wantHash || evidence.InputSHA256 != job.InputSHA256 || evidence.Result != "passed" {
		t.Fatalf("sealed binary identity or result mismatch: %+v", evidence)
	}
}

func buildIsolationProbe(t *testing.T) string {
	t.Helper()
	probe := filepath.Join(t.TempDir(), "isolation-probe")
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", probe, "./testdata/isolationprobe")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build trusted isolation probe: %v %s", err, out)
	}
	return probe
}

func privateTestDir(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestIsolatedVerifierConfigurationFailsClosedWithoutDelegatedCgroups(t *testing.T) {
	probe := filepath.Join(t.TempDir(), "undergrid-verifier")
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", probe, "../../cmd/undergrid-verifier")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build verifier: %v %s", err, out)
	}
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		skipOrFailIsolationQualification(t, err.Error())
	}
	prlimit, err := exec.LookPath("prlimit")
	if err != nil {
		skipOrFailIsolationQualification(t, err.Error())
	}
	config := IsolatedVerifierConfig{
		LocalArtifactConfig: LocalArtifactConfig{Root: privateTestDir(t), Origin: "https://artifacts.example.test", VerifierPath: probe, VerifierSHA256: fileHash(t, probe), ImageSHA256: strings.Repeat("c", 64)},
		BubblewrapPath:      bwrap, BubblewrapSHA256: fileHash(t, bwrap), PrlimitPath: prlimit, PrlimitSHA256: fileHash(t, prlimit), CgroupRoot: "/sys/fs/cgroup",
	}
	s := New("", "development", nil)
	if err = s.SetLocalArtifactRuntime(config.LocalArtifactConfig); err != nil {
		t.Fatal(err)
	}
	if err = s.SetIsolatedVerifierRuntime(config); err == nil {
		t.Skip("host supports delegated hard verifier resource controls")
	}
	if s.artifacts != nil {
		t.Fatalf("failed qualification left fallback runtime configured: %#v", s.artifacts)
	}
}

func TestMalformedIsolatedEnvironmentConfigClearsEarlierLocalRunner(t *testing.T) {
	root := privateTestDir(t)
	verifier := filepath.Join(t.TempDir(), "trusted-verifier")
	if err := os.WriteFile(verifier, []byte("trusted test fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	s := New("", "development", nil)
	if err := s.SetLocalArtifactRuntime(LocalArtifactConfig{
		Root: root, Origin: "https://artifacts.example.test", VerifierPath: verifier,
		VerifierSHA256: fileHash(t, verifier), ImageSHA256: strings.Repeat("a", 64),
	}); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "isolated-config.json")
	if err := os.WriteFile(config, []byte(`{"unknown":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNDERGRID_ISOLATED_VERIFIER_CONFIG_FILE", config)
	t.Setenv("UNDERGRID_LOCAL_ARTIFACT_CONFIG_FILE", "")
	if err := s.ConfigureArtifactsFromEnvironment(); err == nil {
		t.Fatal("malformed isolated config accepted")
	}
	if s.artifacts != nil {
		t.Fatal("malformed isolated config retained the prior local runner")
	}
}

func skipOrFailIsolationQualification(t *testing.T, reason string) {
	t.Helper()
	if os.Getenv("UNDERGRID_REQUIRE_VERIFIER_ISOLATION") == "1" {
		t.Fatalf("required isolated verifier qualification unavailable: %s", reason)
	}
	t.Skip(reason)
}

func fileHash(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
