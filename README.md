# podspawn

[![CI](https://github.com/podspawn/podspawn/actions/workflows/ci.yml/badge.svg)](https://github.com/podspawn/podspawn/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/podspawn/podspawn)](https://github.com/podspawn/podspawn/releases/latest)
[![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue)](LICENSE)

Dev containers backed by Docker. Locally or over SSH.

## Quick start

```bash
curl -sSfL https://podspawn.dev/up | bash
```

## What it does

You want a dev environment. Not a VM, not a raw container, not a cloud workspace.

podspawn gives you **named machines** backed by Docker containers, configured with a Podfile (packages, services, env vars, hooks). Locally, there's no SSH, no root, no daemon. On a server, it hooks into your existing sshd so teammates just `ssh` in.

One binary. Local or remote. Same workflow.

## Local mode

No SSH, no root, no daemon. Just Docker.

```bash
podspawn create dev                    # create a machine
podspawn shell dev                     # attach to it
podspawn run scratch                   # create + attach, ephemeral
podspawn list                          # see machines
podspawn stop dev                      # destroy it
```

Works with OrbStack, Docker Desktop, Colima, or Podman.

## Server mode

Hook into your existing sshd. Every SSH feature works for free.

```
ssh deploy@backend.pod
  -> ProxyCommand resolves backend.pod to your server
  -> sshd calls: podspawn auth-keys deploy
  -> podspawn returns keys with command="podspawn spawn --user deploy"
  -> container created from project's Podfile
  -> companion services (postgres, redis) start on a shared network
  -> you're in a fully configured dev environment
```

SFTP, scp, rsync, port forwarding, agent forwarding, VS Code Remote, JetBrains Gateway, Cursor. Teammates need zero client-side install.

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

Companion services get their own containers on a shared Docker network with DNS discovery.

## What's working

### Local mode
- [x] Named machines (`create`, `run`, `shell`, `list`, `stop`)
- [x] Podfile environments (packages, services, env vars, hooks)
- [x] Default Podfile at `~/.podspawn/podfile.yaml`
- [x] Interactive onboarding wizard
- [ ] `--with` flag for quick package overrides
- [ ] Branch-based dev containers (`-b feat/auth`)

### Server mode
- [x] Native sshd hook via `AuthorizedKeysCommand`
- [x] Three session modes: grace-period, destroy-on-disconnect, persistent
- [x] Persistent containers with bind-mounted home directories
- [x] Per-user bridge network isolation
- [x] Companion services on shared Docker network
- [x] SFTP, scp, rsync, port forwarding, agent forwarding
- [x] VS Code Remote, JetBrains Gateway, Cursor
- [x] Non-root container users with passwordless sudo
- [x] Project routing via `.pod` hostnames
- [ ] Client-side `.pod` routing to multiple servers
- [ ] Per-project branch selection over SSH

### Infrastructure
- [x] SQLite session state (WAL mode, busy timeout, crash recovery)
- [x] Per-user file locking
- [x] Cleanup daemon (grace periods, max lifetimes, orphan reconciliation)
- [x] Structured audit logging (JSON-lines)
- [x] Prometheus-compatible metrics (`podspawn status --prometheus`)
- [x] Security hardening (cap-drop ALL, no-new-privileges, PID limits)
- [x] gVisor runtime support (`runtime: runsc` in config)
- [x] Multi-arch releases (linux/darwin/windows, amd64/arm64)
- [x] deb, rpm, Homebrew
- [x] Self-update (`podspawn update`)
- [ ] Shell completions (bash, zsh, fish)
- [ ] Machine snapshots

## Commands

```
# Local mode
podspawn create            # Create a named machine
podspawn run               # Create + attach, ephemeral
podspawn shell             # Attach to existing machine
podspawn list              # Show machines
podspawn stop              # Destroy a machine

# Server mode
podspawn server-setup      # Configure sshd
podspawn add-user          # Register SSH keys
podspawn add-project       # Register a project with Podfile
podspawn doctor            # Preflight checks

# Client mode
podspawn setup             # Configure ~/.ssh/config
podspawn connect           # ProxyCommand handler (.pod namespace)
podspawn ssh               # SSH wrapper with .pod suffix
podspawn open              # VS Code / Cursor launcher
```

## Comparison

| | Codespaces | Coder | DevPod | podspawn |
|---|---|---|---|---|
| Self-hosted | no | yes | yes | yes |
| SSH daemon | custom | custom | custom | native sshd |
| Config format | devcontainer.json | Terraform | devcontainer.json | Podfile |
| Companion services | Docker Compose | manual | manual | first-class |
| Local mode | no | no | yes | yes |
| Persistent + ephemeral | persistent only | persistent only | persistent only | both |
| Complexity | high | high | medium | low |
| Open source | no | yes | yes | yes (AGPL-3.0) |

## Requirements

**Local mode**: Docker (OrbStack, Docker Desktop, Colima, Podman)
**Server mode**: Docker + OpenSSH 7.4+ on Linux
**Client mode**: SSH client

## Documentation

Full docs at [podspawn.dev](https://podspawn.dev):
- [Tutorial](https://podspawn.dev/docs/guides/tutorial)
- [CLI Reference](https://podspawn.dev/docs/cli/server-commands)
- [Podfile Spec](https://podspawn.dev/docs/podfile/overview)
- [Security Hardening](https://podspawn.dev/docs/guides/security-hardening)

## Status

**Alpha.** Core features work and are tested (290+ unit tests, integration tests across 4 Linux distros). API may change between minor versions. Current release: [v0.4.3](https://github.com/podspawn/podspawn/releases/tag/v0.4.3).

## License

AGPL-3.0. If you run a modified version as a service, you share your changes.
