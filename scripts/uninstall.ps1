# GCM (Git Config Manager) uninstallation script for Windows
# This script removes gcm from $env:USERPROFILE\.local\bin and cleans PATH

param(
    [switch]$Help
)

# Colors and styles for Windows Terminal
$Colors = @{
    Red = "`e[0;31m"
    Green = "`e[0;32m"
    Yellow = "`e[1;33m"
    Blue = "`e[0;34m"
    Purple = "`e[0;35m"
    Cyan = "`e[0;36m"
    White = "`e[1;37m"
    Gray = "`e[0;90m"
    Reset = "`e[0m"
    Bold = "`e[1m"
    Dim = "`e[2m"
}

# Unicode characters for better UI
$Icons = @{
    Checkmark = "✓"
    Crossmark = "✗"
    Arrow = "→"
    Trash = "🗑"
    Warning = "⚠"
    Question = "❓"
    Stop = "🛑"
    Clean = "🧹"
    Shield = "🛡"
    Info = "ℹ"
}

# Get terminal width
$TermWidth = try { $Host.UI.RawUI.WindowSize.Width } catch { 80 }

# Print separator line
function Print-Separator {
    param([string]$Char = "-")
    Write-Host ($Char * $TermWidth)
}

# Print fancy header
function Print-Header {
    Clear-Host
    Print-Separator "═"
    Write-Host ""
    Write-Host ""
    Write-Host "     ██████╗  ██████╗███╗   ███╗"
    Write-Host "    ██╔════╝ ██╔════╝████╗ ████║"
    Write-Host "    ██║  ███╗██║     ██╔████╔██║"
    Write-Host "    ██║   ██║██║     ██║╚██╔╝██║"
    Write-Host "    ╚██████╔╝╚██████╗██║ ╚═╝ ██║"
    Write-Host "     ╚═════╝  ╚═════╝╚═╝     ╚═╝"
    Write-Host ""
    Write-Host ""
    Write-Host "$($Colors.Bold)$($Colors.White)              Git Config Manager Uninstaller$($Colors.Reset)"
    Write-Host "$($Colors.Dim)$($Colors.Gray)            Safe and complete uninstallation process$($Colors.Reset)"
    Write-Host ""
    Print-Separator "═"
    Write-Host ""
}

# Print functions with icons and styling
function Print-Info {
    param([string]$Message)
    Write-Host "$($Colors.Blue)$($Colors.Bold) $($Icons.Info)  INFO$($Colors.Reset) $($Colors.Gray)│$($Colors.Reset) $Message"
}

function Print-Success {
    param([string]$Message)
    Write-Host "$($Colors.Green)$($Colors.Bold) $($Icons.Checkmark)  SUCCESS$($Colors.Reset) $($Colors.Gray)│$($Colors.Reset) $Message"
}

function Print-Warning {
    param([string]$Message)
    Write-Host "$($Colors.Yellow)$($Colors.Bold) $($Icons.Warning)  WARNING$($Colors.Reset) $($Colors.Gray)│$($Colors.Reset) $Message"
}

function Print-Error {
    param([string]$Message)
    Write-Host "$($Colors.Red)$($Colors.Bold) $($Icons.Crossmark)  ERROR$($Colors.Reset) $($Colors.Gray)│$($Colors.Reset) $Message"
}

function Print-Step {
    param([string]$Message)
    Write-Host "$($Colors.Purple)$($Colors.Bold) $($Icons.Arrow)  STEP$($Colors.Reset) $($Colors.Gray)│$($Colors.Reset) $Message"
}

function Print-Clean {
    param([string]$Message)
    Write-Host "$($Colors.Cyan)$($Colors.Bold) $($Icons.Clean)  CLEANING$($Colors.Reset) $($Colors.Gray)│$($Colors.Reset) $Message"
}

# Show help information
function Show-Help {
    Write-Host "GCM uninstaller - Git Config Manager Uninstallation Script for Windows"
    Write-Host ""
    Write-Host "Usage: .\uninstall.ps1 [OPTIONS]"
    Write-Host ""
    Write-Host "Options:"
    Write-Host "  -Help           Show this help message"
    Write-Host ""
    Write-Host "Examples:"
    Write-Host "  .\uninstall.ps1         # Run interactive uninstaller"
    Write-Host "  .\uninstall.ps1 -Help   # Show help"
}

