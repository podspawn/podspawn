#!/usr/bin/env bash
set -eo pipefail

# podspawn interactive setup
# curl -sSfL https://podspawn.dev/up | bash

REPO="podspawn/podspawn"
INSTALL_DIR="/usr/local/bin"

# Colors (disabled if not a terminal)
if [ -t 1 ] 2>/dev/null || [ -e /dev/tty ]; then
    B='\033[1m'
    D='\033[2m'
    G='\033[32m'
    Y='\033[33m'
    C='\033[36m'
    R='\033[0m'
else
    B='' D='' G='' Y='' C='' R=''
fi

info()  { printf "  ${C}::${R} %s\n" "$1"; }
ok()    { printf "  ${G}ok${R} %s\n" "$1"; }
warn()  { printf "  ${Y}!!${R} %s\n" "$1"; }
fail()  { printf "  ${Y}FAIL${R} %s\n" "$1"; }
step()  { printf "\n  ${B}[%s]${R} %s\n" "$1" "$2"; }

# Read from terminal even when piped through curl
HAS_TTY=0
[ -e /dev/tty ] && HAS_TTY=1

ask() {
    if [ "$HAS_TTY" = "0" ]; then
        echo ""
        return
    fi
    printf "  %s " "$1" >/dev/tty
    read -r REPLY </dev/tty
    echo "$REPLY"
}

ask_choice() {
    printf "%s" "$1" >/dev/tty
    read -r REPLY </dev/tty
    echo "$REPLY"
}

# --- Detect OS/arch ---
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) fail "unsupported architecture: $ARCH"; exit 1 ;;
esac

printf "\n"
printf "  ${B}podspawn${R} ${D}-- ephemeral SSH dev containers${R}\n"

# --- Auto-detect mode ---
MODE="client"
if [ -f /etc/ssh/sshd_config ] && command -v docker >/dev/null 2>&1; then
    MODE="server"
    info "detected server (sshd + Docker found)"
else
    info "detected client"
fi

# --- Step 1: Install binary ---
step "1/$([ "$MODE" = "server" ] && echo 5 || echo 2)" "Installing podspawn"

if command -v podspawn >/dev/null 2>&1; then
    CURRENT=$(podspawn version 2>/dev/null | head -1 || echo "unknown")
    ok "already installed ($CURRENT)"
