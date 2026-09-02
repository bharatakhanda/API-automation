param(
    [string]$OutputPath = ".\bin\api-automation-wails-preview.exe"
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    $expectedWails = "v3.0.0-beta.16"
    $actualWails = (& cmd.exe /d /c "wails3 version 2>&1" | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or $actualWails -ne $expectedWails) {
        throw "Expected Wails CLI $expectedWails, got '$actualWails'. Install it with: go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.16"
    }

    $secretPath = Join-Path $root ".local\secrets.json"
    if (-not (Test-Path $secretPath)) {
        throw "Ignored local secrets file is missing: $secretPath"
    }
    $secret = [string]((Get-Content -Raw $secretPath | ConvertFrom-Json).secretKey)
    if ([string]::IsNullOrWhiteSpace($secret)) {
        throw "secretKey is missing from the ignored local secrets file"
    }

    & wails3 generate bindings -b -names -d cmd/api-automation-wails/frontend/src/bindings ./cmd/api-automation-wails
    if ($LASTEXITCODE -ne 0) { throw "Wails binding generation failed" }

    Push-Location "cmd\api-automation-wails\frontend"
    try {
        & npm ci --ignore-scripts
        if ($LASTEXITCODE -ne 0) { throw "Frontend lockfile install failed" }
        & npm run build
        if ($LASTEXITCODE -ne 0) { throw "Frontend build failed" }
    } finally {
        Pop-Location
    }

    $needle = [Text.Encoding]::UTF8.GetBytes($secret)
    foreach ($asset in Get-ChildItem "cmd\api-automation-wails\frontend\bundle" -File -Recurse) {
        $assetBytes = [IO.File]::ReadAllBytes($asset.FullName)
        $assetContainsSecret = $false
        for ($i = 0; $i -le $assetBytes.Length - $needle.Length -and -not $assetContainsSecret; $i++) {
            if ($assetBytes[$i] -ne $needle[0]) { continue }
            $match = $true
            for ($j = 1; $j -lt $needle.Length; $j++) {
                if ($assetBytes[$i + $j] -ne $needle[$j]) { $match = $false; break }
            }
            if ($match) { $assetContainsSecret = $true }
        }
        if ($assetContainsSecret) { throw "A frontend asset contains the configured secret" }
    }

    $parent = Split-Path -Parent $OutputPath
    if ($parent) { New-Item -ItemType Directory -Force -Path $parent | Out-Null }
    $ldflags = "-s -w -H=windowsgui -X api-automation/internal/fiery.DefaultSecretKey=$secret"
    & go build -tags production -trimpath -ldflags $ldflags -o $OutputPath .\cmd\api-automation-wails
    if ($LASTEXITCODE -ne 0) { throw "Wails preview build failed" }

    $resolved = Resolve-Path $OutputPath
    $bytes = [IO.File]::ReadAllBytes($resolved)
    $embedded = $false
    for ($i = 0; $i -le $bytes.Length - $needle.Length -and -not $embedded; $i++) {
        if ($bytes[$i] -ne $needle[0]) { continue }
        $match = $true
        for ($j = 1; $j -lt $needle.Length; $j++) {
            if ($bytes[$i + $j] -ne $needle[$j]) { $match = $false; break }
        }
        if ($match) { $embedded = $true }
    }
    if (-not $embedded) { throw "Embedded secret byte verification failed" }

    $peOffset = [BitConverter]::ToInt32($bytes, 0x3c)
    $subsystem = [BitConverter]::ToUInt16($bytes, $peOffset + 24 + 68)
    if ($subsystem -ne 2) { throw "Expected Windows GUI subsystem 2, got $subsystem" }

    $item = Get-Item $resolved
    $hash = (Get-FileHash -Algorithm SHA256 $resolved).Hash
    Write-Output ("Wails preview build: PASS; version={0}; subsystem={1}; embedded-secret=yes; bytes={2}; timestamp={3:o}; sha256={4}" -f $expectedWails, $subsystem, $item.Length, $item.LastWriteTimeUtc, $hash)
} finally {
    $secret = $null
    $ldflags = $null
    Pop-Location
}
