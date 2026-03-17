#!/usr/bin/env bash
set -eo pipefail

# podspawn interactive setup
# curl -sSfL https://podspawn.dev/up | bash

REPO="podspawn/podspawn"
INSTALL_DIR="/usr/local/bin"

# Colors (disabled if not a terminal)
if [ -t 1 ] 2>/dev/null || [ -e /dev/tty ]; then
    B='\033[1m' D='\033[2m' G='\033[32m' Y='\033[33m' C='\033[36m' R='\033[0m'
else
    B='' D='' G='' Y='' C='' R=''
fi

info()  { printf "  ${C}::${R} %s\n" "$1"; }
ok()    { printf "  ${G}ok${R} %s\n" "$1"; }
warn()  { printf "  ${Y}!!${R} %s\n" "$1"; }
step()  { printf "\n  ${B}[%s]${R} %s\n" "$1" "$2"; }

HAS_TTY=0
[ -e /dev/tty ] && HAS_TTY=1

ask() {
    if [ "$HAS_TTY" = "0" ]; then echo ""; return; fi
    printf "  %s " "$1" >/dev/tty
    read -r REPLY </dev/tty
    echo "$REPLY"
}

ask_choice() {
    if [ "$HAS_TTY" = "0" ]; then echo ""; return; fi
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
    *) printf "  unsupported architecture: %s\n" "$ARCH" >&2; exit 1 ;;
esac

printf "\n"
printf "  ${B}podspawn${R} ${D}-- ephemeral SSH dev containers${R}\n"

# ========================================
# STEP 1: Install binary
# ========================================
step "1" "Installing podspawn"

if command -v podspawn >/dev/null 2>&1; then
    CURRENT=$(podspawn version 2>/dev/null | head -1 || echo "unknown")
    ok "already installed ($CURRENT)"
else
    FETCH="curl"
    if ! command -v curl >/dev/null 2>&1; then
        if command -v wget >/dev/null 2>&1; then FETCH="wget"
        else printf "  curl or wget required\n" >&2; exit 1; fi
    fi

    if [ "$FETCH" = "curl" ]; then
        VERSION=$(curl -sSfL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name"' | cut -d'"' -f4)
    else
        VERSION=$(wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name"' | cut -d'"' -f4)
    fi

    if [ -z "${VERSION:-}" ]; then
        printf "  could not determine latest version\n" >&2; exit 1
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
                printf "  checksum mismatch!\n" >&2; exit 1
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
    sudo chown root:root "$INSTALL_DIR/podspawn" 2>/dev/null || true
    chmod +x "$INSTALL_DIR/podspawn"
    # macOS quarantines binaries downloaded via curl; strip it so Gatekeeper doesn't block
    if [ "$OS" = "darwin" ]; then
        xattr -dr com.apple.quarantine "$INSTALL_DIR/podspawn" 2>/dev/null || true
    fi
    ok "installed ${VERSION}"
fi

# ========================================
# STEP 2: Choose mode
# ========================================
printf "\n"
printf "  How do you want to use podspawn?\n"
printf "    ${B}1${R}) ${G}Local${R}    -- containers on this machine ${D}(try it out)${R}\n"
printf "    ${B}2${R}) Server   -- host containers for a team\n"
printf "    ${B}3${R}) Client   -- connect to a remote server\n"
printf "\n"

MODE=$(ask_choice "  Choice [1-3]: ")

case "$MODE" in
    1) MODE="local" ;;
    2) MODE="server" ;;
    3) MODE="client" ;;
    *) warn "invalid choice, defaulting to local"; MODE="local" ;;
esac