# Check if gcm is installed
function Test-GcmInstallation {
    $installDir = Join-Path $env:USERPROFILE ".local\bin"
    $gcmDir = Join-Path $env:USERPROFILE ".gcm"
    $binaryFound = Test-Path (Join-Path $installDir "gcm.exe")
    $commandFound = $null -ne (Get-Command gcm -ErrorAction SilentlyContinue)
    $dataFound = Test-Path $gcmDir

    Print-Step "Checking GCM installation..."

    Write-Host ""
    Print-Separator "┄"
    Write-Host "$($Colors.Bold)$($Colors.White)Installation Status:$($Colors.Reset)"
    Print-Separator "┄"

    if ($binaryFound) {
        Write-Host "$($Colors.Green) $($Icons.Checkmark)$($Colors.Reset) Binary found: $($Colors.Bold)$(Join-Path $installDir 'gcm.exe')$($Colors.Reset)"
    } else {
        Write-Host "$($Colors.Gray) $($Icons.Crossmark)$($Colors.Reset) Binary: $($Colors.Dim)not found$($Colors.Reset)"
    }

    # Check PATH configuration
    $userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    $pathConfigured = $userPath -like "*$installDir*"

    if ($pathConfigured) {
        Write-Host "$($Colors.Green) $($Icons.Checkmark)$($Colors.Reset) PATH configuration: $($Colors.Bold)Found in user PATH$($Colors.Reset)"
    } else {
        Write-Host "$($Colors.Gray) $($Icons.Crossmark)$($Colors.Reset) PATH configuration: $($Colors.Dim)Not found$($Colors.Reset)"
    }

    if ($commandFound) {
        try {
            $version = & gcm version 2>$null | Select-Object -First 1
        } catch {
            $version = "unknown"
        }
        Write-Host "$($Colors.Green) $($Icons.Checkmark)$($Colors.Reset) Command available: $($Colors.Bold)gcm$($Colors.Reset) $($Colors.Dim)($version)$($Colors.Reset)"
    } else {
        Write-Host "$($Colors.Gray) $($Icons.Crossmark)$($Colors.Reset) Command available: $($Colors.Dim)gcm (not found)$($Colors.Reset)"
    }

    if ($dataFound) {
        $dirSize = "{0:N2} MB" -f ((Get-ChildItem $gcmDir -Recurse -ErrorAction SilentlyContinue | Measure-Object -Property Length -Sum).Sum / 1MB)
        Write-Host "$($Colors.Blue) $($Icons.Info)$($Colors.Reset) Data directory: $($Colors.Bold)$gcmDir$($Colors.Reset) $($Colors.Dim)($dirSize)$($Colors.Reset)"
    } else {
        Write-Host "$($Colors.Gray) $($Icons.Crossmark)$($Colors.Reset) Data directory: $($Colors.Dim)$gcmDir (not found)$($Colors.Reset)"
    }

    Print-Separator "┄"
    Write-Host ""

    return ($binaryFound -or $pathConfigured -or $dataFound -or $commandFound)
}

# Show what will be removed based on option
function Show-RemovalPreview {
    param([string]$Option)

    Write-Host "$($Colors.Bold)$($Colors.White)Removal Preview:$($Colors.Reset)"
    Print-Separator "┄"

    $installDir = Join-Path $env:USERPROFILE ".local\bin"
    $gcmDir = Join-Path $env:USERPROFILE ".gcm"

    # Check binary
    if (Test-Path (Join-Path $installDir "gcm.exe")) {
        Write-Host "$($Colors.Red) $($Icons.Trash)$($Colors.Reset) Binary: $($Colors.Bold)$(Join-Path $installDir 'gcm.exe')$($Colors.Reset)"
    } else {
        Write-Host "$($Colors.Gray) $($Icons.Crossmark)$($Colors.Reset) Binary: $($Colors.Dim)not found$($Colors.Reset)"
    }

    # Check PATH configuration
    $userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    if ($userPath -like "*$installDir*") {
        Write-Host "$($Colors.Red) $($Icons.Trash)$($Colors.Reset) PATH configuration: $($Colors.Bold)User PATH entry$($Colors.Reset)"
    } else {
        Write-Host "$($Colors.Gray) $($Icons.Crossmark)$($Colors.Reset) PATH configuration: $($Colors.Dim)Not found$($Colors.Reset)"
    }

    # Show data directory based on option
    if (Test-Path $gcmDir) {
        $dirSize = "{0:N2} MB" -f ((Get-ChildItem $gcmDir -Recurse -ErrorAction SilentlyContinue | Measure-Object -Property Length -Sum).Sum / 1MB)
        if ($Option -eq "complete") {
            Write-Host "$($Colors.Red) $($Icons.Trash)$($Colors.Reset) Data directory: $($Colors.Bold)$gcmDir$($Colors.Reset) $($Colors.Dim)($dirSize)$($Colors.Reset)"
        } else {
            Write-Host "$($Colors.Green) $($Icons.Shield)$($Colors.Reset) Data directory: $($Colors.Bold)$gcmDir$($Colors.Reset) $($Colors.Dim)($dirSize - will be kept)$($Colors.Reset)"
        }
    } else {
        Write-Host "$($Colors.Gray) $($Icons.Crossmark)$($Colors.Reset) Data directory: $($Colors.Dim)$gcmDir (not found)$($Colors.Reset)"
    }

    Print-Separator "┄"
    Write-Host ""
}

