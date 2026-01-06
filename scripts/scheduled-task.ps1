# NAC Service Media - Scheduled Task Runner
# This script is invoked by Windows Task Scheduler to run the media processing command
# via WSL. It handles logging and ensures proper environment setup.
#
# Usage: powershell.exe -ExecutionPolicy Bypass -File scheduled-task.ps1

param(
    [string]$Recipient = "Jonathan",
    [switch]$DryRun
)

# Get script directory and set up paths
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$ProjectDir = Split-Path -Parent $ScriptDir
$LogDir = Join-Path $ProjectDir "logs"
$DateStamp = Get-Date -Format "yyyy-MM-dd"
$TimeStamp = Get-Date -Format "yyyy-MM-dd_HH-mm-ss"
$LogFile = Join-Path $LogDir "scheduled-run-$TimeStamp.log"

# Create logs directory if it doesn't exist
if (-not (Test-Path $LogDir)) {
    New-Item -ItemType Directory -Path $LogDir -Force | Out-Null
}

# Function to write to both console and log file
function Write-Log {
    param([string]$Message)
    $Timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    $Line = "[$Timestamp] $Message"
    Write-Host $Line
    Add-Content -Path $LogFile -Value $Line
}

Write-Log "=========================================="
Write-Log "NAC Service Media - Scheduled Task"
Write-Log "=========================================="
Write-Log "Date: $DateStamp"
Write-Log "Recipient: $Recipient"
Write-Log "Log file: $LogFile"

# Convert Windows project path to WSL path
$WslProjectDir = (wsl.exe wslpath -u ($ProjectDir -replace '\\', '/')).Trim()
Write-Log "Project dir: $WslProjectDir"
Write-Log ""

# Build the WSL command
# Uses -l (login shell) to ensure PATH is loaded from .profile
# Changes to project directory first so config/config.yaml is found
$WslCommand = "cd $WslProjectDir && nac-service-media process --recipient $Recipient"

if ($DryRun) {
    Write-Log "[DRY RUN] Would execute: wsl.exe -d Ubuntu -- bash -lc `"$WslCommand`""
    exit 0
}

Write-Log "Executing: wsl.exe -d Ubuntu -- bash -lc `"$WslCommand`""
Write-Log ""

# Run the command and capture output
try {
    $Output = wsl.exe -d Ubuntu -- bash -lc $WslCommand 2>&1
    $ExitCode = $LASTEXITCODE

    # Log all output
    foreach ($Line in $Output) {
        Write-Log $Line
    }

    Write-Log ""
    Write-Log "Exit code: $ExitCode"

    if ($ExitCode -eq 0) {
        Write-Log "SUCCESS: Processing completed successfully"
    } else {
        Write-Log "FAILURE: Processing failed with exit code $ExitCode"
    }

    exit $ExitCode
}
catch {
    Write-Log "ERROR: Exception occurred: $_"
    exit 1
}
