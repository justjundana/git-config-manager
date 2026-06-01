// Package shell provides shell integration for GCM.
package shell

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/justjundana/git-config-manager/pkg/logger"
)

// ShellType represents the user's shell.
type ShellType string

const (
	ShellBash       ShellType = "bash"
	ShellZsh        ShellType = "zsh"
	ShellFish       ShellType = "fish"
	ShellPowerShell ShellType = "powershell"
	ShellUnknown    ShellType = "unknown"
)

// Manager handles shell integration.
type Manager struct {
	log *logger.Logger
}

// Test hooks for platform-specific and OS-dependent behaviour.
var (
	shellRuntimeGOOS       = runtime.GOOS
	userHomeDirFn          = os.UserHomeDir
	installAtWriteStringFn = func(f *os.File, s string) (int, error) { return f.WriteString(s) }
)

// NewManager creates a new shell manager.
func NewManager(log *logger.Logger) *Manager {
	return &Manager{log: log}
}

// DetectShell detects the current shell.
func (m *Manager) DetectShell() ShellType {
	shell := os.Getenv("SHELL")
	if shell == "" && shellRuntimeGOOS == "windows" {
		// Check for PowerShell
		if os.Getenv("PSModulePath") != "" {
			return ShellPowerShell
		}
		return ShellUnknown
	}

	base := filepath.Base(shell)
	switch {
	case strings.Contains(base, "zsh"):
		return ShellZsh
	case strings.Contains(base, "bash"):
		return ShellBash
	case strings.Contains(base, "fish"):
		return ShellFish
	case strings.Contains(base, "pwsh"), strings.Contains(base, "powershell"):
		return ShellPowerShell
	default:
		return ShellUnknown
	}
}

// Markers surround the GCM-managed block in shell config files so we can
// cleanly uninstall it later, even if the user has added or removed blank
// lines around it.
const (
	startMarker = "# >>> GCM shell integration >>>"
	endMarker   = "# <<< GCM shell integration <<<"
	// legacyMarker is the old single-line marker. We still detect it during
	// uninstall so existing installations can be cleaned up.
	legacyMarker = "# GCM shell integration"
)

// Install installs shell hooks for the given shell.
func (m *Manager) Install(shell ShellType) (string, error) {
	configFile, err := m.shellConfigFile(shell)
	if err != nil {
		return "", err
	}

	hookCode := m.generateHook(shell)
	if _, err := m.installAt(configFile, hookCode); err != nil {
		return configFile, err
	}

	m.log.Debug("Shell integration installed",
		logger.F("shell", string(shell)),
		logger.F("config", configFile))

	return configFile, nil
}

// installAt writes a GCM-managed block into the file at path. It exists as a
// seam for tests and is used by Install after shell detection.
func (m *Manager) installAt(path, hookCode string) ([]byte, error) {
	existing, _ := os.ReadFile(path)
	if strings.Contains(string(existing), startMarker) || strings.Contains(string(existing), legacyMarker) {
		return nil, fmt.Errorf("shell integration already installed in %s", path)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating config dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	entry := fmt.Sprintf("\n%s\n%s\n%s\n", startMarker, hookCode, endMarker)
	if _, err := installAtWriteStringFn(f, entry); err != nil {
		return nil, fmt.Errorf("writing to %s: %w", path, err)
	}

	content, _ := os.ReadFile(path)
	return content, nil
}

// IsInstalled checks if shell integration is currently installed for the given shell.
func (m *Manager) IsInstalled(shell ShellType) (bool, string) {
	configFile, err := m.shellConfigFile(shell)
	if err != nil {
		return false, ""
	}
	data, err := os.ReadFile(configFile)
	if err != nil {
		return false, configFile
	}
	content := string(data)
	if strings.Contains(content, startMarker) || strings.Contains(content, legacyMarker) {
		return true, configFile
	}
	return false, configFile
}

// Uninstall removes shell hooks. Supports both the current bracketed marker
// format and the legacy single-marker format used in earlier versions.
func (m *Manager) Uninstall(shell ShellType) (string, error) {
	configFile, err := m.shellConfigFile(shell)
	if err != nil {
		return "", err
	}

	if _, err := m.uninstallAt(configFile); err != nil {
		return configFile, err
	}
	return configFile, nil
}

// uninstallAt strips any GCM-managed block from the file at path. Exists as a
// seam for tests; used by Uninstall after shell detection.
func (m *Manager) uninstallAt(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	content := string(data)
	hasNew := strings.Contains(content, startMarker)
	hasLegacy := strings.Contains(content, legacyMarker)
	if !hasNew && !hasLegacy {
		return nil, fmt.Errorf("shell integration not found in %s", path)
	}

	lines := strings.Split(content, "\n")
	var result []string
	inBlock := false
	usingLegacy := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == startMarker:
			inBlock = true
			usingLegacy = false
			continue
		case trimmed == endMarker && !usingLegacy:
			inBlock = false
			continue
		case trimmed == legacyMarker && !inBlock:
			// Legacy block terminated by first blank line.
			inBlock = true
			usingLegacy = true
			continue
		}

		if inBlock {
			// Legacy behavior: empty line signals end of the block.
			if usingLegacy && trimmed == "" {
				inBlock = false
				usingLegacy = false
			}
			continue
		}
		result = append(result, line)
	}

	out := []byte(strings.Join(result, "\n"))
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return nil, fmt.Errorf("writing %s: %w", path, err)
	}
	return out, nil
}