# Animated loading for removal process
function Show-RemovalProgress {
    param([string]$Item)

    $spinChars = @('⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏')
    Write-Host -NoNewline "   $($Colors.Dim)Removing $Item... $($Colors.Reset)"

    for ($i = 0; $i -lt 10; $i++) {
        $spinChar = $spinChars[$i % $spinChars.Length]
        Write-Host -NoNewline "`r   $($Colors.Dim)Removing $Item... $($Colors.Cyan)$spinChar$($Colors.Reset) "
        Start-Sleep -Milliseconds 100
    }
    Write-Host "`r   $($Colors.Green)$($Icons.Checkmark)$($Colors.Reset) Removed $Item successfully.      "
}

# Remove binary with feedback
function Remove-Binary {
    Print-Step "Removing gcm binary..."

    $candidates = @(
        (Join-Path $env:USERPROFILE ".local\bin\gcm.exe"),
        (Join-Path ($env:GOPATH ?? (Join-Path $env:USERPROFILE "go")) "bin\gcm.exe"),
        "C:\usr\local\bin\gcm.exe"
    )

    # Also check where gcm is in PATH
    $gcmCmd = Get-Command gcm -ErrorAction SilentlyContinue
    if ($gcmCmd) {
        $candidates += $gcmCmd.Source
    }

    $removed = $false
    $seen = @{}
    foreach ($candidate in $candidates) {
        if (-not $candidate -or $seen.ContainsKey($candidate)) { continue }
        $seen[$candidate] = $true

        if (Test-Path $candidate) {
            Show-RemovalProgress "binary ($candidate)"
            try {
                Remove-Item -Path $candidate -Force
                Print-Success "Removed gcm from $candidate"
                $removed = $true
            }
            catch {
                Print-Error "Failed to remove $candidate`: $($_.Exception.Message)"
            }
        }
    }

    if (-not $removed) {
        Print-Warning "gcm binary not found in expected locations"
    }
}

# Remove from PATH with feedback
function Remove-FromPath {
    $installDir = Join-Path $env:USERPROFILE ".local\bin"

    Print-Step "Cleaning PATH configuration..."

    $userPath = [Environment]::GetEnvironmentVariable("PATH", "User")

    if ($userPath -like "*$installDir*") {
        Show-RemovalProgress "PATH entry"

        # Remove install directory from PATH
        $pathParts = $userPath -split ";" | Where-Object { $_ -ne $installDir -and $_ -ne "" }
        $newPath = $pathParts -join ";"
        [Environment]::SetEnvironmentVariable("PATH", $newPath, "User")

        Print-Success "Removed $installDir from user PATH"
    } else {
        Print-Info "No GCM PATH configuration found"
    }
}

# Remove entire gcm data directory with feedback
function Remove-GcmDirectory {
    $gcmDir = Join-Path $env:USERPROFILE ".gcm"

    Print-Step "Removing GCM data directory..."

    if (Test-Path $gcmDir) {
        $dirSize = "{0:N2} MB" -f ((Get-ChildItem $gcmDir -Recurse -ErrorAction SilentlyContinue | Measure-Object -Property Length -Sum).Sum / 1MB)
        Print-Info "Removing directory: $gcmDir ($dirSize)"

        Show-RemovalProgress "data directory"
        try {
            Remove-Item -Path $gcmDir -Recurse -Force
            Print-Success "Removed GCM data directory"
        }
        catch {
            Print-Error "Failed to remove data directory: $($_.Exception.Message)"
        }
    } else {
        Print-Warning "GCM directory not found at $gcmDir"
    }
}

