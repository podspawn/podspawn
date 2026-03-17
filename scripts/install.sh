#!/bin/sh
set -e

REPO="podspawn/podspawn"
INSTALL_DIR="/usr/local/bin"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

case "$OS" in
    linux|darwin) ;;
    *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

# Get latest version
if command -v curl >/dev/null 2>&1; then
    VERSION=$(curl -sSf "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
elif command -v wget >/dev/null 2>&1; then
    VERSION=$(wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
else
    echo "curl or wget required" >&2
    exit 1
fi

if [ -z "$VERSION" ]; then
    echo "Failed to determine latest version" >&2
    exit 1
fi

FILENAME="podspawn_${VERSION#v}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${FILENAME}"

echo "Downloading podspawn ${VERSION} for ${OS}/${ARCH}..."

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

if command -v curl >/dev/null 2>&1; then
    curl -sSfL "$URL" -o "$TMPDIR/podspawn.tar.gz"
else
    wget -q "$URL" -O "$TMPDIR/podspawn.tar.gz"
fi

# Verify checksum
CHECKSUM_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"
if command -v curl >/dev/null 2>&1; then
    curl -sSfL "$CHECKSUM_URL" -o "$TMPDIR/checksums.txt"
else
    wget -q "$CHECKSUM_URL" -O "$TMPDIR/checksums.txt"
fi

EXPECTED=$(grep -F "$FILENAME" "$TMPDIR/checksums.txt" | awk '{print $1}')
if [ -z "$EXPECTED" ]; then
    echo "Warning: checksum not found for $FILENAME, skipping verification" >&2
else
    if command -v sha256sum >/dev/null 2>&1; then
        ACTUAL=$(sha256sum "$TMPDIR/podspawn.tar.gz" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        ACTUAL=$(shasum -a 256 "$TMPDIR/podspawn.tar.gz" | awk '{print $1}')
    else
        echo "Warning: no sha256sum or shasum found, skipping verification" >&2
        ACTUAL="$EXPECTED"
    fi
    if [ "$EXPECTED" != "$ACTUAL" ]; then
        echo "Checksum verification failed!" >&2
        echo "  Expected: $EXPECTED" >&2
        echo "  Got:      $ACTUAL" >&2
        exit 1
    fi
    echo "Checksum verified."
fi

tar -xzf "$TMPDIR/podspawn.tar.gz" -C "$TMPDIR"

if [ -w "$INSTALL_DIR" ]; then
    mv "$TMPDIR/podspawn" "$INSTALL_DIR/podspawn"
else
    echo "Installing to ${INSTALL_DIR} (requires sudo)..."
    sudo mv "$TMPDIR/podspawn" "$INSTALL_DIR/podspawn"
fi

sudo chown root:root "$INSTALL_DIR/podspawn" 2>/dev/null || true
chmod +x "$INSTALL_DIR/podspawn"
# macOS quarantines binaries downloaded via curl; strip it so Gatekeeper doesn't block
if [ "$OS" = "darwin" ]; then
    xattr -dr com.apple.quarantine "$INSTALL_DIR/podspawn" 2>/dev/null || true
fi
echo "podspawn ${VERSION} installed to ${INSTALL_DIR}/podspawn"
