param(
  [string]$InstallDirectory = "$env:LOCALAPPDATA\Programs\codex-auth-sync",
  [switch]$Purge
)

$ErrorActionPreference = "Stop"

$task = Get-ScheduledTask -TaskName "CodexAuthSync" -ErrorAction SilentlyContinue
if ($null -ne $task) {
  Stop-ScheduledTask -TaskName "CodexAuthSync" -ErrorAction SilentlyContinue
  Unregister-ScheduledTask -TaskName "CodexAuthSync" -Confirm:$false
}

$binary = Join-Path $InstallDirectory "codex-auth-sync.exe"
Remove-Item -Force $binary -ErrorAction SilentlyContinue
if ((Test-Path $InstallDirectory) -and @(Get-ChildItem -Force $InstallDirectory).Count -eq 0) {
  Remove-Item -Force $InstallDirectory
}
Write-Output "Removed the background task and $binary."

if ($Purge) {
  Remove-Item -Recurse -Force (Join-Path $env:APPDATA "codex-auth-sync") -ErrorAction SilentlyContinue
  Remove-Item -Recurse -Force (Join-Path $HOME ".codex-auth-sync") -ErrorAction SilentlyContinue
  Write-Output "Removed local synchronization configuration, state, logs, and backups."
} else {
  Write-Output "Local synchronization configuration and backups were preserved."
}

Write-Output "codex-auth, ~/.codex, and remote WebDAV data were not modified."
