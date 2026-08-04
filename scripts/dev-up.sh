#!/usr/bin/env bash
# 개발 베드 기동: kind 클러스터 + MySQL 컨테이너.
# 이후 호스트에서 `go run ./cmd/api` 로 로컬 API 를 띄워 kubeconfig 로 kind 에 접속.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CLUSTER=giosk

docker info >/dev/null 2>&1 || { echo "✗ Docker Desktop 을 먼저 실행하세요." >&2; exit 1; }

# 1) kind 멀티노드 클러스터
if ! kind get clusters | grep -qx "$CLUSTER"; then
  echo "→ kind 클러스터 생성 ($CLUSTER)"
  kind create cluster --name "$CLUSTER" --config "$ROOT/deploy/kind/cluster.yaml"
fi
kubectl cluster-info --context "kind-$CLUSTER" >/dev/null

# 2) 개발용 MySQL (호스트 :3306 을 노출해 로컬 API 가 DSN 으로 접속한다)
if ! docker ps --format '{{.Names}}' | grep -qx giosk-mysql; then
  echo "→ MySQL 컨테이너 기동"
  docker run -d --name giosk-mysql \
    -e MYSQL_ROOT_PASSWORD=dev -e MYSQL_DATABASE=giosk \
    -e MYSQL_USER=giosk -e MYSQL_PASSWORD=giosk \
    -p 3306:3306 mysql:8
fi

cat <<'EOF'

✓ 개발 베드 준비됨.
  로컬 API 실행:
    GIOSK_DB_HOST=127.0.0.1 GIOSK_DB_USER=giosk GIOSK_DB_PASS=giosk \
    go run ./cmd/api
  (client-go 는 ~/.kube/config 의 kind-giosk 컨텍스트로 클러스터에 접속)
EOF
