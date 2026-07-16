param(
  [string]$Repository = "morp-ai/codex-auth-sync",
  [string]$Version = "latest",
  [string]$InstallDirectory = "$env:LOCALAPPDATA\Programs\codex-auth-sync"
)

$ErrorActionPreference = "Stop"
$task = Get-ScheduledTask -TaskName "CodexAuthSync" -ErrorAction SilentlyContinue
if ($null -ne $task) {
  Stop-ScheduledTask -TaskName "CodexAuthSync" -ErrorAction SilentlyContinue
}

$installer = Invoke-RestMethod -Uri "https://raw.githubusercontent.com/$Repository/main/scripts/install.ps1"
& ([scriptblock]::Create($installer)) -Repository $Repository -Version $Version -InstallDirectory $InstallDirectory

if ($null -ne $task) {
  Start-ScheduledTask -TaskName "CodexAuthSync"
} else {
  Write-Output "No managed background task was detected. Restart any manually managed daemon to use the new binary."
}

& (Join-Path $InstallDirectory "codex-auth-sync.exe") version
Write-Output "Update complete. codex-auth and its account data were not modified."
