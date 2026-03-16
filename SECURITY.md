# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in podspawn, please report it responsibly.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, email **security@podspawn.dev** with:

- Description of the vulnerability
- Steps to reproduce
- Affected versions
- Any potential mitigations you've identified

We aim to acknowledge reports within 48 hours and provide a fix within 7 days for critical issues.

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest release | Yes |
| Previous minor | Best effort |
| Older | No |

## Security Model

Podspawn hooks into the host's native sshd via `AuthorizedKeysCommand`. The security boundary relies on:

- **OpenSSH** for all protocol-level security (key exchange, encryption, authentication)
- **Docker** for container isolation (namespaces, cgroups, seccomp)
- **Per-user file locking** for session state consistency
- **SQLite with WAL mode** for concurrent state access

### Default Hardening

Containers launch with these defaults (configurable in `/etc/podspawn/config.yaml`):

- `cap-drop: ALL` with selective re-adds (CHOWN, SETUID, SETGID, DAC_OVERRIDE, FOWNER, NET_BIND_SERVICE)
- `no-new-privileges: true`
- `pids-limit: 256`
- Per-user bridge network isolation
- Optional gVisor runtime (`runtime: runsc`)

### Known Limitations

- Containers share the host kernel (standard Docker isolation, not VM-level)
- SSH agent forwarding uses per-PID symlinks in a bind-mounted directory
- The state database (`/var/lib/podspawn/state.db`) requires appropriate filesystem permissions for multi-user access
