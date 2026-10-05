param(
  [string]$Version = $env:SLIMEBOT_VERSION,
  [string]$Repo = $env:SLIMEBOT_REPO
)

$ErrorActionPreference = "Stop"

if (-not $Repo) {
  $Repo = "natsuz0ra/SlimeBot"
}

$sourceDir = if ($MyInvocation.MyCommand.Path) { Split-Path -Parent $MyInvocation.MyCommand.Path } else { (Get-Location).Path }
$installDir = if ($env:SLIMEBOT_INSTALL_DIR) { $env:SLIMEBOT_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "SlimeBot" }
$shimDir = if ($env:SLIMEBOT_BIN_DIR) { $env:SLIMEBOT_BIN_DIR } else { Join-Path $installDir "bin" }
$pathWasConfigured = $false

function Split-PathEntries {
  param([string]$Value)
  if ([string]::IsNullOrWhiteSpace($Value)) {
    return @()
  }
  return @($Value -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
}

function Normalize-PathEntry {
  param([string]$Value)
  return $Value.Trim().TrimEnd('\', '/')
}

function Test-PathEntriesContain {
  param(
    [string[]]$Entries,
    [string]$Target
  )
  $normalizedTarget = Normalize-PathEntry $Target
  foreach ($entry in $Entries) {
    if ([string]::Equals((Normalize-PathEntry $entry), $normalizedTarget, [System.StringComparison]::OrdinalIgnoreCase)) {
      return $true
    }
  }
  return $false
}

function Add-UserPathEntry {
  param([string]$Directory)

  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  $userEntries = @(Split-PathEntries $userPath)
  if (-not (Test-PathEntriesContain $userEntries $Directory)) {
    $userEntries += $Directory
    [Environment]::SetEnvironmentVariable("Path", ($userEntries -join ';'), "User")
    $script:pathWasConfigured = $true
    Write-Host "Added command shim directory to user PATH: $Directory"
  } else {
    Write-Host "Command shim directory is already on user PATH: $Directory"
  }

  $processEntries = @(Split-PathEntries $env:Path)
  if (-not (Test-PathEntriesContain $processEntries $Directory)) {
    $env:Path = (($processEntries + $Directory) -join ';')
  }
}

function Install-FromRelease {
  $goos = "windows"
  switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture) {
    "X64" { $goarch = "amd64" }
    "Arm64" { $goarch = "arm64" }
    default { throw "Unsupported architecture: $([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture)" }
  }

  $resolvedVersion = if ($Version) { $Version } else { "latest" }
  if ($resolvedVersion -eq "latest") {
    $latest = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
    $resolvedVersion = $latest.tag_name
  }
  if (-not $resolvedVersion) {
    throw "Unable to resolve SlimeBot release version."
  }

  $packageName = "slimebot-$resolvedVersion-$goos-$goarch"
  $archive = Join-Path ([System.IO.Path]::GetTempPath()) "$packageName.zip"
  $extractDir = Join-Path ([System.IO.Path]::GetTempPath()) "$packageName-install"
  $url = "https://github.com/$Repo/releases/download/$resolvedVersion/$packageName.zip"

  Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $extractDir
  New-Item -ItemType Directory -Force -Path $extractDir | Out-Null

  Write-Host "Downloading SlimeBot $resolvedVersion for $goos/$goarch..."
  Invoke-WebRequest -Uri $url -OutFile $archive
  Expand-Archive -Force -Path $archive -DestinationPath $extractDir
  & (Join-Path $extractDir "$packageName\install.ps1")
  if ($LASTEXITCODE) {
    exit $LASTEXITCODE
  }
  exit 0
}

if (-not (Test-Path (Join-Path $sourceDir "bin\slimebot.exe")) -or -not (Test-Path (Join-Path $sourceDir "cli\cli.cjs"))) {
  Install-FromRelease
}

New-Item -ItemType Directory -Force -Path $installDir, $shimDir | Out-Null

Remove-Item -Recurse -Force -ErrorAction SilentlyContinue (Join-Path $installDir "bin"), (Join-Path $installDir "cli")
Copy-Item -Recurse -Force (Join-Path $sourceDir "bin") $installDir
Copy-Item -Recurse -Force (Join-Path $sourceDir "cli") $installDir
if (Test-Path (Join-Path $sourceDir "docs")) {
  Copy-Item -Recurse -Force (Join-Path $sourceDir "docs") $installDir
}
Copy-Item -Force (Join-Path $sourceDir "README.md"), (Join-Path $sourceDir "README.zh-CN.md"), (Join-Path $sourceDir "LICENSE") $installDir
if (Test-Path (Join-Path $sourceDir "README.en.md")) {
  Copy-Item -Force (Join-Path $sourceDir "README.en.md") $installDir
}

$slimebotCmd = Join-Path $shimDir "slimebot.cmd"
$slimebotCliCmd = Join-Path $shimDir "slimebot-cli.cmd"
Set-Content -Path $slimebotCmd -Encoding ASCII -Value "@echo off`r`n`"$installDir\bin\slimebot.exe`" %*`r`n"
Set-Content -Path $slimebotCliCmd -Encoding ASCII -Value "@echo off`r`n`"$installDir\bin\slimebot.exe`" cli %*`r`n"
Add-UserPathEntry $shimDir

$homeDir = Join-Path $env:USERPROFILE ".slimebot"
New-Item -ItemType Directory -Force -Path $homeDir | Out-Null
$configPath = Join-Path $homeDir "config.cfg"
if (-not (Test-Path $configPath)) {
  Set-Content -Path $configPath -Encoding ASCII -Value @"
SERVER_PORT=6247
FRONTEND_PORT=7391
DB_PATH=~/.slimebot/storage/data.db
SKILLS_ROOT=~/.slimebot/skills
CHAT_UPLOAD_ROOT=~/.slimebot/storage/chat_uploads
WEB_SEARCH_API_KEY=YOUR_TAVILY_API_KEY
JWT_SECRET=CHANGE_ME_TO_A_RANDOM_SECRET
JWT_EXPIRE=21600
"@
}

Write-Host "SlimeBot installed to $installDir"
Write-Host "Command shims installed to $shimDir"
Write-Host "Run: slimebot"
if ($pathWasConfigured) {
  Write-Host "Open a new terminal to use slimebot directly."
} elseif (-not (Get-Command slimebot -ErrorAction SilentlyContinue)) {
  Write-Host "If your shell cannot find slimebot, add this directory to PATH: $shimDir"
}
Write-Host "Before starting the web service, set JWT_SECRET in $configPath"
