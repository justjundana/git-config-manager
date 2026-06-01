#!/usr/bin/env bash
# GCM (Git Config Manager) installation script
# This script installs gcm to $HOME/.local/bin and configures shell integration
set -e

# Global flags
QUIET_MODE=false
SPECIFIC_VERSION=""
ADD_TO_PATH=false
INIT_AFTER_INSTALL=false

# Parse command line arguments
parse_arguments() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --quiet|-q)
                QUIET_MODE=true
                shift
                ;;
            --version|-v)
                SPECIFIC_VERSION="$2"
                shift 2
                ;;
            --add-to-path)
                ADD_TO_PATH=true
                shift
                ;;
            --init)
                INIT_AFTER_INSTALL=true
                shift
                ;;
            --help|-h)
                show_help
                exit 0
                ;;
            *)
                print_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

# Show help information
show_help() {
    echo "GCM installer - Git Config Manager Installation Script"
    echo
    echo "Usage: $0 [OPTIONS]"
    echo
    echo "Options:"
    echo "  --quiet, -q         Run in quiet mode (minimal output)"
    echo "  --version, -v VER   Install specific version (e.g., v1.0.0)"
    echo "  --add-to-path       Add the install directory to your shell config"
    echo "  --init              Run 'gcm init' after install (mutates shell/git config)"
    echo "  --help, -h          Show this help message"
    echo
    echo "Examples:"
    echo "  $0                  # Install latest version"
    echo "  $0 --quiet          # Install quietly"
    echo "  $0 --version v1.0.0 # Install specific version"
    echo "  $0 --add-to-path    # Install and update shell PATH explicitly"
    echo "  $0 --init           # Install and explicitly initialize GCM"
}

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
UNDERLINE='\033[4m'

# Unicode characters for better UI
CHECKMARK="✓"
CROSSMARK="✗"
ARROW="→"
DOWNLOAD="⬇"
WARNING="⚠"
INSTALL="📦"
INFO="ℹ"
ROCKET="🚀"
GEAR="⚙"

# Terminal width detection
TERM_WIDTH=$(tput cols 2>/dev/null || echo 80)

# Print separator line
print_separator() {
    local char="${1:--}"
    printf "%*s\n" "$TERM_WIDTH" | tr ' ' "$char"
}

# Print fancy header
print_header() {
    [[ "$QUIET_MODE" == "true" ]] && return
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
    echo -e "${BOLD}${WHITE}                Git Config Manager Installer${NC}"
    echo -e "${DIM}${GRAY}            Fast and secure installation process${NC}"
    echo
    print_separator "═"
    echo
}

# Print functions with icons and styling
print_info() {
    [[ "$QUIET_MODE" == "true" ]] && return
    echo -e "${BLUE}${BOLD} ${INFO}  INFO${NC} ${GRAY}│${NC} $1"
}

print_success() {
    [[ "$QUIET_MODE" == "true" ]] && return
    echo -e "${GREEN}${BOLD} ${CHECKMARK}  SUCCESS${NC} ${GRAY}│${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}${BOLD} ${WARNING}  WARNING${NC} ${GRAY}│${NC} $1"
}

print_error() {
    echo -e "${RED}${BOLD} ${CROSSMARK}  ERROR${NC} ${GRAY}│${NC} $1"
}

print_step() {
    [[ "$QUIET_MODE" == "true" ]] && return
    echo -e "${PURPLE}${BOLD} ${ARROW}  STEP${NC} ${GRAY}│${NC} $1"
}

print_install() {
    [[ "$QUIET_MODE" == "true" ]] && return
    echo -e "${CYAN}${BOLD} ${INSTALL}  INSTALLING${NC} ${GRAY}│${NC} $1"
}

# Check if running on Windows (Git Bash)
is_windows() {
    [[ "$OSTYPE" == "msys" || "$OSTYPE" == "win32" ]]
}

