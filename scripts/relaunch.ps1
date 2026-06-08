# #########################################################
# Relaunch the NAVI daemon and CLI (Windows PowerShell version)
# 
# This script will build the navi binaries and start the docker container.
# It will then smoke test the container and print the health status.
# #########################################################

$ErrorActionPreference = "Stop"

# Clear screen
Clear-Host

# Get project root directory
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$NaviDir = Split-Path -Parent $ScriptDir
Set-Location $NaviDir

# Build the navi binaries
Write-Host "Building navi binaries..." -ForegroundColor Cyan
go build -o bin/navid ./cmd/navid
go build -o bin/navi.exe ./cmd/navi

# Ensure Docker is available
Write-Host "Checking Docker status..." -ForegroundColor Cyan
$dockerError = ""
try {
    $null = docker version 2>&1
} catch {
    $dockerError = $_.Exception.Message
}
if ($LASTEXITCODE -ne 0 -or $dockerError -ne "") {
    Write-Error "Error: Docker is not available. Please start Docker Desktop and ensure it is running."
    exit 1
}

# Initialize ~/.navi directory with defaults if empty
Write-Host "Initializing ~/.navi directories..." -ForegroundColor Cyan
$naviHome = Join-Path $env:USERPROFILE ".navi"
$null = New-Item -ItemType Directory -Force -Path (Join-Path $naviHome "data")
$null = New-Item -ItemType Directory -Force -Path (Join-Path $naviHome "config")
$null = New-Item -ItemType Directory -Force -Path (Join-Path $naviHome "skills")

if (Test-Path "config") {
    if (!(Test-Path (Join-Path $naviHome "config/config.yaml")) -and !(Test-Path (Join-Path $naviHome "config/runtime.yaml"))) {
        Write-Host "Copying default configs to ~/.navi/config..."
        Copy-Item -Path "config/*" -Destination (Join-Path $naviHome "config") -Recurse -Force -ErrorAction SilentlyContinue
    }
}
if (Test-Path "skills") {
    if ((Get-ChildItem -Path (Join-Path $naviHome "skills") | Measure-Object).Count -eq 0) {
        Write-Host "Copying default skills to ~/.navi/skills..."
        Copy-Item -Path "skills/*" -Destination (Join-Path $naviHome "skills") -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# Relaunch through Compose
Write-Host "Restarting containers via Docker Compose..." -ForegroundColor Cyan
docker compose down --remove-orphans
docker compose up --build -d

# Wait for container and app to start up (up to 10 seconds)
Write-Host "Waiting for navid to become healthy on http://localhost:6284/health..." -ForegroundColor Cyan
$healthy = $false
for ($i = 1; $i -le 10; $i++) {
    try {
        $response = Invoke-WebRequest -Uri "http://localhost:6284/health" -UseBasicParsing -TimeoutSec 2 -ErrorAction SilentlyContinue
        if ($response.StatusCode -eq 200) {
            $healthy = $true
            break
        }
    } catch {}
    Start-Sleep -Seconds 1
}

if (!$healthy) {
    Write-Error "Error: navid did not become healthy on http://localhost:6284/health."
    exit 1
}

Write-Host "navid is healthy on http://localhost:6284/health" -ForegroundColor Green
# #########################################################
# End of script
# #########################################################
