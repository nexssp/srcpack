[CmdletBinding()]
param(
    [string]$Version,
    [string]$InstallDir,
    [switch]$NoPathUpdate
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

$repo = 'nexssp/srcpack'
$apiBase = 'https://api.github.com/repos/nexssp/srcpack'
$headers = @{ 'User-Agent' = 'srcpack-installer'; 'Accept' = 'application/vnd.github+json' }

if ([string]::IsNullOrWhiteSpace($Version)) { $Version = $env:SRCPACK_VERSION }
if ([string]::IsNullOrWhiteSpace($Version)) { $Version = 'latest' }
if ([string]::IsNullOrWhiteSpace($InstallDir)) { $InstallDir = $env:SRCPACK_INSTALL_DIR }
if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    $localAppData = [Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData)
    if ([string]::IsNullOrWhiteSpace($localAppData)) { throw 'Could not determine the current user LocalApplicationData directory.' }
    $InstallDir = Join-Path $localAppData 'Programs\srcpack'
}
if ($env:SRCPACK_NO_PATH_UPDATE -eq '1') { $NoPathUpdate = $true }

# Windows PowerShell 5.1 may otherwise negotiate an obsolete TLS version.
try { [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12 } catch { }

$nativeArch = $env:PROCESSOR_ARCHITEW6432
if ([string]::IsNullOrWhiteSpace($nativeArch)) { $nativeArch = $env:PROCESSOR_ARCHITECTURE }
if ([string]::IsNullOrWhiteSpace($nativeArch)) { throw 'Could not determine the native Windows architecture.' }
switch -Regex ($nativeArch.ToUpperInvariant()) {
    '^AMD64$|^X64$' { $goarch = 'amd64'; break }
    '^ARM64$' { throw 'The current srcpack release workflow does not publish a Windows ARM64 archive.' }
    default { throw "Unsupported Windows architecture '$nativeArch'; currently supported: amd64." }
}

if ($Version -ieq 'latest') {
    $releaseUri = "$apiBase/releases/latest"
} else {
    $requestedVersion = $Version.Trim()
    if ($requestedVersion -notmatch '^(v)?[A-Za-z0-9][A-Za-z0-9._-]*$') {
        throw "Invalid version '$Version'. Use a tag such as v0.2.0 or a version such as 0.2.0."
    }
    if ($requestedVersion.StartsWith('v', [StringComparison]::OrdinalIgnoreCase)) {
        $tag = "v$($requestedVersion.Substring(1))"
    } else {
        $tag = "v$requestedVersion"
    }
    $releaseUri = "$apiBase/releases/tags/$tag"
}

try {
    $release = Invoke-RestMethod -Method Get -Uri $releaseUri -Headers $headers -ErrorAction Stop
} catch {
    throw "Could not find or read srcpack release '$Version' at GitHub: $($_.Exception.Message)"
}

$tag = [string]$release.tag_name
if ($tag -notmatch '^v([A-Za-z0-9][A-Za-z0-9._-]*)$') { throw "Unexpected release tag '$tag'." }
$releaseVersion = $tag.Substring(1)
$archiveName = "srcpack_${releaseVersion}_windows_${goarch}.zip"
$checksumsName = "srcpack_${releaseVersion}_checksums.txt"
$assetNames = @($release.assets | ForEach-Object { [string]$_.name })
if ($assetNames -notcontains $archiveName) {
    throw "Release $tag does not publish $archiveName. Check the release assets and supported Windows architectures."
}
if ($assetNames -notcontains $checksumsName) { throw "Release $tag does not publish $checksumsName." }

$tempRoot = Join-Path ([IO.Path]::GetTempPath()) ("srcpack-install-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tempRoot -Force | Out-Null
try {
    $archivePath = Join-Path $tempRoot $archiveName
    $checksumsPath = Join-Path $tempRoot $checksumsName
    $unpackDir = Join-Path $tempRoot 'unpacked'
    $releaseBase = "https://github.com/$repo/releases/download/$tag"

    $webParams = @{
        Headers = @{ 'User-Agent' = 'srcpack-installer' }
        ErrorAction = 'Stop'
    }
    if ($PSVersionTable.PSVersion.Major -lt 6) { $webParams.UseBasicParsing = $true }
    Invoke-WebRequest @webParams -Uri "$releaseBase/$archiveName" -OutFile $archivePath | Out-Null
    Invoke-WebRequest @webParams -Uri "$releaseBase/$checksumsName" -OutFile $checksumsPath | Out-Null

    $pattern = '^([0-9a-fA-F]{64})\s+\*?' + [regex]::Escape($archiveName) + '$'
    $expectedHash = $null
    foreach ($line in [IO.File]::ReadAllLines($checksumsPath)) {
        if ($line -match $pattern) {
            $expectedHash = $Matches[1].ToLowerInvariant()
            break
        }
    }
    if ([string]::IsNullOrWhiteSpace($expectedHash)) { throw "$checksumsName has no valid SHA-256 entry for $archiveName." }
    $actualHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualHash -ne $expectedHash) { throw "SHA-256 verification failed for $archiveName." }

    Expand-Archive -LiteralPath $archivePath -DestinationPath $unpackDir -Force
    $sourceBinary = Join-Path $unpackDir 'srcpack.exe'
    if (-not (Test-Path -LiteralPath $sourceBinary -PathType Leaf)) { throw "The release archive does not contain srcpack.exe at its root." }

    $null = & $sourceBinary --help 2>&1
    if ($LASTEXITCODE -ne 0) { throw 'The downloaded srcpack.exe did not pass its --help smoke test.' }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $destination = Join-Path $InstallDir 'srcpack.exe'
    $stagedBinary = Join-Path $InstallDir 'srcpack.exe.new'
    Copy-Item -LiteralPath $sourceBinary -Destination $stagedBinary -Force
    Move-Item -LiteralPath $stagedBinary -Destination $destination -Force

    $pathUpdateSucceeded = $true
    if (-not $NoPathUpdate) {
        $normalizedInstallDir = [IO.Path]::GetFullPath($InstallDir).TrimEnd('\')
        $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        $pathEntries = @()
        if (-not [string]::IsNullOrWhiteSpace($userPath)) {
            $pathEntries = @($userPath -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
        }
        $alreadyInUserPath = $false
        foreach ($entry in $pathEntries) {
            if ([string]::Equals($entry.Trim().TrimEnd('\'), $normalizedInstallDir, [StringComparison]::OrdinalIgnoreCase)) {
                $alreadyInUserPath = $true
                break
            }
        }
        if (-not $alreadyInUserPath) {
            try {
                $newUserPath = (@($pathEntries) + $normalizedInstallDir) -join ';'
                [Environment]::SetEnvironmentVariable('Path', $newUserPath, 'User')
            } catch {
                $pathUpdateSucceeded = $false
                Write-Warning "Installed srcpack, but could not update the user PATH: $($_.Exception.Message)"
            }
        }
        if (($env:Path -split ';') -notcontains $normalizedInstallDir) { $env:Path = "$normalizedInstallDir;$env:Path" }
    }

    Write-Host "Installed srcpack $tag to $destination"
    if ($NoPathUpdate) {
        Write-Host "Run it with: `"$destination`" --help"
    } elseif ($pathUpdateSucceeded) {
        Write-Host 'The user PATH is configured. Existing terminal apps or their parent processes may need a restart (or sign-out) to pick up the change.'
        Write-Host 'Try: srcpack --help'
    } else {
        Write-Host "PATH update failed. To run srcpack now: & `"$destination`" --help"
        Write-Host "Add this directory to your user PATH manually: $normalizedInstallDir"
    }
} finally {
    if (Test-Path -LiteralPath $tempRoot) { Remove-Item -LiteralPath $tempRoot -Recurse -Force }
}
