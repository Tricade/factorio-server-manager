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
foreach ($asset in @("bundle.js", "bundle.js.LICENSE.txt", "index.html", "style.css", "style.js")) {
    Copy-Item -LiteralPath (Join-Path $repoRoot "app\$asset") -Destination $appDestination
}
Copy-Item -LiteralPath (Join-Path $repoRoot "conf.json.example") -Destination (Join-Path $packageRoot "conf.json")

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

Remove-Item -LiteralPath $stageRoot -Recurse -Force

$hash = Get-FileHash -Algorithm SHA256 -LiteralPath $targetZip
[pscustomobject]@{
    Archive = $targetZip
    Bytes = (Get-Item -LiteralPath $targetZip).Length
    SHA256 = $hash.Hash.ToLowerInvariant()
}
