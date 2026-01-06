# NAC Service Media - Scheduled Task Installation
# Creates or updates the Windows Scheduled Task for weekly media processing
#
# Usage:
#   From WSL:  make install-scheduled-task  (recommended)
#   Direct:    powershell.exe -ExecutionPolicy Bypass -File install-scheduled-task.ps1
#
# This script is idempotent - safe to run multiple times.

param(
    [string]$TaskName = "NAC-Service-Media-Weekly",
    [string]$TriggerDay = "Sunday",
    [string]$TriggerTime = "12:30",
    [switch]$Uninstall,
    [switch]$SkipBinaryInstall  # Used when called from Make (which handles binary install)
)

# Get script paths
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$ScheduledTaskScript = Join-Path $ScriptDir "scheduled-task.ps1"

# Verify scheduled-task.ps1 exists
if (-not (Test-Path $ScheduledTaskScript)) {
    Write-Error "scheduled-task.ps1 not found at: $ScheduledTaskScript"
    exit 1
}

# Handle uninstall
if ($Uninstall) {
    Write-Host "Uninstalling scheduled task: $TaskName"
    $ExistingTask = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
    if ($ExistingTask) {
        Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
        Write-Host "SUCCESS: Task '$TaskName' has been removed"
    } else {
        Write-Host "Task '$TaskName' does not exist"
    }
    exit 0
}

Write-Host "=========================================="
Write-Host "NAC Service Media - Task Installation"
Write-Host "=========================================="
Write-Host "Task Name: $TaskName"
Write-Host "Trigger: Every $TriggerDay at $TriggerTime"
Write-Host "Script: $ScheduledTaskScript"
Write-Host ""

$StepNum = 1

if (-not $SkipBinaryInstall) {
    # Step 1: Install the binary via WSL
    Write-Host "Step $StepNum`: Installing binary via WSL..."
    $InstallOutput = wsl.exe -d Ubuntu -- bash -lc "cd ~/src/nac/nac-service-media && make install-detection" 2>&1
    $InstallExitCode = $LASTEXITCODE

    if ($InstallExitCode -ne 0) {
        Write-Host "Output: $InstallOutput"
        Write-Error "Failed to install binary. Exit code: $InstallExitCode"
        exit 1
    }
    Write-Host "Binary installed to ~/go/bin/nac-service-media"
    $StepNum++

    # Step 2: Ensure PATH is set in .bashrc
    Write-Host ""
    Write-Host "Step $StepNum`: Ensuring PATH includes ~/go/bin..."
    $PathCheckCmd = @'
if ! grep -q 'export PATH=.*\$HOME/go/bin' ~/.bashrc 2>/dev/null; then
    echo '' >> ~/.bashrc
    echo '# Go binaries' >> ~/.bashrc
    echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.bashrc
    echo "ADDED"
else
    echo "EXISTS"
fi
'@
    $PathResult = wsl.exe -d Ubuntu -- bash -c $PathCheckCmd
    if ($PathResult -eq "ADDED") {
        Write-Host "Added ~/go/bin to PATH in .bashrc"
    } else {
        Write-Host "PATH already includes ~/go/bin"
    }
    $StepNum++
}

# Create or update the scheduled task
Write-Host ""
Write-Host "Step $StepNum`: Creating/updating scheduled task..."

# Check if task already exists
$ExistingTask = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue

# Define the action
$Action = New-ScheduledTaskAction `
    -Execute "powershell.exe" `
    -Argument "-ExecutionPolicy Bypass -WindowStyle Hidden -File `"$ScheduledTaskScript`""

# Define the trigger (weekly on specified day at specified time)
$DayOfWeek = [System.DayOfWeek]::$TriggerDay
$Trigger = New-ScheduledTaskTrigger -Weekly -DaysOfWeek $DayOfWeek -At $TriggerTime

# Define settings
$Settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -RunOnlyIfNetworkAvailable

# Define principal (run as current user)
$Principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive

if ($ExistingTask) {
    # Update existing task
    Write-Host "Updating existing task..."
    Set-ScheduledTask -TaskName $TaskName -Action $Action -Trigger $Trigger -Settings $Settings -Principal $Principal | Out-Null
    Write-Host "Task updated successfully"
} else {
    # Create new task
    Write-Host "Creating new task..."
    Register-ScheduledTask -TaskName $TaskName -Action $Action -Trigger $Trigger -Settings $Settings -Principal $Principal | Out-Null
    Write-Host "Task created successfully"
}

Write-Host ""
Write-Host "=========================================="
Write-Host "SUCCESS: Installation complete!"
Write-Host "=========================================="
Write-Host ""
Write-Host "The task '$TaskName' is now scheduled to run:"
Write-Host "  - Every $TriggerDay at $TriggerTime"
Write-Host "  - Logs are written to: $ScriptDir\..\logs\"
Write-Host ""
Write-Host "To test the task manually, run:"
Write-Host "  schtasks /run /tn `"$TaskName`""
Write-Host ""
Write-Host "To view the task in Task Scheduler:"
Write-Host "  taskschd.msc"
Write-Host ""
Write-Host "To uninstall:"
Write-Host "  .\install-scheduled-task.ps1 -Uninstall"
