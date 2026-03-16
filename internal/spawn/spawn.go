package spawn

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/podspawn/podspawn/internal/audit"
	"github.com/podspawn/podspawn/internal/config"
	"github.com/podspawn/podspawn/internal/lock"
	"github.com/podspawn/podspawn/internal/podfile"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/state"
	"golang.org/x/term"
)

const sftpServerPath = "/usr/lib/openssh/sftp-server"
const containerAgentDir = "/run/ssh-agent"

type Session struct {
	Username      string
	ProjectName   string                // empty = default image session
	Project       *config.ProjectConfig // nil = use default image
	UserOverrides *config.UserOverrides // nil = no per-user overrides
	Runtime       runtime.Runtime
	Image         string
	Shell         string
	CPUs          float64
	Memory        int64
	Store         state.SessionStore // nil = Phase 0 mode (destroy on exit)
	LockDir       string
	GracePeriod   time.Duration
	MaxLifetime   time.Duration
	Mode          string // "grace-period" | "destroy-on-disconnect"
	Security      config.SecurityConfig
	Audit         *audit.Logger // nil = no audit logging

	pf *podfile.Podfile // cached after first parse
}

func (s *Session) agentHostDir() string {
	return filepath.Join(os.TempDir(), "podspawn-agent-"+s.Username)
}

func (s *Session) containerName() string {
	if s.ProjectName != "" {
		return "podspawn-" + s.Username + "-" + s.ProjectName
	}
	return "podspawn-" + s.Username
}

func (s *Session) Run(ctx context.Context) (int, error) {
	if s.Store != nil {
		containerName, isNew, err := s.ensureContainerWithState(ctx)
		if err != nil {
			return 1, err
		}
		s.ensurePodfileParsed()
		s.runHooks(ctx, containerName, isNew)
		return s.routeSession(ctx, containerName)
	}

	// Phase 0 fallback: no state store
	return s.ensureContainerLegacy(ctx, s.containerName())
}

// ensureContainerWithState uses locking + state store to manage the container.
func (s *Session) ensureContainerWithState(ctx context.Context) (string, bool, error) {
	containerName := s.containerName()

	unlock, err := lock.Acquire(s.LockDir, s.Username)
	if err != nil {
		return "", false, fmt.Errorf("acquiring lock: %w", err)
	}
	defer unlock()

	s.reconcileUser(ctx)

	sess, err := s.Store.GetSession(s.Username, s.ProjectName)
	if err != nil {
		return "", false, fmt.Errorf("checking session state: %w", err)
	}

	if sess != nil {
		alive, existsErr := s.Runtime.ContainerExists(ctx, sess.ContainerName)
		if existsErr != nil {
			slog.Warn("failed to check container, assuming alive", "container", sess.ContainerName, "error", existsErr)
			alive = true
		}
		if !alive {
			slog.Warn("stale session, container gone", "user", s.Username, "container", sess.ContainerName)
			if err := s.Store.DeleteSession(s.Username, s.ProjectName); err != nil {
				return "", false, fmt.Errorf("cleaning stale session: %w", err)
			}
			sess = nil
		}
	}

	if sess != nil {
		if sess.Status == "grace_period" {
			slog.Info("cancelling grace period", "user", s.Username)
			if err := s.Store.CancelGracePeriod(s.Username, s.ProjectName); err != nil {
				return "", false, err
			}
		}
		if _, err := s.Store.UpdateConnections(s.Username, s.ProjectName, 1); err != nil {
			return "", false, err
		}
		slog.Info("reattaching to container", "name", sess.ContainerName, "connections", sess.Connections+1)
		s.Audit.Connect(s.Username, s.ProjectName, sess.ContainerName, sess.Connections+1)
		return sess.ContainerName, false, nil
	}

	image, env, networkID, serviceIDs, err := s.resolveProject(ctx)
	if err != nil {
		return "", false, err
	}

	mounts := s.agentMount()
	capDrop, capAdd, secOpt, pidsLimit, readonlyRoot, tmpfs, runtimeName := s.securityOpts()

	slog.Info("creating container", "name", containerName, "image", image)
	id, err := s.Runtime.CreateContainer(ctx, runtime.ContainerOpts{
		Name:           containerName,
		Image:          image,
		Cmd:            []string{"sleep", "infinity"},
		Env:            env,
		Mounts:         mounts,
		CPUs:           s.CPUs,
		Memory:         s.Memory,
		NetworkID:      networkID,
		NetworkName:    containerName,
		CapDrop:        capDrop,
		CapAdd:         capAdd,
		SecurityOpt:    secOpt,
		PidsLimit:      pidsLimit,
		ReadonlyRootfs: readonlyRoot,
		Tmpfs:          tmpfs,
		RuntimeName:    runtimeName,
		Labels: map[string]string{
			"managed-by":    "podspawn",
			"podspawn-user": s.Username,
		},
	})
	if err != nil {
		s.cleanupServicesAndNetwork(ctx, serviceIDs, networkID)
		return "", false, err
	}
	if err := s.Runtime.StartContainer(ctx, containerName); err != nil {
		_ = s.Runtime.RemoveContainer(ctx, containerName)
		s.cleanupServicesAndNetwork(ctx, serviceIDs, networkID)
		return "", false, err
	}

	now := time.Now().UTC()
	if err := s.Store.CreateSession(&state.Session{
		User:          s.Username,
		Project:       s.ProjectName,
		ContainerID:   id,
		ContainerName: containerName,
		Image:         image,
		Status:        "running",
		Connections:   1,
		CreatedAt:     now,
		LastActivity:  now,
		MaxLifetime:   now.Add(s.MaxLifetime),
		NetworkID:     networkID,
		ServiceIDs:    strings.Join(serviceIDs, ","),
	}); err != nil {
		_ = s.Runtime.RemoveContainer(ctx, containerName)
		s.cleanupServicesAndNetwork(ctx, serviceIDs, networkID)
		return "", false, fmt.Errorf("recording session: %w", err)
	}

	s.Audit.ContainerCreate(s.Username, s.ProjectName, containerName, image)
	s.Audit.Connect(s.Username, s.ProjectName, containerName, 1)
	return containerName, true, nil
}

