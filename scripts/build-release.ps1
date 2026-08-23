[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$buildRoot = Join-Path $repoRoot "build"
$stageRoot = Join-Path $buildRoot "release-staging-current"
$packageRoot = Join-Path $stageRoot "factorio-server-manager"
$targetZip = Join-Path $buildRoot "factorio-server-manager-linux.zip"

New-Item -ItemType Directory -Path $buildRoot -Force | Out-Null

function Assert-BuildChild([string]$Path) {
    $fullPath = [IO.Path]::GetFullPath($Path)
    $expectedPrefix = [IO.Path]::GetFullPath($buildRoot) + [IO.Path]::DirectorySeparatorChar
    if (-not $fullPath.StartsWith($expectedPrefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to modify a path outside the build directory: $fullPath"
    }
}

Assert-BuildChild $stageRoot
Assert-BuildChild $targetZip
if (Test-Path -LiteralPath $stageRoot) {
    Remove-Item -LiteralPath $stageRoot -Recurse -Force
}

New-Item -ItemType Directory -Path (Join-Path $packageRoot "app") -Force | Out-Null

$packageMetadata = Get-Content -LiteralPath (Join-Path $repoRoot "package.json") -Raw | ConvertFrom-Json
$uiVersion = [string]$packageMetadata.version
$uiRevision = (& git -C $repoRoot rev-parse HEAD 2>$null)
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($uiRevision)) {
    $uiRevision = "unknown"
}

function Set-ZipUnixCreatorSystem([string]$Path) {
    # ZipArchive writes the POSIX mode bits below, but marks central-directory
    # entries as DOS-originated. Unix extractors ignore the executable bit in
    # that combination, so mark each entry as Unix after the archive is closed.
    $stream = [IO.File]::Open($Path, [IO.FileMode]::Open, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
    $reader = [IO.BinaryReader]::new($stream, [Text.Encoding]::UTF8, $true)
    try {
        $tailLength = [Math]::Min([int64]65557, $stream.Length)
        $stream.Position = $stream.Length - $tailLength
        $tail = $reader.ReadBytes([int]$tailLength)
        $eocdIndex = -1
        for ($index = $tail.Length - 22; $index -ge 0; $index--) {
            if ($tail[$index] -eq 0x50 -and $tail[$index + 1] -eq 0x4b -and
                $tail[$index + 2] -eq 0x05 -and $tail[$index + 3] -eq 0x06) {
                $eocdIndex = $index
                break
            }
        }
        if ($eocdIndex -lt 0) {
            throw "Release archive has no ZIP end-of-central-directory record."
        }

        $centralSize = [BitConverter]::ToUInt32($tail, $eocdIndex + 12)
        $centralOffset = [BitConverter]::ToUInt32($tail, $eocdIndex + 16)
        if ($centralSize -eq [uint32]::MaxValue -or $centralOffset -eq [uint32]::MaxValue) {
            throw "ZIP64 release archives are not supported by the mode-bit finalizer."
        }

        $centralEnd = [int64]$centralOffset + $centralSize
        $entryOffset = [int64]$centralOffset
        while ($entryOffset -lt $centralEnd) {
            $stream.Position = $entryOffset
            if ($reader.ReadUInt32() -ne 0x02014b50) {
                throw "Release archive central directory is malformed at offset $entryOffset."
            }
            $stream.Position = $entryOffset + 5
            $stream.WriteByte(3)
            $stream.Position = $entryOffset + 28
            $nameLength = $reader.ReadUInt16()
            $extraLength = $reader.ReadUInt16()
            $commentLength = $reader.ReadUInt16()
            $entryOffset += 46 + $nameLength + $extraLength + $commentLength
        }
        if ($entryOffset -ne $centralEnd) {
            throw "Release archive central-directory length is inconsistent."
        }
    } finally {
        $reader.Dispose()
        $stream.Dispose()
    }
}

$previousUiVersion = $env:FSM_UI_VERSION
$previousUiRevision = $env:FSM_UI_REVISION
try {
    if ([string]::IsNullOrWhiteSpace($env:FSM_UI_VERSION)) {
        $env:FSM_UI_VERSION = $uiVersion
    }
    if ([string]::IsNullOrWhiteSpace($env:FSM_UI_REVISION)) {
        $env:FSM_UI_REVISION = $uiRevision
    }
    Push-Location $repoRoot
    try {
        & npm.cmd ci
        if ($LASTEXITCODE -ne 0) {
            throw "Frontend dependency installation failed with exit code $LASTEXITCODE"
        }
        & npm.cmd run build
        if ($LASTEXITCODE -ne 0) {
            throw "Frontend build failed with exit code $LASTEXITCODE"
        }
    } finally {
        Pop-Location
    }
} finally {
    $env:FSM_UI_VERSION = $previousUiVersion
    $env:FSM_UI_REVISION = $previousUiRevision
}

$previousCgo = $env:CGO_ENABLED
$previousGoos = $env:GOOS
$previousGoarch = $env:GOARCH
try {
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    Push-Location (Join-Path $repoRoot "src")
    try {
        & go build -trimpath -ldflags "-s -w" -o (Join-Path $packageRoot "factorio-server-manager") .
        if ($LASTEXITCODE -ne 0) {
            throw "Linux backend build failed with exit code $LASTEXITCODE"
        }
    } finally {
        Pop-Location
    }
} finally {
    $env:CGO_ENABLED = $previousCgo
    $env:GOOS = $previousGoos
    $env:GOARCH = $previousGoarch
}

$appDestination = Join-Path $packageRoot "app"
$releaseAssets = @(
    "apple-touch-icon.png",
    "bundle.js",
    "bundle.js.LICENSE.txt",
    "favicon-32x32.png",
    "favicon.ico",
    "index.html",
    "style.css",
    "style.js"
)
foreach ($asset in $releaseAssets) {
    $source = Join-Path $repoRoot "app\$asset"
    if (-not (Test-Path -LiteralPath $source -PathType Leaf)) {
        throw "Frontend build did not produce required release asset: $asset"
    }
}
Get-ChildItem -LiteralPath (Join-Path $repoRoot "app") -Force |
    Copy-Item -Destination $appDestination -Recurse -Force
Copy-Item -LiteralPath (Join-Path $repoRoot "conf.json.example") -Destination (Join-Path $packageRoot "conf.json")
foreach ($document in @("AI-DISCLOSURE.md", "CHANGELOG.md", "LICENSE", "README.md")) {
    Copy-Item -LiteralPath (Join-Path $repoRoot $document) -Destination $packageRoot
}

if (Test-Path -LiteralPath $targetZip) {
    Remove-Item -LiteralPath $targetZip -Force
}

Add-Type -AssemblyName System.IO.Compression
$fileStream = [IO.File]::Open($targetZip, [IO.FileMode]::CreateNew)
$archive = [IO.Compression.ZipArchive]::new($fileStream, [IO.Compression.ZipArchiveMode]::Create, $false)
try {
    foreach ($file in Get-ChildItem -LiteralPath $stageRoot -Recurse -File) {
        $relativePath = [IO.Path]::GetRelativePath($stageRoot, $file.FullName).Replace("\", "/")
        $entry = $archive.CreateEntry($relativePath, [IO.Compression.CompressionLevel]::Optimal)
        $entry.LastWriteTime = $file.LastWriteTime

        # Unix file type plus 0755 for the executable, 0644 for data files.
        $unixMode = if ($file.Name -eq "factorio-server-manager") { 33261 } else { 33188 }
        $entry.ExternalAttributes = [BitConverter]::ToInt32(
            [BitConverter]::GetBytes([uint32]($unixMode * 65536)),
            0
        )

        $input = [IO.File]::OpenRead($file.FullName)
        $output = $entry.Open()
        try {
            $input.CopyTo($output)
        } finally {
            $output.Dispose()
            $input.Dispose()
        }
    }
} finally {
    $archive.Dispose()
    $fileStream.Dispose()
}
Set-ZipUnixCreatorSystem $targetZip

Remove-Item -LiteralPath $stageRoot -Recurse -Force

$hash = Get-FileHash -Algorithm SHA256 -LiteralPath $targetZip
[pscustomobject]@{
    Archive = $targetZip
    Bytes = (Get-Item -LiteralPath $targetZip).Length
    SHA256 = $hash.Hash.ToLowerInvariant()
}