# Remove git credential config for all hosts
function Remove-GitCredential {
    Print-Step "Cleaning git credential config..."
    $cleaned = $false

    # Common hosts
    foreach ($host in @("https://github.com", "https://gitlab.com", "https://bitbucket.org", "https://dev.azure.com")) {
        $helper = & git config --global "credential.$host.helper" 2>$null
        if ($helper) {
            & git config --global --unset-all "credential.$host.helper" 2>$null
            Print-Success "Removed credential helper for $host"
            $cleaned = $true
        }
        $user = & git config --global "credential.$host.username" 2>$null
        if ($user) {
            & git config --global --unset-all "credential.$host.username" 2>$null
            Print-Success "Removed credential username for $host"
            $cleaned = $true
        }
    }

    # Check for any gcm-related credential helpers
    $allConfig = & git config --global --list 2>$null
    $gcmCreds = $allConfig | Where-Object { $_ -match "credential.*helper.*gcm" }
    foreach ($entry in $gcmCreds) {
        $key = ($entry -split "=")[0]
        & git config --global --unset-all $key 2>$null
        Print-Success "Removed $key"
        $cleaned = $true
    }

    # Global credential.helper
    $globalCred = & git config --global credential.helper 2>$null
    if ($globalCred -match "gcm") {
        & git config --global --unset-all credential.helper 2>$null
        Print-Success "Removed global credential.helper (gcm)"
        $cleaned = $true
    }

    if (-not $cleaned) {
        Print-Info "No GCM credential config found"
    }
}

# Show uninstall options
function Show-UninstallOptions {
    Print-Separator "═"
    Write-Host "$($Colors.Bold)$($Colors.White) $($Icons.Question)  UNINSTALLATION OPTIONS$($Colors.Reset)"
    Print-Separator "═"
    Write-Host ""
    Write-Host "$($Colors.Cyan)$($Colors.Bold)1)$($Colors.Reset) $($Colors.White)Minimal Removal$($Colors.Reset) $($Colors.Dim)(Recommended)$($Colors.Reset)"
    Write-Host "   * Remove gcm binary"
    Write-Host "   * Clean PATH configuration"
    Write-Host "   * $($Colors.Green)Keep$($Colors.Reset) profiles, tokens, SSH keys, and configuration"
    Write-Host ""
    Write-Host "$($Colors.Red)$($Colors.Bold)2)$($Colors.Reset) $($Colors.White)Complete Removal$($Colors.Reset) $($Colors.Dim)(Permanent)$($Colors.Reset)"
    Write-Host "   * Remove gcm binary"
    Write-Host "   * Clean PATH configuration"
    Write-Host "   * $($Colors.Red)Delete$($Colors.Reset) all profiles and configuration (~/.gcm)"
    Write-Host "   * $($Colors.Red)Delete$($Colors.Reset) encrypted tokens, backup archives, audit logs"
    Write-Host ""
    Write-Host "$($Colors.Red)$($Colors.Bold)3)$($Colors.Reset) $($Colors.White)Nuclear Clean$($Colors.Reset) $($Colors.Dim)(Everything - no trace left)$($Colors.Reset)"
    Write-Host "   * Everything in option 2, plus:"
    Write-Host "   * $($Colors.Red)Delete$($Colors.Reset) git global identity (user.name, user.email, signingkey)"
    Write-Host "   * $($Colors.Red)Delete$($Colors.Reset) git credential config for ALL hosts"
    Write-Host "   * $($Colors.Red)Delete$($Colors.Reset) GCM-generated SSH keys + flush from ssh-agent"
    Write-Host "   * $($Colors.Red)Delete$($Colors.Reset) GCM-generated GPG keys"
    Write-Host "   * $($Colors.Red)Delete$($Colors.Reset) git local identity and GCM markers (recursive scan)"
    Write-Host "   * $($Colors.Red)Delete$($Colors.Reset) Windows Credential Manager entries"
    Write-Host "   * $($Colors.Red)Delete$($Colors.Reset) Shell completions and temp files"
    Write-Host ""
    Write-Host "$($Colors.Gray)$($Colors.Bold)4)$($Colors.Reset) $($Colors.White)Cancel$($Colors.Reset)"
    Write-Host "   * Exit without making any changes"
    Write-Host ""
    Print-Separator "┄"
}

