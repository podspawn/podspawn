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
printf "${B}${C}                     __                              ${R}\n"
printf "${B}${C}    ____  ____  ____╱ ╱________  ____ __      ______ ${R}\n"
printf "${B}${C}   ╱ __ ╲╱ __ ╲╱ __  ╱ ___╱ __ ╲╱ __ \`╱ │ ╱│ ╱ ╱ __ ╲${R}\n"
printf "${B}${C}  ╱ ╱_╱ ╱ ╱_╱ ╱ ╱_╱ (__  ) ╱_╱ ╱ ╱_╱ ╱│ │╱ │╱ ╱ ╱ ╱ ╱${R}\n"
printf "${B}${C} ╱ .___╱╲____╱╲__,_╱____╱ .___╱╲__,_╱ │__╱│__╱_╱ ╱_╱ ${R}\n"
printf "${B}${C}╱_╱                    ╱_╱                           ${R}\n"
printf "\n"
printf "  ${D}ephemeral dev containers${R}\n"

# ========================================
# STEP 1: Install binary
# ========================================
step "1" "Installing podspawn"

if command -v podspawn >/dev/null 2>&1; then
    CURRENT=$(podspawn version 2>/dev/null | head -1 || echo "unknown")
    ok "already installed ($CURRENT)"

    printf "\n"
    printf "  What would you like to do?\n"
    printf "    ${B}1${R}) ${G}Update${R}      -- check for a newer version\n"
    printf "    ${B}2${R}) Reinstall   -- clean install of latest\n"
    printf "    ${B}3${R}) Uninstall   -- remove podspawn and config\n"
    printf "    ${B}4${R}) Continue    -- keep current, proceed to setup\n"
    printf "\n"

    ACTION=$(ask_choice "  Choice [1-4]: ")

    case "$ACTION" in
        1)
            sudo podspawn update
            ok "update complete ($(podspawn version 2>/dev/null | head -1))"
            # If already onboarded, no need to re-run setup
            if grep -qi "authorizedkeyscommand.*podspawn" /etc/ssh/sshd_config 2>/dev/null; then
                printf "\n"
                ok "already configured, nothing else to do"
                printf "\n"
                exit 0
            fi
            ;;
        2)
            BINARY_PATH=$(command -v podspawn)
            info "removing ${BINARY_PATH}"
            sudo rm -f "$BINARY_PATH"
            ok "removed old binary"
            # Fall through to the install block below
            FORCE_INSTALL=1
            ;;
        3)
            BINARY_PATH=$(command -v podspawn)
            info "removing ${BINARY_PATH}"
            sudo rm -f "$BINARY_PATH"
            if [ -d /etc/podspawn ]; then
                info "removing /etc/podspawn"
                sudo rm -rf /etc/podspawn
            fi
            if [ -d /var/lib/podspawn ]; then
                info "removing /var/lib/podspawn"
                sudo rm -rf /var/lib/podspawn
            fi
            if [ -d "$HOME/.podspawn" ]; then
                info "removing ~/.podspawn"
                rm -rf "$HOME/.podspawn"
            fi
            ok "podspawn uninstalled"
            printf "\n"
            printf "  ${D}sshd_config changes were left in place.${R}\n"
            printf "  ${D}Restore from backup: sudo cp /etc/ssh/sshd_config.podspawn.bak /etc/ssh/sshd_config${R}\n"
            printf "\n"
            exit 0
            ;;
        4)
            ;; # continue to mode selection
        *)
            warn "invalid choice, continuing with current install"
            ;;
    esac
fi

FORCE_INSTALL="${FORCE_INSTALL:-0}"

