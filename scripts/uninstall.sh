#!/usr/bin/env bash
# GCM (Git Config Manager) uninstallation script
# This script removes gcm from $HOME/.local/bin and cleans shell configuration
set -e

# Colors and styles
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
WHITE='\033[1;37m'
GRAY='\033[0;90m'
NC='\033[0m'

# Style effects
BOLD='\033[1m'
DIM='\033[2m'

# Unicode characters for better UI
CHECKMARK="✓"
CROSSMARK="✗"
ARROW="→"
TRASH="🗑"
WARNING="⚠"
QUESTION="❓"
STOP="🛑"
CLEAN="🧹"
SHIELD="🛡"
INFO="ℹ"

# Terminal width detection
TERM_WIDTH=$(tput cols 2>/dev/null || echo 80)

# Print separator line
print_separator() {
    local char="${1:--}"
    printf "%*s\n" "$TERM_WIDTH" | tr ' ' "$char"
}

# Print fancy header
print_header() {
    clear
    print_separator "═"
    echo
    echo
    echo '     ██████╗  ██████╗███╗   ███╗'
    echo '    ██╔════╝ ██╔════╝████╗ ████║'
    echo '    ██║  ███╗██║     ██╔████╔██║'
    echo '    ██║   ██║██║     ██║╚██╔╝██║'
    echo '    ╚██████╔╝╚██████╗██║ ╚═╝ ██║'
    echo '     ╚═════╝  ╚═════╝╚═╝     ╚═╝'
    echo
    echo
    echo -e "${BOLD}${WHITE}              Git Config Manager Uninstaller${NC}"
    echo -e "${DIM}${GRAY}            Safe and complete uninstallation process${NC}"
    echo
    print_separator "═"
    echo
}

# Print functions with icons and styling
print_info() {
    echo -e "${BLUE}${BOLD} ${INFO}  INFO${NC} ${GRAY}│${NC} $1"
}

print_success() {
    echo -e "${GREEN}${BOLD} ${CHECKMARK}  SUCCESS${NC} ${GRAY}│${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}${BOLD} ${WARNING}  WARNING${NC} ${GRAY}│${NC} $1"
}

print_error() {
    echo -e "${RED}${BOLD} ${CROSSMARK}  ERROR${NC} ${GRAY}│${NC} $1"
}

print_step() {
    echo -e "${PURPLE}${BOLD} ${ARROW}  STEP${NC} ${GRAY}│${NC} $1"
}

print_clean() {
    echo -e "${CYAN}${BOLD} ${CLEAN}  CLEANING${NC} ${GRAY}│${NC} $1"
}

print_question() {
    echo -e "${YELLOW}${BOLD} ${QUESTION}  QUESTION${NC} ${GRAY}│${NC} $1"
}

# User input function
get_user_input() {
    local prompt="$1"
    local response=""

    if [[ -e /dev/tty ]]; then
        read -r -p "$(echo -e "$prompt")" response </dev/tty
    else
        read -r -p "$(echo -e "$prompt")" response
    fi

    echo "$response"
}

# Get shell configuration files
get_shell_configs() {
    local configs=("$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.zshrc")
    [[ -f "$HOME/.config/fish/config.fish" ]] && configs+=("$HOME/.config/fish/config.fish")
    printf '%s ' "${configs[@]}"
}

