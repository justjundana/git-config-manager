# GCM (Git Config Manager) installation script for Windows
# This script installs gcm to $env:USERPROFILE\.local\bin and adds it to PATH

param(
    [switch]$Quiet,
    [string]$Version,
    [switch]$AddToPath,
    [switch]$Init,
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
    Download = "⬇"
    Warning = "⚠"
    Install = "📦"
    Info = "ℹ"
    Rocket = "🚀"
    Gear = "⚙"
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
    if ($Quiet) { return }
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
    Write-Host "$($Colors.Bold)$($Colors.White)                Git Config Manager Installer$($Colors.Reset)"
    Write-Host "$($Colors.Dim)$($Colors.Gray)            Fast and secure installation process$($Colors.Reset)"
    Write-Host ""
    Print-Separator "═"
    Write-Host ""
}

# Print functions with icons and styling
function Print-Info {
    param([string]$Message)
    if ($Quiet) { return }
    Write-Host "$($Colors.Blue)$($Colors.Bold) $($Icons.Info)  INFO$($Colors.Reset) $($Colors.Gray)│$($Colors.Reset) $Message"
}

function Print-Success {
    param([string]$Message)
    if ($Quiet) { return }
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
    if ($Quiet) { return }
    Write-Host "$($Colors.Purple)$($Colors.Bold) $($Icons.Arrow)  STEP$($Colors.Reset) $($Colors.Gray)│$($Colors.Reset) $Message"
}

function Print-Install {
    param([string]$Message)
    if ($Quiet) { return }
    Write-Host "$($Colors.Cyan)$($Colors.Bold) $($Icons.Install)  INSTALLING$($Colors.Reset) $($Colors.Gray)│$($Colors.Reset) $Message"
}

# Show help information
function Show-Help {
    Write-Host "GCM installer - Git Config Manager Installation Script for Windows"
    Write-Host ""
    Write-Host "Usage: .\install.ps1 [OPTIONS]"
    Write-Host ""
    Write-Host "Options:"
    Write-Host "  -Quiet          Run in quiet mode (minimal output)"
    Write-Host "  -Version VER    Install specific version (e.g., v1.0.0)"
    Write-Host "  -AddToPath      Add the install directory to the user PATH"
    Write-Host "  -Init           Run 'gcm init' after install (mutates shell/git config)"
    Write-Host "  -Help           Show this help message"
    Write-Host ""
    Write-Host "Examples:"
    Write-Host "  .\install.ps1                   # Install latest version"
    Write-Host "  .\install.ps1 -Quiet            # Install quietly"
    Write-Host "  .\install.ps1 -Version v1.0.0   # Install specific version"
    Write-Host "  .\install.ps1 -AddToPath        # Install and update PATH explicitly"
    Write-Host "  .\install.ps1 -Init             # Install and explicitly initialize GCM"
}

function Get-ArtifactVersion {
    param([string]$ReleaseVersion)
    if ($ReleaseVersion.StartsWith("v")) {
        return $ReleaseVersion.Substring(1)
    }
    return $ReleaseVersion
}

function Get-ReleaseAssetName {
    param(
        [string]$ReleaseVersion,
        [string]$Platform
    )

    $parts = $Platform -split "/"
    $os = $parts[0]
    $arch = $parts[1]
    $artifactVersion = Get-ArtifactVersion $ReleaseVersion
    $extension = if ($os -eq "windows") { "zip" } else { "tar.gz" }
    return "gcm_${artifactVersion}_${os}_${arch}.${extension}"
}

function Save-Url {
    param(
        [string]$Url,
        [string]$OutFile
    )
    Invoke-WebRequest -Uri $Url -OutFile $OutFile -TimeoutSec 120 -UseBasicParsing
}

function Test-ReleaseChecksum {
    param(
        [string]$ArchivePath,
        [string]$ChecksumsPath
    )

    $fileName = Split-Path $ArchivePath -Leaf
    $entry = Get-Content $ChecksumsPath | Where-Object {
        $parts = $_ -split "\s+"
        $parts.Count -ge 2 -and $parts[1] -eq $fileName
    } | Select-Object -First 1

    if (-not $entry) {
        Print-Error "No checksum entry found for $fileName"
        return $false
    }

    $expected = ($entry -split "\s+")[0].ToLowerInvariant()
    $actual = (Get-FileHash -Algorithm SHA256 -Path $ArchivePath).Hash.ToLowerInvariant()
    if ($actual -ne $expected) {
        Print-Error "Checksum mismatch for $fileName"
        Print-Error "Expected: $expected"
        Print-Error "Actual:   $actual"
        return $false
    }

    Print-Success "Checksum verified for $fileName"
    return $true
}