# Detect current shell and return appropriate config file
detect_shell_config() {
    local shell_name=""
    local config_file=""

    case "$(basename "$SHELL")" in
        zsh)
            shell_name="zsh"
            config_file="$HOME/.zshrc"
            ;;
        bash)
            shell_name="bash"
            if [[ "$OSTYPE" == "darwin"* ]]; then
                config_file="$HOME/.bash_profile"
            else
                config_file="$HOME/.bashrc"
            fi
            ;;
        fish)
            shell_name="fish"
            config_file="$HOME/.config/fish/config.fish"
            ;;
        *)
            if [ -n "$ZSH_VERSION" ]; then
                shell_name="zsh"
                config_file="$HOME/.zshrc"
            elif [ -n "$BASH_VERSION" ]; then
                shell_name="bash"
                if [[ "$OSTYPE" == "darwin"* ]]; then
                    config_file="$HOME/.bash_profile"
                else
                    config_file="$HOME/.bashrc"
                fi
            elif [ -n "$FISH_VERSION" ]; then
                shell_name="fish"
                config_file="$HOME/.config/fish/config.fish"
            else
                shell_name="shell"
                config_file="your shell's configuration file"
            fi
            ;;
    esac

    echo "${shell_name}:${config_file}"
}

# Get restart instruction based on OS and shell
get_restart_instruction() {
    local shell_info
    shell_info=$(detect_shell_config)
    local shell_name=$(echo "$shell_info" | cut -d':' -f1)
    local config_file=$(echo "$shell_info" | cut -d':' -f2)

    if is_windows; then
        echo "Please restart your terminal or PowerShell window"
    else
        case "$shell_name" in
            bash|zsh)
                echo "Please restart your terminal or run 'source $config_file'"
                ;;
            fish)
                echo "Please restart your terminal or run 'source $config_file'"
                ;;
            *)
                echo "Please restart your terminal or reload your shell configuration"
                ;;
        esac
    fi
}

# Detect OS and architecture
detect_platform() {
    local os=""
    local arch=""

    case "$(uname -s)" in
        Linux*)     os=linux;;
        Darwin*)    os=darwin;;
        MINGW*)     os=windows;;
        MSYS*)      os=windows;;
        *)          print_error "Unsupported operating system: $(uname -s)"; exit 1;;
    esac

    case "$(uname -m)" in
        x86_64)     arch=amd64;;
        aarch64)    arch=arm64;;
        arm64)      arch=arm64;;
        armv7l)     arch=arm;;
        i386|i686)  arch=386;;
        *)          print_error "Unsupported architecture: $(uname -m)"; exit 1;;
    esac

    if [[ "$os" == "windows" ]]; then
        arch=amd64
    fi

    echo "${os}/${arch}"
}

# Get shell configuration files
get_shell_configs() {
    local configs=("$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.zshrc")
    [[ -f "$HOME/.config/fish/config.fish" ]] && configs+=("$HOME/.config/fish/config.fish")
    printf '%s ' "${configs[@]}"
}

# Download a URL to a file using curl or wget with fail-fast semantics.
download_file() {
    local url="$1"
    local output="$2"

    if command -v curl >/dev/null 2>&1; then
        if [[ "$QUIET_MODE" == "true" ]]; then
            curl -fsSL -o "$output" "$url"
        else
            curl -fL --progress-bar -o "$output" "$url"
        fi
    elif command -v wget >/dev/null 2>&1; then
        if [[ "$QUIET_MODE" == "true" ]]; then
            wget -qO "$output" "$url"
        else
            wget --show-progress -qO "$output" "$url"
        fi
    else
        print_error "Either curl or wget is required to download gcm"
        return 1
    fi
}

