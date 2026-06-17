# build.ps1 - Windows 本地一键编译脚本
# 将前端（default + classic）和后端编译为单个可执行文件 new-api.exe
#
# 用法:
#   .\build.ps1            # 完整编译（前端 + 后端）
#   .\build.ps1 -SkipFrontend  # 仅编译后端（需已存在 web/default/dist 和 web/classic/dist）
#   .\build.ps1 -FrontendOnly  # 仅编译前端
#   .\build.ps1 -OutputDir C:\output  # 指定输出目录

param(
    [switch]$SkipFrontend,
    [switch]$FrontendOnly,
    [string]$OutputDir = "."
)

$ErrorActionPreference = "Stop"

# ── 颜色输出辅助 ──
function Write-Step($msg)  { Write-Host "`n>> $msg" -ForegroundColor Cyan }
function Write-OK($msg)    { Write-Host "   OK: $msg" -ForegroundColor Green }
function Write-Fail($msg)  { Write-Host "   FAIL: $msg" -ForegroundColor Red }

# ── 项目根目录 ──
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $Root

# ── 读取版本号 ──
$Version = ""
if (Test-Path "VERSION") {
    $raw = Get-Content "VERSION" -Raw
    if ($raw) { $Version = $raw.Trim() }
}
if ([string]::IsNullOrWhiteSpace($Version)) {
    # 尝试从 git tag 获取
    try {
        $Version = (git describe --tags --always 2>$null).Trim()
    } catch {}
}
if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = "v0.0.0-dev"
}
Write-Host "Version: $Version" -ForegroundColor Yellow

# ── 前置检查 ──
Write-Step "检查编译工具"

# Go
$goVersion = & go version 2>$null
if (-not $goVersion) {
    Write-Fail "未找到 Go，请先安装 Go 1.25+"
    exit 1
}
Write-OK "Go: $goVersion"

# Bun
$bunVersion = & bun.cmd --version 2>$null
if (-not $bunVersion) {
    Write-Fail "未找到 Bun，请先安装 Bun (https://bun.sh)"
    exit 1
}
Write-OK "Bun: v$bunVersion"

# ── 编译前端 ──
if (-not $SkipFrontend) {
    Write-Step "安装前端依赖 (bun install)"
    Push-Location "web"
    try {
        & bun.cmd install --frozen-lockfile
        if ($LASTEXITCODE -ne 0) {
            Write-Host "   frozen-lockfile 失败，尝试不加锁安装..." -ForegroundColor Yellow
            & bun.cmd install
        }
        if ($LASTEXITCODE -ne 0) {
            Write-Fail "前端依赖安装失败"
            exit 1
        }
        Write-OK "前端依赖安装完成"
    } finally {
        Pop-Location
    }

    # 构建 default 前端
    Write-Step "编译 default 前端"
    $env:DISABLE_ESLINT_PLUGIN = "true"
    $env:VITE_REACT_APP_VERSION = $Version
    Push-Location "web\default"
    try {
        & bun.cmd run build
        if ($LASTEXITCODE -ne 0) {
            Write-Fail "default 前端编译失败"
            exit 1
        }
        Write-OK "default 前端编译完成 -> web/default/dist"
    } finally {
        Pop-Location
        Remove-Item Env:DISABLE_ESLINT_PLUGIN -ErrorAction SilentlyContinue
        Remove-Item Env:VITE_REACT_APP_VERSION -ErrorAction SilentlyContinue
    }

    # 构建 classic 前端
    Write-Step "编译 classic 前端"
    $env:VITE_REACT_APP_VERSION = $Version
    Push-Location "web\classic"
    try {
        & bun.cmd run build
        if ($LASTEXITCODE -ne 0) {
            Write-Fail "classic 前端编译失败"
            exit 1
        }
        Write-OK "classic 前端编译完成 -> web/classic/dist"
    } finally {
        Pop-Location
        Remove-Item Env:VITE_REACT_APP_VERSION -ErrorAction SilentlyContinue
    }
} else {
    Write-Step "跳过前端编译 (-SkipFrontend)"
    if (-not (Test-Path "web\default\dist\index.html")) {
        Write-Fail "web/default/dist/index.html 不存在，请先编译前端"
        exit 1
    }
    if (-not (Test-Path "web\classic\dist\index.html")) {
        Write-Fail "web/classic/dist/index.html 不存在，请先编译前端"
        exit 1
    }
    Write-OK "前端产物已存在，跳过"
}

if ($FrontendOnly) {
    Write-Host "`n前端编译完成（-FrontendOnly 模式，跳过后端编译）" -ForegroundColor Green
    exit 0
}

# ── 编译后端 ──
Write-Step "编译 Go 后端 (CGO_ENABLED=0)"

$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:GOEXPERIMENT = "greenteagc"

$ldflags = "-s -w -X 'github.com/QuantumNous/new-api/common.Version=$Version'"

$outputName = "new-api.exe"
if ($OutputDir -ne ".") {
    # 确保输出目录存在
    if (-not (Test-Path $OutputDir)) {
        New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
    }
    $outputPath = Join-Path $OutputDir $outputName
} else {
    $outputPath = $outputName
}

try {
    & go build -ldflags $ldflags -o $outputPath
    if ($LASTEXITCODE -ne 0) {
        Write-Fail "Go 编译失败"
        exit 1
    }
    Write-OK "Go 编译完成 -> $outputPath"
} finally {
    Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
    Remove-Item Env:GOEXPERIMENT -ErrorAction SilentlyContinue
}

# ── 完成 ──
$size = [math]::Round((Get-Item $outputPath).Length / 1MB, 1)
Write-Host "`n========================================" -ForegroundColor Green
Write-Host " 编译成功!" -ForegroundColor Green
Write-Host " 输出文件: $outputPath" -ForegroundColor Green
Write-Host " 文件大小: ${size} MB" -ForegroundColor Green
Write-Host " 版本号:   $Version" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host "`n运行: .\$outputName" -ForegroundColor Yellow
Write-Host "默认监听: http://localhost:3000`n" -ForegroundColor Yellow

