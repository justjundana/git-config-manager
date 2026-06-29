@echo off
setlocal enabledelayedexpansion

REM GCM (Git Config Manager) installer wrapper for Windows Command Prompt.
REM The PowerShell installer is the canonical Windows implementation so that
REM checksum verification and opt-in environment mutation
REM stay consistent across Windows entry points.

set "SCRIPT_DIR=%~dp0"
set "PS_INSTALLER=%SCRIPT_DIR%install.ps1"
set "PS_ARGS="

if not exist "%PS_INSTALLER%" (
    echo ERROR: PowerShell installer not found: %PS_INSTALLER%
    exit /b 1
)

:parse_args
if "%~1"=="" goto :run_installer

if /i "%~1"=="--quiet" (
    set "PS_ARGS=!PS_ARGS! -Quiet"
    shift
    goto :parse_args
)
if /i "%~1"=="-q" (
    set "PS_ARGS=!PS_ARGS! -Quiet"
    shift
    goto :parse_args
)
if /i "%~1"=="--version" (
    if "%~2"=="" (
        echo ERROR: --version requires a value.
        exit /b 1
    )
    set "PS_ARGS=!PS_ARGS! -Version \"%~2\""
    shift
    shift
    goto :parse_args
)
if /i "%~1"=="-v" (
    if "%~2"=="" (
        echo ERROR: -v requires a value.
        exit /b 1
    )
    set "PS_ARGS=!PS_ARGS! -Version \"%~2\""
    shift
    shift
    goto :parse_args
)
if /i "%~1"=="--add-to-path" (
    set "PS_ARGS=!PS_ARGS! -AddToPath"
    shift
    goto :parse_args
)
if /i "%~1"=="--init" (
    REM Initialization is on by default; flag kept for backward compatibility.
    shift
    goto :parse_args
)
if /i "%~1"=="--no-init" (
    set "PS_ARGS=!PS_ARGS! -NoInit"
    shift
    goto :parse_args
)
if /i "%~1"=="--help" (
    set "PS_ARGS=!PS_ARGS! -Help"
    shift
    goto :parse_args
)
if /i "%~1"=="-h" (
    set "PS_ARGS=!PS_ARGS! -Help"
    shift
    goto :parse_args
)

echo ERROR: Unknown option: %~1
echo.
powershell -NoProfile -ExecutionPolicy Bypass -File "%PS_INSTALLER%" -Help
exit /b 1

:run_installer
powershell -NoProfile -ExecutionPolicy Bypass -File "%PS_INSTALLER%" %PS_ARGS%
exit /b %ERRORLEVEL%