# Get the latest release version from GitHub
get_latest_version() {
    if [[ -n "$SPECIFIC_VERSION" ]]; then
        echo "$SPECIFIC_VERSION"
        return
    fi

    local version=""
    if command -v curl >/dev/null 2>&1; then
        version=$(curl -fsSL https://api.github.com/repos/justjundana/git-config-manager/releases/latest | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    elif command -v wget >/dev/null 2>&1; then
        version=$(wget -qO- https://api.github.com/repos/justjundana/git-config-manager/releases/latest | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    else
        print_error "Either curl or wget is required to download gcm"
        exit 1
    fi

    if [[ -z "$version" ]]; then
        print_error "Failed to get latest version information"
        exit 1
    fi

    echo "$version"
}

release_asset_filename() {
    local version="$1"
    local platform="$2"
    local os arch artifact_version ext

    os=$(echo "$platform" | cut -d'/' -f1)
    arch=$(echo "$platform" | cut -d'/' -f2)
    artifact_version="${version#v}"
    ext="tar.gz"
    if [[ "$os" == "windows" ]]; then
        ext="zip"
    fi

    echo "gcm_${artifact_version}_${os}_${arch}.${ext}"
}

compute_sha256() {
    local file="$1"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$file" | awk '{print $1}'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$file" | awk '{print $1}'
    else
        print_error "sha256sum or shasum is required to verify release checksums"
        return 1
    fi
}

verify_checksum() {
    local file="$1"
    local checksums="$2"
    local filename expected actual

    filename=$(basename "$file")
    expected=$(awk -v target="$filename" '$2 == target {print $1}' "$checksums")
    if [[ -z "$expected" ]]; then
        print_error "No checksum entry found for $filename"
        return 1
    fi

    actual=$(compute_sha256 "$file")
    if [[ "$actual" != "$expected" ]]; then
        print_error "Checksum mismatch for $filename"
        print_error "Expected: $expected"
        print_error "Actual:   $actual"
        return 1
    fi

    print_success "Checksum verified for $filename"
}

extract_archive() {
    local archive="$1"
    local target_dir="$2"

    mkdir -p "$target_dir"
    case "$archive" in
        *.tar.gz)
            tar -xzf "$archive" -C "$target_dir"
            ;;
        *.zip)
            if command -v unzip >/dev/null 2>&1; then
                unzip -q "$archive" -d "$target_dir"
            elif command -v powershell.exe >/dev/null 2>&1; then
                powershell.exe -NoProfile -Command "Expand-Archive -LiteralPath '$archive' -DestinationPath '$target_dir' -Force"
            else
                print_error "unzip is required to extract Windows release archives"
                return 1
            fi
            ;;
        *)
            print_error "Unsupported release archive: $archive"
            return 1
            ;;
    esac
}

# Verify binary after checksum-protected extraction.
verify_binary() {
    local binary_path="$1"

    if [[ ! -f "$binary_path" ]]; then
        print_error "Binary file not found: $binary_path"
        return 1
    fi

    if [[ ! -x "$binary_path" ]]; then
        print_error "Binary is not executable: $binary_path"
        return 1
    fi

    # Check file size (should be > 1MB for a Go binary)
    local file_size
    if command -v stat >/dev/null 2>&1; then
        case "$(uname -s)" in
            Darwin*) file_size=$(stat -f%z "$binary_path") ;;
            *) file_size=$(stat -c%s "$binary_path") ;;
        esac

        if [[ $file_size -lt 1048576 ]]; then
            print_warning "Binary file seems unusually small ($file_size bytes)"
        fi
    fi

    # Try to get version to ensure it's a valid gcm binary
    if ! "$binary_path" version >/dev/null 2>&1; then
        print_error "Downloaded binary appears to be corrupted or invalid"
        return 1
    fi

    print_success "Binary validation completed"
    return 0
}

# Background spinner PID
_SPINNER_PID=""

# Start a background spinner that runs during actual work
# Uses ASCII-safe characters that work in all terminals
start_spinner() {
    [[ "$QUIET_MODE" == "true" ]] && return
    local msg="$1"
    local color="${2:-$CYAN}"
    (
        trap 'exit 0' TERM
        local chars='|/-\\'
        local i=0
        while true; do
            printf "\r   ${DIM}%s... ${color}%c${NC} " "$msg" "${chars:$((i % 4)):1}"
            i=$((i + 1))
            sleep 0.1
        done
    ) &
    _SPINNER_PID=$!
}

# Stop the spinner with success message
stop_spinner() {
    [[ "$QUIET_MODE" == "true" ]] && return
    local msg="$1"
    if [[ -n "$_SPINNER_PID" ]]; then
        kill "$_SPINNER_PID" 2>/dev/null
        wait "$_SPINNER_PID" 2>/dev/null || true
        _SPINNER_PID=""
    fi
    printf "\r   ${GREEN}${CHECKMARK}${NC} %s successfully.      \n" "$msg"
}

# Stop the spinner with failure
stop_spinner_fail() {
    [[ "$QUIET_MODE" == "true" ]] && return
    local msg="$1"
    if [[ -n "$_SPINNER_PID" ]]; then
        kill "$_SPINNER_PID" 2>/dev/null
        wait "$_SPINNER_PID" 2>/dev/null || true
        _SPINNER_PID=""
    fi
    printf "\r   ${RED}${CROSSMARK}${NC} %s failed.      \n" "$msg"
}