# ========================================
# Prerequisites (local + server modes)
# ========================================
if [ "$MODE" = "local" ] || [ "$MODE" = "server" ]; then

    # macOS-specific: host keys + Remote Login
    if [ "$OS" = "darwin" ]; then
        # Generate host keys if missing
        if ! ls /etc/ssh/ssh_host_* >/dev/null 2>&1; then
            printf "\n"
            info "macOS has no SSH host keys (needed to run as an SSH server)"
            info "These identify your machine to SSH clients, like an SSL certificate for a website."
            info "This is separate from your personal SSH key (~/.ssh/id_ed25519)."
            printf "\n"
            GENKEYS=$(ask "Generate host keys? [Y/n]:")
            if [ "$GENKEYS" != "n" ] && [ "$GENKEYS" != "N" ]; then
                sudo ssh-keygen -A
                ok "host keys generated"
            else
                warn "cannot run as SSH server without host keys"; exit 1
            fi
        fi

        # Enable Remote Login if not running
        if ! sudo launchctl list 2>/dev/null | grep -q "com.openssh.sshd"; then
            printf "\n"
            info "macOS Remote Login (SSH server) is not enabled"
            info "This allows SSH connections to your Mac. You can disable it later"
            info "in System Settings > General > Sharing > Remote Login."
            printf "\n"
            ENABLE_SSH=$(ask "Enable Remote Login? [Y/n]:")
            if [ "$ENABLE_SSH" != "n" ] && [ "$ENABLE_SSH" != "N" ]; then
                sudo systemsetup -setremotelogin on 2>/dev/null || sudo launchctl load -w /System/Library/LaunchDaemons/ssh.plist 2>/dev/null || true
                ok "Remote Login enabled"
            else
                warn "cannot run as SSH server without Remote Login"; exit 1
            fi
        fi
    fi

    MISSING=""

    if ! command -v sshd >/dev/null 2>&1 && ! [ -x /usr/sbin/sshd ]; then
        MISSING="sshd"
    fi
    if ! command -v docker >/dev/null 2>&1; then
        if [ -n "$MISSING" ]; then MISSING="$MISSING + docker"; else MISSING="docker"; fi
    fi

    if [ -n "$MISSING" ]; then
        step "!" "Missing: $MISSING"

        # Detect package manager and install
        if command -v apt-get >/dev/null 2>&1; then
            PKGS=""
            if echo "$MISSING" | grep -q "sshd"; then PKGS="openssh-server"; fi
            if echo "$MISSING" | grep -q "docker"; then
                if [ -n "$PKGS" ]; then PKGS="$PKGS docker.io"; else PKGS="docker.io"; fi
            fi
            info "installing $PKGS (requires sudo)"
            sudo apt-get update -qq
            sudo apt-get install -y -qq $PKGS
            # Start services
            if echo "$MISSING" | grep -q "sshd"; then
                sudo systemctl enable --now ssh 2>/dev/null || sudo service ssh start 2>/dev/null || true
            fi
            if echo "$MISSING" | grep -q "docker"; then
                sudo systemctl enable --now docker 2>/dev/null || sudo service docker start 2>/dev/null || true
            fi
            ok "prerequisites installed"
        elif command -v dnf >/dev/null 2>&1; then
            PKGS=""
            if echo "$MISSING" | grep -q "sshd"; then PKGS="openssh-server"; fi
            if echo "$MISSING" | grep -q "docker"; then
                if [ -n "$PKGS" ]; then PKGS="$PKGS docker"; else PKGS="docker"; fi
            fi
            info "installing $PKGS (requires sudo)"
            sudo dnf install -y -q $PKGS
            if echo "$MISSING" | grep -q "sshd"; then
                sudo systemctl enable --now sshd 2>/dev/null || true
            fi
            if echo "$MISSING" | grep -q "docker"; then
                sudo systemctl enable --now docker 2>/dev/null || true
            fi
            ok "prerequisites installed"
        elif command -v apk >/dev/null 2>&1; then
            PKGS=""
            if echo "$MISSING" | grep -q "sshd"; then PKGS="openssh-server"; fi
            if echo "$MISSING" | grep -q "docker"; then
                if [ -n "$PKGS" ]; then PKGS="$PKGS docker"; else PKGS="docker"; fi
            fi
            info "installing $PKGS (requires sudo)"
            sudo apk add --no-cache $PKGS
            if echo "$MISSING" | grep -q "sshd"; then
                sudo rc-update add sshd 2>/dev/null; sudo rc-service sshd start 2>/dev/null || true
            fi
            if echo "$MISSING" | grep -q "docker"; then
                sudo rc-update add docker 2>/dev/null; sudo rc-service docker start 2>/dev/null || true
            fi
            ok "prerequisites installed"
        else
            warn "could not auto-install prerequisites"
            printf "  Install manually:\n"
            if echo "$MISSING" | grep -q "sshd"; then printf "    openssh-server\n"; fi
            if echo "$MISSING" | grep -q "docker"; then printf "    docker\n"; fi
            exit 1
        fi
    fi
