# Branch Workspaces Spec

## Summary

Branch workspaces let a user create multiple isolated local machines from the same registered project, with each machine pinned to a specific Git branch.

Target commands:

```bash
podspawn create auth-fix --project backend --branch feat/auth-retry
podspawn run auth-fix --project backend --branch feat/auth-retry
```

This is a local-mode feature first. Server-mode branch selection can reuse the same model later, but it should not shape the first implementation.

## Problem

Current local project support is incomplete for real feature-branch work.

What works today:
- A project can be registered with `add-project`.
- A project repo can be cloned at registration time.
- Podfiles already support `branch:`.
- Repo clone helpers and lifecycle hooks already exist.

What is missing:
- `create` and `run` cannot select a registered project directly.
- A machine cannot be created from the same project at multiple branches in a predictable way.
- There is no strict rule for branch precedence between CLI and Podfile.
- There is no explicit contract for when clone happens relative to `on_create`.

Without that, branch-based environments are a half-built promise.

## Goals

- Let one user create multiple local machines from one registered project.
- Make branch selection explicit and deterministic.
- Ensure first boot produces a ready-to-use workspace.
- Preserve isolation between machines, even when they come from the same project.
- Reuse existing Podfile image build, services, hooks, and session-state machinery.

## Non-goals

- No new remote/server UX in the first version.
- No automatic branch switching on an existing machine.
- No worktree-based sharing between machines.
- No snapshotting or warm-pool optimization.

## User-facing behavior

### Commands

`create` and `run` gain:

```bash
--project string   Registered project name
--branch string    Branch to clone for this machine
```

Examples:

```bash
podspawn create auth-fix --project backend --branch feat/auth-retry
podspawn create auth-fix-main --project backend
podspawn run scratch --project backend --branch feat/spike
```

### Branch precedence

1. `--branch` flag
2. Podfile `branch:` field
3. Repository default branch

This is reversible and easy to explain. Any other rule will confuse users.

### Machine lifecycle

On first create:
- Resolve the registered project.
- Resolve the effective branch using the precedence rules above.
- Clone the repo into a machine-specific workspace.
- Start container and services.
- Run `on_create`.
- Run `on_start`.

On reattach with `shell`:
- Reuse the existing machine and workspace as-is.
- Do not re-clone.
- Do not re-run `on_create` once the machine is marked initialized.
- Run `on_start`.

For ephemeral `run --project`:
- Clone into a temporary workspace under `~/.podspawn/workspaces/.tmp-<name>-<unix-nano>/`.
- Remove that workspace on normal teardown.
- If the process is interrupted mid-run or mid-clone, leftover `.tmp-` directories may need manual cleanup.

### Isolation model

Each machine gets its own workspace copy. Two machines created from the same project must not share a checkout.

Example:

```bash
podspawn create auth-a --project backend --branch feat/a
podspawn create auth-b --project backend --branch feat/b
```

`auth-a` and `auth-b` must have separate repositories and separate Git state.

## Data model

Machine state should record enough information to explain and reproduce the workspace:

- machine name
- project name
- repo URL
- effective branch
- workspace path inside container
- created timestamp

If branch is not persisted, the UX becomes opaque and debugging gets harder.

## Implementation approach

### Option A: clone inside the container

Why it works:
- Keeps the container self-contained.
- Reuses existing hook behavior naturally.

Why it might fail:
- Slower first boot.
- Git must exist in every usable image.
- Re-clone and retry behavior is harder to inspect from the host.

Why not choose it:
- You already have local project state and host-side repo helpers. This adds more runtime coupling than needed.

### Option B: clone on the host into a machine-specific workspace, then mount/copy into the container

Why it works:
- Clear separation between machine workspace state and container lifecycle.
- Easier to inspect, debug, and clean up.
- Fits local mode better.

Why it might fail:
- Requires a clear host workspace layout.
- Cleanup logic must remove machine workspaces correctly.

Why choose it:
- It is boring and predictable. That matters more than elegance here.

Recommended host layout:

```text
/var/lib/podspawn/workspaces/<user>/<machine>/
```

For pure local mode on a non-root install, a user-owned equivalent under `~/.podspawn/workspaces/<machine>/` may be needed. The implementation should use one location derived from active mode, not mix both silently.

## Hook ordering

Required order:

1. Create workspace
2. Clone repository at effective branch
3. Start container
4. Run `on_create`
5. Run `on_start`

Reason:
- `on_create` often runs package install or bootstrap commands that require the repo contents.
- Running it before clone is wrong.

## Failure semantics

If clone fails:
- Machine creation fails.
- No running machine should be left behind.
- Partial workspace should be removed.

If container start fails after clone:
- Machine creation fails.
- Partial workspace may be kept for debugging only if that policy is explicit.
- Default should be cleanup.

If `on_create` fails:
- Machine create fails with a specific error.
- Workspace may be preserved because the repo checkout is valuable for inspection.
- This is a reversible decision and should be documented.
- The machine stays uninitialized, so the next `create` or `shell` retries `on_create`.
- `on_create` must therefore be idempotent. A normal `stop` followed by `shell` does not rerun it once initialization succeeded.

## Validation

- `--project` must reference a registered project.
- `--branch` requires `--project`.
- `--branch` must be non-empty if provided.
- Machine names keep existing validation rules.
- Reusing an existing machine name with a different branch must fail clearly, not mutate the old machine.

## Testing

Minimum coverage:

- `create --project` uses registered project metadata.
- `--branch` overrides Podfile `branch:`.
- Podfile `branch:` is used when CLI flag is absent.
- Default branch is used when neither is set.
- Two machines from one project get separate workspaces.
- Reattach does not re-clone or rerun `on_create`.
- Clone failure cleans up partial machine state.
- `on_create` runs after clone and can see repo files.

Integration coverage should use real Docker for at least one end-to-end path.

## Open questions

- Should local branch workspaces live under a root-owned path, a user-owned path, or be mode-dependent?
- Should `stop` remove the workspace by default, or only the container?
- Do we want a future `podspawn reset <machine>` to recreate from the original branch cleanly?

## Recommendation

Build the local-mode version first with host-side, machine-specific workspaces and strict branch precedence:

1. `--branch`
2. Podfile `branch:`
3. Repo default branch

Anything more abstract is premature.