# Download, checksum-verify, and extract the release archive.
download_binary() {
    local version="$1"
    local platform="$2"
    local install_dir="$3"

    local os=$(echo "$platform" | cut -d'/' -f1)
    local arch=$(echo "$platform" | cut -d'/' -f2)

    local binary_name="gcm"
    if [[ "$os" == "windows" ]]; then
        binary_name="gcm.exe"
    fi

    local asset_name
    asset_name=$(release_asset_filename "$version" "$platform")
    local base_url="https://github.com/justjundana/git-config-manager/releases/download/${version}"
    local archive_url="${base_url}/${asset_name}"
    local checksums_url="${base_url}/checksums.txt"
    local tmp_dir archive_path checksums_path extract_dir extracted_binary
    tmp_dir=$(mktemp -d 2>/dev/null || mktemp -d -t gcm-install)
    archive_path="${tmp_dir}/${asset_name}"
    checksums_path="${tmp_dir}/checksums.txt"
    extract_dir="${tmp_dir}/extract"

    print_step "Downloading gcm ${version} for ${platform}..."
    print_info "Archive URL: $archive_url"
    print_info "Checksums URL: $checksums_url"

    mkdir -p "$install_dir"

    echo -e "   ${DIM}Downloading release archive...${NC}"
    if ! download_file "$archive_url" "$archive_path"; then
        print_error "Failed to download gcm archive from $archive_url"
        rm -rf "$tmp_dir"
        exit 1
    fi
    if ! download_file "$checksums_url" "$checksums_path"; then
        print_error "Failed to download release checksums from $checksums_url"
        rm -rf "$tmp_dir"
        exit 1
    fi
    echo -e "   ${GREEN}${CHECKMARK}${NC} Downloaded release archive and checksums."

    if ! verify_checksum "$archive_path" "$checksums_path"; then
        rm -f "$archive_path"
        rm -rf "$tmp_dir"
        exit 1
    fi

    if ! extract_archive "$archive_path" "$extract_dir"; then
        rm -rf "$tmp_dir"
        exit 1
    fi
    extracted_binary=$(find "$extract_dir" -type f -name "$binary_name" -perm -u+x -print -quit 2>/dev/null || true)
    if [[ -z "$extracted_binary" ]]; then
        extracted_binary=$(find "$extract_dir" -type f -name "$binary_name" -print -quit 2>/dev/null || true)
    fi
    if [[ -z "$extracted_binary" ]]; then
        print_error "Release archive did not contain $binary_name"
        rm -rf "$tmp_dir"
        exit 1
    fi

    cp "$extracted_binary" "${install_dir}/${binary_name}"
    chmod +x "${install_dir}/${binary_name}"

    if ! verify_binary "${install_dir}/${binary_name}"; then
        print_error "Binary validation failed"
        rm -f "${install_dir}/${binary_name}"
        rm -rf "$tmp_dir"
        exit 1
    fi

    rm -rf "$tmp_dir"
    print_success "Installed verified gcm binary to ${install_dir}/${binary_name}"
}

# Add to PATH only when explicitly requested.
add_to_path_if_requested() {
    local install_dir="$1"

    if [[ "$ADD_TO_PATH" != "true" ]]; then
        print_info "Shell PATH was not modified. To opt in, rerun with --add-to-path or add this manually:"
        echo "  export PATH=\"${install_dir}:\$PATH\""
        return
    fi

    print_step "Configuring shell PATH..."
    configure_path_manually "$install_dir"
}

# Run gcm init only when explicitly requested.
run_init_if_requested() {
    local install_dir="$1"
    local gcm_binary="${install_dir}/gcm"

    if [[ "$INIT_AFTER_INSTALL" != "true" ]]; then
        print_info "GCM initialization was not run. Run 'gcm init' when you are ready to install shell hooks."
        return
    fi

    if [ ! -x "$gcm_binary" ]; then
        print_error "gcm binary not found or not executable at $gcm_binary"
        exit 1
    fi

    print_step "Initializing GCM by explicit request..."
    start_spinner "Installing shell configuration" "$PURPLE"

    local init_output
    if init_output=$("$gcm_binary" init 2>&1); then
        stop_spinner "Installed shell configuration"
        print_success "Shell configuration completed successfully"
        if [[ -n "$init_output" ]]; then
            echo "$init_output"
        fi
    else
        stop_spinner_fail "Shell configuration"
        print_error "gcm init failed. No fallback shell mutation was applied."
        [[ -n "$init_output" ]] && echo "$init_output"
        exit 1
    fi
}

