#!/bin/sh
set -e

REPO="o1x3/podspawn"
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

tar -xzf "$TMPDIR/podspawn.tar.gz" -C "$TMPDIR"

if [ -w "$INSTALL_DIR" ]; then
    mv "$TMPDIR/podspawn" "$INSTALL_DIR/podspawn"
else
    echo "Installing to ${INSTALL_DIR} (requires sudo)..."
    sudo mv "$TMPDIR/podspawn" "$INSTALL_DIR/podspawn"
fi

chmod +x "$INSTALL_DIR/podspawn"
echo "podspawn ${VERSION} installed to ${INSTALL_DIR}/podspawn"
