// Command isolationprobe is a compiled test fixture for the bwrap process
// boundary. It is never configured outside tests and is not provider evidence.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"syscall"
	"time"

	"undergrid/internal/verification"
)

func main() {
	if verification.CloseInheritedDescriptors() != nil {
		os.Exit(2)
	}
	cleanDescriptors := onlyStandardDescriptors()
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, verification.MaxEnvelopeBytes+1))
	if err != nil || len(raw) > verification.MaxEnvelopeBytes {
		os.Exit(2)
	}
	job, err := verification.ReadJob(bytes.NewReader(raw))
	if err != nil {
		os.Exit(2)
	}
	flags := []bool{
		isAbsent("/etc/passwd"),
		isAbsent("/root"),
		!hasEnv("HOME") && !hasEnv("DATABASE_URL"),
		cannotReachHostNetwork(probeTarget(job.Data)),
		limitsApply(job.Data),
		tmpfsIsBounded(),
		capabilitiesDropped(),
		cleanDescriptors,
	}
	e := verification.Verify(job, time.Now(), os.Getenv("UNDERGRID_VERIFIER_SHA256"))
	all := true
	for _, ok := range flags {
		all = all && ok
	}
	if all {
		e.Code = "isolation_probe_passed"
	} else {
		for i, ok := range flags {
			if !ok {
				e.Code = fmt.Sprintf("isolation_probe_failed_%d", i)
				if i == 7 {
					e.Code = "isolation_probe_failed_fds_" + descriptorNames()
				}
				break
			}
		}
	}
	e.ImageSHA256 = os.Getenv("UNDERGRID_VERIFIER_IMAGE_SHA256")
	_ = json.NewEncoder(os.Stdout).Encode(e)
}

func descriptorNames() string {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return "unreadable"
	}
	var names []string
	for _, entry := range entries {
		link, _ := os.Readlink("/proc/self/fd/" + entry.Name())
		names = append(names, entry.Name()+"="+link)
	}
	return strings.Join(names, "-")
}

func hasEnv(name string) bool { _, ok := os.LookupEnv(name); return ok }
func isAbsent(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, os.ErrNotExist)
}
func probeTarget(data []byte) string {
	var input struct {
		ProbeTarget string `json:"probe_target"`
	}
	_ = json.Unmarshal(data, &input)
	return input.ProbeTarget
}
func cannotReachHostNetwork(target string) bool {
	conn, err := net.DialTimeout("tcp", target, 100*time.Millisecond)
	if conn != nil {
		_ = conn.Close()
		return false
	}
	return err != nil
}
func limitsApply(data []byte) bool {
	var input struct {
		SkipLimits bool `json:"skip_limits"`
	}
	_ = json.Unmarshal(data, &input)
	if input.SkipLimits {
		return true
	}
	b, err := os.ReadFile("/proc/self/limits")
	if err != nil {
		return false
	}
	s := string(b)
	return strings.Contains(s, "Max cpu time              30                  30") &&
		strings.Contains(s, "Max file size             65536               65536") &&
		strings.Contains(s, "Max processes             16                  16") &&
		strings.Contains(s, "Max open files            32                  32")
}
func tmpfsIsBounded() bool {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/tmp", &stat); err != nil {
		return false
	}
	return stat.Blocks*uint64(stat.Bsize) <= 16<<20
}
func capabilitiesDropped() bool {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "CapEff:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "CapEff:")) == "0000000000000000"
		}
	}
	return false
}

func onlyStandardDescriptors() bool {
	directory, err := os.Open("/proc/self/fd")
	if err != nil {
		return false
	}
	defer directory.Close()
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return false
	}
	self := fmt.Sprint(directory.Fd())
	for _, entry := range entries {
		name := entry.Name()
		if name == self || name == "0" || name == "1" || name == "2" {
			continue
		}
		link, _ := os.Readlink("/proc/self/fd/" + name)
		if link != "anon_inode:[eventpoll]" && link != "anon_inode:[eventfd]" {
			return false
		}
	}
	return true
}
