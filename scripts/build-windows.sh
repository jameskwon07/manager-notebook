#!/usr/bin/env bash
# Windows 실행 파일 묶음을 만듭니다.
#   scripts/build-windows.sh <VERSION> [OUT_DIR=dist]
# 결과: OUT_DIR/manager-notebook-<VERSION>-windows-{amd64,arm64}.zip, SHA256SUMS.txt, README-windows.txt
# zip 안: manager-notebook.exe, run-intake.cmd(요청 접수함만), run-intake-demo.cmd, run-demo.cmd(전체 체험), README.txt(UTF-8 BOM·CRLF, 메모장에서 한글이 보이게), LICENSE.txt
set -euo pipefail

VERSION=${1:?버전을 주세요. 예: scripts/build-windows.sh 2.1.0}
VERSION=${VERSION#v}
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
OUT=${2:-dist}
case $OUT in /*) ;; *) OUT="$ROOT/$OUT" ;; esac
cd "$ROOT"

crlf() { awk '{ sub(/\r$/, ""); printf "%s\r\n", $0 }' "$@"; }
sha() { if command -v sha256sum >/dev/null; then sha256sum "$@"; else shasum -a 256 "$@"; fi; }

rm -rf "$OUT"
mkdir -p "$OUT"
for arch in amd64 arm64; do
  stage=$(mktemp -d)
  CGO_ENABLED=0 GOOS=windows GOARCH=$arch go build -trimpath -buildvcs=false \
    -ldflags "-s -w -X main.version=$VERSION" -o "$stage/manager-notebook.exe" ./cmd/manager-notebook
  { printf '\xEF\xBB\xBF'; sed "s/{{VERSION}}/$VERSION/g" packaging/windows/README.txt | crlf; } > "$stage/README.txt"
  crlf packaging/windows/run-demo.cmd > "$stage/run-demo.cmd"
  crlf packaging/windows/run-intake.cmd > "$stage/run-intake.cmd"
  crlf packaging/windows/run-intake-demo.cmd > "$stage/run-intake-demo.cmd"
  crlf LICENSE > "$stage/LICENSE.txt"
  # 같은 소스면 같은 zip이 나오도록 시각을 고정합니다.
  (cd "$stage" && TZ=UTC touch -t 202601010000 manager-notebook.exe README.txt run-demo.cmd run-intake.cmd run-intake-demo.cmd LICENSE.txt \
    && zip -q -X -D "$OUT/manager-notebook-$VERSION-windows-$arch.zip" manager-notebook.exe run-intake.cmd run-intake-demo.cmd run-demo.cmd README.txt LICENSE.txt)
  rm -rf "$stage"
done
{ printf '\xEF\xBB\xBF'; sed "s/{{VERSION}}/$VERSION/g" packaging/windows/README.txt | crlf; } > "$OUT/README-windows.txt"
(cd "$OUT" && sha manager-notebook-"$VERSION"-windows-*.zip > SHA256SUMS.txt)
ls -l "$OUT"
cat "$OUT/SHA256SUMS.txt"