fi

# ========================================
# LOCAL MODE -- server + client, zero prompts
# ========================================
if [ "$MODE" = "local" ]; then
    USERNAME=$(whoami)

    step "2" "Configuring sshd"
    if grep -qi "authorizedkeyscommand.*podspawn" /etc/ssh/sshd_config 2>/dev/null; then
        ok "already configured"
    else
        info "running server-setup (requires sudo)"
        sudo podspawn server-setup
        ok "sshd configured"
    fi

    step "3" "Registering your SSH key"
    if [ -f "/etc/podspawn/keys/$USERNAME" ]; then
        ok "user $USERNAME already registered"
    else
        KEY_PATH="$HOME/.ssh/id_ed25519"
        if [ -f "${KEY_PATH}.pub" ]; then
            info "using existing key ${KEY_PATH}.pub"
        else
            info "generating ed25519 key at $KEY_PATH"
            mkdir -p "$HOME/.ssh"
            chmod 700 "$HOME/.ssh"
            ssh-keygen -t ed25519 -f "$KEY_PATH" -N "" -q </dev/tty 2>/dev/null || ssh-keygen -t ed25519 -f "$KEY_PATH" -N "" -q
            ok "key generated"
        fi
        sudo podspawn add-user "$USERNAME" --key-file "${KEY_PATH}.pub"
        ok "registered $USERNAME"
    fi

    # Unlock account for key auth (sshd rejects locked accounts before checking keys)
    sudo usermod -p '*' "$USERNAME" 2>/dev/null || true
    # Add to docker group so containers can be created without sudo
    sudo usermod -aG docker "$USERNAME" 2>/dev/null || true

    step "4" "Configuring SSH client"
    if grep -qi "Host \*.pod" "$HOME/.ssh/config" 2>/dev/null; then
        ok "~/.ssh/config already has *.pod block"
    else
        podspawn setup 2>/dev/null
        ok "added *.pod block"
    fi

    step "5" "Diagnostics"
    sudo podspawn doctor 2>/dev/null || true

    # Enable cleanup
    if command -v systemctl >/dev/null 2>&1; then
        if [ -f /etc/systemd/system/podspawn-cleanup.timer ]; then
            sudo systemctl enable --now podspawn-cleanup.timer 2>/dev/null || true
        fi
    fi

    printf "\n"
    printf "  ${B}${G}Ready.${R} Try it:\n"
    printf "\n"
    printf "    ${C}ssh %s@localhost${R}\n" "$USERNAME"
    printf "    ${C}ssh %s@localhost.pod${R}\n" "$USERNAME"
    printf "    ${C}podspawn ssh %s@localhost${R}\n" "$USERNAME"
    printf "\n"
    printf "  Docs: ${C}https://podspawn.dev${R}\n"
    printf "\n"
fi