# Detect platform (Windows architecture)
function Get-Platform {
    $arch = if ($env:PROCESSOR_ARCHITECTURE -eq "AMD64" -or $env:PROCESSOR_ARCHITEW6432 -eq "AMD64") {
        "amd64"
    } elseif ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") {
        "arm64"
    } else {
        "amd64"
    }
    return "windows/$arch"
}

# Get the latest release version from GitHub
function Get-LatestVersion {
    if ($Version) {
        return $Version
    }

    try {
        $response = Invoke-RestMethod -Uri "https://api.github.com/repos/justjundana/git-config-manager/releases/latest" -TimeoutSec 30
        return $response.tag_name
    }
    catch {
        Print-Error "Failed to get latest version information"
        Print-Info "Error: $($_.Exception.Message)"
        exit 1
    }
}

# Verify binary (basic validation)
function Test-Binary {
    param([string]$BinaryPath)

    if (-not (Test-Path $BinaryPath)) {
        Print-Error "Binary file not found: $BinaryPath"
        return $false
    }

    # Check file size (should be > 1MB for a Go binary)
    $fileSize = (Get-Item $BinaryPath).Length
    if ($fileSize -lt 1048576) {
        Print-Warning "Binary file seems unusually small ($fileSize bytes)"
    }

    # Try to get version to ensure it's a valid gcm binary
    try {
        $null = & $BinaryPath version 2>$null
        Print-Success "Binary validation completed"
        return $true
    }
    catch {
        Print-Error "Downloaded binary appears to be corrupted or invalid"
        return $false
    }
}

# ASCII spinner characters (compatible with all terminals)
$SpinChars = @('|', '/', '-', '\\')

# Download, checksum-verify, and extract the release archive.
function Download-Binary {
    param(
        [string]$DownloadVersion,
        [string]$Platform,
        [string]$InstallDir
    )

    $assetName = Get-ReleaseAssetName $DownloadVersion $Platform
    $baseUrl = "https://github.com/justjundana/git-config-manager/releases/download/$DownloadVersion"
    $archiveUrl = "$baseUrl/$assetName"
    $checksumsUrl = "$baseUrl/checksums.txt"
    $binaryPath = Join-Path $InstallDir "gcm.exe"
    $tempDir = Join-Path ([System.IO.Path]::GetTempPath()) "gcm-install-$([System.Guid]::NewGuid().ToString('N'))"
    $archivePath = Join-Path $tempDir $assetName
    $checksumsPath = Join-Path $tempDir "checksums.txt"
    $extractDir = Join-Path $tempDir "extract"

    Print-Step "Downloading gcm $DownloadVersion for $Platform..."
    Print-Info "Archive URL: $archiveUrl"
    Print-Info "Checksums URL: $checksumsUrl"

    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }
    New-Item -ItemType Directory -Path $tempDir -Force | Out-Null
    New-Item -ItemType Directory -Path $extractDir -Force | Out-Null

    if (-not $Quiet) {
        Write-Host "   $($Colors.Dim)Downloading release archive...$($Colors.Reset)"
        $ProgressPreference = 'Continue'
    } else {
        $ProgressPreference = 'SilentlyContinue'
    }

    try {
        Save-Url $archiveUrl $archivePath
        Save-Url $checksumsUrl $checksumsPath
    }
    catch {
        if (-not $Quiet) {
            Write-Host "   $($Colors.Red)$($Icons.Crossmark)$($Colors.Reset) Download failed."
        }
        Print-Error "Failed to download gcm release archive or checksums"
        Print-Info "Error: $($_.Exception.Message)"
        Remove-Item $tempDir -Recurse -Force -ErrorAction SilentlyContinue
        exit 1
    }

    if (-not $Quiet) {
        Write-Host "   $($Colors.Green)$($Icons.Checkmark)$($Colors.Reset) Downloaded release archive and checksums."
    }

    if (-not (Test-ReleaseChecksum $archivePath $checksumsPath)) {
        Remove-Item $tempDir -Recurse -Force -ErrorAction SilentlyContinue
        exit 1
    }

    try {
        Expand-Archive -LiteralPath $archivePath -DestinationPath $extractDir -Force
    }
    catch {
        Print-Error "Failed to extract release archive"
        Print-Info "Error: $($_.Exception.Message)"
        Remove-Item $tempDir -Recurse -Force -ErrorAction SilentlyContinue
        exit 1
    }

    $extractedBinary = Get-ChildItem -Path $extractDir -Recurse -File -Filter "gcm.exe" | Select-Object -First 1
    if (-not $extractedBinary) {
        Print-Error "Release archive did not contain gcm.exe"
        Remove-Item $tempDir -Recurse -Force -ErrorAction SilentlyContinue
        exit 1
    }

    Copy-Item -LiteralPath $extractedBinary.FullName -Destination $binaryPath -Force
    Remove-Item $tempDir -Recurse -Force -ErrorAction SilentlyContinue

    if (-not (Test-Binary $binaryPath)) {
        Print-Error "Binary validation failed"
        Remove-Item $binaryPath -Force -ErrorAction SilentlyContinue
        exit 1
    }

    Print-Success "Installed verified gcm binary to $binaryPath"
    return $binaryPath
}

