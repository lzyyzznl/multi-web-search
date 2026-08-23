#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

VERSION="$(git -C .. describe --tags --always 2>/dev/null || echo "dev")"
LDFLAGS="-X 'github.com/lzyyzznl/multi-web-search/cmd.Version=${VERSION}'"

echo "🔨 构建 multi-web-search (v${VERSION})..."

# 本地平台构建（当前机器的二进制）
CGO_ENABLED=0 go build -ldflags "$LDFLAGS" -o multi-web-search .
echo "  ✅ 本地构建: multi-web-search"

# 多平台交叉编译矩阵
PLATFORMS=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
)
DIST="../dist"
mkdir -p "$DIST"

for p in "${PLATFORMS[@]}"; do
  GOOS="${p%/*}"
  GOARCH="${p#*/}"
  OUT="$DIST/multi-web-search-${GOOS}-${GOARCH}"
  if [ "$GOOS" = "windows" ]; then
    OUT="${OUT}.exe"
  fi
  if [ "$GOOS" = "$(go env GOOS)" ] && [ "$GOARCH" = "$(go env GOARCH)" ]; then
    continue # 跳过当前平台，已由本地构建覆盖
  fi
  echo "  → 交叉编译 ${GOOS}/${GOARCH} ..."
  CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -ldflags "$LDFLAGS" -o "$OUT" .
done

# 本地二进制复制到 skills/ 和 bin/
SKILLS_DIR="../skills/multi-web-search"
mkdir -p "$SKILLS_DIR" ../bin
cp multi-web-search "$SKILLS_DIR/scripts/multi-web-search"
cp multi-web-search ../bin/multi-web-search

echo "✅ 构建完成"
echo "  本地: multi-web-search"
echo "  分发: $DIST/"
echo "  skills/multi-web-search/scripts/multi-web-search"
echo "  bin/multi-web-search"
