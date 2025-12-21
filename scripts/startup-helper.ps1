# Startup helper for mobile-relay on Windows
# The relay binary includes iOS 17.4+ tunnel management
# Usage: startup-helper.ps1 <command>
# Commands: install, uninstall, status, running, start, stop, logs, clear-logs

param(
    [Parameter(Position=0)]
    [string]$Command
)

$ErrorActionPreference = "Stop"

$TaskName = "MobileDockerRelay"
$ExtBase = "$env:USERPROFILE\.docker\extensions\aluedeke_mobile-docker-extension"
$RelayDir = "$ExtBase\host"
$RelayPath = "$RelayDir\mobile-relay.exe"
$LogFile = "$RelayDir\mobile-relay.log"
$PidFile = "$RelayDir\mobile-relay.pid"

function Test-RelayRunning {
    $process = Get-Process -Name "mobile-relay" -ErrorAction SilentlyContinue
    return $null -ne $process
}

function Get-RelayProcess {
    return Get-Process -Name "mobile-relay" -ErrorAction SilentlyContinue
}

function Cmd-Install {
    # Check if relay exists
    if (-not (Test-Path $RelayPath)) {
        Write-Output "Error: mobile-relay.exe not found at $RelayPath"
        Write-Output "Make sure the Docker extension is installed first."
        exit 1
    }

    # Remove existing task if present
    $existingTask = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
    if ($existingTask) {
        Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
    }

    # Create scheduled task for auto-start at login
    $action = New-ScheduledTaskAction -Execute $RelayPath -Argument "-port 27015 -addr 127.0.0.1 -tunnel-port 60105" -WorkingDirectory $RelayDir
    $trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
    $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
    $principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive -RunLevel Limited

    Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger -Settings $settings -Principal $principal -Description "Mobile Docker Extension Relay - bridges iOS devices to Docker containers" | Out-Null

    # Start the task immediately
    Start-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue

    Write-Output "installed"
}

function Cmd-Uninstall {
    # Remove scheduled task
    $existingTask = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
    if ($existingTask) {
        Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
    }

    # Stop any running relay process
    $process = Get-RelayProcess
    if ($process) {
        Stop-Process -Name "mobile-relay" -Force -ErrorAction SilentlyContinue
    }

    # Clean up PID file
    if (Test-Path $PidFile) {
        Remove-Item $PidFile -Force
    }

    Write-Output "uninstalled"
}

function Cmd-Status {
    $task = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
    if ($task) {
        Write-Output "installed"
    } else {
        Write-Output "not_installed"
    }
}

function Cmd-Running {
    if (Test-RelayRunning) {
        Write-Output "running"
    } else {
        Write-Output "stopped"
    }
}

function Cmd-Start {
    if (Test-RelayRunning) {
        Write-Output "already_running"
        return
    }

    # Check if relay exists
    if (-not (Test-Path $RelayPath)) {
        Write-Output "Error: mobile-relay.exe not found at $RelayPath"
        exit 1
    }

    # Clean up stale PID file
    if (Test-Path $PidFile) {
        Remove-Item $PidFile -Force
    }

    # Start relay in background
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = $RelayPath
    $psi.Arguments = "-port 27015 -addr 127.0.0.1 -tunnel-port 60105"
    $psi.WorkingDirectory = $RelayDir
    $psi.UseShellExecute = $false
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.CreateNoWindow = $true

    $process = [System.Diagnostics.Process]::Start($psi)

    # Save PID
    $process.Id | Out-File -FilePath $PidFile -Encoding ASCII

    # Give it a moment to start
    Start-Sleep -Seconds 1

    if (Test-RelayRunning) {
        Write-Output "started"
    } else {
        Write-Output "failed"
    }
}

function Cmd-Stop {
    # Stop relay process
    $process = Get-RelayProcess
    if ($process) {
        Stop-Process -Name "mobile-relay" -Force -ErrorAction SilentlyContinue
    }

    # Clean up PID file
    if (Test-Path $PidFile) {
        Remove-Item $PidFile -Force
    }

    Write-Output "stopped"
}

function Cmd-Logs {
    Write-Output "=== mobile-relay logs (includes tunnel manager) ==="
    if (Test-Path $LogFile) {
        Get-Content $LogFile -Tail 50
    }
}

function Cmd-ClearLogs {
    if (Test-Path $LogFile) {
        Clear-Content $LogFile
    }
    Write-Output "cleared"
}

# Main
switch ($Command) {
    "install" { Cmd-Install }
    "uninstall" { Cmd-Uninstall }
    "status" { Cmd-Status }
    "running" { Cmd-Running }
    "start" { Cmd-Start }
    "stop" { Cmd-Stop }
    "logs" { Cmd-Logs }
    "clear-logs" { Cmd-ClearLogs }
    default {
        Write-Output "Usage: startup-helper.ps1 {install|uninstall|status|running|start|stop|logs|clear-logs}"
        exit 1
    }
}
