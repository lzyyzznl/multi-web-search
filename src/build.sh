#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

echo "🔨 构建 multi-web-search..."

CGO_ENABLED=0 go build -o multi-web-search .

SKILLS_DIR="../skills/multi-web-search"
mkdir -p "$SKILLS_DIR" ../bin

cp multi-web-search "$SKILLS_DIR/multi-web-search"
cp multi-web-search ../bin/multi-web-search

echo "✅ 构建完成"
echo "  skills/multi-web-search/multi-web-search"
echo "  bin/multi-web-search"