if ! command -v podspawn >/dev/null 2>&1 || [ "$FORCE_INSTALL" = "1" ]; then
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
# Prerequisites (server mode only needs sshd + Docker, local only needs Docker)
# ========================================
if [ "$MODE" = "server" ]; then

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
                if sudo systemsetup -setremotelogin on 2>/dev/null; then
                    ok "Remote Login enabled"
                elif sudo launchctl load -w /System/Library/LaunchDaemons/ssh.plist 2>/dev/null; then
                    ok "Remote Login enabled"
                else
                    warn "could not enable Remote Login automatically"
                    info "Enable it manually: System Settings > General > Sharing > Remote Login"
                    info "Then re-run this script"
                    exit 1
                fi
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
# LOCAL MODE -- Docker only, no SSH, no root
# ========================================
if [ "$MODE" = "local" ]; then
    USERNAME=$(whoami)

    step "2" "Docker access"
    if docker info >/dev/null 2>&1; then
        ok "docker is accessible"
    else
        if groups "$USERNAME" 2>/dev/null | grep -q docker; then
            warn "you're in the docker group but Docker isn't responding"
            info "try: newgrp docker, or log out and back in"
        else
            info "adding $USERNAME to docker group (requires sudo)"
            sudo usermod -aG docker "$USERNAME" 2>/dev/null || true
            warn "log out and back in for group membership to take effect"
        fi
    fi

    step "3" "Local config"
    mkdir -p "$HOME/.podspawn"

    # Default values
    PS_IMAGE="ubuntu:24.04"
    PS_SHELL="/bin/bash"
    PS_PACKAGES=""
    PS_DB=""
    PS_CACHE=""

    if [ ! -f "$HOME/.podspawn/config.yaml" ]; then
        printf "\n"
        printf "  ${B}Customize your default machine?${R} ${D}(you can change this later)${R}\n"
        printf "\n"
        CUSTOMIZE=$(ask "  Set up now? [Y/n]:")

        if [ "$CUSTOMIZE" != "n" ] && [ "$CUSTOMIZE" != "N" ]; then

            # Use case
            printf "\n"
            printf "  ${B}What are you building?${R}\n"
            printf "    ${B}1${R}) ${G}Web development${R}     ${D}-- Node.js, Python, databases${R}\n"
            printf "    ${B}2${R}) Systems programming  ${D}-- Go, Rust, C${R}\n"
            printf "    ${B}3${R}) Data science         ${D}-- Python, Jupyter, pandas${R}\n"
            printf "    ${B}4${R}) General purpose      ${D}-- bare ubuntu${R}\n"
            printf "\n"
            USECASE=$(ask_choice "  Choice [1-4]: ")

            case "$USECASE" in
                1) PS_PACKAGES="nodejs,python3,git,curl,ripgrep" ;;
                2) PS_PACKAGES="git,curl,build-essential,ripgrep" ;;
                3) PS_PACKAGES="python3,python3-pip,git,curl" ;;
                4) PS_PACKAGES="git,curl" ;;
                *) PS_PACKAGES="git,curl" ;;
            esac

            # Shell
            printf "\n"
            printf "  ${B}Default shell?${R}\n"
            printf "    ${B}1${R}) ${G}bash${R}\n"
            printf "    ${B}2${R}) zsh\n"
            printf "    ${B}3${R}) fish\n"
            printf "\n"
            SHELL_CHOICE=$(ask_choice "  Choice [1-3]: ")

            case "$SHELL_CHOICE" in
                2) PS_SHELL="/bin/zsh"; PS_PACKAGES="${PS_PACKAGES},zsh" ;;
                3) PS_SHELL="/usr/bin/fish"; PS_PACKAGES="${PS_PACKAGES},fish" ;;
                *) PS_SHELL="/bin/bash" ;;
            esac

            # Base image
            printf "\n"
            printf "  ${B}Base image?${R}\n"
            printf "    ${B}1${R}) ${G}Ubuntu 24.04${R}       ${D}(recommended)${R}\n"
            printf "    ${B}2${R}) Debian 12\n"
            printf "    ${B}3${R}) Alpine 3.20        ${D}(minimal)${R}\n"
            printf "\n"
            IMG_CHOICE=$(ask_choice "  Choice [1-3]: ")

            case "$IMG_CHOICE" in
                2) PS_IMAGE="debian:12" ;;
                3) PS_IMAGE="alpine:3.20" ;;
                *) PS_IMAGE="ubuntu:24.04" ;;
            esac

            # Database
            printf "\n"
            printf "  ${B}Include a database?${R}\n"
            printf "    ${B}1${R}) None\n"
            printf "    ${B}2${R}) ${G}PostgreSQL${R}\n"
            printf "    ${B}3${R}) MySQL\n"
            printf "    ${B}4${R}) Redis only\n"
            printf "\n"
            DB_CHOICE=$(ask_choice "  Choice [1-4]: ")

            case "$DB_CHOICE" in
                2) PS_DB="postgres" ;;
                3) PS_DB="mysql" ;;
                4) PS_CACHE="redis" ;;
                *) ;;
            esac

            # Cache (if not already selected redis-only)
            if [ "$PS_CACHE" != "redis" ] && [ -n "$PS_DB" ]; then
                printf "\n"
                printf "  ${B}Include Redis?${R}\n"
                printf "    ${B}1${R}) No\n"
                printf "    ${B}2${R}) ${G}Yes${R}\n"
                printf "\n"
                CACHE_CHOICE=$(ask_choice "  Choice [1-2]: ")
                if [ "$CACHE_CHOICE" = "2" ]; then
                    PS_CACHE="redis"
                fi
            fi

            ok "preferences saved"
        fi

        # Write config
        cat > "$HOME/.podspawn/config.yaml" <<YAML
