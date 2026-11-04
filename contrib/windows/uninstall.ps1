# Removes the scheduled task registered by install.ps1.
[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

if (Get-ScheduledTask -TaskName "randamu" -ErrorAction SilentlyContinue) {
    Unregister-ScheduledTask -TaskName "randamu" -Confirm:$false
    Write-Host "Removed scheduled task 'randamu'."
} else {
    Write-Host "No scheduled task 'randamu' found; nothing to do."
}