# Show completion message
function Show-Completion {
    param([string]$Mode)

    Write-Host ""
    Print-Separator "═"
    Write-Host ""

    switch ($Mode) {
        "nuclear" {
            Write-Host "$($Colors.Green)$($Colors.Bold) $($Icons.Checkmark)  NUCLEAR CLEAN SUCCESSFUL - NO TRACE LEFT!$($Colors.Reset)"
            Write-Host ""
            Print-Separator "┄"
            Write-Host "$($Colors.Bold)$($Colors.White)What was removed:$($Colors.Reset)"
            Write-Host " * gcm binary (from all locations)"
            Write-Host " * PATH configuration"
            Write-Host " * Git global identity (user.name, user.email, signingkey, gpgsign)"
            Write-Host " * Git local identity and GCM markers (recursive scan)"
            Write-Host " * Git credential config for ALL hosts"
            Write-Host " * GCM-generated SSH keys + flushed from ssh-agent"
            Write-Host " * GCM-generated GPG keys"
            Write-Host " * All profiles, tokens, config, backups, cache"
            Write-Host " * Windows Credential Manager entries"
            Write-Host " * Shell completions and temp files"
            Write-Host " * Project markers (.gcm-profile, gcm-session)"
        }
        "complete" {
            Write-Host "$($Colors.Green)$($Colors.Bold) $($Icons.Checkmark)  COMPLETE UNINSTALLATION SUCCESSFUL!$($Colors.Reset)"
            Write-Host ""
            Print-Separator "┄"
            Write-Host "$($Colors.Bold)$($Colors.White)What was removed:$($Colors.Reset)"
            Write-Host " * gcm binary"
            Write-Host " * PATH configuration"
            Write-Host " * All profiles and configuration"
            Write-Host " * Encrypted tokens and audit logs"
        }
        default {
            Write-Host "$($Colors.Green)$($Colors.Bold) $($Icons.Checkmark)  MINIMAL UNINSTALLATION COMPLETE!$($Colors.Reset)"
            Write-Host ""
            Print-Separator "┄"
            Write-Host "$($Colors.Bold)$($Colors.White)What was removed:$($Colors.Reset)"
            Write-Host " * gcm binary"
            Write-Host " * PATH configuration"
            Write-Host ""
            Write-Host "$($Colors.Bold)$($Colors.White)What was kept:$($Colors.Reset)"
            Write-Host " * Profiles and configuration in ~\.gcm"
            Write-Host " * SSH keys (in ~\.ssh)"
            Write-Host " * Encrypted tokens and backup archives"
        }
    }

    Print-Separator "┄"
    Write-Host "$($Colors.Bold)$($Colors.White)Final Steps:$($Colors.Reset)"
    Write-Host " 1. Restart your PowerShell/Command Prompt"
    Write-Host " 2. Verify with 'Get-Command gcm' (should show error)"

    if ($Mode -eq "minimal") {
        Write-Host " 3. Manually remove '~\.gcm' if you change your mind later"
    }

    Print-Separator "┄"
    Write-Host "Thank you for using GCM!"
    Print-Separator "═"
    Write-Host ""
}

