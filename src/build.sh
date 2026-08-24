#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

VERSION="$(git -C .. describe --tags --always 2>/dev/null | sed 's/^v//' || echo "dev")"
LDFLAGS="-X 'github.com/lzyyzznl/multi-web-search/cmd.Version=${VERSION}'"

echo "🔨 构建 multi-web-search (v${VERSION})..."

DIST="../dist"
mkdir -p "$DIST"

# 本地平台构建（当前机器，按平台命名输出到 dist/，与其他平台统一供 Release 上传）
CGO_ENABLED=0 go build -ldflags "$LDFLAGS" -o "$DIST/multi-web-search-$(go env GOOS)-$(go env GOARCH)" .
echo "  ✅ 本地构建: $DIST/multi-web-search-$(go env GOOS)-$(go env GOARCH)"

# 多平台交叉编译矩阵
PLATFORMS=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
)

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

echo "✅ 构建完成"
echo "  分发: $DIST/（上传到 GitHub Release 供各平台拉取）"
