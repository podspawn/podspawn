package doctor

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Result struct {
	Name   string
	Status Status
	Detail string
}

type Status int

const (
	Pass Status = iota
	Warn
	Fail
)

type CheckFunc func(ctx context.Context) Result

func AllChecks(cfg CheckConfig) []CheckFunc {
	return []CheckFunc{
		cfg.checkDocker,
		cfg.checkDockerSocket,
		cfg.checkPort22,
		cfg.checkSSHDVersion,
		cfg.checkSSHDConfig,
		cfg.checkAuthKeysCommand,
		cfg.checkPodspawnDir,
		cfg.checkKeyDir,
		cfg.checkStateDir,
		cfg.checkLockDir,
		cfg.checkDiskSpace,
		cfg.checkDefaultImage,
	}
}

type CheckConfig struct {
	SSHDConfigPath string
	KeyDir         string
	StateDir       string
	LockDir        string
	DefaultImage   string
}

func DefaultCheckConfig() CheckConfig {
	return CheckConfig{
		SSHDConfigPath: "/etc/ssh/sshd_config",
		KeyDir:         "/etc/podspawn/keys",
		StateDir:       "/var/lib/podspawn",
		LockDir:        "/var/lib/podspawn/locks",
		DefaultImage:   "ubuntu:24.04",
	}
}

func RunAll(ctx context.Context, cfg CheckConfig, w io.Writer) (passed, warned, failed int) {
	checks := AllChecks(cfg)
	p := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
	for _, check := range checks {
		result := check(ctx)
		switch result.Status {
		case Pass:
			passed++
			p("  [pass] %s\n", result.Name)
		case Warn:
			warned++
			p("  [warn] %s: %s\n", result.Name, result.Detail)
		case Fail:
			failed++
			p("  [FAIL] %s: %s\n", result.Name, result.Detail)
		}
	}
	return
}

func (c CheckConfig) checkPort22(_ context.Context) Result {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:22", 2*time.Second)
	if err != nil {
		return Result{"SSH port 22", Warn, "nothing listening on port 22; enable sshd or check your firewall"}
	}
	defer conn.Close() //nolint:errcheck

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 256)
	n, _ := conn.Read(buf)
	banner := string(buf[:n])

	// OrbStack doesn't send an SSH banner on port 22 (it's a TCP proxy),
	// or it sends the VM's sshd banner which contains "Ubuntu" or similar.
	// Native macOS sshd sends "SSH-2.0-OpenSSH_X.Y" without distro info.
	// Linux system sshd sends "SSH-2.0-OpenSSH_X.Y" or with distro suffix.
	out, _ := exec.Command("lsof", "-i", ":22", "-sTCP:LISTEN", "-Pn", "-t").Output()
	pids := strings.TrimSpace(string(out))
	if pids != "" {
		for _, pid := range strings.Split(pids, "\n") {
			procOut, _ := exec.Command("ps", "-p", pid, "-o", "comm=").Output()
			proc := strings.TrimSpace(string(procOut))
			if strings.Contains(strings.ToLower(proc), "orbstack") {
				return Result{"SSH port 22", Warn,
					"OrbStack is bound to port 22; stop Linux machines with: orb stop --all (Docker still works). Or use: podspawn shell"}
			}
		}
	}

	if strings.Contains(banner, "SSH-2.0-OpenSSH") {
		return Result{"SSH port 22", Pass, strings.TrimSpace(banner)}
	}

	return Result{"SSH port 22", Warn, fmt.Sprintf("unexpected service on port 22 (banner: %q)", strings.TrimSpace(banner))}
}

func (c CheckConfig) checkDocker(ctx context.Context) Result {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.ServerVersion}}").Output()
	if err != nil {
		return Result{"Docker daemon", Fail, "docker not running or not in PATH"}
	}
	version := strings.TrimSpace(string(out))
	return Result{"Docker daemon", Pass, fmt.Sprintf("v%s", version)}
}

func (c CheckConfig) checkDockerSocket(_ context.Context) Result {
	sock := "/var/run/docker.sock"
	info, err := os.Stat(sock)
	if err != nil {
		return Result{"Docker socket", Fail, fmt.Sprintf("%s not found", sock)}
	}
	mode := info.Mode()
	if mode&0006 == 0 {
		return Result{"Docker socket", Warn, fmt.Sprintf("%s not world-accessible (mode %o); non-root users need docker group membership", sock, mode.Perm())}
	}
	return Result{"Docker socket", Pass, ""}
}

