param(
  [string]$Server = $(if ($env:CODEX_AUTH_SYNC_SERVER_URL) { $env:CODEX_AUTH_SYNC_SERVER_URL } else { "https://app.koofr.net/dav/Koofr" }),
  [string]$Username = $env:CODEX_AUTH_SYNC_WEBDAV_USERNAME,
  [string]$WebDAVPassword = $env:CODEX_AUTH_SYNC_WEBDAV_PASSWORD,
  [string]$SyncKey = $env:CODEX_AUTH_SYNC_KEY,
  [string]$Cluster = $(if ($env:CODEX_AUTH_SYNC_CLUSTER_ID) { $env:CODEX_AUTH_SYNC_CLUSTER_ID } else { "personal" }),
  [ValidateSet("local", "group", "global")]
  [string]$ActiveScope = $(if ($env:CODEX_AUTH_SYNC_ACTIVE_SCOPE) { $env:CODEX_AUTH_SYNC_ACTIVE_SCOPE } else { "group" }),
  [string]$ActiveGroup = $(if ($env:CODEX_AUTH_SYNC_ACTIVE_GROUP) { $env:CODEX_AUTH_SYNC_ACTIVE_GROUP } else { "main" }),
  [ValidateRange(1, 3600)]
  [int]$Interval = $(if ($env:CODEX_AUTH_SYNC_SCAN_INTERVAL) { [int]$env:CODEX_AUTH_SYNC_SCAN_INTERVAL } else { 5 }),
  [ValidateSet("auto", "seed", "join", "merge", "local")]
  [string]$Role = $(if ($env:CODEX_AUTH_SYNC_ROLE) { $env:CODEX_AUTH_SYNC_ROLE } else { "auto" }),
  [string]$ReplacementCluster = $env:CODEX_AUTH_SYNC_REPLACEMENT_CLUSTER,
  [string]$Repository = "morp-ai/codex-auth-sync",
  [switch]$SkipCodexInstall,
  [switch]$NoService
)

$ErrorActionPreference = "Stop"

function Read-PlainSecret([string]$Prompt) {
  $secure = Read-Host -Prompt $Prompt -AsSecureString
  $pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
  try {
    return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer)
  } finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer)
  }
}

if ([string]::IsNullOrWhiteSpace($Username)) {
  $Username = Read-Host -Prompt "WebDAV username"
}
if ([string]::IsNullOrWhiteSpace($WebDAVPassword)) {
  $WebDAVPassword = Read-PlainSecret "WebDAV app password"
}
if ([string]::IsNullOrWhiteSpace($Username) -or [string]::IsNullOrWhiteSpace($WebDAVPassword)) {
  throw "WebDAV username and app password are required."
}

function Refresh-Path {
  $machinePath = [Environment]::GetEnvironmentVariable("Path", "Machine")
  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  $env:Path = "$machinePath;$userPath"
}

function Install-NodeIfMissing {
  if ($null -ne (Get-Command npm.cmd -ErrorAction SilentlyContinue)) {
    return
  }
  $winget = Get-Command winget.exe -ErrorAction SilentlyContinue
  if ($null -eq $winget) {
    throw "Node.js and npm are missing, and winget is unavailable. Install Node.js LTS and run this script again."
  }
  Write-Output "Node.js and npm were not found; installing Node.js LTS with winget."
  & $winget.Source install --id OpenJS.NodeJS.LTS --exact --accept-package-agreements --accept-source-agreements
  if ($LASTEXITCODE -ne 0) {
    throw "Node.js installation failed."
  }
  Refresh-Path
  if ($null -eq (Get-Command npm.cmd -ErrorAction SilentlyContinue)) {
    $nodeDirectory = Join-Path $env:ProgramFiles "nodejs"
    if (Test-Path $nodeDirectory) {
      $env:Path = "$nodeDirectory;$env:Path"
    }
  }
  if ($null -eq (Get-Command npm.cmd -ErrorAction SilentlyContinue)) {
    throw "npm is still unavailable after installing Node.js. Open a new PowerShell window and run this script again."
  }
}