# Remove git global/local identity set by GCM
function Remove-GitIdentity {
    Print-Step "Removing git identity configuration..."
    $cleaned = $false

    foreach ($key in @("user.name", "user.email", "user.signingkey", "commit.gpgsign", "gpg.format", "gpg.program", "core.sshCommand", "tag.gpgsign", "tag.forceSignAnnotated")) {
        $val = & git config --global $key 2>$null
        if ($val) {
            & git config --global --unset-all $key 2>$null
            Print-Success "Unset git global $key"
            $cleaned = $true
        }
    }

    # Clean local repo if inside one
    $isRepo = & git rev-parse --is-inside-work-tree 2>$null
    if ($isRepo -eq "true") {
        foreach ($key in @("user.name", "user.email", "user.signingkey", "commit.gpgsign")) {
            $val = & git config --local $key 2>$null
            if ($val) {
                & git config --local --unset-all $key 2>$null
                Print-Success "Unset git local $key"
                $cleaned = $true
            }
        }
        $gitRoot = & git rev-parse --show-toplevel 2>$null
        if ($gitRoot) {
            $profileMarker = Join-Path $gitRoot ".gcm-profile"
            $sessionMarker = Join-Path $gitRoot ".git\gcm-session"
            if (Test-Path $profileMarker) {
                Remove-Item -Path $profileMarker -Force
                Print-Success "Removed .gcm-profile marker"
                $cleaned = $true
            }
            if (Test-Path $sessionMarker) {
                Remove-Item -Path $sessionMarker -Force
                Print-Success "Removed .git/gcm-session marker"
                $cleaned = $true
            }
        }
    }

    if (-not $cleaned) {
        Print-Info "No git identity configuration found"
    }
}

# Remove SSH keys generated by GCM
function Remove-SshKeys {
    Print-Step "Removing GCM-generated SSH keys..."

    $gcmDir = Join-Path $env:USERPROFILE ".gcm"
    $profilesDir = Join-Path $gcmDir "profiles"
    $sshDir = Join-Path $env:USERPROFILE ".ssh"
    $sshFound = @()

    if (Test-Path $profilesDir) {
        Get-ChildItem -Path $profilesDir -Filter "*.yaml" -ErrorAction SilentlyContinue | ForEach-Object {
            $profileName = $_.BaseName
            foreach ($prefix in @("id_ed25519", "id_rsa", "id_ecdsa")) {
                $keyPath = Join-Path $sshDir "${prefix}_${profileName}"
                if (Test-Path $keyPath) { $sshFound += $keyPath }
                if (Test-Path "$keyPath.pub") { $sshFound += "$keyPath.pub" }
            }
        }
    }

    if ($sshFound.Count -eq 0) {
        Print-Info "No GCM-generated SSH keys found"
        return
    }

    foreach ($f in $sshFound) {
        Remove-Item -Path $f -Force -ErrorAction SilentlyContinue
    }

    # Remove from ssh-agent
    if (Get-Command ssh-add -ErrorAction SilentlyContinue) {
        foreach ($f in $sshFound) {
            if ($f -notmatch "\.pub$") {
                & ssh-add -d $f 2>$null
            }
        }
    }

    Print-Success "Removed $($sshFound.Count) SSH key file(s)"
}

# Remove GPG keys generated by GCM
function Remove-GpgKeys {
    Print-Step "Removing GCM-generated GPG keys..."

    if (-not (Get-Command gpg -ErrorAction SilentlyContinue)) {
        Print-Info "GPG not installed - skipping"
        return
    }

    $gcmDir = Join-Path $env:USERPROFILE ".gcm"
    $profilesDir = Join-Path $gcmDir "profiles"
    $gpgKeyIds = @()

    if (Test-Path $profilesDir) {
        Get-ChildItem -Path $profilesDir -Filter "*.yaml" -ErrorAction SilentlyContinue | ForEach-Object {
            $content = Get-Content $_.FullName -Raw -ErrorAction SilentlyContinue
            if ($content -match "key_id:\s*[`"']?([A-Fa-f0-9]+)[`"']?") {
                $gpgKeyIds += $Matches[1]
            }
        }
    }

    if ($gpgKeyIds.Count -eq 0) {
        Print-Info "No GCM GPG key IDs found"
        return
    }

    foreach ($kid in $gpgKeyIds) {
        & gpg --batch --yes --delete-secret-and-public-key $kid 2>$null
        if ($LASTEXITCODE -eq 0) {
            Print-Success "Deleted GPG key $kid"
        } else {
            Print-Warning "GPG key $kid not found in keyring (already deleted?)"
        }
    }
}