// GenerateInitScript returns the shell init script for the given shell.
func (m *Manager) GenerateInitScript(shell ShellType) string {
	return m.generateHook(shell)
}

// GenerateCompletionScript returns shell completion script.
func (m *Manager) GenerateCompletionScript(shell ShellType) (string, error) {
	gcmPath, err := exec.LookPath("gcm")
	if err != nil {
		gcmPath = "gcm"
	}

	switch shell {
	case ShellBash:
		return fmt.Sprintf(`# GCM bash completion
if command -v %s &>/dev/null; then
  eval "$(%s completion bash)"
fi`, gcmPath, gcmPath), nil

	case ShellZsh:
		return fmt.Sprintf(`# GCM zsh completion
if command -v %s &>/dev/null; then
  eval "$(%s completion zsh)"
fi`, gcmPath, gcmPath), nil

	case ShellFish:
		return fmt.Sprintf(`# GCM fish completion
if command -v %s &>/dev/null
  %s completion fish | source
end`, gcmPath, gcmPath), nil

	case ShellPowerShell:
		return fmt.Sprintf(`# GCM PowerShell completion
if (Get-Command %s -ErrorAction SilentlyContinue) {
  %s completion powershell | Out-String | Invoke-Expression
}`, gcmPath, gcmPath), nil

	default:
		return "", fmt.Errorf("unsupported shell: %s", shell)
	}
}

func (m *Manager) generateHook(shell ShellType) string {
	switch shell {
	case ShellBash:
		return `_gcm_auto_switch() {
  if [ -f .gcm-profile ]; then
    gcm refresh --silent 2>/dev/null
  fi
}
[[ "$PROMPT_COMMAND" != *'_gcm_auto_switch'* ]] && PROMPT_COMMAND="_gcm_auto_switch;${PROMPT_COMMAND}"

# Prompt indicator (hidden when default profile is active)
_gcm_precmd() {
  local profile=$(gcm current --short --hide-default 2>/dev/null)
  if [ -n "$profile" ]; then
    _GCM_PROMPT="($profile) "
  else
    _GCM_PROMPT=""
  fi
}
[[ "$PROMPT_COMMAND" != *'_gcm_precmd'* ]] && PROMPT_COMMAND="_gcm_precmd;${PROMPT_COMMAND}"
[[ "$PS1" != *'$_GCM_PROMPT'* ]] && PS1='${_GCM_PROMPT}'"$PS1"`

	case ShellZsh:
		return `_gcm_auto_switch() {
  if [[ -f .gcm-profile ]]; then
    gcm refresh --silent 2>/dev/null
  fi
}
autoload -U add-zsh-hook 2>/dev/null
if (( $+functions[add-zsh-hook] )); then
  add-zsh-hook chpwd _gcm_auto_switch
else
  chpwd_functions+=(_gcm_auto_switch)
fi

# Prompt indicator (hidden when default profile is active)
_gcm_precmd() {
  local profile=$(gcm current --short --hide-default 2>/dev/null)
  if [[ -n "$profile" ]]; then
    _GCM_PROMPT="($profile) "
  else
    _GCM_PROMPT=""
  fi
}
if (( $+functions[add-zsh-hook] )); then
  add-zsh-hook precmd _gcm_precmd
fi
setopt PROMPT_SUBST
[[ "$PROMPT" != *'_GCM_PROMPT'* ]] && PROMPT='${_GCM_PROMPT}'"$PROMPT"`

	case ShellFish:
		return `function _gcm_auto_switch --on-variable PWD
  if test -f .gcm-profile
    gcm refresh --silent 2>/dev/null
  end
end

function _gcm_fish_prompt --on-event fish_prompt
  set -l profile (gcm current --short --hide-default 2>/dev/null)
  if test -n "$profile"
    echo -n "($profile) "
  end
end`

	case ShellPowerShell:
		return `function _gcm_auto_switch {
  if (Test-Path .gcm-profile) {
    gcm refresh --silent 2>$null
  }
}

# Register prompt function
if (-not $Global:_gcmOrigPrompt) {
  $Global:_gcmOrigPrompt = $function:prompt
  function Global:prompt {
    _gcm_auto_switch
    $profile = gcm current --short --hide-default 2>$null
    if ($profile) { Write-Host "($profile) " -NoNewline -ForegroundColor Cyan }
    & $Global:_gcmOrigPrompt
  }
}`

	default:
		return "# Unsupported shell"
	}
}

func (m *Manager) shellConfigFile(shell ShellType) (string, error) {
	home, err := userHomeDirFn()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}

	switch shell {
	case ShellBash:
		// Prefer .bashrc, fallback to .bash_profile
		bashrc := filepath.Join(home, ".bashrc")
		if _, err := os.Stat(bashrc); err == nil {
			return bashrc, nil
		}
		return filepath.Join(home, ".bash_profile"), nil

	case ShellZsh:
		return filepath.Join(home, ".zshrc"), nil

	case ShellFish:
		return filepath.Join(home, ".config", "fish", "config.fish"), nil

	case ShellPowerShell:
		if shellRuntimeGOOS == "windows" {
			return filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1"), nil
		}
		return filepath.Join(home, ".config", "powershell", "Microsoft.PowerShell_profile.ps1"), nil

	default:
		return "", fmt.Errorf("unsupported shell: %s", shell)
	}
}
