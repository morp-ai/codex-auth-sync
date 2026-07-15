param(
  [string]$BinaryPath = "",
  [string]$ConfigPath = "$env:APPDATA\codex-auth-sync\config.json"
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($BinaryPath)) {
  $command = Get-Command codex-auth-sync.exe -ErrorAction SilentlyContinue
  if ($null -eq $command) {
    throw "A valid codex-auth-sync.exe path is required."
  }
  $BinaryPath = $command.Source
}

$action = New-ScheduledTaskAction -Execute $BinaryPath -Argument "daemon --config `"$ConfigPath`""
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$settings = New-ScheduledTaskSettingsSet -RestartCount 5 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero)
Register-ScheduledTask -TaskName "CodexAuthSync" -Action $action -Trigger $trigger -Settings $settings -Description "Codex Auth Sync Agent" -Force | Out-Null
Start-ScheduledTask -TaskName "CodexAuthSync"
Write-Output "Installed and started the CodexAuthSync scheduled task."
