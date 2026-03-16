//go:build sshd

package sshd

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/wait"
	"golang.org/x/crypto/ssh"
)

type SSHHarness struct {
	Host       string
	Port       int
	Container  testcontainers.Container
	PrivateKey ssh.Signer
	Docker     *client.Client

	users []string
}

// SetupHarness creates the shared test harness (called from TestMain).
func SetupHarness() (*SSHHarness, error) {
	ctx := context.Background()

	keyData, err := os.ReadFile(testdataPath("keys/testuser"))
	if err != nil {
		return nil, fmt.Errorf("reading test private key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return nil, fmt.Errorf("parsing test private key: %w", err)
	}

	if err := buildBinary(); err != nil {
		return nil, err
	}

	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker not available: %w", err)
	}
	if _, err := dockerClient.Ping(ctx); err != nil {
		return nil, fmt.Errorf("docker daemon not responding: %w", err)
	}

	dockerfile := testDockerfile()

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			FromDockerfile: testcontainers.FromDockerfile{
				Context:    testdataDir(),
				Dockerfile: dockerfile,
			},
			ExposedPorts: []string{"22/tcp"},
			Binds:        []string{"/var/run/docker.sock:/var/run/docker.sock"},
			WaitingFor:   wait.ForListeningPort("22/tcp").WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		return nil, fmt.Errorf("starting sshd container: %w", err)
	}

	if err := initStateDB(ctx, ctr); err != nil {
		ctr.Terminate(ctx) //nolint:errcheck
		return nil, err
	}

	if err := configureSSHD(ctx, ctr); err != nil {
		ctr.Terminate(ctx) //nolint:errcheck
		return nil, err
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		ctr.Terminate(ctx) //nolint:errcheck
		return nil, fmt.Errorf("getting container host: %w", err)
	}
	mappedPort, err := ctr.MappedPort(ctx, "22")
	if err != nil {
		ctr.Terminate(ctx) //nolint:errcheck
		return nil, fmt.Errorf("getting mapped port: %w", err)
	}

	return &SSHHarness{
		Host:       host,
		Port:       mappedPort.Int(),
		Container:  ctr,
		PrivateKey: signer,
		Docker:     dockerClient,
	}, nil
}

// TeardownAll cleans up all containers (called from TestMain after all tests).
func (h *SSHHarness) TeardownAll() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, user := range h.users {
		name := "podspawn-" + user
		_ = h.Docker.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
	}

	if h.Container != nil {
		if err := h.Container.Terminate(ctx); err != nil {
			log.Printf("warning: failed to terminate sshd container: %v", err)
		}
	}
	if h.Docker != nil {
		h.Docker.Close()
	}
}

func (h *SSHHarness) AddUser(t *testing.T, username string) {
	t.Helper()
	ctx := context.Background()

	pubKey, err := os.ReadFile(testdataPath("keys/testuser.pub"))
	if err != nil {
		t.Fatalf("reading test public key: %v", err)
	}

	cmds := [][]string{
		{"useradd", "-m", "-s", "/bin/bash", username},
		{"usermod", "-p", "*", username}, // unlock for key auth (Alpine sshd rejects locked accounts)
		{"mkdir", "-p", "/etc/podspawn/keys"},
		{"sh", "-c", fmt.Sprintf("echo '%s' > /etc/podspawn/keys/%s", strings.TrimSpace(string(pubKey)), username)},
	}
	for _, cmd := range cmds {
		code, _, err := execInContainer(ctx, h.Container, cmd)
		if err != nil {
			t.Fatalf("adding user %s: %v", username, err)
		}
		if code != 0 {
			t.Fatalf("adding user %s: command %v exited %d", username, cmd, code)
		}
	}

	h.users = append(h.users, username)
}

