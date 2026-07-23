#!/usr/bin/env bash
# kind + helm 설치 (Docker Desktop 이 켜져 있어야 함).
set -euo pipefail

need() { command -v "$1" >/dev/null 2>&1; }

if ! docker info >/dev/null 2>&1; then
  echo "✗ Docker 데몬에 연결 불가 — Docker Desktop 을 먼저 실행하세요." >&2
  exit 1
fi

if ! need kind; then
  echo "→ kind 설치"
  go install sigs.k8s.io/kind@v0.24.0
  echo "  (\$(go env GOPATH)/bin 이 PATH 에 있어야 합니다)"
fi

if ! need helm; then
  echo "→ helm 설치 안내: https://helm.sh/docs/intro/install/ (winget install Helm.Helm 등)"
fi

echo "✓ 도구 확인 완료"
kind version || true
helm version --short || true
