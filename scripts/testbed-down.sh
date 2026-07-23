#!/usr/bin/env bash
# 테스트베드 정리. 기본은 보존형(클러스터 유지, 포트포워드만 종료).
#   --all 지정 시 kind 클러스터 + MySQL + 레지스트리 컨테이너까지 완전 삭제.
set -euo pipefail
CLUSTER=giosk

pkill -f "port-forward.*kps-kube-prometheus-stack-prometheus" 2>/dev/null || true
echo "✓ Prometheus 포트포워드 종료"

if [ "${1:-}" = "--all" ]; then
  kind delete cluster --name "$CLUSTER" 2>/dev/null || true
  docker rm -f giosk-mysql giosk-registry 2>/dev/null || true
  echo "✓ kind 클러스터 + MySQL + 레지스트리 삭제"
else
  echo "  (클러스터/DB 보존. 완전 삭제: bash scripts/testbed-down.sh --all)"
fi