if (-not $SkipCodexInstall) {
  $packages = @()
  $existingCodex = Get-Command codex.cmd -ErrorAction SilentlyContinue
  if ($null -eq $existingCodex) {
    $existingCodex = Get-Command codex.exe -ErrorAction SilentlyContinue
  }
  if ($null -ne $existingCodex) {
    Write-Output "Existing OpenAI Codex CLI installation detected; keeping it."
  } else {
    $packages += "@openai/codex"
  }
  $existingCodexAuth = Get-Command codex-auth.cmd -ErrorAction SilentlyContinue
  if ($null -eq $existingCodexAuth) {
    $existingCodexAuth = Get-Command codex-auth.exe -ErrorAction SilentlyContinue
  }
  if ($null -ne $existingCodexAuth) {
    Write-Output "Existing codex-auth installation detected; keeping it."
  } else {
    $packages += "@loongphy/codex-auth"
  }
  if ($packages.Count -gt 0) {
    Install-NodeIfMissing
    Write-Output "Installing missing Codex tools: $($packages -join ', ')"
    & npm.cmd install --global @packages
    if ($LASTEXITCODE -ne 0) {
      throw "Installing the missing Codex tools failed."
    }
  }
}

$installScript = Invoke-RestMethod -Uri "https://raw.githubusercontent.com/$Repository/main/scripts/install.ps1"
& ([scriptblock]::Create($installScript)) -Repository $Repository

$installDirectory = Join-Path $env:LOCALAPPDATA "Programs\codex-auth-sync"
$SyncBinary = Join-Path $installDirectory "codex-auth-sync.exe"
if (-not (Test-Path $SyncBinary)) {
  $command = Get-Command codex-auth-sync.exe -ErrorAction SilentlyContinue
  if ($null -eq $command) {
    throw "codex-auth-sync installation failed."
  }
  $SyncBinary = $command.Source
}

$CodexHome = if ($env:CODEX_HOME) { $env:CODEX_HOME } else { Join-Path $HOME ".codex" }
$ConfigDirectory = Join-Path $env:APPDATA "codex-auth-sync"
$ConfigPath = Join-Path $ConfigDirectory "config.json"
$StatePath = Join-Path $ConfigDirectory "state.json"
$BackupRoot = Join-Path $HOME ".codex-auth-sync\backups"

function Get-ClusterHash([string]$Value) {
  $sha = [Security.Cryptography.SHA256]::Create()
  try {
    $bytes = [Text.Encoding]::UTF8.GetBytes($Value)
    return ([BitConverter]::ToString($sha.ComputeHash($bytes))).Replace("-", "").ToLowerInvariant()
  } finally {
    $sha.Dispose()
  }
}

function Test-RemoteCluster {
  $hash = Get-ClusterHash $Cluster
  $url = $Server.TrimEnd("/") + "/codex-auth-sync-v1-$hash.json"
  $pair = "${Username}:${WebDAVPassword}"
  $authorization = "Basic " + [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($pair))
  try {
    $response = Invoke-WebRequest -Uri $url -Method Get -Headers @{ Authorization = $authorization; "Cache-Control" = "no-cache" } -UseBasicParsing
    return $response.StatusCode -ge 200 -and $response.StatusCode -lt 300
  } catch {
    $status = 0
    if ($null -ne $_.Exception.Response) {
      $status = [int]$_.Exception.Response.StatusCode
    }
    if ($status -eq 404) {
      return $false
    }
    if ($status -eq 401 -or $status -eq 403) {
      throw "WebDAV authentication failed with HTTP $status."
    }
    throw "WebDAV state probe failed with HTTP $status."
  }
}

function Get-LocalAccountCount {
  $accountsDirectory = Join-Path $CodexHome "accounts"
  if (-not (Test-Path $accountsDirectory)) {
    return 0
  }
  return @(Get-ChildItem -Path $accountsDirectory -Filter "*.auth.json" -File -ErrorAction SilentlyContinue).Count
}

