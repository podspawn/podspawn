# Contributing to podspawn

## Development setup

```bash
git clone https://github.com/podspawn/podspawn.git
cd podspawn
make build
make hooks   # install pre-commit hook (gofmt + vet + lint + tests)
```

Requirements:
- Go 1.25+
- Docker
- golangci-lint v2

## Running tests

```bash
make test              # unit tests (~5s)
make test-integration  # Docker runtime tests (requires Docker)
make test-sshd         # full SSH pipeline tests (requires Docker)
make test-sshd-all     # sshd tests across Ubuntu, Debian, Rocky, Alpine
make lint              # go vet + golangci-lint
```

The pre-commit hook runs `gofmt`, `go vet`, `golangci-lint`, and all unit tests. If it fails, the commit is blocked.

## Code style

The short version:

- **Boring over clever.** Simple code that works beats elegant code that doesn't.
- **No restating comments.** Comment *why*, not *what*.
- **Conventional Commits.** `feat(spawn): add agent forwarding` not `Added agent forwarding feature`.
- **Test behavior, not implementation.** Mock the `Runtime` interface, not internal functions.
- **Error messages are specific.** `"creating container %s: %w"` not `"an error occurred"`.

## Project structure

```
cmd/              # Cobra command handlers
internal/
  adduser/        # User registration + key management
  audit/          # Structured audit logging
  authkeys/       # AuthorizedKeysCommand handler
  cleanup/        # Session cleanup (grace, lifetime, orphans)
  config/         # Server + client config parsing
  doctor/         # System diagnostic checks
  lock/           # Per-user file locking
  metrics/        # Prometheus-compatible metrics
  podfile/        # Podfile parsing, Dockerfile generation, services
  runtime/        # Docker SDK wrapper (Runtime interface)
  serversetup/    # sshd_config manipulation
  spawn/          # Session lifecycle (the core)
  sshconfig/      # Client SSH config generation
  state/          # SQLite session state
test/sshd/        # Full SSH pipeline integration tests
```

## CI

Workflows run on Blacksmith runners (`runs-on: blacksmith-*`). Two things to know:

1. **The Blacksmith GitHub App must be installed on this repository** for capacity provisioning to work. Without it, jobs requesting `runs-on: blacksmith-*` will queue while runners are routed to sibling repos in the org. This is the failure mode most commonly mistaken for a hung run.

2. **Job DAG:** `lint` runs first and warms the Go module cache for all build tags. The downstream jobs (`unit-test`, `macos-build`, `integration`, `sshd-integration` matrix) parallelize behind it. If a PR is in draft mode, all jobs skip.

The Blacksmith dashboard (app.blacksmith.sh) shows per-step timing, cache hit rates, and aggregated test analytics across runs.

## Pull request process

1. Branch from `main`
2. Write tests for new behavior
3. `make test && make lint` must pass
4. Signed commits required (`git config commit.gpgsign true`)
5. Linear history (no merge commits)
6. CI must pass: `lint`, `unit-test`, `macos-build`, `integration`, `sshd-integration (ubuntu|debian|rocky|alpine|ubuntu-arm64)`

## What's valued

- Bug fixes with reproduction tests
- Test coverage for untested paths
- Documentation improvements (docs site is at `podspawn/podspawn-docs`)
- Security hardening

## What to avoid

- Large refactors without discussion
- New dependencies without justification (can we do it in <50 lines?)
- Features not in the spec (`podspawn-spec-v2.md`) without an issue first
