# podspawn

Ephemeral dev containers over SSH. No daemon, no custom server — just your existing sshd.

## The idea

Every tool in this space (ContainerSSH, Coder, DevPod) builds its own SSH server. That's thousands of lines of protocol code reimplementing what OpenSSH already does.

podspawn doesn't do that. It's a single binary that hooks into your existing sshd via `AuthorizedKeysCommand`. Two lines in sshd_config, and you have ephemeral containers. The whole server side is ~3,000 lines of Go that pipe I/O between sshd and Docker.

Because sshd handles the protocol, you get every SSH feature for free: SFTP, scp, rsync, port forwarding, agent forwarding, VS Code Remote, JetBrains Gateway. All of it works. podspawn just manages container lifecycle.

## How it works

```
ssh deploy@backend.pod
  → ProxyCommand resolves backend.pod to your server
  → sshd calls: podspawn auth-keys deploy
  → podspawn returns keys with command="podspawn spawn --user deploy"
  → sshd authenticates, forces the command
  → podspawn creates container from project's Podfile
  → companion services (postgres, redis) start on a shared network
  → you're in a fully configured dev environment
  → exit → grace period starts → reconnect within 60s → same container
```

Real system users are unaffected. If podspawn doesn't recognize the username, it returns nothing and sshd falls through to normal `~/.ssh/authorized_keys`.

## The `.pod` namespace

On the client side, a `ProxyCommand` entry in `~/.ssh/config` gives you magic hostnames:

```
ssh alice@work.pod
```

The `.pod` suffix isn't a real TLD. The ProxyCommand intercepts it before DNS is ever queried, looks up the actual server from `~/.podspawn/config.yaml`, and connects. Same pattern DevPod uses with `.devpod`.

Set it up once:

```bash
podspawn setup    # appends *.pod config to ~/.ssh/config
```

Or skip the client binary entirely and SSH straight to the server. You lose the namespace magic but everything else works fine.

## Quick start

```bash
# Build
make build

# Server setup (configures sshd, creates directories)
sudo ./podspawn server-setup

# Register a user
sudo podspawn add-user deploy --github your-github-username

# Register a project
sudo podspawn add-project myapp --repo github.com/you/myapp

# Client setup (adds *.pod to ~/.ssh/config)
podspawn setup

# Connect
ssh deploy@myapp.pod
```

## Podfile

Define your dev environment in `podfile.yaml`:

```yaml
base: ubuntu:24.04
packages:
  - nodejs@22
  - python@3.12
  - ripgrep
  - fzf
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

Register a project:

```bash
sudo podspawn add-project backend --repo github.com/company/backend
```

Images are pre-built at registration time, not during SSH connections. Companion services get their own containers on a shared Docker network with DNS discovery (your app reaches postgres at `postgres:5432`).

## What works

- Local SSH key auth (no network calls at auth time)
- Interactive shell with full TTY support (resize, raw mode)
- Command execution with exit code propagation
- SFTP, scp, rsync
- Grace period lifecycle (survive network blips)
- Session state in SQLite with connection tracking
- Podfile-based environment definitions with package version pinning
- Companion services via Docker SDK (not docker compose)
- Image caching via content-addressed SHA-256 tags
- Client-side `.pod` namespace routing via ProxyCommand
- Resource limits (CPU, memory) per-project and per-user
- Dotfiles repo cloning and lifecycle hooks (on_create, on_start)
- Per-user config overrides
- `verify-image` compatibility checker

## What's coming

- devcontainer.json fallback
- Agent forwarding (bind-mount SSH_AUTH_SOCK)
- gVisor runtime option
- Cleanup daemon for expired sessions
- OIDC auth provider

## Requirements

- Go 1.23+
- Docker (or OrbStack, Podman, anything with a Docker-compatible API)
- OpenSSH 7.4+ (needs `AuthorizedKeysCommand` and the `restrict` keyword)

## Why now

Gitpod pivoted to AI agents (rebranded as "Ona", September 2025). Hocus is dead (archived September 2024). ContainerSSH requires replacing your SSH server entirely. The self-hosted "just SSH in" niche is underserved at the exact moment AI coding agents need disposable environments most.

## License

AGPL-3.0. If you run a modified version as a service, you share your changes.
