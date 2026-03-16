# podspawn

[![CI](https://github.com/podspawn/podspawn/actions/workflows/ci.yml/badge.svg)](https://github.com/podspawn/podspawn/actions/workflows/ci.yml)

Ephemeral dev containers over SSH. No daemon, no custom server -- just your existing sshd.

## The idea

Every tool in this space (ContainerSSH, Coder, DevPod) builds its own SSH server. That's thousands of lines of protocol code reimplementing what OpenSSH already does.

podspawn doesn't do that. It's a single binary that hooks into your existing sshd via `AuthorizedKeysCommand`. Two lines in sshd_config, and you have ephemeral containers. Because sshd handles the protocol, every SSH feature works out of the box: SFTP, scp, rsync, port forwarding, agent forwarding, VS Code Remote, JetBrains Gateway.

## How it works

```
ssh deploy@backend.pod
  -> ProxyCommand resolves backend.pod to your server
  -> sshd calls: podspawn auth-keys deploy
  -> podspawn returns keys with command="podspawn spawn --user deploy"
  -> sshd authenticates, forces the command
  -> container created from project's Podfile
  -> companion services (postgres, redis) start on a shared network
  -> you're in a fully configured dev environment
  -> exit -> grace period -> reconnect within 60s -> same container
```

Real system users are unaffected. If podspawn doesn't recognize the username, it returns nothing and sshd falls through to normal auth.

## Quick start

**Server (interactive, one line):**
```bash
curl -sSf https://podspawn.dev/up | bash
```

This installs the binary, configures sshd, walks you through SSH key setup, and runs diagnostics. Works on any Linux server with Docker.

**Client (macOS/Linux):**
```bash
curl -sSf https://podspawn.dev/up | bash
```

Auto-detects client mode (no sshd), installs the binary, and configures `~/.ssh/config` for the `.pod` namespace.

**Windows (manual):**

Add to `~/.ssh/config`:
```
Host *.pod
    ProxyCommand podspawn connect %r %h %p
    UserKnownHostsFile /dev/null
    StrictHostKeyChecking no
```

Or skip the client binary entirely and SSH straight to the server.

**Self-update:**
```bash
podspawn update
```

For the full walkthrough, see the [tutorial](https://podspawn.dev/docs/guides/tutorial).

## Podfile

Define your dev environment in `podfile.yaml`:

```yaml
base: ubuntu:24.04
packages:
  - nodejs@22
  - python@3.12
  - ripgrep
shell: /bin/zsh
services:
  - name: postgres
    image: postgres:16
    env:
      POSTGRES_PASSWORD: devpass
  - name: redis
    image: redis:7
env:
  DATABASE_URL: "postgres://postgres:devpass@postgres:5432/dev"
on_create: "make setup"
on_start: "echo welcome back"
```

Images are pre-built at registration time, not during SSH. Companion services get their own containers on a shared Docker network with DNS discovery.

Also supports `devcontainer.json` as a fallback.

## Features

- **Native sshd** -- two lines of config, every SSH feature works
- **Podfile environments** -- packages, services, dotfiles, hooks
- **devcontainer.json fallback** -- existing devcontainers work too
- **Security by default** -- cap-drop ALL, no-new-privileges, PID limits, per-user network isolation
- **gVisor support** -- `runtime: runsc` in config for kernel-level isolation
- **Agent forwarding** -- SSH_AUTH_SOCK bind-mounted into containers
- **Grace period lifecycle** -- survive network blips, configurable TTLs
- **Session state** -- SQLite with connection counting, per-user file locking
- **Cleanup daemon** -- expires grace periods, enforces lifetimes, reconciles orphans
- **Audit logging** -- structured JSON-lines for every session event
- **Prometheus metrics** -- `podspawn status --prometheus`
- **Doctor command** -- 11 preflight checks for setup validation
- **Multi-arch** -- linux/darwin, amd64/arm64, deb/rpm, Homebrew

## Commands

```
podspawn server-setup      # Configure sshd
podspawn add-user          # Register SSH keys
podspawn add-project       # Clone repo + build Podfile image
podspawn connect           # ProxyCommand handler (.pod namespace)
podspawn setup             # Configure client ~/.ssh/config
podspawn list              # Active sessions
podspawn stop              # Destroy a session
podspawn cleanup           # Reconcile orphans + enforce TTLs
podspawn status            # System metrics
podspawn doctor            # Preflight checks
podspawn list-users        # Registered users
podspawn remove-user       # Remove user + sessions
podspawn remove-project    # Deregister project
podspawn verify-image      # Check image compatibility
```

## Requirements

- Docker (or OrbStack, Podman with Docker-compatible API)
- OpenSSH 7.4+ (needs `AuthorizedKeysCommand` and `restrict` keyword)
- Linux server for production (macOS for development via OrbStack)

## Documentation

Full docs at [podspawn.dev](https://podspawn.dev), including:
- [Tutorial](https://podspawn.dev/docs/guides/tutorial) -- end-to-end walkthrough
- [Troubleshooting](https://podspawn.dev/docs/guides/troubleshooting) -- common issues
- [CLI Reference](https://podspawn.dev/docs/cli/server-commands) -- every command
- [Podfile Spec](https://podspawn.dev/docs/podfile/overview) -- environment definition
- [Security](https://podspawn.dev/docs/guides/security-hardening) -- hardening guide

## Why now

Gitpod pivoted to AI agents (rebranded as "Ona", September 2025). Hocus is dead (archived September 2024). ContainerSSH requires replacing your SSH server entirely. The self-hosted "just SSH in" niche is underserved at the exact moment AI coding agents need disposable environments most.

## License

AGPL-3.0. If you run a modified version as a service, you share your changes.
