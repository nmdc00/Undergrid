package network

// This file owns the isolated verifier process boundary. Artifact bytes and
// persistence remain in artifact_storage.go; this runner only starts the fixed
// verifier with pinned launchers, bounded resources, and cleanup.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"undergrid/internal/verification"
)

func validatePinnedExecutable(path, expected string, rootOwned bool) error {
	f, err := openPinnedExecutable(path, expected, rootOwned)
	if err != nil {
		return err
	}
	return f.Close()
}

func openPinnedExecutable(path, expected string, rootOwned bool) (*os.File, error) {
	if !filepath.IsAbs(path) || !artifactHash(expected) {
		return nil, errors.New("absolute path and lowercase SHA-256 required")
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0022 != 0 || st.Mode()&0111 == 0 {
		_ = f.Close()
		return nil, errors.New("regular executable without group/world write required")
	}
	if rootOwned {
		stat, ok := st.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != 0 {
			_ = f.Close()
			return nil, errors.New("runner executable must be root-owned")
		}
	}
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil || hex.EncodeToString(h.Sum(nil)) != expected {
		_ = f.Close()
		return nil, errors.New("pinned executable digest mismatch")
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func sealedExecutable(path, expected string, rootOwned bool) (*os.File, error) {
	source, err := openPinnedExecutable(path, expected, rootOwned)
	if err != nil {
		return nil, err
	}
	defer source.Close()
	name, err := syscall.BytePtrFromString("undergrid-trusted")
	if err != nil {
		return nil, err
	}
	trap := memfdCreateNumber(runtime.GOARCH)
	if trap == 0 {
		return nil, errors.New("sealed executable memfd is unsupported on this architecture")
	}
	fd, _, errno := syscall.RawSyscall(trap, uintptr(unsafe.Pointer(name)), uintptr(1|2), 0) // CLOEXEC | ALLOW_SEALING
	if errno != 0 {
		return nil, errno
	}
	sealed := os.NewFile(fd, "undergrid-trusted")
	h := sha256.New()
	if _, err = io.Copy(io.MultiWriter(sealed, h), source); err != nil || hex.EncodeToString(h.Sum(nil)) != expected {
		_ = sealed.Close()
		return nil, errors.New("trusted executable changed while sealing")
	}
	if err = sealed.Chmod(0700); err != nil {
		_ = sealed.Close()
		return nil, err
	}
	const fAddSeals = 1033
	const seals = 1 | 2 | 4 | 8 // SEAL_SEAL | SHRINK | GROW | WRITE
	if _, _, errno = syscall.Syscall(syscall.SYS_FCNTL, uintptr(sealed.Fd()), fAddSeals, seals); errno != 0 {
		_ = sealed.Close()
		return nil, errno
	}
	if _, err = sealed.Seek(0, io.SeekStart); err != nil {
		_ = sealed.Close()
		return nil, err
	}
	return sealed, nil
}

func memfdCreateNumber(arch string) uintptr {
	// Linux syscall numbers for the architectures supported by the Go toolchain.
	switch arch {
	case "amd64":
		return 319
	case "386":
		return 356
	case "arm":
		return 385
	case "arm64", "riscv64", "loong64":
		return 279
	case "ppc64", "ppc64le":
		return 360
	case "s390x":
		return 350
	default:
		return 0
	}
}

func staticExecutable(path string) error {
	f, err := elf.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, p := range f.Progs {
		if p.Type == elf.PT_INTERP {
			return errors.New("dynamic interpreter present")
		}
	}
	return nil
}

func validateDelegatedCgroup(root string) error {
	if !filepath.IsAbs(root) {
		return errors.New("isolated verifier requires a delegated cgroup v2 root")
	}
	st, err := os.Lstat(root)
	if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return errors.New("invalid delegated cgroup root")
	}
	owner, ok := st.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uint32(os.Geteuid()) || st.Mode().Perm()&0022 != 0 {
		return errors.New("delegated cgroup root must be owned by the service and not group/world writable")
	}
	controllers, err := os.ReadFile(filepath.Join(root, "cgroup.controllers"))
	if err != nil {
		return errors.New("cgroup v2 controllers unavailable")
	}
	subtree, err := os.ReadFile(filepath.Join(root, "cgroup.subtree_control"))
	if err != nil {
		return errors.New("cgroup v2 delegation unavailable")
	}
	for _, name := range []string{"cpu", "memory", "pids"} {
		if !containsWord(string(controllers), name) || !containsWord(string(subtree), name) {
			return errors.New("delegated cgroup must enable cpu, memory and pids controllers")
		}
	}
	if _, err = os.Stat(filepath.Join(root, "cgroup.kill")); err != nil {
		return errors.New("delegated cgroup must support cgroup.kill cleanup")
	}
	cg, err := newVerifierCgroup(root)
	if err != nil {
		return err
	}
	return cg.close()
}

func containsWord(list, want string) bool {
	for _, item := range strings.Fields(list) {
		if item == want {
			return true
		}
	}
	return false
}

type verifierCgroup struct {
	path string
	fd   *os.File
}

func newVerifierCgroup(root string) (*verifierCgroup, error) {
	for attempt := 0; attempt < 3; attempt++ {
		path := filepath.Join(root, "undergrid-verifier-"+NewID())
		if err := os.Mkdir(path, 0700); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return nil, errors.New("cannot create delegated verifier cgroup")
		}
		limits := map[string]string{"memory.max": "268435456", "memory.swap.max": "0", "pids.max": "32", "cpu.max": "100000 100000"}
		for name, value := range limits {
			if err := os.WriteFile(filepath.Join(path, name), []byte(value), 0600); err != nil {
				_ = os.Remove(path)
				return nil, errors.New("delegated cgroup lacks required hard resource limit: " + name)
			}
		}
		if _, err := os.Stat(filepath.Join(path, "cgroup.kill")); err != nil {
			_ = os.Remove(path)
			return nil, errors.New("delegated cgroup lacks cgroup.kill")
		}
		fd, err := os.Open(path)
		if err != nil {
			_ = os.Remove(path)
			return nil, errors.New("cannot open verifier cgroup")
		}
		return &verifierCgroup{path: path, fd: fd}, nil
	}
	return nil, errors.New("cannot allocate verifier cgroup")
}

func (cg *verifierCgroup) close() error {
	defer func() {
		if cg.fd != nil {
			_ = cg.fd.Close()
		}
	}()
	if err := cg.kill(); err != nil {
		return err
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		events, err := os.ReadFile(filepath.Join(cg.path, "cgroup.events"))
		if err == nil && strings.Contains(string(events), "populated 0\n") {
			if cg.fd != nil {
				_ = cg.fd.Close()
			}
			return os.Remove(cg.path)
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("verifier cgroup did not empty")
}
func (cg *verifierCgroup) kill() error {
	return os.WriteFile(filepath.Join(cg.path, "cgroup.kill"), []byte("1"), 0600)
}
func (rt *localArtifactRuntime) runIsolated(parent context.Context, j verification.Job) (stdout, stderr boundedEvidenceWriter, runErr error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	// Open and hash the exact verifier inode that bubblewrap receives. Pin and
	// recheck host launchers for every run; runner replacement fails closed.
	verifier, err := sealedExecutable(rt.config.VerifierPath, rt.config.VerifierSHA256, false)
	if err != nil {
		return stdout, stderr, err
	}
	defer verifier.Close()
	bwrap, err := sealedExecutable(rt.sandbox.BubblewrapPath, rt.sandbox.BubblewrapSHA256, true)
	if err != nil {
		return stdout, stderr, err
	}
	defer bwrap.Close()
	prlimit, err := sealedExecutable(rt.sandbox.PrlimitPath, rt.sandbox.PrlimitSHA256, true)
	if err != nil {
		return stdout, stderr, err
	}
	defer prlimit.Close()
	cg, err := newVerifierCgroup(rt.sandbox.CgroupRoot)
	if err != nil {
		return stdout, stderr, err
	}
	defer func() {
		if closeErr := cg.close(); runErr == nil && closeErr != nil {
			runErr = closeErr
		}
	}()
	// ExtraFiles become fd 3, 4 and 5. Executing the launchers through those
	// descriptors removes the hash-to-exec pathname race.
	args := []string{
		"--cpu=30:30", "--fsize=65536:65536", "--nproc=16:16", "--nofile=32:32", "--",
		"/proc/self/fd/4", "--unshare-all", "--unshare-user", "--die-with-parent", "--new-session", "--disable-userns",
		"--proc", "/proc", "--dev", "/dev", "--size", "16777216", "--tmpfs", "/tmp",
		"--size", "16777216", "--tmpfs", "/work", "--chdir", "/work", "--clearenv",
		"--setenv", "UNDERGRID_VERIFIER_IMAGE_SHA256", rt.config.ImageSHA256,
		"--setenv", "UNDERGRID_VERIFIER_SHA256", rt.config.VerifierSHA256,
		"--cap-drop", "ALL", "--uid", "65534", "--gid", "65534",
		"--ro-bind-fd", "5", "/verifier", "--", "/verifier",
	}
	cmd := exec.CommandContext(ctx, "/proc/self/fd/3", args...)
	cmd.ExtraFiles = []*os.File{prlimit, bwrap, verifier}
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL, UseCgroupFD: true, CgroupFD: int(cg.fd.Fd())}
	cmd.Stdin = bytes.NewReader(canonical(j))
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Cancel = func() error {
		_ = cg.kill()
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	// Pdeathsig is tied to the creating OS thread on Linux, so keep that thread
	// attached until the child has exited and cgroup cleanup has run.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err = cmd.Start(); err != nil {
		return stdout, stderr, err
	}
	err = cmd.Wait()
	if err != nil {
		return stdout, stderr, err
	}
	if stdout.overflow || stderr.overflow {
		return stdout, stderr, errors.New("verifier output limit")
	}
	return stdout, stderr, nil
}