func (h *SSHHarness) Dial(t *testing.T, username string) *ssh.Client {
	t.Helper()
	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(h.PrivateKey),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // test-only
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", h.Host, h.Port)
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		t.Fatalf("SSH dial to %s as %s: %v", addr, username, err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func (h *SSHHarness) dialE(username string) (*ssh.Client, error) {
	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(h.PrivateKey),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // test-only
		Timeout:         10 * time.Second,
	}
	return ssh.Dial("tcp", fmt.Sprintf("%s:%d", h.Host, h.Port), config)
}

// initStateDB creates the SQLite database with world-writable permissions.
// podspawn runs as the authenticated user, so the first user to connect would
// own the DB + WAL files, locking out other users. We initialize as root, then
// chmod so all test users can write.
func initStateDB(ctx context.Context, ctr testcontainers.Container) error {
	cmds := [][]string{
		// Run spawn as root; it creates the DB via state.Open(), then fails trying
		// to exec into a container (which is fine, the DB is already created).
		{"sh", "-c", "/usr/local/bin/podspawn spawn --user _dbinit 2>/dev/null; true"},
		{"sh", "-c", "chmod 666 /var/lib/podspawn/state.db /var/lib/podspawn/state.db-wal /var/lib/podspawn/state.db-shm 2>/dev/null; true"},
	}
	for _, cmd := range cmds {
		_, _, err := execInContainer(ctx, ctr, cmd)
		if err != nil {
			return fmt.Errorf("init state db (%v): %w", cmd, err)
		}
	}

	code, _, err := execInContainer(ctx, ctr, []string{"test", "-f", "/var/lib/podspawn/state.db"})
	if err != nil || code != 0 {
		return fmt.Errorf("state.db was not created by init run; check podspawn binary and config")
	}
	return nil
}

func configureSSHD(ctx context.Context, ctr testcontainers.Container) error {
	sshdBlock := `
# Added by podspawn test harness
AuthorizedKeysCommand /usr/local/bin/podspawn auth-keys %u %t %k
AuthorizedKeysCommandUser root
`
	cmds := [][]string{
		{"sh", "-c", fmt.Sprintf("echo '%s' >> /etc/ssh/sshd_config", sshdBlock)},
		{"sshd", "-t"},
		{"sh", "-c", "kill -HUP $(cat /run/sshd.pid 2>/dev/null || pgrep sshd | head -1)"},
	}
	for _, cmd := range cmds {
		code, output, err := execInContainer(ctx, ctr, cmd)
		if err != nil {
			return fmt.Errorf("configuring sshd (%v): %w", cmd, err)
		}
		if code != 0 {
			return fmt.Errorf("configuring sshd (%v): exited %d, output: %s", cmd, code, output)
		}
	}
	return nil
}

func execInContainer(ctx context.Context, ctr testcontainers.Container, cmd []string) (int, string, error) {
	code, reader, err := ctr.Exec(ctx, cmd, tcexec.Multiplexed())
	if err != nil {
		return 0, "", err
	}
	out, _ := io.ReadAll(reader)
	return code, string(out), nil
}

func buildBinary() error {
	outputPath := filepath.Join(testdataDir(), "podspawn")
	env := os.Environ()
	env = append(env, "GOOS=linux", "GOARCH="+targetArch(), "CGO_ENABLED=0")

	cmd := exec.Command("go", "build", "-o", outputPath, ".")
	moduleRoot := filepath.Join(testdataDir(), "..", "..", "..")
	cmd.Dir = moduleRoot
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("building podspawn binary: %w\n%s", err, output)
	}
	return nil
}

func targetArch() string {
	if runtime.GOARCH == "arm64" {
		return "arm64"
	}
	return "amd64"
}

func testdataDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "testdata")
}

func testdataPath(name string) string {
	return filepath.Join(testdataDir(), name)
}

// testDockerfile returns the Dockerfile to use based on PODSPAWN_TEST_DISTRO.
// Supported: ubuntu (default), debian, rocky, alpine.
func testDockerfile() string {
	distro := os.Getenv("PODSPAWN_TEST_DISTRO")
	switch distro {
	case "debian":
		return "Dockerfile.debian"
	case "rocky":
		return "Dockerfile.rocky"
	case "alpine":
		return "Dockerfile.alpine"
	default:
		return "Dockerfile"
	}
}

func (h *SSHHarness) ContainerExists(t *testing.T, username string) bool {
	t.Helper()
	name := "podspawn-" + username
	_, err := h.Docker.ContainerInspect(context.Background(), name)
	return err == nil
}

func (h *SSHHarness) ContainerID(t *testing.T, username string) string {
	t.Helper()
	name := "podspawn-" + username
	info, err := h.Docker.ContainerInspect(context.Background(), name)
	if err != nil {
		t.Fatalf("inspecting container %s: %v", name, err)
	}
	return info.ID
}

func (h *SSHHarness) ContainerResources(t *testing.T, username string) (cpus float64, memBytes int64) {
	t.Helper()
	name := "podspawn-" + username
	info, err := h.Docker.ContainerInspect(context.Background(), name)
	if err != nil {
		t.Fatalf("inspecting container %s: %v", name, err)
	}
	cpus = float64(info.HostConfig.NanoCPUs) / 1e9
	memBytes = info.HostConfig.Memory
	return
}

func (h *SSHHarness) WaitContainerGone(t *testing.T, username string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !h.ContainerExists(t, username) {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("container podspawn-%s still exists after %v", username, timeout)
}

func (h *SSHHarness) RemoveUserContainer(t *testing.T, username string) {
	t.Helper()
	name := "podspawn-" + username
	err := h.Docker.ContainerRemove(context.Background(), name, container.RemoveOptions{Force: true})
	if err != nil {
		t.Fatalf("removing container %s: %v", name, err)
	}
}

func (h *SSHHarness) ExecInHarness(t *testing.T, cmd []string) (int, string) {
	t.Helper()
	code, output, err := execInContainer(context.Background(), h.Container, cmd)
	if err != nil {
		t.Fatalf("exec in harness (%v): %v", cmd, err)
	}
	return code, output
}

func (h *SSHHarness) SSHPort() string {
	return strconv.Itoa(h.Port)
}
