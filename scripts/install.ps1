param(
  [string]$Repository = "morp-ai/codex-auth-sync",
  [string]$Version = "latest",
  [string]$InstallDirectory = "$env:LOCALAPPDATA\Programs\codex-auth-sync"
)

$ErrorActionPreference = "Stop"

$architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
switch ($architecture) {
  "x64" { $arch = "amd64" }
  "arm64" { $arch = "arm64" }
  default { throw "Unsupported CPU architecture: $architecture" }
}

$asset = "codex-auth-sync-windows-$arch.exe"
if ($Version -eq "latest") {
  $baseUrl = "https://github.com/$Repository/releases/latest/download"
} else {
  $baseUrl = "https://github.com/$Repository/releases/download/$Version"
}

$temporaryDirectory = Join-Path ([System.IO.Path]::GetTempPath()) ("codex-auth-sync-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $temporaryDirectory | Out-Null
try {
  $binaryPath = Join-Path $temporaryDirectory $asset
  $checksumPath = Join-Path $temporaryDirectory "checksums.txt"
  Write-Output "Downloading $asset"
  Invoke-WebRequest -Uri "$baseUrl/$asset" -OutFile $binaryPath
  Invoke-WebRequest -Uri "$baseUrl/checksums.txt" -OutFile $checksumPath

  $checksumLine = Get-Content $checksumPath | Where-Object { $_ -match "\s+$([regex]::Escape($asset))$" } | Select-Object -First 1
  if ([string]::IsNullOrWhiteSpace($checksumLine)) {
    throw "The release checksum file does not contain $asset."
  }
  $expected = ($checksumLine -split "\s+")[0].ToLowerInvariant()
  $actual = (Get-FileHash -Algorithm SHA256 $binaryPath).Hash.ToLowerInvariant()
  if ($actual -ne $expected) {
    throw "Checksum verification failed for $asset."
  }

  New-Item -ItemType Directory -Force -Path $InstallDirectory | Out-Null
  $destination = Join-Path $InstallDirectory "codex-auth-sync.exe"
  Copy-Item -Force $binaryPath $destination

  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  $pathEntries = @($userPath -split ";" | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
  if ($pathEntries -notcontains $InstallDirectory) {
    $newPath = (($pathEntries + $InstallDirectory) -join ";")
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    Write-Output "Added $InstallDirectory to the user PATH. Open a new terminal to use it."
  }
  if (($env:Path -split ";") -notcontains $InstallDirectory) {
    $env:Path = "$InstallDirectory;$env:Path"
  }
  Write-Output "Installed codex-auth-sync to $destination"
} finally {
  Remove-Item -Recurse -Force $temporaryDirectory -ErrorAction SilentlyContinue
}
