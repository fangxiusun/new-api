#!/usr/bin/env bash
# build.sh - Linux 本地一键编译脚本
# 将前端（default + classic）和后端编译为单个可执行文件 new-api
#
# 用法:
#   ./build.sh                  # 完整编译（前端 + 后端）
#   ./build.sh --skip-frontend  # 仅编译后端（需已存在 web/default/dist 和 web/classic/dist）
#   ./build.sh --frontend-only  # 仅编译前端
#   ./build.sh --output-dir /tmp/out  # 指定输出目录

set -euo pipefail

# ── 颜色输出辅助 ──
CYAN='\033[0;36m'
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

step()  { echo -e "\n${CYAN}>> $*${NC}"; }
ok()    { echo -e "   ${GREEN}OK: $*${NC}"; }
fail()  { echo -e "   ${RED}FAIL: $*${NC}"; }

# ── 参数解析 ──
SKIP_FRONTEND=false
FRONTEND_ONLY=false
OUTPUT_DIR="."

while [[ $# -gt 0 ]]; do
    case "$1" in
        --skip-frontend)  SKIP_FRONTEND=true;  shift ;;
        --frontend-only)  FRONTEND_ONLY=true;  shift ;;
        --output-dir)     OUTPUT_DIR="$2";     shift 2 ;;
        -h|--help)
            echo "用法: ./build.sh [--skip-frontend] [--frontend-only] [--output-dir DIR]"
            exit 0
            ;;
        *)
            echo "未知参数: $1"
            exit 1
            ;;
    esac
done

# ── 切换到项目根目录（脚本所在目录） ──
ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

# ── 读取版本号 ──
VERSION=""
if [[ -f "VERSION" ]]; then
    raw="$(cat VERSION 2>/dev/null || true)"
    if [[ -n "$raw" ]]; then
        VERSION="$(echo "$raw" | tr -d '[:space:]')"
    fi
fi
if [[ -z "$VERSION" ]]; then
    VERSION="$(git describe --tags --always 2>/dev/null || true)"
fi
if [[ -z "$VERSION" ]]; then
    VERSION="v0.0.0-dev"
fi
echo -e "${YELLOW}Version: ${VERSION}${NC}"

# ── 前置检查 ──
step "检查编译工具"

if ! command -v go &>/dev/null; then
    fail "未找到 Go，请先安装 Go 1.25+"
    exit 1
fi
ok "Go: $(go version)"

if ! command -v bun &>/dev/null; then
    fail "未找到 Bun，请先安装 Bun (https://bun.sh)"
    exit 1
fi
ok "Bun: v$(bun --version)"

# ── 编译前端 ──
if [[ "$SKIP_FRONTEND" == false ]]; then

    step "安装前端依赖 (bun install)"
    pushd web >/dev/null
    if ! bun install --frozen-lockfile; then
        echo -e "   ${YELLOW}frozen-lockfile 失败，尝试不加锁安装...${NC}"
        bun install
    fi
    ok "前端依赖安装完成"
    popd >/dev/null

    # 构建 default 前端
    step "编译 default 前端"
    pushd web/default >/dev/null
    DISABLE_ESLINT_PLUGIN=true \
    VITE_REACT_APP_VERSION="$VERSION" \
        bun run build
    ok "default 前端编译完成 -> web/default/dist"
    popd >/dev/null

    # 构建 classic 前端
    step "编译 classic 前端"
    pushd web/classic >/dev/null
    VITE_REACT_APP_VERSION="$VERSION" \
        bun run build
    ok "classic 前端编译完成 -> web/classic/dist"
    popd >/dev/null

else
    step "跳过前端编译 (--skip-frontend)"
    if [[ ! -f "web/default/dist/index.html" ]]; then
        fail "web/default/dist/index.html 不存在，请先编译前端"
        exit 1
    fi
    if [[ ! -f "web/classic/dist/index.html" ]]; then
        fail "web/classic/dist/index.html 不存在，请先编译前端"
        exit 1
    fi
    ok "前端产物已存在，跳过"
fi

if [[ "$FRONTEND_ONLY" == true ]]; then
    echo -e "\n${GREEN}前端编译完成（--frontend-only 模式，跳过后端编译）${NC}"
    exit 0
fi

# ── 编译后端 ──
step "编译 Go 后端 (CGO_ENABLED=0)"

export CGO_ENABLED=0
export GOOS=linux
export GOARCH=amd64
export GOEXPERIMENT=greenteagc

LDFLAGS="-s -w -X 'github.com/QuantumNous/new-api/common.Version=${VERSION}'"

OUTPUT_NAME="new-api"
if [[ "$OUTPUT_DIR" != "." ]]; then
    mkdir -p "$OUTPUT_DIR"
    OUTPUT_PATH="${OUTPUT_DIR}/${OUTPUT_NAME}"
else
    OUTPUT_PATH="$OUTPUT_NAME"
fi

go build -ldflags "$LDFLAGS" -o "$OUTPUT_PATH"
ok "Go 编译完成 -> $OUTPUT_PATH"

# ── 完成 ──
SIZE=$(du -h "$OUTPUT_PATH" | cut -f1)
echo -e "\n${GREEN}========================================"
echo -e " 编译成功!"
echo -e " 输出文件: $OUTPUT_PATH"
echo -e " 文件大小: $SIZE"
echo -e " 版本号:   $VERSION"
echo -e "========================================${NC}"
echo -e "\n${YELLOW}运行: ./$OUTPUT_PATH${NC}"
echo -e "${YELLOW}默认监听: http://localhost:3000${NC}\n"