# Manual PATH configuration fallback
configure_path_manually() {
    local install_dir="$1"
    local shell_info
    shell_info=$(detect_shell_config)
    local shell_name=$(echo "$shell_info" | cut -d':' -f1)
    local config_file=$(echo "$shell_info" | cut -d':' -f2)

    local path_line="export PATH=\"${install_dir}:\$PATH\""
    local marker_start="# >>> GCM (Git Config Manager) >>>"
    local marker_end="# <<< GCM (Git Config Manager) <<<"

    if [[ "$config_file" == "your shell's configuration file" ]]; then
        print_warning "Could not detect shell config file. Please add manually:"
        echo "  $path_line"
        return
    fi

    # Check if already configured
    if [[ -f "$config_file" ]] && grep -q "$marker_start" "$config_file" 2>/dev/null; then
        print_info "PATH already configured in $config_file"
        return
    fi

    # Append PATH configuration
    {
        echo ""
        echo "$marker_start"
        echo "$path_line"
        echo "$marker_end"
    } >> "$config_file"

    print_success "Added gcm to PATH in $config_file"
}

# Show system information
show_system_info() {
    local platform="$1"
    local version="$2"
    local install_dir="$3"

    print_separator "┄"
    echo -e "${BOLD}${WHITE}System Information:${NC}"
    print_separator "┄"

    local os=$(echo "$platform" | cut -d'/' -f1)
    local arch=$(echo "$platform" | cut -d'/' -f2)

    local os_capitalized=$(echo "$os" | awk '{print toupper(substr($0,1,1)) substr($0,2)}')

    echo -e "${GREEN} ${CHECKMARK}${NC} Operating System: ${BOLD}${os_capitalized}${NC}"
    echo -e "${GREEN} ${CHECKMARK}${NC} Architecture: ${BOLD}${arch}${NC}"
    echo -e "${GREEN} ${CHECKMARK}${NC} Version: ${BOLD}${version}${NC}"
    echo -e "${BLUE} ${INFO}${NC} Install Directory: ${BOLD}${install_dir}${NC}"

    print_separator "┄"
    echo
}

# Show completion message
show_completion() {
    local version="$1"
    local restart_instruction="$2"

    echo
    print_separator "═"
    echo
    echo -e "${GREEN}${BOLD} ${ROCKET}  INSTALLATION SUCCESSFUL!${NC}"
    echo
    print_separator "┄"
    echo -e "${BOLD}${WHITE}What was installed:${NC}"
    echo " • gcm binary"
    echo " • Release checksum verification"
    if [[ "$ADD_TO_PATH" == "true" ]]; then
        echo " • Shell PATH configuration"
    fi
    if [[ "$INIT_AFTER_INSTALL" == "true" ]]; then
        echo " • Shell integration (auto-switch on cd)"
    fi
    print_separator "┄"
    echo -e "${BOLD}${WHITE}Next Steps:${NC}"
    echo " 1. $restart_instruction"
    echo " 2. Verify with 'gcm version'"
    echo " 3. Initialize with 'gcm init' when you want shell hooks"
    echo " 4. Create your first profile with 'gcm profile create <name>'"
    print_separator "┄"
    echo -e "${BOLD}${WHITE}Quick Commands:${NC}"
    echo " • gcm profile create work   - Create a profile"
    echo " • gcm use work              - Switch to a profile"
    echo " • gcm ssh generate work     - Generate SSH key"
    echo " • gcm github login-oauth    - Authenticate with GitHub"
    echo " • gcm doctor                - Check system health"
    print_separator "┄"
    echo "Welcome to GCM! 🎉"
    print_separator "═"
    echo
}

