#!/usr/bin/env bash
set -euo pipefail

# Usage: ./scripts/release.sh [major|minor|patch]
# Default: patch

BUMP="${1:-patch}"
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BOLD='\033[1m'
RESET='\033[0m'

err() { printf "${RED}✗ %s${RESET}\n" "$*" >&2; exit 1; }
ok()  { printf "${GREEN}✓${RESET} %s\n" "$*"; }

# --- preflight ---

printf "${BOLD}Preflight checks${RESET}\n"

BRANCH=$(git rev-parse --abbrev-ref HEAD)
[[ "$BRANCH" == "main" ]] || err "on branch '$BRANCH', not main"
ok "on main"

[[ -z "$(git status --porcelain)" ]] || err "uncommitted changes"
ok "working tree clean"

git fetch origin --tags --quiet
[[ "$(git rev-parse HEAD)" == "$(git rev-parse origin/main)" ]] || err "local differs from origin/main (push or pull first)"
ok "up to date with origin"

CURRENT=$(git describe --tags --abbrev=0 2>/dev/null) || err "no reachable tags (run git fetch --tags)"
CURRENT="${CURRENT#v}"
ok "current version: v${CURRENT}"

# --- bump ---

IFS='.' read -r MAJOR MINOR PATCH <<< "$CURRENT"

case "$BUMP" in
    major) MAJOR=$((MAJOR + 1)); MINOR=0; PATCH=0 ;;
    minor) MINOR=$((MINOR + 1)); PATCH=0 ;;
    patch) PATCH=$((PATCH + 1)) ;;
    *) echo "Usage: $0 [major|minor|patch]"; exit 1 ;;
esac

NEW="v${MAJOR}.${MINOR}.${PATCH}"

echo ""
printf "${BOLD}v${CURRENT}${RESET} → ${BOLD}${GREEN}${NEW}${RESET} (${BUMP})\n"
echo ""

read -p "Tag and push ${NEW}? [y/N] " -r
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 0
fi

echo ""
git tag -s "${NEW}" -m "Release ${NEW}"
ok "tagged ${NEW}"

git push origin "${NEW}"
ok "pushed ${NEW}"

echo ""
printf "${GREEN}${BOLD}Done.${RESET} goreleaser will pick it up.\n"
echo "https://github.com/podspawn/podspawn/actions"
