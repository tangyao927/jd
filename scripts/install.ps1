[CmdletBinding()]
param(
    [string]$Bind = 'jd',
    [string]$Binary,
    [string]$Version = 'latest',
    [switch]$Source,
    [switch]$DryRun,
    [switch]$Uninstall,
    [switch]$Purge,
    [switch]$Yes
)

$ErrorActionPreference = 'Stop'
$StartMarker = '# >>> jd initialize >>>'
$EndMarker = '# <<< jd initialize <<<'
if ($Bind -notmatch '^[A-Za-z_][A-Za-z0-9_]*$') { throw "Invalid binding: $Bind" }
if ($Version -ne 'latest' -and $Version -notmatch '^v[0-9]+\.[0-9]+\.[0-9]+$') { throw "Invalid version: $Version; use latest or vX.Y.Z" }
if ($Source -and $Binary) { throw '-Source and -Binary cannot be used together' }
if ($PSBoundParameters.ContainsKey('Version') -and ($Source -or $Binary)) { throw '-Version cannot be combined with -Source or -Binary' }

$ProjectRoot = $null
if ($PSScriptRoot) {
    $CandidateRoot = Split-Path -Parent $PSScriptRoot
    $ModulePath = Join-Path $CandidateRoot 'go.mod'
    $MainPath = Join-Path $CandidateRoot 'cmd\jd\main.go'
    if ((Test-Path -PathType Leaf $ModulePath) -and (Test-Path -PathType Leaf $MainPath)) {
        $ModuleLine = Get-Content -LiteralPath $ModulePath | Select-Object -First 1
        if ($ModuleLine -eq 'module github.com/tangyao927/jd') { $ProjectRoot = $CandidateRoot }
    }
}
$UseSource = [bool]$Source
if (-not $PSBoundParameters.ContainsKey('Version') -and -not $Binary -and $ProjectRoot) { $UseSource = $true }
if ($Source -and -not $ProjectRoot) { throw '-Source requires scripts/install.ps1 from a jd source checkout' }