# Remove credential store entries
function Remove-CredentialStore {
    Print-Step "Cleaning credential store entries..."

    $cleaned = $false

    # Windows Credential Manager
    if (Get-Command cmdkey -ErrorAction SilentlyContinue) {
        $creds = & cmdkey /list 2>$null
        if ($creds -match "github\.com|gcm") {
            & cmdkey /delete:git:https://github.com 2>$null
            & cmdkey /delete:github.com 2>$null
            Print-Success "Removed github.com from Windows Credential Manager"
            $cleaned = $true
        }
    }

    # .git-credentials file
    $credFile = Join-Path $env:USERPROFILE ".git-credentials"
    if (Test-Path $credFile) {
        $lines = Get-Content $credFile | Where-Object { $_ -notmatch "github\.com|gitlab\.com" }
        Set-Content -Path $credFile -Value $lines -Force
        Print-Success "Removed github/gitlab entries from .git-credentials"
        $cleaned = $true
    }

    if (-not $cleaned) {
        Print-Info "No credential store entries found"
    }
}

# Remove project markers recursively
function Remove-ProjectMarkers {
    Print-Step "Scanning for .gcm-profile and gcm-session markers..."

    $markersFound = 0
    $scanDirs = @(
        (Join-Path $env:USERPROFILE "projects"),
        (Join-Path $env:USERPROFILE "Projects"),
        (Join-Path $env:USERPROFILE "dev"),
        (Join-Path $env:USERPROFILE "Dev"),
        (Join-Path $env:USERPROFILE "src"),
        (Join-Path $env:USERPROFILE "work"),
        (Join-Path $env:USERPROFILE "repos"),
        (Join-Path $env:USERPROFILE "code")
    )

    foreach ($dir in $scanDirs) {
        if (-not (Test-Path $dir)) { continue }
        Get-ChildItem -Path $dir -Filter ".gcm-profile" -Recurse -Depth 4 -Force -ErrorAction SilentlyContinue | ForEach-Object {
            Remove-Item $_.FullName -Force
            $markersFound++
        }
        Get-ChildItem -Path $dir -Filter "gcm-session" -Recurse -Depth 5 -Force -ErrorAction SilentlyContinue | Where-Object {
            $_.Directory.Name -eq ".git"
        } | ForEach-Object {
            Remove-Item $_.FullName -Force
            $markersFound++
        }
    }

    if ($markersFound -gt 0) {
        Print-Success "Removed $markersFound project marker(s)"
    } else {
        Print-Info "No project markers found"
    }
}

# Remove shell completions and temp files
function Remove-CompletionsAndTemp {
    Print-Step "Removing completions and temp files..."

    # PowerShell completion module
    $psModulePaths = @(
        (Join-Path $env:USERPROFILE "Documents\PowerShell\Modules\GcmCompletion"),
        (Join-Path $env:USERPROFILE "Documents\WindowsPowerShell\Modules\GcmCompletion")
    )

    foreach ($modPath in $psModulePaths) {
        if (Test-Path $modPath) {
            Remove-Item -Path $modPath -Recurse -Force
            Print-Success "Removed PowerShell completion module"
        }
    }

    # Temp files
    Get-ChildItem -Path $env:TEMP -Filter "gcm-*" -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force

    Print-Success "Cleaned temp files"
}