# Add to PATH only when explicitly requested.
function Add-ToPath {
    param([string]$InstallDir)

    $gcmBinary = Join-Path $InstallDir "gcm.exe"

    if (-not (Test-Path $gcmBinary)) {
        Print-Error "gcm binary not found at $gcmBinary"
        exit 1
    }

    Print-Step "Configuring Windows environment..."

    $userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    $pathEntries = @()
    if ($userPath) {
        $pathEntries = $userPath -split ";" | Where-Object { $_ }
    }

    if ($pathEntries -notcontains $InstallDir) {
        $newPath = if ($userPath) { "$userPath;$InstallDir" } else { $InstallDir }
        [Environment]::SetEnvironmentVariable("PATH", $newPath, "User")
        Print-Success "Added $InstallDir to user PATH"
    } else {
        Print-Info "Install directory already in PATH"
    }

    $env:PATH = "$InstallDir;$env:PATH"
}

# Run gcm init only when explicitly requested.
function Run-Init {
    param([string]$InstallDir)

    $gcmBinary = Join-Path $InstallDir "gcm.exe"

    if (-not (Test-Path $gcmBinary)) {
        Print-Error "gcm binary not found at $gcmBinary"
        exit 1
    }

    Print-Step "Initializing GCM by explicit request..."

    if (-not $Quiet) {
        Write-Host -NoNewline "   $($Colors.Dim)Configuring shell integration... $($Colors.Purple)$($SpinChars[0])$($Colors.Reset) "
    }

    try {
        $initOutput = & $gcmBinary init 2>&1
        if ($LASTEXITCODE -eq 0) {
            if (-not $Quiet) {
                Write-Host "`r   $($Colors.Green)$($Icons.Checkmark)$($Colors.Reset) Configured shell integration successfully.      "
            }
            Print-Success "Shell integration configured successfully"
            if ($initOutput -and -not $Quiet) {
                Write-Host $initOutput
            }
        } else {
            if (-not $Quiet) {
                Write-Host "`r   $($Colors.Yellow)$($Icons.Warning)$($Colors.Reset) Shell integration had issues.      "
            }
            Print-Warning "Shell integration had issues. You may need to run 'gcm init' manually."
        }
    }
    catch {
        if (-not $Quiet) {
            Write-Host "`r   $($Colors.Red)$($Icons.Crossmark)$($Colors.Reset) Shell integration failed.      "
        }
        Print-Error "Could not run 'gcm init'. No fallback shell mutation was applied."
        Print-Info "Error: $($_.Exception.Message)"
        exit 1
    }
}