$RemoteExists = Test-RemoteCluster
$LocalAccounts = Get-LocalAccountCount

if ($Role -eq "auto" -and -not $RemoteExists -and $LocalAccounts -eq 0 -and -not $SkipCodexInstall) {
  $codexAuth = Get-Command codex-auth.cmd -ErrorAction SilentlyContinue
  if ($null -eq $codexAuth) {
    $codexAuth = Get-Command codex-auth.exe -ErrorAction SilentlyContinue
  }
  if ($null -eq $codexAuth) {
    throw "codex-auth is unavailable after installation."
  }
  $startLogin = Read-Host -Prompt "No local accounts were found. Start 'codex-auth login' now? [Y/n]"
  if ([string]::IsNullOrWhiteSpace($startLogin) -or $startLogin -match '^(?i:y|yes)$') {
    & $codexAuth.Source login
    if ($LASTEXITCODE -ne 0) {
      throw "codex-auth login failed."
    }
    $LocalAccounts = Get-LocalAccountCount
  }
}

if ($Role -eq "auto") {
  if (-not $RemoteExists -and $LocalAccounts -gt 0) {
    $Role = "seed"
  } elseif ($RemoteExists -and $LocalAccounts -eq 0) {
    $Role = "join"
  } elseif (-not $RemoteExists) {
    throw "The remote cluster is empty and no local accounts were found. Run 'codex-auth login' on the first server, then run this script again."
  } else {
    Write-Output "Both local accounts and remote synchronization data exist."
    Write-Output "Choose 'merge' for a union, 'cloud' to use the cloud as authoritative, or 'local' to create a new local-authoritative cluster."
    $selection = Read-Host -Prompt "Deployment role [merge/cloud/local]"
    if ([string]::IsNullOrWhiteSpace($selection) -or $selection -eq "merge") {
      $Role = "merge"
    } elseif ($selection -eq "cloud" -or $selection -eq "join") {
      $Role = "join"
    } elseif ($selection -eq "local") {
      $Role = "local"
    } else {
      throw "Role must be merge, cloud, or local."
    }
  }
}

if ($Role -eq "local") {
  if ($LocalAccounts -eq 0) {
    throw "Local-authoritative mode requires at least one local codex-auth account."
  }
  if ([string]::IsNullOrWhiteSpace($ReplacementCluster)) {
    $ReplacementCluster = Read-Host -Prompt "New replacement cluster ID"
  }
  if ([string]::IsNullOrWhiteSpace($ReplacementCluster) -or $ReplacementCluster -eq $Cluster) {
    throw "Local-authoritative mode requires a different, previously unused replacement cluster ID."
  }
  $Cluster = $ReplacementCluster
  $RemoteExists = Test-RemoteCluster
  if ($RemoteExists) {
    throw "The replacement cluster already contains data. Choose a new replacement cluster ID."
  }
  $Role = "seed"
  Write-Output "Using new local-authoritative cluster: $Cluster"
}

if ($Role -eq "seed") {
  if ($RemoteExists) {
    throw "Seed mode requires an empty remote cluster. Choose a new cluster ID or use merge mode."
  }
  if ($LocalAccounts -eq 0) {
    throw "Seed mode requires at least one local codex-auth account. Run 'codex-auth login' and then run this script again."
  }
} elseif ($Role -eq "join" -and -not $RemoteExists) {
  throw "Join mode requires an existing remote cluster."
}

if ([string]::IsNullOrWhiteSpace($SyncKey)) {
  if ($Role -eq "seed") {
    $generated = (& $SyncBinary keygen --json | ConvertFrom-Json)
    $SyncKey = $generated.sync_key
    Write-Output "Generated synchronization key. Store it in a password manager; every server needs the same value:"
    Write-Output $SyncKey
  } else {
    $SyncKey = Read-PlainSecret "Shared synchronization key"
  }
}

