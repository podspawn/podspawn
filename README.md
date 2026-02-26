# podspawn

Ephemeral dev containers over SSH. No daemon, no custom server — just your existing sshd.

## The idea

Every tool in this space (ContainerSSH, Coder, DevPod) builds its own SSH server. That's thousands of lines of protocol code reimplementing what OpenSSH already does.

podspawn doesn't do that. It's a single binary that hooks into your existing sshd via `AuthorizedKeysCommand`. Two lines in sshd_config, and you have ephemeral containers. The whole server side is ~3,000 lines of Go that pipe I/O between sshd and Docker.

Because sshd handles the protocol, you get every SSH feature for free: SFTP, scp, rsync, port forwarding, agent forwarding, VS Code Remote, JetBrains Gateway. All of it works. podspawn just manages container lifecycle.

## How it works

```
ssh deploy@your-server
  → sshd calls: podspawn auth-keys deploy
  → podspawn returns keys with command="podspawn spawn --user deploy"
  → sshd authenticates, forces the command
  → podspawn creates container, pipes stdin/stdout
  → you're in a container. you don't know it.
  → exit → container removed
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

# Register a user's SSH key
sudo mkdir -p /etc/podspawn/keys
sudo cp ~/.ssh/id_ed25519.pub /etc/podspawn/keys/deploy

# Test locally (without sshd integration)
./podspawn auth-keys deploy
SSH_ORIGINAL_COMMAND="echo hello" ./podspawn spawn --user deploy
```

## What works today

- Local SSH key auth (no network calls at auth time)
- Interactive shell with full TTY support (resize, raw mode)
- Command execution with exit code propagation
- Per-user containers (reconnect within grace period → same container)
- Auto-pull missing Docker images

## What's coming

- Grace period lifecycle (survive network blips)
- Session state in SQLite
- Podfile-based environment definitions
- GitHub/OIDC key import
- SFTP, scp, rsync
- Client-side `.pod` namespace routing
- Resource limits and network isolation
- gVisor runtime option

## Requirements

- Go 1.22+
- Docker (or OrbStack, Podman, anything with a Docker-compatible API)
- OpenSSH 7.4+ (needs `AuthorizedKeysCommand` and the `restrict` keyword)

## Why now

Gitpod pivoted to AI agents (rebranded as "Ona", September 2025). Hocus is dead (archived September 2024). ContainerSSH requires replacing your SSH server entirely. The self-hosted "just SSH in" niche is underserved at the exact moment AI coding agents need disposable environments most.

## License

AGPL-3.0. If you run a modified version as a service, you share your changes.
