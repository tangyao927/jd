[CmdletBinding()]
param(
    [string]$Bind = 'jd',
    [string]$Binary,
    [switch]$DryRun,
    [switch]$Uninstall,
    [switch]$Purge,
    [switch]$Yes
)

$ErrorActionPreference = 'Stop'
$StartMarker = '# >>> jd initialize >>>'
$EndMarker = '# <<< jd initialize <<<'
if ($Bind -notmatch '^[A-Za-z_][A-Za-z0-9_]*$') { throw "Invalid binding: $Bind" }

$ProjectRoot = Split-Path -Parent $PSScriptRoot
$BinDir = if ($env:JD_INSTALL_BIN_DIR) { $env:JD_INSTALL_BIN_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\jd' }
$Destination = Join-Path $BinDir 'jd.exe'
$ProfilePath = if ($env:JD_INSTALL_PROFILE) { $env:JD_INSTALL_PROFILE } else { $PROFILE.CurrentUserCurrentHost }
$DataDir = if ($env:JD_DATA_DIR) { $env:JD_DATA_DIR } else { Join-Path $env:LOCALAPPDATA 'jd' }
$ConfigDir = if ($env:JD_CONFIG_DIR) { $env:JD_CONFIG_DIR } else { Join-Path $env:APPDATA 'jd' }

function Remove-ManagedBlock([string]$Text) {
    $Pattern = '(?ms)^' + [regex]::Escape($StartMarker) + '.*?^' + [regex]::Escape($EndMarker) + '\r?\n?'
    return [regex]::Replace($Text, $Pattern, '')
}

$EscapedDestination = $Destination.Replace("'", "''")
$Block = @"
$StartMarker
`$global:__jd_install_bin = '$EscapedDestination'
Invoke-Expression (& `$global:__jd_install_bin init powershell --bind '$Bind' | Out-String)
Invoke-Expression (& `$global:__jd_install_bin completion powershell --bind '$Bind' | Out-String)
Remove-Variable __jd_install_bin -Scope Global -ErrorAction SilentlyContinue
$EndMarker
"@

if ($Uninstall) {
    if ($DryRun) { Write-Output "Would remove managed block from $ProfilePath and binary $Destination"; exit 0 }
    if (Test-Path $ProfilePath) {
        $Text = Get-Content -Raw $ProfilePath
        Set-Content -NoNewline -Path $ProfilePath -Value (Remove-ManagedBlock $Text)
    }
    Remove-Item -Force -ErrorAction SilentlyContinue $Destination
    if ($Purge) {
        if (-not $Yes) {
            $Answer = Read-Host "Delete jd data in $DataDir and $ConfigDir? [y/N]"
            if ($Answer -notmatch '^(y|yes)$') { Write-Output 'Data preserved.'; exit 0 }
        }
        foreach ($Target in @($DataDir, $ConfigDir)) {
            if ([string]::IsNullOrWhiteSpace($Target) -or $Target -eq [IO.Path]::GetPathRoot($Target) -or $Target -eq $HOME) {
                throw "Refusing unsafe purge target: $Target"
            }
            Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $Target
        }
    }
    Write-Output "Uninstalled jd; restart PowerShell or reload $ProfilePath."
    exit 0
}

$TemporaryDirectory = $null
if (-not $Binary) {
    $TemporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) ("jd-install-" + [guid]::NewGuid())
    New-Item -ItemType Directory -Force $TemporaryDirectory | Out-Null
    $Binary = Join-Path $TemporaryDirectory 'jd.exe'
    Push-Location $ProjectRoot
    try { & go build -trimpath -o $Binary ./cmd/jd; if ($LASTEXITCODE -ne 0) { throw 'go build failed' } }
    finally { Pop-Location }
}
if (-not (Test-Path $Binary)) { throw "Binary not found: $Binary" }

$Existing = Get-Command $Bind -ErrorAction SilentlyContinue
$ExistingProfile = if (Test-Path $ProfilePath) { Get-Content -Raw $ProfilePath } else { '' }
$HadMarker = $ExistingProfile -like "*$StartMarker*"
if ($Existing -and $Existing.Source -ne $Destination -and $ExistingProfile -notlike "*$StartMarker*") {
    throw "Binding $Bind already exists; choose -Bind NAME"
}
if ($DryRun) { Write-Output "Would install $Binary to $Destination and update $ProfilePath`n$Block"; exit 0 }

New-Item -ItemType Directory -Force $BinDir | Out-Null
Copy-Item -Force $Binary $Destination
$ProfileDirectory = Split-Path -Parent $ProfilePath
New-Item -ItemType Directory -Force $ProfileDirectory | Out-Null
if ((Test-Path $ProfilePath) -and $ExistingProfile -notlike "*$StartMarker*") {
    Copy-Item $ProfilePath ($ProfilePath + '.jd-backup.' + (Get-Date -Format 'yyyyMMddHHmmss'))
}
$CleanProfile = Remove-ManagedBlock $ExistingProfile
if ($CleanProfile -and -not $CleanProfile.EndsWith("`n")) { $CleanProfile += "`n" }
Set-Content -NoNewline -Path $ProfilePath -Value ($CleanProfile + $Block + "`n")

if (-not $HadMarker -and -not [Console]::IsInputRedirected) {
    $Candidates = @(
        (Join-Path $HOME 'Projects'),
        (Join-Path $HOME 'projects'),
        (Join-Path $HOME 'Developer'),
        (Join-Path $HOME 'dev'),
        (Join-Path $HOME 'work'),
        (Join-Path $HOME 'src'),
        (Join-Path $HOME 'source\repos')
    ) | Where-Object { Test-Path -PathType Container $_ }
    if ($Candidates.Count -gt 0) {
        Write-Output 'Detected project roots:'
        $Candidates | ForEach-Object { Write-Output "  $_" }
        $Answer = Read-Host 'Index all of them now? [Y/n]'
        if ($Answer -notmatch '^(n|no)$') {
            foreach ($Candidate in $Candidates) { & $Destination root add $Candidate }
        }
    }
}

if ($TemporaryDirectory) { Remove-Item -Recurse -Force $TemporaryDirectory }
Write-Output "Installed jd at $Destination. Restart PowerShell or reload $ProfilePath."