# ========================================
# SERVER MODE -- full setup with key choice
# ========================================
if [ "$MODE" = "server" ]; then
    USERNAME=$(whoami)

    step "2" "Configuring sshd"
    if grep -qi "authorizedkeyscommand.*podspawn" /etc/ssh/sshd_config 2>/dev/null; then
        ok "already configured"
    else
        info "running server-setup (requires sudo)"
        sudo podspawn server-setup
        ok "sshd configured"
    fi

    step "3" "Setting up your user"
    if [ -f "/etc/podspawn/keys/$USERNAME" ]; then
        ok "user $USERNAME already registered"
    else
        printf "\n"
        printf "  How do you want to register SSH keys?\n"
        printf "    ${B}1${R}) Import from GitHub\n"

        HAS_KEY=""
        if [ -f "$HOME/.ssh/id_ed25519.pub" ]; then
            HAS_KEY="$HOME/.ssh/id_ed25519.pub"
            printf "    ${B}2${R}) Use existing key (~/.ssh/id_ed25519.pub)\n"
        elif [ -f "$HOME/.ssh/id_rsa.pub" ]; then
            HAS_KEY="$HOME/.ssh/id_rsa.pub"
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
                if [ -n "$GH_USER" ]; then
                    sudo podspawn add-user "$USERNAME" --github "$GH_USER"
                    ok "registered with GitHub keys"
                else warn "skipped"; fi
                ;;
            2)
                if [ -n "$HAS_KEY" ]; then
                    sudo podspawn add-user "$USERNAME" --key-file "$HAS_KEY"
                    ok "registered"
                else warn "no existing key found"; fi
                ;;
            3)
                KEY_PATH="$HOME/.ssh/id_ed25519"
                if [ ! -f "$KEY_PATH" ]; then
                    mkdir -p "$HOME/.ssh" && chmod 700 "$HOME/.ssh"
                    ssh-keygen -t ed25519 -f "$KEY_PATH" -N "" -q </dev/tty 2>/dev/null || ssh-keygen -t ed25519 -f "$KEY_PATH" -N "" -q
                    ok "key generated"
                fi
                sudo podspawn add-user "$USERNAME" --key-file "${KEY_PATH}.pub"
                ok "registered"
                ;;
            4)
                PUB_KEY=$(ask "Paste your public key:")
                if [ -n "$PUB_KEY" ]; then
                    sudo podspawn add-user "$USERNAME" --key "$PUB_KEY"
                    ok "registered"
                else warn "skipped"; fi
                ;;
            *) warn "skipped key registration" ;;
        esac
    fi

    # Unlock account for key auth + docker group
    sudo usermod -p '*' "$USERNAME" 2>/dev/null || true
    sudo usermod -aG docker "$USERNAME" 2>/dev/null || true

    step "4" "Diagnostics"
    sudo podspawn doctor 2>/dev/null || true

    step "5" "Cleanup daemon"
    if command -v systemctl >/dev/null 2>&1; then
        if systemctl is-active podspawn-cleanup.timer >/dev/null 2>&1; then
            ok "timer already running"
        elif [ -f /etc/systemd/system/podspawn-cleanup.timer ]; then
            sudo systemctl enable --now podspawn-cleanup.timer 2>/dev/null || true
            ok "timer started"
        else
            info "run: podspawn cleanup --daemon"
        fi
    else
        info "run: podspawn cleanup --daemon"
    fi

    printf "\n"
    printf "  ${B}${G}Server ready.${R}\n"
    printf "\n"
    printf "  Test:     ${C}ssh %s@localhost${R}\n" "$USERNAME"
    printf "  Status:   ${C}podspawn status${R}\n"
    printf "  Sessions: ${C}podspawn list${R}\n"
    printf "  Docs:     ${C}https://podspawn.dev${R}\n"
    printf "\n"
fi

# ========================================
# CLIENT MODE -- connect to remote server
# ========================================
if [ "$MODE" = "client" ]; then

    step "2" "Configuring SSH client"
    if grep -qi "Host \*.pod" "$HOME/.ssh/config" 2>/dev/null; then
        ok "~/.ssh/config already has *.pod block"
    else
        podspawn setup 2>/dev/null
        ok "added *.pod block"
    fi

    SERVER=$(ask "Server hostname (e.g., devbox.company.com):")
    if [ -n "$SERVER" ]; then
        mkdir -p "$HOME/.podspawn"
        cat > "$HOME/.podspawn/config.yaml" <<YAML
servers:
  default: ${SERVER}
YAML
        ok "default server: $SERVER"
    fi

    printf "\n"
    printf "  ${B}${G}Client ready.${R}\n"
    printf "\n"
    if [ -n "${SERVER:-}" ]; then
        printf "  Connect:  ${C}ssh you@%s${R}\n" "$SERVER"
        printf "  Or:       ${C}ssh you@project.pod${R}\n"
    else
        printf "  Connect:  ${C}ssh you@yourserver.com${R}\n"
    fi
    printf "  Docs:     ${C}https://podspawn.dev${R}\n"
    printf "\n"
fi