else
    FETCH="curl"
    if ! command -v curl >/dev/null 2>&1; then
        if command -v wget >/dev/null 2>&1; then
            FETCH="wget"
        else
            fail "curl or wget required"; exit 1
        fi
    fi

    if [ "$FETCH" = "curl" ]; then
        VERSION=$(curl -sSf "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name"' | cut -d'"' -f4)
    else
        VERSION=$(wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name"' | cut -d'"' -f4)
    fi

    if [ -z "${VERSION:-}" ]; then
        fail "could not determine latest version; check your internet connection"
        exit 1
    fi

    FILENAME="podspawn_${VERSION#v}_${OS}_${ARCH}.tar.gz"
    URL="https://github.com/${REPO}/releases/download/${VERSION}/${FILENAME}"

    info "downloading ${VERSION} for ${OS}/${ARCH}"
    TMPDIR=$(mktemp -d)
    trap 'rm -rf "$TMPDIR"' EXIT

    if [ "$FETCH" = "curl" ]; then
        curl -sSfL "$URL" -o "$TMPDIR/podspawn.tar.gz"
        curl -sSfL "https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt" -o "$TMPDIR/checksums.txt" 2>/dev/null || true
    else
        wget -q "$URL" -O "$TMPDIR/podspawn.tar.gz"
        wget -q "https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt" -O "$TMPDIR/checksums.txt" 2>/dev/null || true
    fi

    if [ -f "$TMPDIR/checksums.txt" ]; then
        EXPECTED=$(grep -F "$FILENAME" "$TMPDIR/checksums.txt" | awk '{print $1}')
        if [ -n "${EXPECTED:-}" ]; then
            if command -v sha256sum >/dev/null 2>&1; then
                ACTUAL=$(sha256sum "$TMPDIR/podspawn.tar.gz" | awk '{print $1}')
            elif command -v shasum >/dev/null 2>&1; then
                ACTUAL=$(shasum -a 256 "$TMPDIR/podspawn.tar.gz" | awk '{print $1}')
            fi
            if [ -n "${ACTUAL:-}" ] && [ "$EXPECTED" != "$ACTUAL" ]; then
                fail "checksum mismatch! aborting"; exit 1
            fi
            info "checksum verified"
        fi
    fi

    tar -xzf "$TMPDIR/podspawn.tar.gz" -C "$TMPDIR"

    if [ -w "$INSTALL_DIR" ]; then
        mv "$TMPDIR/podspawn" "$INSTALL_DIR/podspawn"
    else
        info "installing to ${INSTALL_DIR} (requires sudo)"
        sudo mv "$TMPDIR/podspawn" "$INSTALL_DIR/podspawn"
    fi
    chmod +x "$INSTALL_DIR/podspawn"
    ok "installed ${VERSION}"
fi

# ========================================
# SERVER MODE
# ========================================
if [ "$MODE" = "server" ]; then

    # --- Step 2: Configure sshd ---
    step "2/5" "Configuring sshd"
    if grep -qi "authorizedkeyscommand.*podspawn" /etc/ssh/sshd_config 2>/dev/null; then
        ok "already configured"
    else
        info "running server-setup (requires sudo)"
        sudo podspawn server-setup
        ok "sshd configured"
    fi

    # --- Step 3: SSH key setup ---
    step "3/5" "Setting up your user"
    USERNAME=$(whoami)

    if [ -f "/etc/podspawn/keys/$USERNAME" ]; then
        ok "user $USERNAME already registered"
    else
        printf "\n"
        printf "  How do you want to register SSH keys?\n"
        printf "    ${B}1${R}) Import from GitHub\n"

        HAS_ED25519=""
        HAS_RSA=""
        [ -f "$HOME/.ssh/id_ed25519.pub" ] && HAS_ED25519="1"
        [ -f "$HOME/.ssh/id_rsa.pub" ] && HAS_RSA="1"

        if [ -n "$HAS_ED25519" ]; then
            printf "    ${B}2${R}) Use existing key (~/.ssh/id_ed25519.pub)\n"
        elif [ -n "$HAS_RSA" ]; then
            printf "    ${B}2${R}) Use existing key (~/.ssh/id_rsa.pub)\n"
        else
            printf "    ${D}2) No existing key found${R}\n"
        fi

        printf "    ${B}3${R}) Generate a new ed25519 key\n"
        printf "    ${B}4${R}) Paste a public key\n"
        printf "\n"

        CHOICE=$(ask_choice "  Choice [1-4]: ")

        case "$CHOICE" in
            1)
                GH_USER=$(ask "GitHub username:")
                if [ -z "$GH_USER" ]; then
                    warn "skipped key registration"
                else
                    sudo podspawn add-user "$USERNAME" --github "$GH_USER"
                    ok "registered with GitHub keys"
                fi
                ;;
            2)
                if [ -n "$HAS_ED25519" ]; then
                    sudo podspawn add-user "$USERNAME" --key-file "$HOME/.ssh/id_ed25519.pub"
                    ok "registered with ed25519 key"
                elif [ -n "$HAS_RSA" ]; then
                    sudo podspawn add-user "$USERNAME" --key-file "$HOME/.ssh/id_rsa.pub"
                    ok "registered with RSA key"
                else
                    warn "no existing key found; choose option 1 or 3"
                fi
                ;;
            3)
                KEY_PATH="$HOME/.ssh/id_ed25519"
                if [ -f "$KEY_PATH" ]; then
                    info "key already exists at $KEY_PATH"
                else
                    info "generating ed25519 key"
                    ssh-keygen -t ed25519 -f "$KEY_PATH" -N "" -q </dev/tty
                    ok "key generated at $KEY_PATH"
                fi
                sudo podspawn add-user "$USERNAME" --key-file "${KEY_PATH}.pub"
                ok "registered with new key"
                ;;
            4)
                PUB_KEY=$(ask "Paste your public key:")
                if [ -z "$PUB_KEY" ]; then
                    warn "skipped key registration"
                else
                    sudo podspawn add-user "$USERNAME" --key "$PUB_KEY"
                    ok "registered with pasted key"
                fi
                ;;
            *)
                warn "invalid choice; skipping key registration"
                printf "  Run manually: sudo podspawn add-user %s --github YOUR_GITHUB\n" "$USERNAME"
                ;;
        esac
    fi

    # --- Step 4: Diagnostics ---
    step "4/5" "Diagnostics"
    sudo podspawn doctor 2>/dev/null || true

    # --- Step 5: Cleanup daemon ---
    step "5/5" "Cleanup daemon"
    if command -v systemctl >/dev/null 2>&1; then
        if systemctl is-active podspawn-cleanup.timer >/dev/null 2>&1; then
            ok "timer already running"
        elif [ -f /etc/systemd/system/podspawn-cleanup.timer ]; then
            sudo systemctl enable --now podspawn-cleanup.timer 2>/dev/null || true
            ok "timer started"
        else
            info "no systemd unit found; run: podspawn cleanup --daemon"
        fi
    else
        info "systemd not found; run: podspawn cleanup --daemon"
    fi

    # --- Done ---
    printf "\n"
    printf "  ${B}${G}Server ready.${R}\n"
    printf "\n"
    printf "  Test it:\n"
    printf "    ${C}ssh %s@localhost${R}\n" "$USERNAME"
    printf "    ${C}ssh %s@localhost.pod${R}  ${D}(.pod namespace)${R}\n" "$USERNAME"
    printf "\n"
    printf "  Commands:\n"
    printf "    ${C}podspawn status${R}        system health\n"
    printf "    ${C}podspawn list${R}          active sessions\n"
    printf "    ${C}podspawn list-users${R}    registered users\n"
    printf "    ${C}podspawn doctor${R}        diagnostics\n"
    printf "\n"
    printf "  Docs: ${C}https://podspawn.dev${R}\n"
    printf "\n"
