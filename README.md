# podspawn

[![CI](https://github.com/podspawn/podspawn/actions/workflows/ci.yml/badge.svg)](https://github.com/podspawn/podspawn/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/podspawn/podspawn)](https://github.com/podspawn/podspawn/releases/latest)
[![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue)](LICENSE)

Clone a repo. Run one command. Get the full dev environment.

```bash
git clone github.com/yourorg/project
cd project
podspawn dev
```

That's it. Podspawn reads the repo's `podfile.yaml`, builds a cached image, starts companion services (postgres, redis), mounts your code, and drops you into a shell. Works locally with Docker or remotely over SSH.

## Install

```bash
curl -sSfL https://podspawn.dev/up | bash
```

## How it works

A Podfile defines your dev environment:

```yaml
extends: ubuntu-dev
packages:
  - go@1.25
  - nodejs@22
services:
  - name: postgres
    image: postgres:16
    env: { POSTGRES_PASSWORD: devpass }
env:
  DATABASE_URL: "postgres://postgres:devpass@postgres:5432/dev"
on_create: |
  go mod download
  npm install
```

`extends: ubuntu-dev` inherits common tools (git, ripgrep, fzf, neovim, jq). Your Podfile adds what's specific to your project. Podfiles compose -- a child inherits from a base and overrides what it needs.

## Three ways to use it

### podspawn dev (local, one command)

No SSH, no root, no daemon. Just Docker.

```bash
podspawn dev                    # shell into Podfile environment
podspawn dev -- make test       # run a command and exit
podspawn down                   # stop everything
podspawn init                   # scaffold a Podfile for your project
```

Your code is bind-mounted into the container. Edits on your host appear instantly inside. Companion services are accessible by name (`postgres:5432`).

### Named machines (local, persistent)

For longer-lived environments that aren't tied to a specific repo:

```bash
podspawn create backend         # create a named machine
podspawn shell backend          # attach to it
podspawn run scratch            # one-shot throwaway
podspawn list                   # see what's running
podspawn stop backend           # tear it down
```

### Server mode (remote, over SSH)

Hook into your existing sshd. Every SSH feature works.

```
ssh deploy@backend.pod
  -> sshd calls podspawn auth-keys
  -> container created from project Podfile
  -> companion services start
  -> you're in
```

SFTP, scp, rsync, port forwarding, agent forwarding, VS Code Remote, JetBrains Gateway, Cursor. Teammates need zero client-side install.

## Podfile features

- **Packages**: `go@1.25`, `nodejs@22`, `python@3.13`, `rust@stable` -- version-pinned installs
- **Services**: postgres, redis, or any Docker image as a companion container
- **Extends**: inherit from base Podfiles, override what you need
- **Bang-replace**: `packages!:` to fully replace instead of merge
- **Lifecycle hooks**: `on_create` (once) and `on_start` (every attach)
- **Port forwarding**: auto-resolve conflicts, manual, or strict
- **Mount modes**: bind (live sync), copy (one-shot), or none
- **Session modes**: grace-period, destroy-on-disconnect, persistent

## What's working

### Dev environments
- [x] `podspawn dev` -- one-command setup from Podfile
- [x] `podspawn down` -- teardown with `--clean` for volumes
- [x] `podspawn init` -- scaffolding with project type detection
- [x] `podspawn prebuild` -- pre-build images for CI
- [x] Podfile `extends` with deep merge and bang-replace
- [x] `mount: copy` via Docker CopyToContainer
- [x] Port forwarding (auto, manual, expose strategies)
- [x] Embedded templates: go, node, python, rust, fullstack, minimal
- [x] Template registry with `--update` from [podspawn/podfiles](https://github.com/podspawn/podfiles)
- [x] devcontainer.json fallback

### Local mode
- [x] Named machines (`create`, `run`, `shell`, `list`, `stop`)
- [x] Podfile environments (packages, services, env vars, hooks)
- [x] Default Podfile at `~/.podspawn/podfile.yaml`

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

### Infrastructure
- [x] SQLite session state (WAL mode, crash recovery)
- [x] Cleanup daemon (grace periods, max lifetimes, orphan reconciliation)
- [x] Structured audit logging (JSON-lines)
- [x] Prometheus metrics (`podspawn status --prometheus`)
- [x] Security hardening (cap-drop ALL, no-new-privileges, PID limits, gVisor)
- [x] Multi-arch releases (linux/darwin/windows, amd64/arm64, deb/rpm/Homebrew)

## Roadmap

What's coming next, roughly in priority order:

| Feature | Status | Description |
|---------|--------|-------------|
| Dev integration tests | Planned | End-to-end tests for `podspawn dev` with real Docker |
| Image baking | Planned | Bake container user into image at build time for faster cold starts (~600ms) |
| `prebuild --push` | Planned | Push pre-built images to ghcr.io for CI |
| `podspawn sync` | Planned | Manual push/pull file sync between host and container |
| Warm container pool | Exploring | Pre-start containers in background for instant attach |
| Branch-based environments | Exploring | `podspawn dev -b feat/auth` for per-branch containers |
| Shell completions | Exploring | bash, zsh, fish completions |

See [project_next_phase.md](https://github.com/podspawn/podspawn) for detailed design notes.

## Dogfooding

podspawn uses its own tool for development. See [`podfile.yaml`](podfile.yaml) in this repo:

```bash
git clone github.com/podspawn/podspawn
cd podspawn
podspawn dev
# Go 1.25, golangci-lint, delve, pre-commit hooks -- all ready
```

## Commands

```
# Dev environments
podspawn dev               # Start from CWD Podfile
podspawn down              # Stop dev environment
podspawn init              # Scaffold a Podfile
podspawn prebuild          # Pre-build image for CI

# Named machines
podspawn create            # Create a named machine
podspawn run               # Create + attach, ephemeral
podspawn shell             # Attach to existing machine
podspawn list              # Show machines
podspawn stop              # Destroy a machine

# Server administration
podspawn server-setup      # Configure sshd
podspawn add-user          # Register SSH keys
podspawn add-project       # Register a project with Podfile
podspawn doctor            # Preflight checks

# Client
podspawn setup             # Configure ~/.ssh/config
podspawn ssh               # SSH wrapper with .pod routing
podspawn open              # VS Code / Cursor launcher
```

## Comparison

| | Codespaces | Coder | DevPod | podspawn |
|---|---|---|---|---|
| Self-hosted | no | yes | yes | yes |
| One-command setup | yes | no | yes | yes |
| SSH server | custom | custom | custom | native sshd |
| Config format | devcontainer.json | Terraform | devcontainer.json | Podfile |
| Companion services | Docker Compose | manual | manual | first-class |
| Extends / composition | features system | no | features system | `extends` + merge |
| Local mode | no | no | yes | yes |
| Persistent + ephemeral | persistent only | persistent only | persistent only | both |
| Complexity | high | high | medium | low |
| Open source | no | yes (Apache) | yes (MPL) | yes (AGPL) |

## Requirements

**Local mode**: Docker (OrbStack, Docker Desktop, Colima, Podman)
**Server mode**: Docker + OpenSSH 7.4+ on Linux
**Client mode**: SSH client

## Documentation

Full docs at [podspawn.dev](https://podspawn.dev):
- [Getting Started](https://podspawn.dev/docs/dev-environments/quickstart)
- [Podfile Reference](https://podspawn.dev/docs/dev-environments/reference)
- [Real-World Examples](https://podspawn.dev/docs/dev-environments/examples)
- [Extending Podfiles](https://podspawn.dev/docs/dev-environments/extending)
- [CLI Reference](https://podspawn.dev/docs/cli/server-commands)
- [Security Hardening](https://podspawn.dev/docs/guides/security-hardening)

## Status

**Alpha.** Core features work and are tested (350+ unit tests, integration tests across 4 Linux distros). API may change between minor versions.

## License

AGPL-3.0. If you run a modified version as a service, you share your changes.
