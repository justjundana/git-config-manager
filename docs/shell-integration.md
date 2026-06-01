# Shell Integration

GCM integrates with your shell to provide **auto-switching** when you `cd` into projects and a **prompt indicator** showing the active profile.

---

## Supported Shells

| Shell      | Config File                               | Auto-Switch | Prompt |
| ---------- | ----------------------------------------- | ----------- | ------ |
| Bash       | `~/.bashrc` (or `~/.bash_profile`)        | ✓           | ✓      |
| Zsh        | `~/.zshrc`                                | ✓           | ✓      |
| Fish       | `~/.config/fish/config.fish`              | ✓           | ✓      |
| PowerShell | `$PROFILE` (platform-dependent)           | ✓           | ✓      |

---

## Installation

```bash
gcm init
```

GCM auto-detects your shell from the `SHELL` environment variable (or `PSModulePath` on Windows) and appends a marked block to your config file. It also registers GCM's built-in credential helper for configured provider hosts, which serves tokens from GCM's encrypted store instead of the system keychain.

After running `gcm init`, **restart your shell** for the hooks to take effect:

```bash
exec $SHELL
# or
source ~/.zshrc    # for zsh
source ~/.bashrc   # for bash
```

If shell detection fails, GCM will tell you:

```
Could not detect your shell
Set SHELL environment variable and retry: SHELL=/bin/zsh gcm init
```

> **Workaround:** Set the `SHELL` environment variable to your shell's path before running `gcm init`. For example: `SHELL=/bin/zsh gcm init`. If that still fails, you can manually add the hook block to your shell config (see [What Gets Installed](#what-gets-installed) below).

---

## What Gets Installed

`gcm init` appends a block to your shell config file wrapped in markers:

```bash
# >>> GCM shell integration >>>
# … hook code …
# <<< GCM shell integration <<<
```

The hook code provides two features:

`gcm init` also registers GCM as the git credential helper for configured provider hosts. It does not clear global git identity values unless you pass `--clear-global-identity`.

### 1. Auto-Switching

When you `cd` into a directory containing a `.gcm-profile` file, the hook calls `gcm refresh --silent` to activate the pinned profile.

**Bash** — uses `PROMPT_COMMAND`:
```bash
_gcm_auto_switch() {
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
[[ "$PS1" != *'$_GCM_PROMPT'* ]] && PS1='${_GCM_PROMPT}'"$PS1"
```

**Zsh** — uses `chpwd` + `precmd` hooks:
```zsh
_gcm_auto_switch() {
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
[[ "$PROMPT" != *'_GCM_PROMPT'* ]] && PROMPT='${_GCM_PROMPT}'"$PROMPT"
```

**Fish** — uses `--on-variable PWD` + `fish_prompt` event:
```fish
function _gcm_auto_switch --on-variable PWD
  if test -f .gcm-profile
    gcm refresh --silent 2>/dev/null
  end
end

function _gcm_fish_prompt --on-event fish_prompt
  set -l profile (gcm current --short --hide-default 2>/dev/null)
  if test -n "$profile"
    echo -n "($profile) "
  end
end
```

**PowerShell** — hooks into the `prompt` function:
```powershell
function _gcm_auto_switch {
  if (Test-Path .gcm-profile) {
    gcm refresh --silent 2>$null
  }
}
```
  }
}
```

### 2. Prompt Indicator

When a non-default profile is active, your prompt shows the profile name:

```
(work) ~/projects/work-project $
```

This is powered by a `precmd` (zsh) / `PROMPT_COMMAND` (bash) hook that runs `gcm current --short --hide-default` before every prompt. The result is stored in a `$_GCM_PROMPT` variable which is embedded in your `PS1`/`PROMPT`.

**Key behavior:**
- Only shows when you've switched **away** from your default profile
- Uses `--hide-default` so your prompt stays clean in the common case
- The variable approach avoids spawning a subshell on every keystroke

---

## Auto-Switching

### How It Works

1. Shell hook fires on every directory change
2. Hook checks for a `.gcm-profile` file in the new directory
3. If found, `gcm refresh --silent` reads the file and activates that profile
4. Git config and SSH agent are updated; the `_GCM_PROMPT` shell variable reflects the new profile

### Create `.gcm-profile`

The recommended way:

```bash
cd ~/projects/work
gcm use work --local
```

This creates a `.gcm-profile` file in the directory containing the profile name.

You can also create it manually:

```bash
echo "work" > ~/projects/work/.gcm-profile
```

### `.gcm-profile` in Version Control

Whether to commit `.gcm-profile` depends on your team:

- **Solo project** — commit it, so it follows you across machines
- **Team project** — add to `.gitignore`, since profile names are personal
- **Monorepo** — each developer adds their own `.gcm-profile` locally

```bash
# Add to .gitignore if you don't want it shared
echo ".gcm-profile" >> .gitignore
```

---

## Uninstalling

### Automatic

GCM can remove its own hooks:

```bash
# Detect shell automatically
gcm init  # (if a future --uninstall flag is added)
```

### Manual

Open your shell config file and delete everything between the markers:

```bash
# >>> GCM shell integration >>>
…  ← delete all of this
# <<< GCM shell integration <<<
```

Then restart your shell:

```bash
exec $SHELL
```

---

## Troubleshooting

### Hooks not loading after `gcm init`

**Restart your shell** — `source ~/.zshrc` usually works, but `exec $SHELL` is more reliable.

### Auto-switch not triggering

1. Verify `.gcm-profile` exists and contains a valid profile name:
   ```bash
   cat .gcm-profile
   gcm profile list    # confirm the name matches
   ```
2. Verify integration is installed:
   ```bash
   grep "GCM shell integration" ~/.zshrc   # adjust for your shell
   ```
3. Test manually:
   ```bash
   gcm refresh    # should activate the profile
   ```

### Prompt indicator missing

- Make sure `gcm current --short` returns a profile name:
  ```bash
  gcm current --short
  ```
- Some custom prompt themes override `PS1`/`PROMPT` after GCM's hook. Move the GCM block **after** your theme's configuration.

### Integration already installed

If you run `gcm init` again, it will tell you:

```
shell integration already installed in ~/.zshrc
```

To reinstall, manually remove the old block first (see [Uninstalling](#uninstalling)), then run `gcm init` again.