// ensureContainerLegacy is the Phase 0 path: no state, no locking.
func (s *Session) ensureContainerLegacy(ctx context.Context, containerName string) (int, error) {
	exists, err := s.Runtime.ContainerExists(ctx, containerName)
	if err != nil {
		return 1, fmt.Errorf("checking container %s: %w", containerName, err)
	}

	if !exists {
		capDrop, capAdd, secOpt, pidsLimit, readonlyRoot, tmpfs, runtimeName := s.securityOpts()
		slog.Info("creating container", "name", containerName, "image", s.Image)
		_, err := s.Runtime.CreateContainer(ctx, runtime.ContainerOpts{
			Name:           containerName,
			Image:          s.Image,
			Cmd:            []string{"sleep", "infinity"},
			Mounts:         s.agentMount(),
			CPUs:           s.CPUs,
			Memory:         s.Memory,
			CapDrop:        capDrop,
			CapAdd:         capAdd,
			SecurityOpt:    secOpt,
			PidsLimit:      pidsLimit,
			ReadonlyRootfs: readonlyRoot,
			Tmpfs:          tmpfs,
			RuntimeName:    runtimeName,
			Labels: map[string]string{
				"managed-by":    "podspawn",
				"podspawn-user": s.Username,
			},
		})
		if err != nil {
			return 1, err
		}
		if err := s.Runtime.StartContainer(ctx, containerName); err != nil {
			return 1, err
		}
	} else {
		slog.Info("reattaching to container", "name", containerName)
	}

	return s.routeSession(ctx, containerName)
}

func (s *Session) routeSession(ctx context.Context, containerName string) (int, error) {
	agentEnv := s.prepareAgentForwarding()
	origCmd := os.Getenv("SSH_ORIGINAL_COMMAND")

	if origCmd != "" {
		s.Audit.Command(s.Username, s.ProjectName, origCmd)
	}

	switch {
	case origCmd == "":
		return s.interactiveShell(ctx, containerName, agentEnv)
	case isSFTP(origCmd):
		return s.execCommand(ctx, containerName, sftpServerPath, agentEnv)
	default:
		return s.execCommand(ctx, containerName, origCmd, agentEnv)
	}
}

func isSFTP(cmd string) bool {
	if cmd == "internal-sftp" {
		return true
	}
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return false
	}
	return filepath.Base(fields[0]) == "sftp-server"
}

func (s *Session) interactiveShell(ctx context.Context, containerName string, env []string) (int, error) {
	stdinFd := int(os.Stdin.Fd())
	if term.IsTerminal(stdinFd) {
		oldState, err := term.MakeRaw(stdinFd)
		if err != nil {
			return 1, fmt.Errorf("setting raw terminal: %w", err)
		}
		defer term.Restore(stdinFd, oldState) //nolint:errcheck // best-effort restore
	}

	exitCode, err := s.Runtime.Exec(ctx, containerName, runtime.ExecOpts{
		Cmd:    []string{s.Shell},
		TTY:    true,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Env:    env,
		ExecIDCallback: func(execID string) {
			go handleResize(ctx, s.Runtime, execID)
		},
	})
	if err != nil {
		return 1, err
	}

	return exitCode, nil
}

