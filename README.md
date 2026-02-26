# podspawn

Ephemeral dev containers over SSH. No daemon, no custom server — just native sshd.

## What is this

podspawn hooks into your existing OpenSSH server via `AuthorizedKeysCommand`. When someone SSHes in, podspawn spawns a Docker container and routes the session into it. When they disconnect, the container goes away.

No port 2222. No TLS termination. No new attack surface. Your sshd handles the protocol, podspawn handles the containers.

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

## Features (current)

- Local SSH key auth (no external dependencies at auth time)
- Interactive shell with full TTY support (resize, raw mode)
- Command execution with exit code propagation
- Per-user containers (reattach on reconnect)
- Auto-pull missing Docker images

## Planned

- Grace period lifecycle (survive network flickers)
- Session state persistence (SQLite)
- Podfile-based environment definitions
- GitHub/OIDC key import
- SFTP, scp, rsync support
- Client-side `.pod` namespace routing
- Resource limits and network isolation

## Requirements

- Go 1.22+
- Docker (or OrbStack, Podman, anything with a Docker-compatible API)
- OpenSSH server with `AuthorizedKeysCommand` support (7.4+)

## License

AGPL-3.0. If you run a modified version as a service, you must share your changes.