# Show system information
function Show-SystemInfo {
    param(
        [string]$Platform,
        [string]$InfoVersion,
        [string]$InstallDir
    )

    if ($Quiet) { return }

    Print-Separator "┄"
    Write-Host "$($Colors.Bold)$($Colors.White)System Information:$($Colors.Reset)"
    Print-Separator "┄"

    $parts = $Platform -split "/"
    $arch = $parts[1]

    Write-Host "$($Colors.Green) $($Icons.Checkmark)$($Colors.Reset) Operating System: $($Colors.Bold)Windows$($Colors.Reset)"
    Write-Host "$($Colors.Green) $($Icons.Checkmark)$($Colors.Reset) Architecture: $($Colors.Bold)$arch$($Colors.Reset)"
    Write-Host "$($Colors.Green) $($Icons.Checkmark)$($Colors.Reset) Version: $($Colors.Bold)$InfoVersion$($Colors.Reset)"
    Write-Host "$($Colors.Blue) $($Icons.Info)$($Colors.Reset) Install Directory: $($Colors.Bold)$InstallDir$($Colors.Reset)"

    Print-Separator "┄"
    Write-Host ""
}

# Show completion message
function Show-Completion {
    param(
        [string]$CompletionVersion,
        [bool]$PathUpdated,
        [bool]$Initialized
    )

    Write-Host ""
    Print-Separator "═"
    Write-Host ""
    Write-Host "$($Colors.Green)$($Colors.Bold) $($Icons.Rocket)  INSTALLATION SUCCESSFUL!$($Colors.Reset)"
    Write-Host ""
    Print-Separator "┄"
    Write-Host "$($Colors.Bold)$($Colors.White)What was installed:$($Colors.Reset)"
    Write-Host " • gcm binary"
    Write-Host " • Release checksum verification"
    if ($PathUpdated) {
        Write-Host " • Windows PATH configuration"
    }
    if ($Initialized) {
        Write-Host " • Shell integration"
    }
    Print-Separator "┄"
    Write-Host "$($Colors.Bold)$($Colors.White)Next Steps:$($Colors.Reset)"
    Write-Host " 1. Restart your PowerShell/Command Prompt"
    Write-Host " 2. Verify with 'gcm version'"
    Write-Host " 3. Initialize with 'gcm init' when you want shell hooks"
    Write-Host " 4. Create your first profile with 'gcm profile create <name>'"
    Print-Separator "┄"
    Write-Host "$($Colors.Bold)$($Colors.White)Quick Commands:$($Colors.Reset)"
    Write-Host " • gcm profile create work   - Create a profile"
    Write-Host " • gcm use work              - Switch to a profile"
    Write-Host " • gcm ssh generate work     - Generate SSH key"
    Write-Host " • gcm github login-oauth    - Authenticate with GitHub"
    Write-Host " • gcm doctor                - Check system health"
    Print-Separator "┄"
    Write-Host "Welcome to GCM! 🎉"
    Print-Separator "═"
    Write-Host ""
}

# Check if gcm is already installed
function Test-ExistingInstallation {
    $installDir = Join-Path $env:USERPROFILE ".local\bin"
    $gcmDir = Join-Path $env:USERPROFILE ".gcm"
    $binaryPath = Join-Path $installDir "gcm.exe"
    $binaryFound = Test-Path $binaryPath
    $binaryValid = $false
    $commandFound = $false

    Print-Step "Checking for existing installation..."

    # Verify binary is actually valid (not corrupted/partial)
    if ($binaryFound) {
        try {
            $null = & $binaryPath version 2>$null
            if ($LASTEXITCODE -eq 0) { $binaryValid = $true }
        } catch {}
    }

    # Check if gcm command is available and works
    $gcmCmd = Get-Command gcm -ErrorAction SilentlyContinue
    if ($gcmCmd) {
        try {
            $null = & gcm version 2>$null
            if ($LASTEXITCODE -eq 0) { $commandFound = $true }
        } catch {}
    }

    # If binary exists but is broken, remove it and proceed
    if ($binaryFound -and -not $binaryValid) {
        Print-Warning "Found corrupted/incomplete gcm binary at $binaryPath"
        Print-Info "Removing broken binary and proceeding with fresh install..."
        Remove-Item -Path $binaryPath -Force -ErrorAction SilentlyContinue
        Write-Host ""
        return
    }

    # Only block if we have a WORKING installation
    if ($binaryValid -or $commandFound) {
        Write-Host ""
        Print-Separator "┄"
        Write-Host "$($Colors.Bold)$($Colors.White)Existing Installation Detected:$($Colors.Reset)"
        Print-Separator "┄"

        if ($binaryValid) {
            Write-Host "$($Colors.Green) $($Icons.Checkmark)$($Colors.Reset) Binary found: $($Colors.Bold)$binaryPath$($Colors.Reset)"
        }

        if ($commandFound) {
            try {
                $existingVersion = & gcm version 2>$null | Select-Object -First 1
            } catch {
                $existingVersion = "unknown"
            }
            Write-Host "$($Colors.Green) $($Icons.Checkmark)$($Colors.Reset) Command available: $($Colors.Bold)gcm$($Colors.Reset) $($Colors.Dim)($existingVersion)$($Colors.Reset)"
        }

        if (Test-Path $gcmDir) {
            $dirSize = "{0:N2} MB" -f ((Get-ChildItem $gcmDir -Recurse -ErrorAction SilentlyContinue | Measure-Object -Property Length -Sum).Sum / 1MB)
            Write-Host "$($Colors.Blue) $($Icons.Info)$($Colors.Reset) Data directory: $($Colors.Bold)$gcmDir$($Colors.Reset) $($Colors.Dim)($dirSize)$($Colors.Reset)"
        }

        Print-Separator "┄"
        Write-Host ""
        Print-Warning "GCM is already installed on this system!"
        Write-Host ""
        Print-Separator "┄"
        Write-Host "$($Colors.Bold)$($Colors.White)What you can do:$($Colors.Reset)"
        Write-Host " • Run 'gcm version' to check current version"
        Write-Host " • Run 'gcm --help' to see available commands"
        Write-Host " • Use the uninstaller script first if you need to reinstall"
        Write-Host " • Run 'gcm doctor' to check system health"
        Print-Separator "┄"
        Write-Host ""
        Print-Separator "═"
        Write-Host "$($Colors.Dim)$($Colors.Gray)Installation cancelled - gcm already exists$($Colors.Reset)"
        Print-Separator "═"
        Write-Host ""
        exit 0
    } else {
        Print-Success "No existing installation found - proceeding with fresh install"
        Write-Host ""
    }
}

