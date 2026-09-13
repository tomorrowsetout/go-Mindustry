# mdt-server 编译脚本（Windows PowerShell）
# 用法：
#   .\scripts\build.ps1
#   .\scripts\build.ps1 -Test
#   .\scripts\build.ps1 -Race
#   .\scripts\build.ps1 -Cgo          # 启用 cgo（需本机 C++ 工具链）
#   .\scripts\build.ps1 -Cross linux
#   .\scripts\build.ps1 -OutDir dist

param(
    [switch]$Test,
    [switch]$Race,
    [switch]$Cgo,
    [switch]$Clean,
    [string]$OutDir = "bin",
    [string]$Name = "mdt-server",
    [string]$Cross = "",
    [string]$Version = "dev",
    [string]$Commit = ""
)

$ErrorActionPreference = "Stop"
try {
    [Console]::OutputEncoding = [System.Text.Encoding]::UTF8
    $OutputEncoding = [System.Text.Encoding]::UTF8
} catch {}
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

if (-not $Commit) {
    try { $Commit = (git rev-parse --short HEAD 2>$null) } catch { $Commit = "none" }
    if (-not $Commit) { $Commit = "none" }
}

Write-Host "== mdt-server build ==" -ForegroundColor Cyan
Write-Host "root:     $root"
Write-Host "version:  $Version  commit: $Commit"
Write-Host "out:      $OutDir"
Write-Host "cgo:      $Cgo   race: $Race   cross: $(if ($Cross) { $Cross } else { '-' })"

if ($Clean) {
    Write-Host "Cleaning $OutDir ..." -ForegroundColor Yellow
    if (Test-Path $OutDir) {
        Get-ChildItem $OutDir -Filter "$Name*" -ErrorAction SilentlyContinue | Remove-Item -Force -ErrorAction SilentlyContinue
    }
}

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

# CGO：本机无 gcc 时保持关闭（纯 Go nativespatial 回退）
$env:CGO_ENABLED = if ($Cgo) { "1" } else { "0" }

$ldflags = @(
    "-X", "mdt-server/internal/buildinfo.Version=$Version",
    "-X", "mdt-server/internal/buildinfo.Commit=$Commit"
) -join " "

$goflags = @("-trimpath", "-ldflags", $ldflags)
if ($Race) { $goflags = @("-race") + $goflags }

$outExe = Join-Path $OutDir "$Name.exe"
$pkg = "./cmd/mdt-server"

if ($Cross -eq "linux") {
    $env:GOOS = "linux"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
    $outExe = Join-Path $OutDir "$Name-linux64"
    Write-Host "Cross-compile: linux/amd64" -ForegroundColor Yellow
} elseif ($Cross -eq "macos") {
    $env:GOOS = "darwin"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
    $outExe = Join-Path $OutDir "$Name-macos64"
    Write-Host "Cross-compile: darwin/amd64" -ForegroundColor Yellow
} elseif ($Cross) {
    throw "Unsupported -Cross value: $Cross (use linux | macos, or empty for host)"
} else {
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
}

Write-Host "go build $($goflags -join ' ') -o $outExe $pkg" -ForegroundColor DarkGray
& go build @goflags -o $outExe $pkg
if ($LASTEXITCODE -ne 0) { throw "go build failed ($LASTEXITCODE)" }

$size = [math]::Round((Get-Item $outExe).Length / 1MB, 2)
Write-Host "OK  $outExe  (${size} MB)" -ForegroundColor Green

if ($Test -and -not $Cross) {
    Write-Host "Running focused tests..." -ForegroundColor Cyan
    & go test -count=1 ./cmd/mdt-server/ ./internal/world/ ./internal/net/ ./internal/sim/ ./internal/nativespatial/
    if ($LASTEXITCODE -ne 0) { throw "tests failed ($LASTEXITCODE)" }
    Write-Host "Tests OK" -ForegroundColor Green
}

# 若从仓库根启动，提示可执行文件路径
Write-Host ""
Write-Host "启动示例:" -ForegroundColor Cyan
Write-Host "  .\$outExe"
Write-Host "  .\$outExe -config configs/config.toml -world assets/worlds/22908.msav"