# Check if gcm is already installed
check_existing_installation() {
    local install_dir="$HOME/.local/bin"
    local gcm_dir="$HOME/.gcm"
    local shell_configs_str
    shell_configs_str=$(get_shell_configs)
    local shell_configs=($shell_configs_str)
    local binary_found=false
    local binary_valid=false
    local command_found=false

    print_step "Checking for existing installation..."

    # Check if gcm binary exists in common locations
    local found_path=""
    if [[ -f "$install_dir/gcm" ]]; then
        binary_found=true
        found_path="$install_dir/gcm"
    elif [[ -f "/usr/local/bin/gcm" ]]; then
        binary_found=true
        found_path="/usr/local/bin/gcm"
    fi

    # Verify binary is actually valid (executable and can run)
    if [[ "$binary_found" == true && -x "$found_path" ]]; then
        if "$found_path" version >/dev/null 2>&1; then
            binary_valid=true
        fi
    fi

    # Check if gcm command is available in PATH and works
    if command -v gcm >/dev/null 2>&1; then
        if gcm version >/dev/null 2>&1; then
            command_found=true
        fi
    fi

    # If binary exists but is broken (corrupted/partial download), remove it
    if [[ "$binary_found" == true && "$binary_valid" == false ]]; then
        print_warning "Found corrupted/incomplete gcm binary at $found_path"
        print_info "Removing broken binary and proceeding with fresh install..."
        rm -f "$found_path"
        echo
        return
    fi

    # Only block if we have a WORKING installation
    if [[ "$binary_valid" == true || "$command_found" == true ]]; then
        echo
        print_separator "┄"
        echo -e "${BOLD}${WHITE}Existing Installation Detected:${NC}"
        print_separator "┄"

        if [[ "$binary_valid" == true ]]; then
            echo -e "${GREEN} ${CHECKMARK}${NC} Binary found: ${BOLD}${found_path}${NC}"
        fi

        if [[ "$command_found" == true ]]; then
            local version=$(gcm version 2>/dev/null | head -1 || echo "unknown")
            echo -e "${GREEN} ${CHECKMARK}${NC} Command available: ${BOLD}gcm${NC} ${DIM}($version)${NC}"
        fi

        if [[ -d "$gcm_dir" ]]; then
            local dir_size=$(du -sh "$gcm_dir" 2>/dev/null | cut -f1 || echo "unknown")
            echo -e "${BLUE} ${INFO}${NC} Data directory: ${BOLD}$gcm_dir${NC} ${DIM}($dir_size)${NC}"
        fi

        print_separator "┄"
        echo
        print_warning "GCM is already installed on this system!"
        echo
        print_separator "┄"
        echo -e "${BOLD}${WHITE}What you can do:${NC}"
        echo " • Run 'gcm version' to check current version"
        echo " • Run 'gcm --help' to see available commands"
        echo " • Use the uninstaller script first if you need to reinstall"
        echo " • Run 'gcm doctor' to check system health"
        print_separator "┄"
        echo
        print_separator "═"
        echo -e "${DIM}${GRAY}Installation cancelled - gcm already exists${NC}"
        print_separator "═"
        echo
        exit 0
    else
        print_success "No existing installation found - proceeding with fresh install"
        echo
    fi
}

# Main installation function
main() {
    parse_arguments "$@"

    print_header

    print_info "Starting GCM installation process..."
    echo

    # Check for existing installation first
    check_existing_installation

    # Detect platform
    print_step "Detecting system platform..."
    local platform
    platform=$(detect_platform)
    print_success "Detected platform: ${BOLD}$platform${NC}"
    echo

    # Get latest version
    print_step "Fetching latest version information..."
    local version
    version=$(get_latest_version)
    print_success "Latest version: ${BOLD}$version${NC}"
    echo

    # Set installation directory
    local install_dir="$HOME/.local/bin"
    print_info "Installation directory: ${BOLD}$install_dir${NC}"
    echo

    # Show system info
    show_system_info "$platform" "$version" "$install_dir"

    # Download binary
    download_binary "$version" "$platform" "$install_dir"
    echo

    # Configure PATH only when explicitly requested.
    add_to_path_if_requested "$install_dir"
    echo

    # Initialize only when explicitly requested.
    run_init_if_requested "$install_dir"
    echo

    # Verify installation
    print_step "Verifying installation..."
    if "$install_dir/gcm" version >/dev/null 2>&1; then
        local installed_version=$("$install_dir/gcm" version 2>/dev/null | head -1 || echo "unknown")
        print_success "Installation verified: ${BOLD}$installed_version${NC}"

        local restart_instruction
        restart_instruction=$(get_restart_instruction)

        show_completion "$version" "$restart_instruction"
    else
        print_warning "Installation completed, but verification failed"
        echo
        print_separator "┄"
        echo -e "${BOLD}${WHITE}Manual Steps Required:${NC}"
        echo " 1. Restart your terminal"
        echo " 2. Try running 'gcm version'"
        echo " 3. If issues persist, run 'gcm init' manually"
        print_separator "┄"
        echo
    fi
}

# Trap to ensure clean exit and kill spinner
trap '
    if [[ -n "$_SPINNER_PID" ]]; then
        kill "$_SPINNER_PID" 2>/dev/null
        wait "$_SPINNER_PID" 2>/dev/null || true
    fi
    echo -e "\n${RED}Installation interrupted. Partial installation may have occurred.${NC}"
    exit 1
' INT TERM

# Run main function
main "$@"