$timestamp = [DateTime]::UtcNow.ToString("yyyyMMddTHHmmssZ")
New-Item -ItemType Directory -Force -Path $BackupRoot | Out-Null

$existingTask = Get-ScheduledTask -TaskName "CodexAuthSync" -ErrorAction SilentlyContinue
if ($null -ne $existingTask) {
  Stop-ScheduledTask -TaskName "CodexAuthSync" -ErrorAction SilentlyContinue
}

if ((Test-Path $ConfigPath) -or (Test-Path $StatePath)) {
  $destination = Join-Path $BackupRoot "sync-config-$timestamp"
  New-Item -ItemType Directory -Force -Path $destination | Out-Null
  if (Test-Path $ConfigPath) { Copy-Item -Path $ConfigPath -Destination (Join-Path $destination "config.json") }
  if (Test-Path $StatePath) { Copy-Item -Path $StatePath -Destination (Join-Path $destination "state.json") }
  Write-Output "Previous synchronization configuration backed up to $destination"
}

if ($Role -eq "join") {
  $activeAuth = Join-Path $CodexHome "auth.json"
  $accountsDirectory = Join-Path $CodexHome "accounts"
  if ((Test-Path $activeAuth) -or (Test-Path $accountsDirectory)) {
    $destination = Join-Path $BackupRoot "codex-auth-$timestamp"
    New-Item -ItemType Directory -Force -Path $destination | Out-Null
    if (Test-Path $activeAuth) { Copy-Item -Path $activeAuth -Destination (Join-Path $destination "auth.json") }
    if (Test-Path $accountsDirectory) { Copy-Item -Recurse -Path $accountsDirectory -Destination (Join-Path $destination "accounts") }
    Remove-Item -Force $activeAuth -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force $accountsDirectory -ErrorAction SilentlyContinue
    Write-Output "Local codex-auth data backed up to $destination"
  }
}
Remove-Item -Force $StatePath -ErrorAction SilentlyContinue

$oldPassword = $env:CODEX_AUTH_SYNC_WEBDAV_PASSWORD
$oldKey = $env:CODEX_AUTH_SYNC_KEY
try {
  $env:CODEX_AUTH_SYNC_WEBDAV_PASSWORD = $WebDAVPassword
  $env:CODEX_AUTH_SYNC_KEY = $SyncKey
  & $SyncBinary init --force --backend webdav --server $Server --webdav-username $Username --cluster $Cluster --active-scope $ActiveScope --active-group $ActiveGroup --interval $Interval
  if ($LASTEXITCODE -ne 0) { throw "codex-auth-sync initialization failed." }
} finally {
  $env:CODEX_AUTH_SYNC_WEBDAV_PASSWORD = $oldPassword
  $env:CODEX_AUTH_SYNC_KEY = $oldKey
}

& $SyncBinary sync
if ($LASTEXITCODE -ne 0) { throw "The first synchronization failed." }
& $SyncBinary status
if ($LASTEXITCODE -ne 0) { throw "Synchronization status validation failed." }

if (-not $NoService) {
  $serviceScript = Invoke-RestMethod -Uri "https://raw.githubusercontent.com/$Repository/main/scripts/install-service.ps1"
  & ([scriptblock]::Create($serviceScript)) -BinaryPath $SyncBinary -ConfigPath $ConfigPath
} else {
  Write-Output "Background task installation skipped."
  Write-Output "Run 'codex-auth-sync now' whenever an immediate synchronization is needed."
}

Write-Output "Bootstrap complete."
Write-Output "Role: $Role"
Write-Output "Cluster: $Cluster"
if (-not $NoService) {
  Write-Output "The synchronization task starts at Windows sign-in."
}
Write-Output "Run 'codex-auth list' to view synchronized accounts."
Write-Output "Run 'codex-auth-sync now' to synchronize immediately."
Write-Output "Run 'codex-auth-sync status' to verify synchronization health."