# Main installation function
function Main {
    if ($Help) {
        Show-Help
        exit 0
    }

    Print-Header

    Print-Info "Starting GCM installation process..."
    Write-Host ""

    Test-ExistingInstallation

    # Detect platform
    Print-Step "Detecting system platform..."
    $platform = Get-Platform
    Print-Success "Detected platform: $($Colors.Bold)$platform$($Colors.Reset)"
    Write-Host ""

    # Get latest version
    Print-Step "Fetching latest version information..."
    $latestVersion = Get-LatestVersion
    Print-Success "Latest version: $($Colors.Bold)$latestVersion$($Colors.Reset)"
    Write-Host ""

    # Set installation directory
    $installDir = Join-Path $env:USERPROFILE ".local\bin"
    Print-Info "Installation directory: $($Colors.Bold)$installDir$($Colors.Reset)"
    Write-Host ""

    # Show system info
    Show-SystemInfo $platform $latestVersion $installDir

    # Download binary
    $binaryPath = Download-Binary $latestVersion $platform $installDir
    Write-Host ""

    if ($AddToPath) {
        Add-ToPath $installDir
        Write-Host ""
    } else {
        Print-Info "User PATH was not modified. To opt in, rerun with -AddToPath or add this manually: $installDir"
        Write-Host ""
    }

    if ($Init) {
        Run-Init $installDir
        Write-Host ""
    } else {
        Print-Info "GCM initialization was not run. Run 'gcm init' when you are ready to install shell hooks."
        Write-Host ""
    }

    # Verify installation
    Print-Step "Verifying installation..."
    try {
        $null = & $binaryPath version 2>$null
        $installedVersion = & $binaryPath version 2>$null | Select-Object -First 1
        Print-Success "Installation verified: $($Colors.Bold)$installedVersion$($Colors.Reset)"
        Show-Completion $latestVersion $AddToPath.IsPresent $Init.IsPresent
    }
    catch {
        Print-Warning "Installation completed, but verification failed"
        Write-Host ""
        Print-Separator "┄"
        Write-Host "$($Colors.Bold)$($Colors.White)Manual Steps Required:$($Colors.Reset)"
        Write-Host " 1. Restart your PowerShell/Command Prompt"
        Write-Host " 2. Try running 'gcm version'"
        Write-Host " 3. If issues persist, run 'gcm init' manually"
        Print-Separator "┄"
        Write-Host ""
    }
}

# Trap for clean exit
trap {
    Write-Host ""
    Print-Error "Installation interrupted. Partial installation may have occurred."
    exit 1
}

# Run main function
Main
