# podspawn

[![CI](https://github.com/podspawn/podspawn/actions/workflows/ci.yml/badge.svg)](https://github.com/podspawn/podspawn/actions/workflows/ci.yml)

Ephemeral dev containers. Locally or over SSH.

## The idea

You need a dev environment. Not a VM (too heavy), not a raw container (too bare), not a cloud workspace (too slow to spin up, too expensive to keep).

podspawn gives you **named machines** backed by Docker containers, configured with a Podfile (packages, services, env vars, hooks). Locally, it's zero-friction: no SSH, no root, no daemon. On a server, it hooks into your existing sshd so teammates just `ssh` in.

One binary. Local or remote. Same workflow.

## Quick start

```bash
curl -sSfL https://podspawn.dev/up | bash
```

### Local mode (no SSH, no root)

```bash
podspawn create dev                    # create a machine
podspawn shell dev                     # attach to it
podspawn run scratch                   # ephemeral throwaway
podspawn list                          # see machines
podspawn stop dev                      # destroy it
```

Just install podspawn + have Docker (OrbStack, Docker Desktop, Colima, Podman).

### Server mode (for teams)

```
ssh deploy@backend.pod
  -> ProxyCommand resolves backend.pod to your server
  -> sshd calls: podspawn auth-keys deploy
  -> podspawn returns keys with command="podspawn spawn --user deploy"
  -> container created from project's Podfile
  -> companion services (postgres, redis) start on a shared network
  -> you're in a fully configured dev environment
```

Every SSH feature works: SFTP, scp, rsync, port forwarding, agent forwarding, VS Code Remote, JetBrains Gateway. Teammates need zero client-side install.

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

## Features

- **Local machines** -- `podspawn create/run/shell`, no SSH needed, no root
- **Server mode** -- hooks into native sshd, every SSH feature works
- **Podfile environments** -- packages, services, dotfiles, hooks
- **devcontainer.json fallback** -- existing devcontainers work too
- **Security by default** -- cap-drop ALL, no-new-privileges, PID limits, per-user network isolation
- **gVisor support** -- `runtime: runsc` in config for kernel-level isolation
- **Grace period lifecycle** -- survive network blips, configurable TTLs
- **Session state** -- SQLite with connection counting, per-user file locking
- **Cleanup daemon** -- expires grace periods, enforces lifetimes, reconciles orphans
- **Audit logging** -- structured JSON-lines for every session event
- **Prometheus metrics** -- `podspawn status --prometheus`
- **Multi-arch** -- linux/darwin/windows, amd64/arm64, deb/rpm, Homebrew

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
podspawn add-project       # Clone repo + build Podfile image
podspawn doctor            # Preflight checks

# Client mode
podspawn setup             # Configure ~/.ssh/config
podspawn connect           # ProxyCommand handler (.pod namespace)
podspawn ssh               # SSH wrapper with .pod suffix
podspawn open              # VS Code / Cursor launcher
```

## Requirements

**Local mode**: Docker (OrbStack, Docker Desktop, Colima, Podman)
**Server mode**: Docker + OpenSSH 7.4+ on Linux
**Client mode**: SSH client

## Documentation

Full docs at [podspawn.dev](https://podspawn.dev), including:
- [Tutorial](https://podspawn.dev/docs/guides/tutorial) -- end-to-end walkthrough
- [CLI Reference](https://podspawn.dev/docs/cli/server-commands) -- every command
- [Podfile Spec](https://podspawn.dev/docs/podfile/overview) -- environment definition
- [Security](https://podspawn.dev/docs/guides/security-hardening) -- hardening guide

## Why podspawn

| | OrbStack | DevPod | podspawn |
|---|---|---|---|
| Platform | macOS only | macOS/Linux/Windows | macOS/Linux/Windows |
| Config | GUI | devcontainer.json | Podfile (simpler) |
| Services | manual | manual | first-class |
| Remote/teams | no | limited | native SSH |
| AI agent sandbox | no | no | isolated containers |
| Open source | no | yes | yes (AGPL-3.0) |

## License

AGPL-3.0. If you run a modified version as a service, you share your changes.