$BinDir = if ($env:JD_INSTALL_BIN_DIR) { $env:JD_INSTALL_BIN_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\jd' }
$Destination = Join-Path $BinDir 'jd.exe'
$ProfilePath = if ($env:JD_INSTALL_PROFILE) { $env:JD_INSTALL_PROFILE } else { $PROFILE.CurrentUserCurrentHost }
$DataDir = if ($env:JD_DATA_DIR) { $env:JD_DATA_DIR } else { Join-Path $env:LOCALAPPDATA 'jd' }
$ConfigDir = if ($env:JD_CONFIG_DIR) { $env:JD_CONFIG_DIR } else { Join-Path $env:APPDATA 'jd' }

function Remove-ManagedBlock([string]$Text) {
    $Pattern = '(?ms)^' + [regex]::Escape($StartMarker) + '.*?^' + [regex]::Escape($EndMarker) + '\r?\n?'
    return [regex]::Replace($Text, $Pattern, '')
}

function Write-Utf8NoBom([string]$Path, [string]$Text) {
    [IO.File]::WriteAllText($Path, $Text, (New-Object Text.UTF8Encoding($false)))
}

function Get-Sha256([string]$Path) {
    $Stream = $null
    $Hasher = $null
    try {
        $Stream = [IO.File]::OpenRead($Path)
        $Hasher = [Security.Cryptography.SHA256]::Create()
        $Hash = $Hasher.ComputeHash($Stream)
        return ([BitConverter]::ToString($Hash)).Replace('-', '').ToLowerInvariant()
    } finally {
        if ($Hasher) { $Hasher.Dispose() }
        if ($Stream) { $Stream.Dispose() }
    }
}

function Get-ReleaseFile([string]$Uri, [string]$OutFile) {
    try {
        Invoke-WebRequest -UseBasicParsing -Uri $Uri -OutFile $OutFile
    } catch {
        throw "release download failed: $Uri`nFrom a source checkout, run .\scripts\install.ps1 -Source"
    }
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
        Write-Utf8NoBom $ProfilePath (Remove-ManagedBlock $Text)
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

$Existing = Get-Command $Bind -ErrorAction SilentlyContinue
$ExistingProfile = if (Test-Path $ProfilePath) { Get-Content -Raw $ProfilePath } else { '' }
$HadMarker = $ExistingProfile -like "*$StartMarker*"
if ($Existing -and $Existing.Source -ne $Destination -and -not $HadMarker) {
    throw "Binding $Bind already exists; choose -Bind NAME"
}
if ($DryRun) {
    $SourceDescription = if ($Binary) { $Binary } elseif ($UseSource) { 'the current source checkout' } else { "jd release $Version" }
    Write-Output "Would install $SourceDescription to $Destination and update $ProfilePath`n$Block"
    exit 0
}

$TemporaryDirectory = $null
try {
    if (-not $Binary) {
        $TemporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) ("jd-install-" + [guid]::NewGuid())
        New-Item -ItemType Directory -Force $TemporaryDirectory | Out-Null
        if ($UseSource) {
            $Binary = Join-Path $TemporaryDirectory 'jd.exe'
            Push-Location $ProjectRoot
            try { & go build -trimpath -o $Binary ./cmd/jd; if ($LASTEXITCODE -ne 0) { throw 'go build failed' } }
            finally { Pop-Location }
        } else {
            $RawArchitecture = try { [Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString() } catch { $env:PROCESSOR_ARCHITECTURE }
            $Architecture = switch ($RawArchitecture.ToUpperInvariant()) {
                'X64' { 'amd64' }
                'AMD64' { 'amd64' }
                'ARM64' { 'arm64' }
                default { throw "Unsupported architecture: $RawArchitecture" }
            }
            $ReleaseBase = if ($env:JD_RELEASE_BASE_URL) { $env:JD_RELEASE_BASE_URL.TrimEnd('/') } else { 'https://github.com/tangyao927/jd/releases' }
            $DownloadBase = if ($Version -eq 'latest') { "$ReleaseBase/latest/download" } else { "$ReleaseBase/download/$Version" }
            $Asset = "jd_windows_$Architecture.zip"
            $ArchivePath = Join-Path $TemporaryDirectory $Asset
            $ChecksumPath = Join-Path $TemporaryDirectory 'checksums.txt'
            Get-ReleaseFile "$DownloadBase/$Asset" $ArchivePath
            Get-ReleaseFile "$DownloadBase/checksums.txt" $ChecksumPath
            $Pattern = '^([A-Fa-f0-9]{64})\s+\*?' + [regex]::Escape($Asset) + '$'
            $ChecksumLine = Get-Content $ChecksumPath | Where-Object { $_ -match $Pattern } | Select-Object -First 1
            if (-not $ChecksumLine) { throw "Checksum missing for $Asset" }
            [void]($ChecksumLine -match $Pattern)
            $Expected = $Matches[1].ToLowerInvariant()
            $Actual = Get-Sha256 $ArchivePath
            if ($Actual -ne $Expected) { throw "Checksum mismatch for $Asset" }
            Expand-Archive -Path $ArchivePath -DestinationPath $TemporaryDirectory -Force
            $Binary = Join-Path $TemporaryDirectory 'jd.exe'
        }
    }
    if (-not (Test-Path -PathType Leaf $Binary)) { throw "Binary not found: $Binary" }

    New-Item -ItemType Directory -Force $BinDir | Out-Null
    Copy-Item -Force $Binary $Destination
    $ProfileDirectory = Split-Path -Parent $ProfilePath
    New-Item -ItemType Directory -Force $ProfileDirectory | Out-Null
    if ((Test-Path $ProfilePath) -and -not $HadMarker) {
        Copy-Item $ProfilePath ($ProfilePath + '.jd-backup.' + (Get-Date -Format 'yyyyMMddHHmmss'))
    }
    $CleanProfile = Remove-ManagedBlock $ExistingProfile
    if ($CleanProfile -and -not $CleanProfile.EndsWith("`n")) { $CleanProfile += "`n" }
    Write-Utf8NoBom $ProfilePath ($CleanProfile + $Block + "`n")

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
} finally {
    if ($TemporaryDirectory) { Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $TemporaryDirectory }
}

Write-Output "Installed jd at $Destination. Restart PowerShell or reload $ProfilePath."