func (c CheckConfig) checkSSHDVersion(_ context.Context) Result {
	out, _ := exec.Command("sshd", "-V").CombinedOutput()
	// sshd -V writes to stderr and exits non-zero on some versions
	output := string(out)
	if strings.Contains(output, "OpenSSH") {
		// Extract version number
		for _, line := range strings.Split(output, "\n") {
			if strings.Contains(line, "OpenSSH") {
				return Result{"OpenSSH version", Pass, strings.TrimSpace(line)}
			}
		}
	}
	// Try ssh -V as fallback
	out2, _ := exec.Command("ssh", "-V").CombinedOutput()
	output2 := string(out2)
	if strings.Contains(output2, "OpenSSH") {
		for _, line := range strings.Split(output2, "\n") {
			if strings.Contains(line, "OpenSSH") {
				return Result{"OpenSSH version", Pass, strings.TrimSpace(line)}
			}
		}
	}
	return Result{"OpenSSH version", Fail, "sshd not found; install openssh-server"}
}

func (c CheckConfig) checkSSHDConfig(_ context.Context) Result {
	err := exec.Command("sshd", "-t").Run()
	if err != nil {
		return Result{"sshd config valid", Fail, "sshd -t failed; fix your sshd_config before proceeding"}
	}
	return Result{"sshd config valid", Pass, ""}
}

func (c CheckConfig) checkAuthKeysCommand(_ context.Context) Result {
	data, err := os.ReadFile(c.SSHDConfigPath)
	if err != nil {
		return Result{"AuthorizedKeysCommand", Fail, fmt.Sprintf("cannot read %s: %v", c.SSHDConfigPath, err)}
	}

	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "authorizedkeyscommand") && strings.Contains(trimmed, "podspawn") {
			return Result{"AuthorizedKeysCommand", Pass, "configured for podspawn"}
		}
	}
	return Result{"AuthorizedKeysCommand", Warn, "not configured; run 'podspawn server-setup'"}
}

func (c CheckConfig) checkPodspawnDir(_ context.Context) Result {
	return checkDirExists("/etc/podspawn", "podspawn config dir")
}

func (c CheckConfig) checkKeyDir(_ context.Context) Result {
	r := checkDirExists(c.KeyDir, "key directory")
	if r.Status != Pass {
		return r
	}
	info, _ := os.Stat(c.KeyDir)
	mode := info.Mode().Perm()
	if mode&0002 != 0 {
		return Result{"key directory", Warn, fmt.Sprintf("%s is world-writable (mode %o)", c.KeyDir, mode)}
	}
	return r
}

func (c CheckConfig) checkStateDir(_ context.Context) Result {
	r := checkDirExists(c.StateDir, "state directory")
	if r.Status != Pass {
		return r
	}
	testFile := filepath.Join(c.StateDir, ".doctor-write-test")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		return Result{"state directory", Fail, fmt.Sprintf("%s not writable: %v", c.StateDir, err)}
	}
	_ = os.Remove(testFile)
	return r
}

func (c CheckConfig) checkLockDir(_ context.Context) Result {
	return checkDirExists(c.LockDir, "lock directory")
}

// checkDiskSpace is in disk_unix.go / disk_windows.go

func (c CheckConfig) checkDefaultImage(ctx context.Context) Result {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "docker", "image", "inspect", c.DefaultImage, "--format", "{{.ID}}").Output()
	if err != nil {
		return Result{"default image", Warn, fmt.Sprintf("%s not cached; first connection will pull it (slow)", c.DefaultImage)}
	}
	id := strings.TrimSpace(string(out))
	if len(id) > 19 {
		id = id[:19]
	}
	return Result{"default image", Pass, fmt.Sprintf("%s cached (%s)", c.DefaultImage, id)}
}

func checkDirExists(path, name string) Result {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{name, Fail, fmt.Sprintf("%s does not exist; run 'podspawn server-setup'", path)}
		}
		return Result{name, Fail, fmt.Sprintf("cannot stat %s: %v", path, err)}
	}
	if !info.IsDir() {
		return Result{name, Fail, fmt.Sprintf("%s exists but is not a directory", path)}
	}
	return Result{name, Pass, ""}
}
