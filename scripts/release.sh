#!/usr/bin/env bash
set -euo pipefail

# Usage: ./scripts/release.sh [major|minor|patch]
# Default: patch

ARG="${1:-patch}"

# Get current version from latest tag
CURRENT=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
CURRENT="${CURRENT#v}"

# Direct version (e.g., 0.1.0 or v0.1.0)
if [[ "$ARG" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    NEW="v${ARG#v}"
else
    IFS='.' read -r MAJOR MINOR PATCH <<< "$CURRENT"
    case "$ARG" in
        major) MAJOR=$((MAJOR + 1)); MINOR=0; PATCH=0 ;;
        minor) MINOR=$((MINOR + 1)); PATCH=0 ;;
        patch) PATCH=$((PATCH + 1)) ;;
        *) echo "Usage: $0 [major|minor|patch|X.Y.Z]"; exit 1 ;;
    esac
    NEW="v${MAJOR}.${MINOR}.${PATCH}"
fi

echo "Current: v${CURRENT}"
echo "New:     ${NEW}"
echo ""

# Confirm
read -p "Tag and push ${NEW}? [y/N] " -r
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 0
fi

# Tag and push
git tag -s "${NEW}" -m "Release ${NEW}"
git push origin "${NEW}"

echo ""
echo "Tagged ${NEW} and pushed."
echo "GitHub Actions will build and release automatically."
echo "Watch: https://github.com/podspawn/podspawn/actions"