func (s *Session) execCommand(ctx context.Context, containerName, origCmd string, env []string) (int, error) {
	exitCode, err := s.Runtime.Exec(ctx, containerName, runtime.ExecOpts{
		Cmd:    []string{"sh", "-c", origCmd},
		TTY:    false,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Env:    env,
	})
	if err != nil {
		return 1, err
	}
	return exitCode, nil
}

func (s *Session) securityOpts() (capDrop, capAdd, secOpt []string, pidsLimit int64, readonlyRoot bool, tmpfs map[string]string, runtimeName string) {
	sec := s.Security
	capDrop = sec.CapDrop
	capAdd = sec.CapAdd
	if sec.NoNewPrivs {
		secOpt = append(secOpt, "no-new-privileges:true")
	}
	pidsLimit = sec.PidsLimit
	readonlyRoot = sec.ReadonlyRoot
	tmpfs = sec.Tmpfs
	runtimeName = sec.RuntimeName
	return
}

// agentMount returns the bind mount for SSH agent forwarding.
// Creates the host-side directory if it doesn't exist.
func (s *Session) agentMount() []runtime.Mount {
	hostDir := s.agentHostDir()
	if err := os.MkdirAll(hostDir, 0700); err != nil {
		slog.Warn("failed to create agent dir", "path", hostDir, "error", err)
		return nil
	}
	return []runtime.Mount{{
		Source: hostDir,
		Target: containerAgentDir,
	}}
}

// prepareAgentForwarding sets up the agent socket for a session exec.
// If SSH_AUTH_SOCK is set, links the socket into the shared mount dir
// using a per-PID filename to avoid races between concurrent sessions.
func (s *Session) prepareAgentForwarding() []string {
	hostSock := os.Getenv("SSH_AUTH_SOCK")
	if hostSock == "" {
		return nil
	}

	hostDir := s.agentHostDir()
	if err := os.MkdirAll(hostDir, 0700); err != nil {
		slog.Warn("agent forwarding: failed to create dir", "path", hostDir, "error", err)
		return nil
	}

	// Per-PID socket name prevents races between concurrent sessions
	sockName := fmt.Sprintf("agent-%d.sock", os.Getpid())
	target := filepath.Join(hostDir, sockName)
	containerSock := filepath.Join(containerAgentDir, sockName)

	_ = os.Remove(target)

	if err := os.Symlink(hostSock, target); err != nil {
		slog.Warn("agent forwarding: failed to symlink socket", "source", hostSock, "target", target, "error", err)
		return nil
	}
	slog.Debug("agent forwarding enabled", "host_sock", hostSock, "container_sock", containerSock)
	return []string{"SSH_AUTH_SOCK=" + containerSock}
}

func handleResize(ctx context.Context, rt runtime.Runtime, execID string) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	if w, h, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
		_ = rt.ResizeExec(ctx, execID, uint(h), uint(w))
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-sigCh:
			w, h, err := term.GetSize(int(os.Stdin.Fd()))
			if err != nil {
				continue
			}
			_ = rt.ResizeExec(ctx, execID, uint(h), uint(w))
		}
	}
}

// reconcileUser cleans up stale state for the current user only.
func (s *Session) reconcileUser(ctx context.Context) {
	// Crash recovery: connections=0 with no grace expiry
	stale, err := s.Store.StaleZeroConnections(s.Username, s.ProjectName)
	if err != nil {
		slog.Warn("reconcile: failed to check stale sessions", "error", err)
		return
	}
	if stale != nil {
		slog.Info("reconcile: cleaning up stale session", "user", stale.User, "container", stale.ContainerName)
		cleanupSessionResources(ctx, s.Runtime, stale)
		_ = s.Store.DeleteSession(stale.User, stale.Project)
	}

	// Expired grace period for this user/project
	sess, err := s.Store.GetSession(s.Username, s.ProjectName)
	if err != nil || sess == nil {
		return
	}
	if sess.Status == "grace_period" && sess.GraceExpiry.Valid && sess.GraceExpiry.Time.Before(time.Now()) {
		slog.Info("reconcile: grace period expired", "user", sess.User, "container", sess.ContainerName)
		cleanupSessionResources(ctx, s.Runtime, sess)
		_ = s.Store.DeleteSession(sess.User, sess.Project)
	}
}

