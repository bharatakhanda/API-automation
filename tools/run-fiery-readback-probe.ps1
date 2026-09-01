param(
    [Parameter(Mandatory=$true)] [string] $Server,
    [Parameter(Mandatory=$true)] [string] $JobId,
    [string] $OutDir = "$env:USERPROFILE\Downloads\captures",
    [string] $Api = "v5",
    [int] $Repeat = 1,
    [string] $Interval = "5s",
    [string] $SecretsFile = ".\.local\secrets.json",
    [switch] $UsePostmanCookie
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Split-Path -Parent $ScriptDir

if (!(Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Host "Go is not installed or not in PATH. Install Go from https://go.dev/dl/ and retry." -ForegroundColor Red
    exit 1
}

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

Push-Location $RepoRoot
try {
    if ($UsePostmanCookie) {
        $cookie = Read-Host "Paste Postman Cookie header, for example _session_id=..."
        $env:FIERY_COOKIE = $cookie
        go run .\cmd\fiery-readback-probe -server $Server -job $JobId -api $Api -out-dir $OutDir -repeat $Repeat -interval $Interval
    } else {
        if (!(Test-Path $SecretsFile) -and [string]::IsNullOrWhiteSpace($env:FIERY_SECRET)) {
            Write-Host "Secret key not found." -ForegroundColor Yellow
            Write-Host "Create .local\secrets.json in the repository with one of these shapes:" -ForegroundColor Yellow
            Write-Host '{ "secretKey": "PASTE_ACCESSRIGHTS_KEY_HERE" }'
            Write-Host 'or set $env:FIERY_SECRET before running.'
            exit 1
        }
        $secure = Read-Host "Enter Fiery admin password" -AsSecureString
        $bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
        try {
            $env:FIERY_PASSWORD = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr)
            go run .\cmd\fiery-readback-probe -server $Server -job $JobId -api $Api -out-dir $OutDir -repeat $Repeat -interval $Interval -secrets-file $SecretsFile
        } finally {
            [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
        }
    }
} finally {
    Pop-Location
    Remove-Item Env:\FIERY_COOKIE -ErrorAction SilentlyContinue
    Remove-Item Env:\FIERY_PASSWORD -ErrorAction SilentlyContinue
}
