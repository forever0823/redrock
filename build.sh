#!/bin/bash
# 跨平台静态编译脚本 (Linux/macOS)
# 编译 controller 和 agent 到 bin/ 目录，CGO_ENABLED=0 保证纯静态链接
# agent 会将 scripts/ 目录嵌入二进制，部署时只需单个可执行文件

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

BIN_DIR="bin"
mkdir -p "$BIN_DIR"

TARGETS=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
)

echo "=== 编译 controller ==="
for target in "${TARGETS[@]}"; do
    GOOS="${target%/*}"
    GOARCH="${target#*/}"
    OUTPUT="../$BIN_DIR/controller-${GOOS}-${GOARCH}"
    if [ "$GOOS" = "windows" ]; then
        OUTPUT="${OUTPUT}.exe"
    fi
    echo "  -> $target  => $OUTPUT"
    CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -C controller -o "$OUTPUT" .
done

echo "=== 编译 agent ==="
for target in "${TARGETS[@]}"; do
    GOOS="${target%/*}"
    GOARCH="${target#*/}"
    OUTPUT="../$BIN_DIR/agent-${GOOS}-${GOARCH}"
    if [ "$GOOS" = "windows" ]; then
        OUTPUT="${OUTPUT}.exe"
    fi
    echo "  -> $target  => $OUTPUT"
    CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -C agent -o "$OUTPUT" .
done

echo ""
echo "编译完成，输出文件:"
ls -lh "$BIN_DIR"/