// ensurePodfileParsed loads the Podfile on reattach (where resolveProject
// doesn't run). Needed so on_start hooks fire on every connection.
func (s *Session) ensurePodfileParsed() {
	if s.pf != nil || s.Project == nil {
		return
	}
	raw, err := podfile.FindAndRead(s.Project.LocalPath)
	if err != nil {
		slog.Warn("could not load podfile for hooks", "error", err)
		return
	}
	pf, err := podfile.Parse(bytes.NewReader(raw))
	if err != nil {
		slog.Warn("could not parse podfile for hooks", "error", err)
		return
	}
	s.pf = pf
}

func (s *Session) runHooks(ctx context.Context, containerName string, isNew bool) {
	if s.pf == nil {
		return
	}

	if isNew {
		if s.pf.Dotfiles != nil {
			if err := podfile.CloneDotfiles(ctx, s.Runtime, containerName, s.pf.Dotfiles); err != nil {
				slog.Warn("dotfiles setup failed", "error", err)
			}
		}
		for _, repo := range s.pf.Repos {
			if err := podfile.CloneRepoInContainer(ctx, s.Runtime, containerName, repo); err != nil {
				slog.Warn("repo clone failed", "url", repo.URL, "error", err)
			}
		}
		podfile.RunHook(ctx, s.Runtime, containerName, "on_create", s.pf.OnCreate)
	}

	podfile.RunHook(ctx, s.Runtime, containerName, "on_start", s.pf.OnStart)
}

func (s *Session) applyUserOverrides() {
	uo := s.UserOverrides
	if uo == nil {
		return
	}
	if uo.Image != "" {
		s.Image = uo.Image
	}
	if uo.CPUs > 0 {
		s.CPUs = uo.CPUs
	}
	if uo.Memory != "" {
		if mem, err := config.ParseMemory(uo.Memory); err == nil {
			s.Memory = mem
		}
	}
	if uo.Shell != "" {
		s.Shell = uo.Shell
	}
	if uo.Dotfiles != nil && s.pf != nil {
		s.pf.Dotfiles = &podfile.DotfilesConfig{
			Repo:    uo.Dotfiles.Repo,
			Install: uo.Dotfiles.Install,
		}
	}
	if len(uo.Env) > 0 && s.pf != nil {
		if s.pf.Env == nil {
			s.pf.Env = make(map[string]string)
		}
		for k, v := range uo.Env {
			s.pf.Env[k] = v
		}
	}
}

// userNetworkName returns the per-user network name for isolation.
func (s *Session) userNetworkName() string {
	if s.ProjectName != "" {
		return fmt.Sprintf("podspawn-%s-%s-net", s.Username, s.ProjectName)
	}
	return fmt.Sprintf("podspawn-%s-net", s.Username)
}

// resolveProject loads the Podfile (if a project is configured), resolves the
// cached image, and creates companion services. Returns the image to use,
// env vars, network ID, and service container IDs.
func (s *Session) resolveProject(ctx context.Context) (image string, env []string, networkID string, serviceIDs []string, err error) {
	// Always create a per-user network for isolation
	netName := s.userNetworkName()
	networkID, err = s.Runtime.CreateNetwork(ctx, netName)
	if err != nil {
		return "", nil, "", nil, fmt.Errorf("creating user network: %w", err)
	}

	if s.Project == nil {
		s.applyUserOverrides()
		return s.Image, nil, networkID, nil, nil
	}

	raw, err := podfile.FindAndRead(s.Project.LocalPath)
	if err != nil {
		_ = s.Runtime.RemoveNetwork(ctx, networkID)
		return "", nil, "", nil, fmt.Errorf("loading podfile for %s: %w", s.ProjectName, err)
	}
	pf, err := podfile.Parse(bytes.NewReader(raw))
	if err != nil {
		_ = s.Runtime.RemoveNetwork(ctx, networkID)
		return "", nil, "", nil, fmt.Errorf("parsing podfile for %s: %w", s.ProjectName, err)
	}
	s.pf = pf

	tag := podfile.ComputeTag(s.ProjectName, raw)
	exists, err := s.Runtime.ImageExists(ctx, tag)
	if err != nil {
		_ = s.Runtime.RemoveNetwork(ctx, networkID)
		return "", nil, "", nil, fmt.Errorf("checking image %s: %w", tag, err)
	}
	if !exists {
		_ = s.Runtime.RemoveNetwork(ctx, networkID)
		return "", nil, "", nil, fmt.Errorf("image %s not built; run: podspawn update-project %s", tag, s.ProjectName)
	}
	image = tag

	if pf.Resources.CPUs > 0 {
		s.CPUs = pf.Resources.CPUs
	}
	if pf.Resources.Memory != "" {
		mem, _ := config.ParseMemory(pf.Resources.Memory)
		s.Memory = mem
	}
	if pf.Shell != "" {
		s.Shell = pf.Shell
	}

	// User overrides take priority over Podfile values
	s.applyUserOverrides()

	for k, v := range pf.Env {
		expanded := strings.ReplaceAll(v, "${PODSPAWN_USER}", s.Username)
		expanded = strings.ReplaceAll(expanded, "${PODSPAWN_PROJECT}", s.ProjectName)
		env = append(env, k+"="+expanded)
	}
	sort.Strings(env)

	if len(pf.Services) > 0 {
		serviceIDs, err = podfile.StartServices(ctx, s.Runtime, pf.Services, networkID, s.containerName())
		if err != nil {
			_ = s.Runtime.RemoveNetwork(ctx, networkID)
			return "", nil, "", nil, fmt.Errorf("starting services: %w", err)
		}
	}

	return image, env, networkID, serviceIDs, nil
}

