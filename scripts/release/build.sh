#!/usr/bin/env bash
set -Eeuo pipefail

export COPYFILE_DISABLE=1
export COPY_EXTENDED_ATTRIBUTES_DISABLE=1

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT_DIR="${ROOT_DIR}/release"
VERSION="${VERSION:-$(date +%Y.%m.%d-%H%M)}"

rm -rf "${OUT_DIR}"
mkdir -p "${OUT_DIR}" "${OUT_DIR}/tmp"

echo "==> Building frontend dist"
(
  cd "${ROOT_DIR}/frontend"
  npm ci
  npm run build
)

echo "==> Building backend control binaries"
(
  cd "${ROOT_DIR}/backend"
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o "${OUT_DIR}/tmp/tele-auto-control-linux-amd64" ./cmd/control
  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o "${OUT_DIR}/tmp/tele-auto-control-linux-arm64" ./cmd/control
)

echo "==> Packaging frontend web bundle"
tar -C "${ROOT_DIR}/frontend/dist" -czf "${OUT_DIR}/tele-auto-go-web_${VERSION}.tar.gz" .

echo "==> Packaging linux amd64"
mkdir -p "${OUT_DIR}/pkg-amd64/bin"
cp "${OUT_DIR}/tmp/tele-auto-control-linux-amd64" "${OUT_DIR}/pkg-amd64/bin/tele-auto-control"
cp "${ROOT_DIR}/deploy/tele-auto.env.example" "${OUT_DIR}/pkg-amd64/tele-auto.env.example"
cp "${ROOT_DIR}/deploy/tele-auto.service" "${OUT_DIR}/pkg-amd64/tele-auto.service"
tar -C "${OUT_DIR}/pkg-amd64" -czf "${OUT_DIR}/tele-auto-go_${VERSION}_linux_amd64.tar.gz" .

echo "==> Packaging linux arm64"
mkdir -p "${OUT_DIR}/pkg-arm64/bin"
cp "${OUT_DIR}/tmp/tele-auto-control-linux-arm64" "${OUT_DIR}/pkg-arm64/bin/tele-auto-control"
cp "${ROOT_DIR}/deploy/tele-auto.env.example" "${OUT_DIR}/pkg-arm64/tele-auto.env.example"
cp "${ROOT_DIR}/deploy/tele-auto.service" "${OUT_DIR}/pkg-arm64/tele-auto.service"
tar -C "${OUT_DIR}/pkg-arm64" -czf "${OUT_DIR}/tele-auto-go_${VERSION}_linux_arm64.tar.gz" .

echo "==> Artifacts ready in ${OUT_DIR}"
ls -lh "${OUT_DIR}"/*.tar.gz