local:
  image: ${PS_IMAGE}
  shell: ${PS_SHELL}
  cpus: 2
  memory: 2g
  max_lifetime: 24h
  mode: grace-period
YAML
        ok "created ~/.podspawn/config.yaml"

        # Write default Podfile if packages or services were selected
        if [ -n "$PS_PACKAGES" ] || [ -n "$PS_DB" ] || [ -n "$PS_CACHE" ]; then
            {
                printf "base: %s\n" "$PS_IMAGE"
                printf "shell: %s\n" "$PS_SHELL"

                if [ -n "$PS_PACKAGES" ]; then
                    printf "packages:\n"
                    echo "$PS_PACKAGES" | tr ',' '\n' | while read -r pkg; do
                        [ -n "$pkg" ] && printf "  - %s\n" "$pkg"
                    done
                fi

                if [ -n "$PS_DB" ] || [ -n "$PS_CACHE" ]; then
                    printf "services:\n"
                    if [ "$PS_DB" = "postgres" ]; then
                        printf "  - name: postgres\n"
                        printf "    image: postgres:16\n"
                        printf "    env:\n"
                        printf "      POSTGRES_PASSWORD: devpass\n"
                    elif [ "$PS_DB" = "mysql" ]; then
                        printf "  - name: mysql\n"
                        printf "    image: mysql:8\n"
                        printf "    env:\n"
                        printf "      MYSQL_ROOT_PASSWORD: devpass\n"
                    fi
                    if [ "$PS_CACHE" = "redis" ]; then
                        printf "  - name: redis\n"
                        printf "    image: redis:7\n"
                    fi
                fi
            } > "$HOME/.podspawn/default.podfile.yaml"
            ok "created ~/.podspawn/default.podfile.yaml"
        fi
    else
        ok "config exists"
    fi

    printf "\n"
    printf "  ${D}Edit anytime: ~/.podspawn/config.yaml${R}\n"
    printf "  ${D}Full reference: ${C}https://podspawn.dev/docs/podfile/overview${R}\n"
    printf "\n"
    printf "  ${B}${G}Ready.${R} Try it:\n"
    printf "\n"
    printf "    ${C}podspawn run scratch${R}             ${D}# ephemeral throwaway${R}\n"
    printf "    ${C}podspawn create dev${R}              ${D}# persistent machine${R}\n"
    printf "    ${C}podspawn shell dev${R}               ${D}# attach to existing${R}\n"
    printf "    ${C}podspawn list${R}                    ${D}# see machines${R}\n"
    printf "    ${C}podspawn stop dev${R}                ${D}# destroy machine${R}\n"
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