fi

# ========================================
# CLIENT MODE
# ========================================
if [ "$MODE" = "client" ]; then

    step "2/2" "Configuring SSH client"

    if grep -qi "Host \*.pod" "$HOME/.ssh/config" 2>/dev/null; then
        ok "~/.ssh/config already has *.pod block"
    else
        podspawn setup
        ok "added *.pod block to ~/.ssh/config"
    fi

    # Ask for default server
    SERVER=$(ask "Default server (e.g., devbox.company.com, or Enter to skip):")
    if [ -n "$SERVER" ]; then
        mkdir -p "$HOME/.podspawn"
        cat > "$HOME/.podspawn/config.yaml" <<YAML
servers:
  default: ${SERVER}
YAML
        ok "wrote ~/.podspawn/config.yaml"
    fi

    printf "\n"
    printf "  ${B}${G}Client ready.${R}\n"
    printf "\n"
    if [ -n "${SERVER:-}" ]; then
        printf "  Connect:\n"
        printf "    ${C}ssh you@%s${R}     ${D}(direct)${R}\n" "$SERVER"
        printf "    ${C}ssh you@project.pod${R}   ${D}(.pod namespace)${R}\n"
    else
        printf "  Connect:\n"
        printf "    ${C}ssh you@yourserver.com${R}\n"
    fi
    printf "\n"
    printf "  Docs: ${C}https://podspawn.dev${R}\n"
    printf "\n"
fi
