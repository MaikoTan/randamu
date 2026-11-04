# Registers a scheduled task that runs randamu daily. The task runs as the
# current user, so no administrator prompt appears.
#
#   powershell -ExecutionPolicy Bypass -File install.ps1 [-ConfigPath PATH] [-At TIME] [-RandomDelay HH:MM:SS]
#
# Example: install.ps1 -At 21:00 -RandomDelay 01:00:00
[CmdletBinding()]
param(
    # Config file passed to randamu; the script does not create it.
    [string]$ConfigPath = "$env:APPDATA\randamu\config.json",
    # Daily start time for the task.
    [string]$At = "09:00",
    # Random delay added to the start time.
    [string]$RandomDelay = "01:00:00"
)

$ErrorActionPreference = "Stop"

$exe = Join-Path $PSScriptRoot "randamu.exe"
if (-not (Test-Path $exe)) {
    throw "randamu.exe not found next to install.ps1 ($PSScriptRoot)"
}
if (-not (Test-Path $ConfigPath)) {
    Write-Warning "Config file $ConfigPath does not exist yet; randamu uses defaults until you create it."
}

$action = New-ScheduledTaskAction -Execute $exe -Argument "--config `"$ConfigPath`""
$trigger = New-ScheduledTaskTrigger -Daily -At $At
$trigger.RandomDelay = $RandomDelay
# Run only with a network connection; the token comes from the config's
# token sources, not from the task.
$settings = New-ScheduledTaskSettingsSet -StartWhenAvailable

Register-ScheduledTask -TaskName "randamu" -Action $action -Trigger $trigger `
    -Settings $settings -Description "randamu periodic pixiv image fetcher" -Force

Write-Host "Registered scheduled task 'randamu' (daily at $At, delay $RandomDelay)."
Write-Host "Set PIXIV_REFRESH_TOKEN as a user environment variable, or put the token in the config's token sources."
Write-Host "Test now with: Start-ScheduledTask -TaskName randamu"