# Main uninstallation function
function Main {
    if ($Help) {
        Show-Help
        exit 0
    }

    Print-Header

    Print-Info "Starting GCM uninstallation process..."
    Write-Host ""

    # Check if gcm is installed
    $installed = Test-GcmInstallation

    if (-not $installed) {
        Print-Warning "GCM does not appear to be installed on this system"
        Write-Host ""
        Print-Separator "┄"
        Write-Host "$($Colors.Bold)$($Colors.White)No GCM installation found!$($Colors.Reset)"
        Print-Separator "┄"
        Write-Host "It looks like GCM is not installed or has already been removed."
        Write-Host "Common reasons:"
        Write-Host " * GCM was never installed"
        Write-Host " * GCM was already uninstalled"
        Write-Host " * GCM was installed in a different location"
        Print-Separator "┄"
        Write-Host ""

        $response = Read-Host "Do you want to clean any remaining traces? (y/N)"
        if ($response -ne "y" -and $response -ne "Y") {
            Write-Host ""
            Print-Info "Exiting without making changes"
            Print-Separator "═"
            Write-Host "$($Colors.Dim)$($Colors.Gray)No changes were made to your system.$($Colors.Reset)"
            Print-Separator "═"
            Write-Host ""
            exit 0
        }

        Write-Host ""
        Print-Info "Proceeding with cleanup of any remaining traces..."
        Write-Host ""
    } else {
        Print-Success "GCM installation detected"
        Write-Host ""
    }

    # Show uninstall options
    Show-UninstallOptions

    $response = Read-Host "Choose an option (1/2/3/4)"
    Write-Host ""

    switch ($response) {
        "1" {
            Print-Info "Proceeding with minimal removal..."
            Write-Host ""
            Show-RemovalPreview "minimal"

            Print-Separator "┄"
            Write-Host "$($Colors.Yellow)$($Colors.Bold) $($Icons.Stop)  FINAL CONFIRMATION$($Colors.Reset)"
            Print-Separator "┄"

            $confirm = Read-Host "Proceed with minimal removal? (y/N)"
            if ($confirm -eq "y" -or $confirm -eq "Y") {
                Write-Host ""
                Remove-Binary
                Write-Host ""
                Remove-FromPath
                Write-Host ""
                Show-Completion "minimal"
            } else {
                Write-Host ""
                Print-Info "Uninstallation cancelled by user"
                Print-Separator "═"
                Write-Host "$($Colors.Dim)$($Colors.Gray)No changes were made to your system.$($Colors.Reset)"
                Print-Separator "═"
                Write-Host ""
            }
        }
        "2" {
            Print-Info "Proceeding with complete removal..."
            Write-Host ""
            Show-RemovalPreview "complete"

            Print-Separator "┄"
            Write-Host "$($Colors.Red)$($Colors.Bold) $($Icons.Stop)  DANGER: COMPLETE REMOVAL$($Colors.Reset)"
            Print-Separator "┄"
            Write-Host "$($Colors.Red)This will permanently delete ALL GCM data including profiles, tokens, and backups!$($Colors.Reset)"
            Print-Separator "┄"

            $confirm = Read-Host "Type 'DELETE' to confirm complete removal"
            if ($confirm -eq "DELETE") {
                Write-Host ""
                Remove-Binary
                Write-Host ""
                Remove-FromPath
                Write-Host ""
                Remove-GcmDirectory
                Write-Host ""
                Show-Completion "complete"
            } else {
                Write-Host ""
                Print-Info "Uninstallation cancelled - confirmation text did not match"
                Print-Separator "═"
                Write-Host "$($Colors.Dim)$($Colors.Gray)No changes were made to your system.$($Colors.Reset)"
                Print-Separator "═"
                Write-Host ""
            }
        }
        "3" {
            Print-Info "Proceeding with NUCLEAR clean..."
            Write-Host ""
            Show-RemovalPreview "complete"

            Print-Separator "┄"
            Write-Host "$($Colors.Red)$($Colors.Bold) $($Icons.Stop)  DANGER: NUCLEAR CLEAN - NO TRACE LEFT$($Colors.Reset)"
            Print-Separator "┄"
            Write-Host "$($Colors.Red)This will permanently delete EVERYTHING: binary, data, git identity, credentials!$($Colors.Reset)"
            Print-Separator "┄"

            $confirm = Read-Host "Type 'NUKE' to confirm nuclear clean"
            if ($confirm -eq "NUKE") {
                Write-Host ""
                Remove-Binary
                Write-Host ""
                Remove-FromPath
                Write-Host ""
                Remove-GitIdentity
                Write-Host ""
                Remove-GitCredential
                Write-Host ""
                Remove-SshKeys
                Write-Host ""
                Remove-GpgKeys
                Write-Host ""
                Remove-GcmDirectory
                Write-Host ""
                Remove-CredentialStore
                Write-Host ""
                Remove-ProjectMarkers
                Write-Host ""
                Remove-CompletionsAndTemp
                Write-Host ""
                Show-Completion "nuclear"
            } else {
                Write-Host ""
                Print-Info "Uninstallation cancelled - confirmation text did not match"
                Print-Separator "═"
                Write-Host "$($Colors.Dim)$($Colors.Gray)No changes were made to your system.$($Colors.Reset)"
                Print-Separator "═"
                Write-Host ""
            }
        }
        default {
            Write-Host ""
            Print-Info "Uninstallation cancelled by user"
            Print-Separator "═"
            Write-Host "$($Colors.Dim)$($Colors.Gray)No changes were made to your system.$($Colors.Reset)"
            Print-Separator "═"
            Write-Host ""
        }
    }
}

# Trap for clean exit
trap {
    Write-Host ""
    Print-Error "Uninstallation interrupted. Partial changes may have been made."
    exit 1
}

# Run main function
Main
