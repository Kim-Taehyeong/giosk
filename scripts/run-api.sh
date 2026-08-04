#!/usr/bin/env bash
# 로컬 API 기동(테스트베드 기준 풀 env). 먼저 scripts/testbed-up.sh 로 인프라를 올린 뒤 실행.
#   하이브리드 + 크레딧 + NFS + LoadBalancer + Prometheus + 데이터셋 + 인클러스터 레지스트리(Kaniko)
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT/backend"

export GIOSK_DB_HOST=127.0.0.1 GIOSK_DB_PORT=3306 GIOSK_DB_USER=giosk GIOSK_DB_PASS=giosk GIOSK_DB_NAME=giosk
export GIOSK_ADMIN_USERNAME=admin GIOSK_ADMIN_PASSWORD=admin123
export GIOSK_DEPLOYMENT_MODE=hybrid GIOSK_BILLING_MODE=credit
export GIOSK_PERSISTENCE_CLASS=nfs GIOSK_NFS_CLASS=nfs
export GIOSK_SESSION_EXPOSE=loadbalancer
export GIOSK_PROMETHEUS_URL=http://localhost:9090
export GIOSK_AGENT_TOKEN=agent-secret
export GIOSK_DATASETS_ENABLED=true
# 데이터셋 정규 NFS 경로(<base>/dataset/<name>).
# 테스트베드: ganesha 프로비저너의 export 루트(/export)는 RO 의사루트라 임의 서브디렉터리를 만들 수 없다.
# RWX PVC(giosk-ds-root)를 하나 만들어 그 PV 의 NFS 경로를 베이스로 사용(쓰기 가능).
# 운영(외부 NFS)에서는 GIOSK_DATASETS_NFS_SERVER/PATH 를 실제 export(예: nfs:/export)로 직접 지정하면 됨.
kubectl create namespace giosk-grp-datasets >/dev/null 2>&1 || true
kubectl apply -n giosk-grp-datasets -f - >/dev/null 2>&1 <<'YAML'
apiVersion: v1
kind: PersistentVolumeClaim
metadata: { name: giosk-ds-root }
spec:
  accessModes: [ReadWriteMany]
  storageClassName: nfs
  resources: { requests: { storage: 50Gi } }
YAML
kubectl wait -n giosk-grp-datasets --for=jsonpath='{.status.phase}'=Bound pvc/giosk-ds-root --timeout=60s >/dev/null 2>&1 || true
_DSPV=$(kubectl get pvc giosk-ds-root -n giosk-grp-datasets -o jsonpath='{.spec.volumeName}' 2>/dev/null)
export GIOSK_DATASETS_NFS_SERVER=$(kubectl get pv "$_DSPV" -o jsonpath='{.spec.nfs.server}' 2>/dev/null)
export GIOSK_DATASETS_NFS_PATH=$(kubectl get pv "$_DSPV" -o jsonpath='{.spec.nfs.path}' 2>/dev/null)
# 데이터셋 노드 로컬 캐시 루트(hostPath). //이중슬래시는 git-bash 경로변환 회피(MSYS_NO_PATHCONV).
export GIOSK_DATASET_CACHE_HOSTPATH=//dataset-cache
export GIOSK_REGISTRY=giosk-registry:5000   # 이미지 빌드 푸시 + 세션 풀 주소
export GIOSK_COSIGN_KEY_SECRET=giosk-cosign-key   # 빌드 후 cosign 서명(키 없으면 빌드 ns 에 자동 생성)
# 노드로컬 스크래치. //scratch 의 이중 슬래시는 git-bash(MSYS)의 경로 자동변환을 피하기 위함(리눅스에선 /scratch 와 동일).
export GIOSK_SCRATCH_ENABLED=true MSYS_NO_PATHCONV=1
export GIOSK_SCRATCH_HOSTPATH=//scratch

echo "→ go run ./cmd/api  (admin/admin123, http://localhost:8080)"
exec go run ./cmd/api