# Check if gcm is installed
check_gcm_installation() {
    local install_dir="$HOME/.local/bin"
    local gcm_dir="$HOME/.gcm"
    local shell_configs_str
    shell_configs_str=$(get_shell_configs)
    local shell_configs=($shell_configs_str)
    local binary_found=false
    local config_found=false
    local data_found=false

    print_step "Checking GCM installation..."

    # Check binary in common locations
    local found_paths=()
    for candidate in \
      "$(command -v gcm 2>/dev/null || true)" \
      "$install_dir/gcm" \
      "/usr/local/bin/gcm" \
      "${GOPATH:-${HOME}/go}/bin/gcm" \
      "${HOME}/bin/gcm"; do
      [[ -n "$candidate" && -f "$candidate" ]] || continue
      local real_p
      real_p=$(realpath "$candidate" 2>/dev/null || echo "$candidate")
      local dup=false
      for existing in "${found_paths[@]+"${found_paths[@]}"}"; do
        [[ "$existing" == "$real_p" ]] && dup=true && break
      done
      $dup || found_paths+=("$real_p")
    done

    if [[ ${#found_paths[@]} -gt 0 ]]; then
        binary_found=true
    fi

    # Check shell configurations
    for shell_config in "${shell_configs[@]}"; do
        if [[ -f "$shell_config" ]] && grep -q "GCM" "$shell_config" 2>/dev/null; then
            config_found=true
            break
        fi
    done

    # Check data directory
    if [[ -d "$gcm_dir" ]]; then
        data_found=true
    fi

    # Check if gcm command is available in PATH
    local command_found=false
    if command -v gcm >/dev/null 2>&1; then
        command_found=true
    fi

    echo
    print_separator "┄"
    echo -e "${BOLD}${WHITE}Installation Status:${NC}"
    print_separator "┄"

    if [[ "$binary_found" == true ]]; then
        for fp in "${found_paths[@]}"; do
            echo -e "${GREEN} ${CHECKMARK}${NC} Binary found: ${BOLD}${fp}${NC}"
        done
    else
        echo -e "${GRAY} ${CROSSMARK}${NC} Binary: ${DIM}not found${NC}"
    fi

    if [[ "$config_found" == true ]]; then
        echo -e "${GREEN} ${CHECKMARK}${NC} Shell configuration: ${BOLD}Found${NC}"
    else
        echo -e "${GRAY} ${CROSSMARK}${NC} Shell configuration: ${DIM}No GCM configuration found${NC}"
    fi

    if [[ "$command_found" == true ]]; then
        local version=$(gcm version 2>/dev/null | head -1 || echo "unknown")
        echo -e "${GREEN} ${CHECKMARK}${NC} Command available: ${BOLD}gcm${NC} ${DIM}($version)${NC}"
    else
        echo -e "${GRAY} ${CROSSMARK}${NC} Command available: ${DIM}gcm (not in PATH)${NC}"
    fi

    if [[ "$data_found" == true ]]; then
        local dir_size=$(du -sh "$gcm_dir" 2>/dev/null | cut -f1 || echo "unknown")
        echo -e "${BLUE} ${INFO}${NC} Data directory: ${BOLD}$gcm_dir${NC} ${DIM}($dir_size)${NC}"
    else
        echo -e "${GRAY} ${CROSSMARK}${NC} Data directory: ${DIM}$gcm_dir (not found)${NC}"
    fi

    print_separator "┄"
    echo

    if [[ "$binary_found" == true || "$config_found" == true || "$data_found" == true || "$command_found" == true ]]; then
        return 0
    else
        return 1
    fi
}

# Show what will be removed based on option
show_removal_preview() {
    local option="$1"

    echo -e "${BOLD}${WHITE}Removal Preview:${NC}"
    print_separator "┄"

    local install_dir="$HOME/.local/bin"
    local gcm_dir="$HOME/.gcm"
    local shell_configs_str
    shell_configs_str=$(get_shell_configs)
    local shell_configs=($shell_configs_str)

    # Check binary in all locations
    local bin_found=false
    for candidate in \
      "$(command -v gcm 2>/dev/null || true)" \
      "$install_dir/gcm" \
      "/usr/local/bin/gcm" \
      "${GOPATH:-${HOME}/go}/bin/gcm" \
      "${HOME}/bin/gcm"; do
      [[ -n "$candidate" && -f "$candidate" ]] || continue
      echo -e "${RED} ${TRASH}${NC} Binary: ${BOLD}${candidate}${NC}"
      bin_found=true
    done
    if [[ "$bin_found" == false ]]; then
        echo -e "${GRAY} ${CROSSMARK}${NC} Binary: ${DIM}not found${NC}"
    fi

    # Check shell configurations
    local config_found=false
    for shell_config in "${shell_configs[@]}"; do
        if [[ -f "$shell_config" ]] && grep -q "GCM" "$shell_config" 2>/dev/null; then
            echo -e "${RED} ${TRASH}${NC} Shell config: ${BOLD}$shell_config${NC}"
            config_found=true
        fi
    done

    if [[ "$config_found" == false ]]; then
        echo -e "${GRAY} ${CROSSMARK}${NC} Shell configs: ${DIM}No GCM configuration found${NC}"
    fi

    # Show data directory based on option
    if [[ -d "$gcm_dir" ]]; then
        local dir_size=$(du -sh "$gcm_dir" 2>/dev/null | cut -f1 || echo "unknown")
        if [[ "$option" == "complete" ]]; then
            echo -e "${RED} ${TRASH}${NC} Data directory: ${BOLD}$gcm_dir${NC} ${DIM}($dir_size)${NC}"
        else
            echo -e "${GREEN} ${SHIELD}${NC} Data directory: ${BOLD}$gcm_dir${NC} ${DIM}($dir_size - will be kept)${NC}"
        fi
    else
        echo -e "${GRAY} ${CROSSMARK}${NC} Data directory: ${DIM}$gcm_dir (not found)${NC}"
    fi

    print_separator "┄"
    echo
}

# Animated loading for removal process
show_removal_progress() {
    local item="$1"
    local delay=0.1
    local spinstr='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
    local temp

    echo -n "   ${DIM}Removing $item... ${NC}"
    for i in {1..10}; do
        temp=${spinstr#?}
        printf "\r   ${DIM}Removing $item... ${CYAN}%c${NC} " "$spinstr"
        spinstr=$temp${spinstr%"$temp"}
        sleep $delay
    done
    printf "\r   ${GREEN}${CHECKMARK}${NC} Removed $item successfully.      \n"
}

# Remove binary with feedback
remove_binary() {
    local install_dir="$HOME/.local/bin"

    print_step "Removing gcm binary..."

    # Find all gcm binaries
    local binaries=()
    for candidate in \
      "$(command -v gcm 2>/dev/null || true)" \
      "$install_dir/gcm" \
      "/usr/local/bin/gcm" \
      "${GOPATH:-${HOME}/go}/bin/gcm" \
      "${HOME}/bin/gcm"; do
      [[ -n "$candidate" && -f "$candidate" ]] || continue
      local real_p
      real_p=$(realpath "$candidate" 2>/dev/null || echo "$candidate")
      local dup=false
      for existing in "${binaries[@]+"${binaries[@]}"}"; do
        [[ "$existing" == "$real_p" ]] && dup=true && break
      done
      $dup || binaries+=("$real_p")
    done

    if [[ ${#binaries[@]} -eq 0 ]]; then
        print_warning "gcm binary not found in expected locations"
        return
    fi

    for bin_path in "${binaries[@]}"; do
        show_removal_progress "binary ($bin_path)"
        if rm -f "$bin_path" 2>/dev/null; then
            print_success "Removed gcm from $bin_path"
        else
            # Might need sudo for /usr/local/bin
            if sudo rm -f "$bin_path" 2>/dev/null; then
                print_success "Removed gcm from $bin_path (with sudo)"
            else
                print_error "Failed to remove $bin_path (permission denied)"
            fi
        fi
    done

    # Clear shell hash table
    hash -r 2>/dev/null || true
}

# Remove from PATH with feedback
remove_from_path() {
    local shell_configs_str
    shell_configs_str=$(get_shell_configs)
    local shell_configs=($shell_configs_str)
    local configs_modified=0

    print_step "Cleaning shell configurations..."

    for shell_config in "${shell_configs[@]}"; do
        if [[ -f "$shell_config" ]]; then
            # Check for GCM shell integration markers
            if grep -q ">>> GCM" "$shell_config" 2>/dev/null; then
                show_removal_progress "$(basename "$shell_config") configuration"

                # Remove GCM shell integration block
                sed '/# >>> GCM/,/# <<< GCM/d' "$shell_config" > "${shell_config}.tmp"
                mv "${shell_config}.tmp" "$shell_config"

                # Clean up extra blank lines
                awk 'NF || prev_blank {print} {prev_blank = !NF}' "$shell_config" > "${shell_config}.tmp" && mv "${shell_config}.tmp" "$shell_config"

                print_success "Cleaned GCM configuration in $(basename "$shell_config")"
                ((configs_modified++))
            fi
        fi
    done

    if [[ $configs_modified -eq 0 ]]; then
        print_info "No shell configurations found with GCM setup"
    else
        print_success "Cleaned $configs_modified shell configuration(s)"
    fi
}

# Remove entire gcm data directory with feedback
remove_gcm_dir() {
    local gcm_dir="$HOME/.gcm"

    print_step "Removing GCM data directory..."

    if [[ -d "$gcm_dir" ]]; then
        local dir_size=$(du -sh "$gcm_dir" 2>/dev/null | cut -f1 || echo "unknown size")
        print_info "Removing directory: $gcm_dir ($dir_size)"

        show_removal_progress "data directory"
        rm -rf "$gcm_dir"
        print_success "Removed GCM data directory"
    else
        print_warning "GCM directory not found at $gcm_dir"
    fi
}

# Remove git credential config for github.com
remove_git_credential() {
    print_step "Cleaning git credential config..."

    # Common hosts GCM might be configured for
    local cred_hosts=("https://github.com" "https://gitlab.com" "https://bitbucket.org" "https://dev.azure.com")
    local cleaned=false

    for host in "${cred_hosts[@]}"; do
        local helper_val
        helper_val=$(git config --global "credential.${host}.helper" 2>/dev/null || true)
        if [[ -n "$helper_val" ]]; then
            git config --global --unset-all "credential.${host}.helper" 2>/dev/null || true
            print_success "Removed credential helper for ${host}"
            cleaned=true
        fi
        local user_val
        user_val=$(git config --global "credential.${host}.username" 2>/dev/null || true)
        if [[ -n "$user_val" ]]; then
            git config --global --unset-all "credential.${host}.username" 2>/dev/null || true
            print_success "Removed credential username for ${host}"
            cleaned=true
        fi
    done

    # Remove any other credential entries referencing gcm
    local extra_keys
    extra_keys=$(git config --global --list 2>/dev/null | grep -i "credential.*helper.*gcm" | cut -d= -f1 || true)
    if [[ -n "$extra_keys" ]]; then
        while IFS= read -r key; do
            [[ -n "$key" ]] && git config --global --unset-all "$key" 2>/dev/null || true
            print_success "Removed $key"
            cleaned=true
        done <<< "$extra_keys"
    fi

    # Global credential.helper if it references gcm
    local global_cred
    global_cred=$(git config --global credential.helper 2>/dev/null || true)
    if echo "$global_cred" | grep -qi "gcm"; then
        git config --global --unset-all credential.helper 2>/dev/null || true
        print_success "Removed global credential.helper (gcm)"
        cleaned=true
    fi

    if [[ "$cleaned" == false ]]; then
        print_info "No GCM credential config found"
    fi
}

# Remove SSH keys generated by GCM
remove_ssh_keys() {
    local gcm_dir="$HOME/.gcm"
    local profiles_dir="${gcm_dir}/profiles"

    print_step "Checking for GCM-generated SSH keys..."

    local ssh_found=()
    if [[ -d "$profiles_dir" ]]; then
        for yaml in "$profiles_dir"/*.yaml; do
            [[ -f "$yaml" ]] || continue
            local profile_name
            profile_name=$(basename "$yaml" .yaml)
            for prefix in id_ed25519 id_rsa id_ecdsa; do
                local key_path="${HOME}/.ssh/${prefix}_${profile_name}"
                [[ -f "$key_path" ]]       && ssh_found+=("$key_path")
                [[ -f "${key_path}.pub" ]] && ssh_found+=("${key_path}.pub")
            done
        done
    fi

    if [[ ${#ssh_found[@]} -eq 0 ]]; then
        print_info "No GCM-generated SSH keys found"
        return
    fi

    echo "  Found SSH keys:"
    for f in "${ssh_found[@]}"; do echo "    $f"; done

    for f in "${ssh_found[@]}"; do
        rm -f "$f"
    done
    # Remove from ssh-agent
    if command -v ssh-add &>/dev/null; then
        for f in "${ssh_found[@]}"; do
            [[ "$f" == *.pub ]] && continue
            ssh-add -d "$f" 2>/dev/null || true
        done
    fi
    print_success "Removed ${#ssh_found[@]} SSH key file(s)"
}

# Remove GPG keys generated by GCM
remove_gpg_keys() {
    local gcm_dir="$HOME/.gcm"
    local profiles_dir="${gcm_dir}/profiles"

    print_step "Checking for GCM-generated GPG keys..."

    if ! command -v gpg &>/dev/null; then
        print_info "GPG not installed — skipping"
        return
    fi

    local gpg_key_ids=()
    if [[ -d "$profiles_dir" ]]; then
        for yaml in "$profiles_dir"/*.yaml; do
            [[ -f "$yaml" ]] || continue
            local kid
            kid=$(grep -E '^\s*key_id:' "$yaml" 2>/dev/null | awk '{print $2}' | tr -d '"' || true)
            [[ -n "$kid" ]] && gpg_key_ids+=("$kid")
        done
    fi

    if [[ ${#gpg_key_ids[@]} -eq 0 ]]; then
        print_info "No GCM GPG key IDs found in profiles"
        return
    fi

    echo "  Found GPG key IDs:"
    for kid in "${gpg_key_ids[@]}"; do echo "    $kid"; done

    for kid in "${gpg_key_ids[@]}"; do
        local fingerprint
        fingerprint=$(gpg --with-colons --fingerprint "$kid" 2>/dev/null \
            | awk -F: '/^fpr:/{print $10; exit}')
        if [[ -z "$fingerprint" ]]; then
            print_warning "GPG key $kid not found in keyring (already deleted?)"
            continue
        fi
        gpg --batch --yes --delete-secret-keys "$fingerprint" 2>/dev/null && \
            print_success "Deleted GPG secret key $kid" || \
            print_error "Failed to delete GPG secret key $kid"
        gpg --batch --yes --delete-keys "$fingerprint" 2>/dev/null && \
            print_success "Deleted GPG public key $kid" || \
            print_error "Failed to delete GPG public key $kid"
    done
}

# Remove git global/local identity set by GCM
remove_git_identity() {
    print_step "Removing git identity configuration..."

    local cleaned=false
    for key in user.name user.email user.signingkey commit.gpgsign gpg.format gpg.program core.sshCommand tag.gpgsign tag.forceSignAnnotated; do
        if git config --global "$key" &>/dev/null; then
            git config --global --unset-all "$key" 2>/dev/null || true
            print_success "Unset git global $key"
            cleaned=true
        fi
    done

    # Clean local repo if inside one
    if git rev-parse --is-inside-work-tree &>/dev/null; then
        local git_root
        git_root=$(git rev-parse --show-toplevel)
        for key in user.name user.email user.signingkey commit.gpgsign; do
            if git config --local "$key" &>/dev/null; then
                git config --local --unset-all "$key" 2>/dev/null || true
                print_success "Unset git local $key"
                cleaned=true
            fi
        done
        # Remove GCM markers
        if [[ -f "${git_root}/.gcm-profile" ]]; then
            rm -f "${git_root}/.gcm-profile"
            print_success "Removed .gcm-profile marker"
            cleaned=true
        fi
        if [[ -f "${git_root}/.git/gcm-session" ]]; then
            rm -f "${git_root}/.git/gcm-session"
            print_success "Removed .git/gcm-session marker"
            cleaned=true
        fi
    fi

    if [[ "$cleaned" == false ]]; then
        print_info "No git identity configuration found"
    fi
}

# Remove macOS Keychain entries
remove_keychain_entries() {
    print_step "Removing macOS Keychain entries..."

    if [[ "$(uname)" != "Darwin" ]] || ! command -v security &>/dev/null; then
        print_info "Not macOS or security command unavailable — skipping"
        return
    fi

    local cleaned=false

    # Remove ALL github.com internet password entries
    while security delete-internet-password -s github.com &>/dev/null; do
        cleaned=true
    done
    $cleaned && print_success "Removed github.com internet passwords from Keychain"

    # Remove generic passwords
    local cleaned2=false
    while security delete-generic-password -s github.com &>/dev/null; do
        cleaned2=true
    done
    $cleaned2 && print_success "Removed github.com generic passwords from Keychain"

    # Remove GCM-labeled entries
    local cleaned3=false
    while security delete-generic-password -l "gcm" &>/dev/null; do
        cleaned3=true
    done
    $cleaned3 && print_success "Removed gcm-labeled entries from Keychain"

    if [[ "$cleaned" == false && "$cleaned2" == false && "$cleaned3" == false ]]; then
        print_info "No GCM-related Keychain entries found"
    fi
}

# Remove git credential cache/store file entries
remove_credential_store() {
    print_step "Cleaning git credential cache & store..."

    local cleaned=false

    # Kill credential cache daemon
    if pgrep -f "git-credential-cache--daemon" &>/dev/null; then
        git credential-cache exit 2>/dev/null || true
        print_success "Stopped git-credential-cache daemon"
        cleaned=true
    fi

    # Clean ~/.git-credentials
    local cred_store="${HOME}/.git-credentials"
    if [[ -f "$cred_store" ]] && grep -qi "github.com\|gitlab.com" "$cred_store" 2>/dev/null; then
        grep -vi "github.com\|gitlab.com" "$cred_store" > "${cred_store}.tmp" 2>/dev/null || true
        mv "${cred_store}.tmp" "$cred_store"
        chmod 600 "$cred_store"
        print_success "Removed github/gitlab entries from $cred_store"
        cleaned=true
    fi

    # Clean XDG credential store
    local xdg_cred="${XDG_CONFIG_HOME:-${HOME}/.config}/git/credentials"
    if [[ -f "$xdg_cred" ]] && grep -qi "github.com\|gitlab.com" "$xdg_cred" 2>/dev/null; then
        grep -vi "github.com\|gitlab.com" "$xdg_cred" > "${xdg_cred}.tmp" 2>/dev/null || true
        mv "${xdg_cred}.tmp" "$xdg_cred"
        chmod 600 "$xdg_cred"
        print_success "Removed github/gitlab entries from $xdg_cred"
        cleaned=true
    fi

    if [[ "$cleaned" == false ]]; then
        print_info "No credential cache/store entries found"
    fi
}

# Scan and remove .gcm-profile markers from project directories
remove_project_markers() {
    print_step "Scanning for .gcm-profile and gcm-session markers..."

    local markers_found=0
    local scan_dirs=("${HOME}/projects" "${HOME}/Projects" "${HOME}/dev" "${HOME}/Dev" "${HOME}/src" "${HOME}/work" "${HOME}/Work" "${HOME}/repos" "${HOME}/code")

    for dir in "${scan_dirs[@]}"; do
        [[ -d "$dir" ]] || continue
        while IFS= read -r -d '' marker; do
            rm -f "$marker"
            markers_found=$((markers_found + 1))
        done < <(find "$dir" -maxdepth 4 -name ".gcm-profile" -print0 2>/dev/null)

        while IFS= read -r -d '' marker; do
            rm -f "$marker"
            markers_found=$((markers_found + 1))
        done < <(find "$dir" -maxdepth 5 -path "*/.git/gcm-session" -print0 2>/dev/null)
    done

    if [[ $markers_found -gt 0 ]]; then
        print_success "Removed $markers_found project marker(s)"
    else
        print_info "No project markers found"
    fi
}

# Remove shell completion files
remove_completions() {
    print_step "Removing shell completion files..."

    local completion_paths=(
        "${HOME}/.zsh/completions/_gcm"
        "${HOME}/.local/share/zsh/site-functions/_gcm"
        "/usr/local/share/zsh/site-functions/_gcm"
        "${HOME}/.bash_completion.d/gcm"
        "/etc/bash_completion.d/gcm"
        "${HOME}/.local/share/bash-completion/completions/gcm"
        "${HOME}/.config/fish/completions/gcm.fish"
        "/usr/local/share/fish/vendor_completions.d/gcm.fish"
    )

    local found=false
    for cpath in "${completion_paths[@]}"; do
        if [[ -f "$cpath" ]]; then
            rm -f "$cpath"
            print_success "Removed $cpath"
            found=true
        fi
    done

    if [[ "$found" == false ]]; then
        print_info "No GCM completion files found"
    fi
}

# Remove XDG config and temp files
remove_xdg_and_temp() {
    print_step "Removing XDG config and temp files..."

    local xdg_gcm="${XDG_CONFIG_HOME:-${HOME}/.config}/gcm"
    if [[ -d "$xdg_gcm" ]]; then
        rm -rf "$xdg_gcm"
        print_success "Removed $xdg_gcm"
    fi

    # Remove temp files
    find /tmp -maxdepth 1 -name "gcm-*" -exec rm -rf {} + 2>/dev/null || true

    # Remove leftover backup files from previous resets
    local shell_files=("${HOME}/.zshrc" "${HOME}/.bashrc" "${HOME}/.bash_profile" "${HOME}/.profile" "${HOME}/.zprofile")
    for rc in "${shell_files[@]}"; do
        [[ -f "${rc}.gcm-reset-backup" ]] && rm -f "${rc}.gcm-reset-backup"
    done

    # Clear shell hash table
    hash -r 2>/dev/null || true
    print_success "Cleared shell hash table and temp files"
}

# Show uninstall options
show_uninstall_options() {
    print_separator "═"
    echo -e "${BOLD}${WHITE} ${QUESTION}  UNINSTALLATION OPTIONS${NC}"
    print_separator "═"
    echo
    echo -e "${CYAN}${BOLD}1)${NC} ${WHITE}Minimal Removal${NC} ${DIM}(Recommended)${NC}"
    echo "   • Remove gcm binary"
    echo "   • Clean shell integration"
    echo -e "   • ${GREEN}Keep${NC} profiles, tokens, SSH keys, and configuration"
    echo
    echo -e "${RED}${BOLD}2)${NC} ${WHITE}Complete Removal${NC} ${DIM}(Permanent)${NC}"
    echo "   • Remove gcm binary"
    echo "   • Clean shell integration"
    echo -e "   • ${RED}Delete${NC} all profiles and configuration (~/.gcm)"
    echo -e "   • ${RED}Delete${NC} encrypted tokens, backup archives, audit logs"
    echo
    echo -e "${RED}${BOLD}3)${NC} ${WHITE}Nuclear Clean${NC} ${DIM}(Everything — no trace left)${NC}"
    echo "   • Everything in option 2, plus:"
    echo -e "   • ${RED}Delete${NC} git global identity (user.name, user.email, signingkey)"
    echo -e "   • ${RED}Delete${NC} git credential config for ALL hosts"
    echo -e "   • ${RED}Delete${NC} GCM-generated SSH keys and GPG keys"
    echo -e "   • ${RED}Delete${NC} SSH agent loaded keys"
    echo -e "   • ${RED}Delete${NC} git local identity and GCM markers (recursive scan)"
    echo -e "   • ${RED}Delete${NC} macOS Keychain entries for github.com"
    echo -e "   • ${RED}Delete${NC} git credential cache/store entries"
    echo -e "   • ${RED}Delete${NC} shell completion files"
    echo -e "   • ${RED}Delete${NC} XDG config, temp files, hash cache"
    echo
    echo -e "${GRAY}${BOLD}4)${NC} ${WHITE}Cancel${NC}"
    echo "   • Exit without making any changes"
    echo
    print_separator "┄"
}

# Show completion message
show_completion() {
    local mode="$1"

    echo
    print_separator "═"
    echo

    case "$mode" in
        nuclear)
            echo -e "${GREEN}${BOLD} ${CHECKMARK}  NUCLEAR CLEAN SUCCESSFUL — NO TRACE LEFT!${NC}"
            echo
            print_separator "┄"
            echo -e "${BOLD}${WHITE}What was removed:${NC}"
            echo " • gcm binary (from all locations)"
            echo " • Shell integration (all shell rc files)"
            echo " • Git global identity (user.name, user.email, signingkey, gpgsign)"
            echo " • Git local identity and GCM markers (recursive scan)"
            echo " • Git credential config for all hosts"
            echo " • Git credential cache/store entries"
            echo " • GCM-generated SSH keys + flushed from ssh-agent"
            echo " • GCM-generated GPG keys (secret + public)"
            echo " • All profiles, tokens, config, backups, cache (~/.gcm)"
            echo " • macOS Keychain entries"
            echo " • Shell completion files"
            echo " • XDG config, temp files"
            ;;
        complete)
            echo -e "${GREEN}${BOLD} ${CHECKMARK}  COMPLETE UNINSTALLATION SUCCESSFUL!${NC}"
            echo
            print_separator "┄"
            echo -e "${BOLD}${WHITE}What was removed:${NC}"
            echo " • gcm binary (from all locations)"
            echo " • Shell integration and PATH configurations"
            echo " • All profiles, tokens, and configuration (~/.gcm)"
            ;;
        *)
            echo -e "${GREEN}${BOLD} ${CHECKMARK}  MINIMAL UNINSTALLATION COMPLETE!${NC}"
            echo
            print_separator "┄"
            echo -e "${BOLD}${WHITE}What was removed:${NC}"
            echo " • gcm binary"
            echo " • Shell integration and PATH configurations"
            echo
            echo -e "${BOLD}${WHITE}What was kept:${NC}"
            echo " • Profiles and configuration in ~/.gcm"
            echo " • SSH keys (in ~/.ssh)"
            echo " • Encrypted tokens and backup archives"
            ;;
    esac

    print_separator "┄"
    echo -e "${BOLD}${WHITE}Final Steps:${NC}"
    echo " 1. Restart your terminal to complete the process"
    echo " 2. Verify with 'which gcm' (should show 'not found')"
    if [[ "$mode" == "minimal" ]]; then
        echo " 3. Manually remove '~/.gcm' if you change your mind later"
    fi
    print_separator "┄"
    echo "Thank you for using GCM!"
    print_separator "═"
    echo
}

# Main uninstallation function
main() {
    print_header

    print_info "Starting GCM uninstallation process..."
    echo

    # Check if gcm is installed
    if ! check_gcm_installation; then
        print_warning "GCM does not appear to be installed on this system"
        echo
        print_separator "┄"
        echo -e "${BOLD}${WHITE}No GCM installation found!${NC}"
        print_separator "┄"
        echo "It looks like GCM is not installed or has already been removed."
        echo "Common reasons:"
        echo " • GCM was never installed"
        echo " • GCM was already uninstalled"
        echo " • GCM was installed in a different location"
        echo " • Installation was incomplete or corrupted"
        print_separator "┄"
        echo

        local response
        response=$(get_user_input "Do you want to clean any remaining traces? ${DIM}(y/N):${NC} ")

        if [[ ! "$response" =~ ^[Yy]$ ]]; then
            echo
            print_info "Exiting without making changes"
            print_separator "═"
            echo -e "${DIM}${GRAY}No changes were made to your system.${NC}"
            print_separator "═"
            echo
            exit 0
        fi

        echo
        print_info "Proceeding with cleanup of any remaining traces..."
        echo
    else
        print_success "GCM installation detected"
        echo
    fi

    # Show uninstall options
    show_uninstall_options

    # Get user choice
    local response
    response=$(get_user_input "Choose an option ${DIM}(1/2/3/4):${NC} ")

    echo

    case "$response" in
        1)
            print_info "Proceeding with minimal removal..."
            echo
            show_removal_preview "minimal"

            print_separator "┄"
            echo -e "${YELLOW}${BOLD} ${STOP}  FINAL CONFIRMATION${NC}"
            print_separator "┄"
            local confirm
            confirm=$(get_user_input "Proceed with minimal removal? ${DIM}(y/N):${NC} ")

            if [[ "$confirm" =~ ^[Yy]$ ]]; then
                echo
                remove_binary
                echo
                remove_from_path
                echo
                show_completion "minimal"
            else
                echo
                print_info "Uninstallation cancelled by user"
                print_separator "═"
                echo -e "${DIM}${GRAY}No changes were made to your system.${NC}"
                print_separator "═"
                echo
            fi
            ;;

        2)
            print_info "Proceeding with complete removal..."
            echo
            show_removal_preview "complete"

            print_separator "┄"
            echo -e "${RED}${BOLD} ${STOP}  DANGER: COMPLETE REMOVAL${NC}"
            print_separator "┄"
            echo -e "${RED}This will permanently delete ALL GCM data including profiles, tokens, and backups!${NC}"
            print_separator "┄"
            local confirm
            confirm=$(get_user_input "Type 'DELETE' to confirm complete removal: ")

            if [[ "$confirm" == "DELETE" ]]; then
                echo
                remove_binary
                echo
                remove_from_path
                echo
                remove_gcm_dir
                echo
                show_completion "complete"
            else
                echo
                print_info "Uninstallation cancelled - confirmation text did not match"
                print_separator "═"
                echo -e "${DIM}${GRAY}No changes were made to your system.${NC}"
                print_separator "═"
                echo
            fi
            ;;

        3)
            print_info "Proceeding with NUCLEAR clean..."
            echo
            show_removal_preview "complete"

            print_separator "┄"
            echo -e "${RED}${BOLD} ${STOP}  DANGER: NUCLEAR CLEAN — NO TRACE LEFT${NC}"
            print_separator "┄"
            echo -e "${RED}This will permanently delete EVERYTHING: binary, data, SSH keys, GPG keys, git identity, credentials!${NC}"
            print_separator "┄"
            local confirm
            confirm=$(get_user_input "Type 'NUKE' to confirm nuclear clean: ")

            if [[ "$confirm" == "NUKE" ]]; then
                echo
                remove_binary
                echo
                remove_from_path
                echo
                remove_git_identity
                echo
                remove_git_credential
                echo
                remove_ssh_keys
                echo
                remove_gpg_keys
                echo
                remove_gcm_dir
                echo
                remove_keychain_entries
                echo
                remove_credential_store
                echo
                remove_project_markers
                echo
                remove_completions
                echo
                remove_xdg_and_temp
                echo
                show_completion "nuclear"
            else
                echo
                print_info "Uninstallation cancelled - confirmation text did not match"
                print_separator "═"
                echo -e "${DIM}${GRAY}No changes were made to your system.${NC}"
                print_separator "═"
                echo
            fi
            ;;

        *)
            echo
            print_info "Uninstallation cancelled by user"
            print_separator "═"
            echo -e "${DIM}${GRAY}No changes were made to your system.${NC}"
            print_separator "═"
            echo
            ;;
    esac
}

# Trap to ensure clean exit
trap 'echo -e "\n${RED}Uninstallation interrupted. Partial changes may have been made.${NC}"; exit 1' INT TERM

# Run main function
main
