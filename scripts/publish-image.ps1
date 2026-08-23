[CmdletBinding(SupportsShouldProcess = $true, ConfirmImpact = "Medium")]
param(
    [Parameter(Mandatory = $true)]
    [ValidateNotNullOrEmpty()]
    [string]$Image,

    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string[]]$Tag = @("local"),

    [Parameter()]
    [ValidateSet("linux/amd64")]
    [string]$Platform = "linux/amd64",

    [Parameter()]
    [string]$SourceUrl = "",

    [Parameter()]
    [switch]$Push,

    [Parameter()]
    [switch]$NoCache
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "Docker was not found in PATH."
}

if ($Image.Contains("://") -or $Image -match "\s" -or $Image.EndsWith("/")) {
    throw "Image must be a registry/repository name without protocol or trailing slash."
}

if ($Image -cne $Image.ToLowerInvariant()) {
    throw "Container registry/repository names must be lowercase."
}

$lastSlash = $Image.LastIndexOf("/")
$lastColon = $Image.LastIndexOf(":")
if ($lastColon -gt $lastSlash) {
    throw "Pass image tags via -Tag, not as part of -Image. Registry ports such as registry.example:5000/team/image are supported."
}

foreach ($currentTag in $Tag) {
    if ($currentTag -notmatch "^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$") {
        throw "Invalid image tag: $currentTag"
    }
}
if (@($Tag | Select-Object -Unique).Count -ne $Tag.Count) {
    throw "Image tags must be unique."
}

$releaseTags = @($Tag | Where-Object { $_ -match "^[0-9]+\.[0-9]+\.[0-9]+$" })
if ($releaseTags.Count -gt 1) {
    throw "Only one SemVer tag can describe an image build."
}
if ($Push -and (@($Tag | Where-Object { $_ -eq "latest" }).Count -gt 0 -or $releaseTags.Count -gt 0)) {
    throw "Publish latest and immutable SemVer tags through the GitHub release workflow. This script only pushes non-release tags."
}
$imageVersion = if ($releaseTags.Count -eq 1) { $releaseTags[0] } else { $Tag[0] }

& docker buildx version | Out-Null
if ($LASTEXITCODE -ne 0) {
    throw "Docker Buildx is not available."
}

$revision = (& git -C $repoRoot rev-parse --short=12 HEAD 2>$null)
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($revision)) {
    $revision = "unknown"
}
$dirtyPaths = @(& git -C $repoRoot status --porcelain 2>$null)
if ($LASTEXITCODE -eq 0 -and $dirtyPaths.Count -gt 0) {
    if ($Push) {
        throw "Refusing to publish from a dirty worktree. Commit or stash the changes first."
    }
    $revision = "${revision}-dirty"
}

$created = [DateTime]::UtcNow.ToString("yyyy-MM-ddTHH:mm:ssZ")
$sourceArgument = @()
if ([string]::IsNullOrWhiteSpace($SourceUrl)) {
    $SourceUrl = (& git -C $repoRoot remote get-url origin 2>$null)
    if ($LASTEXITCODE -eq 0 -and $SourceUrl -match '^git@github\.com:(.+)\.git$') {
        $SourceUrl = "https://github.com/$($Matches[1])"
    } elseif ($LASTEXITCODE -eq 0) {
        $SourceUrl = $SourceUrl -replace '\.git$', ''
    } else {
        $SourceUrl = ""
    }
}
if (-not [string]::IsNullOrWhiteSpace($SourceUrl)) {
    $sourceArgument = @("--build-arg", "SOURCE_URL=$SourceUrl")
}

$buildArguments = @(
    "buildx", "build",
    "--platform", $Platform,
    "--file", "docker/Dockerfile.registry",
    "--build-arg", "BUILD_DATE=$created",
    "--build-arg", "VERSION=$imageVersion",
    "--build-arg", "VCS_REF=$revision"
)
$buildArguments += $sourceArgument

foreach ($currentTag in $Tag) {
    $buildArguments += @("--tag", "${Image}:$currentTag")
}

if ($NoCache) {
    $buildArguments += "--no-cache"
}

if ($Push) {
    $buildArguments += "--push"
} else {
    # --load keeps the image local. It intentionally never publishes by default.
    $buildArguments += "--load"
}

$buildArguments += "."
$references = $Tag | ForEach-Object { "${Image}:$_" }
$action = if ($Push) { "Build and push" } else { "Build locally" }
$target = $references -join ", "

if (-not $PSCmdlet.ShouldProcess($target, $action)) {
    return
}

Push-Location $repoRoot
try {
    & docker @buildArguments
    if ($LASTEXITCODE -ne 0) {
        throw "Docker buildx failed with exit code $LASTEXITCODE"
    }
} finally {
    Pop-Location
}

[pscustomobject]@{
    Images = $references
    Platform = $Platform
    Published = [bool]$Push
    Revision = $revision
}