func (s *Session) cleanupServicesAndNetwork(ctx context.Context, serviceIDs []string, networkID string) {
	if len(serviceIDs) > 0 {
		podfile.StopServices(ctx, s.Runtime, serviceIDs)
	}
	if networkID != "" {
		_ = s.Runtime.RemoveNetwork(ctx, networkID)
	}
}

func cleanupSessionResources(ctx context.Context, rt runtime.Runtime, sess *state.Session) {
	_ = rt.RemoveContainer(ctx, sess.ContainerName)
	if sess.ServiceIDs != "" {
		podfile.StopServices(ctx, rt, strings.Split(sess.ServiceIDs, ","))
	}
	if sess.NetworkID != "" {
		_ = rt.RemoveNetwork(ctx, sess.NetworkID)
	}
}

// Disconnect handles session teardown: decrements connection count,
// starts grace period or destroys container.
// Must be called with context that has enough timeout for cleanup.
func (s *Session) Disconnect(ctx context.Context) {
	if s.Store == nil {
		// Phase 0: always destroy
		containerName := s.containerName()
		if err := s.Runtime.RemoveContainer(ctx, containerName); err != nil {
			slog.Warn("cleanup failed", "container", containerName, "error", err)
		}
		return
	}

	unlock, err := lock.Acquire(s.LockDir, s.Username)
	if err != nil {
		slog.Error("disconnect: failed to acquire lock", "user", s.Username, "error", err)
		return
	}
	defer unlock()

	count, err := s.Store.UpdateConnections(s.Username, s.ProjectName, -1)
	if err != nil {
		slog.Error("disconnect: failed to decrement connections", "user", s.Username, "error", err)
		return
	}

	s.Audit.Disconnect(s.Username, s.ProjectName, s.containerName(), count)

	if count > 0 {
		slog.Info("session still active", "user", s.Username, "connections", count)
		return
	}

	if s.Mode == "destroy-on-disconnect" || s.GracePeriod == 0 {
		sess, err := s.Store.GetSession(s.Username, s.ProjectName)
		if err != nil || sess == nil {
			slog.Warn("disconnect: session not found for cleanup", "user", s.Username)
			return
		}
		slog.Info("destroying container", "user", s.Username, "container", sess.ContainerName)
		cleanupSessionResources(ctx, s.Runtime, sess)
		_ = s.Store.DeleteSession(s.Username, s.ProjectName)
		s.Audit.ContainerDestroy(s.Username, s.ProjectName, sess.ContainerName, "disconnect")
		return
	}

	expiry := time.Now().Add(s.GracePeriod)
	slog.Info("starting grace period", "user", s.Username, "expires", expiry)
	if err := s.Store.SetGracePeriod(s.Username, s.ProjectName, expiry); err != nil {
		slog.Error("failed to set grace period", "user", s.Username, "error", err)
	}
}

// RunAndCleanup runs the session and handles disconnect after.
// Returns the exit code to pass to os.Exit.
func (s *Session) RunAndCleanup(ctx context.Context) int {
	exitCode, err := s.Run(ctx)
	if err != nil {
		slog.Error("session failed", "user", s.Username, "error", err)
	}

	cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.Disconnect(cleanupCtx)

	return exitCode
}
