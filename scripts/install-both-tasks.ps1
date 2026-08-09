# NAC Service Media - Install Both Scheduled Tasks
# Creates or updates both Windows Scheduled Tasks for weekly media processing
#
# Usage:
#   From WSL:  make install-scheduled-task  (recommended)
#   Direct:    powershell.exe -ExecutionPolicy Bypass -File install-both-tasks.ps1
#
# This script is idempotent - safe to run multiple times.

param(
    [switch]$Uninstall,
    [switch]$SkipBinaryInstall  # Used when called from Make (which handles binary install)
)

# Get script directory
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$InstallScript = Join-Path $ScriptDir "install-scheduled-task.ps1"

# Verify install-scheduled-task.ps1 exists
if (-not (Test-Path $InstallScript)) {
    Write-Error "install-scheduled-task.ps1 not found at: $InstallScript"
    exit 1
}

# Legacy task name from before the Sunday/Wednesday split.
# Earlier versions installed a single "NAC-Service-Media-Weekly" task on
# Sunday at 12:30 PM. If left in place alongside the new Sunday task it fires
# concurrently and causes duplicate uploads/emails.
$LegacyTaskName = "NAC-Service-Media-Weekly"

# Task Scheduler history is disabled by default. Without it, a skipped run
# (machine asleep, unmet launch condition) leaves no record anywhere -- no event
# and no log file, since scheduled-task.ps1 never starts. Turn it on so missed
# weeks are diagnosable after the fact. Requires elevation; non-fatal if refused.
function Enable-TaskHistory {
    $LogName = 'Microsoft-Windows-TaskScheduler/Operational'
    try {
        $State = wevtutil gl $LogName 2>$null | Select-String 'enabled:'
        if ($State -and $State.ToString() -match 'true') {
            Write-Host "Task Scheduler history already enabled"
            return
        }
        wevtutil sl $LogName /e:true 2>&1 | Out-Null
        if ($LASTEXITCODE -ne 0) {
            throw "wevtutil exited with $LASTEXITCODE"
        }
        Write-Host "Enabled Task Scheduler history"
    }
    catch {
        Write-Host "NOTE: Could not enable Task Scheduler history: $_"
        Write-Host "      Re-run this script as Administrator, or enable it manually in"
        Write-Host "      taskschd.msc (right pane -> Enable All Tasks History)."
    }
}

function Remove-LegacyTask {
    $existing = Get-ScheduledTask -TaskName $LegacyTaskName -ErrorAction SilentlyContinue
    if ($existing) {
        Write-Host "Removing legacy task: $LegacyTaskName"
        Unregister-ScheduledTask -TaskName $LegacyTaskName -Confirm:$false
        Write-Host "Legacy task removed"
    }
}

# Handle uninstall
if ($Uninstall) {
    Write-Host "=========================================="
    Write-Host "NAC Service Media - Uninstalling Tasks"
    Write-Host "=========================================="
    Write-Host ""

    # Uninstall Sunday task
    Write-Host "Uninstalling Sunday task..."
    & $InstallScript -TaskName "NAC-Service-Media-Weekly-Sunday" -Uninstall
    Write-Host ""

    # Uninstall Wednesday task
    Write-Host "Uninstalling Wednesday task..."
    & $InstallScript -TaskName "NAC-Service-Media-Weekly-Wednesday" -Uninstall
    Write-Host ""

    # Uninstall legacy task if present
    Remove-LegacyTask
    Write-Host ""

    Write-Host "=========================================="
    Write-Host "Uninstallation complete"
    Write-Host "=========================================="
    exit 0
}

Write-Host "=========================================="
Write-Host "NAC Service Media - Installing Both Tasks"
Write-Host "=========================================="
Write-Host ""

$InstallArgs = @{
    SkipBinaryInstall = $SkipBinaryInstall
}

# Remove legacy single-task installation if present (would fire alongside the
# new Sunday task and cause duplicate runs).
Remove-LegacyTask
Write-Host ""

Enable-TaskHistory
Write-Host ""

# Install Sunday task (12:30 PM)
Write-Host "Installing Sunday task (12:30 PM)..."
Write-Host "--------------------------------------"
& $InstallScript -TaskName "NAC-Service-Media-Weekly-Sunday" -TriggerDay "Sunday" -TriggerTime "12:30" @InstallArgs
Write-Host ""

# Install Wednesday task (9:30 PM)
Write-Host "Installing Wednesday task (9:30 PM)..."
Write-Host "--------------------------------------"
& $InstallScript -TaskName "NAC-Service-Media-Weekly-Wednesday" -TriggerDay "Wednesday" -TriggerTime "21:30" @InstallArgs
Write-Host ""

Write-Host "=========================================="
Write-Host "SUCCESS: Both tasks installed!"
Write-Host "=========================================="
Write-Host ""
Write-Host "Tasks created:"
Write-Host "  1. NAC-Service-Media-Weekly-Sunday    - Every Sunday at 12:30 PM"
Write-Host "  2. NAC-Service-Media-Weekly-Wednesday - Every Wednesday at 9:30 PM"
Write-Host ""
Write-Host "To test the tasks manually, run:"
Write-Host "  schtasks /run /tn `"NAC-Service-Media-Weekly-Sunday`""
Write-Host "  schtasks /run /tn `"NAC-Service-Media-Weekly-Wednesday`""
Write-Host ""
Write-Host "To view the tasks in Task Scheduler:"
Write-Host "  taskschd.msc"
Write-Host ""
Write-Host "To uninstall:"
Write-Host "  .\install-both-tasks.ps1 -Uninstall"
Write-Host ""
