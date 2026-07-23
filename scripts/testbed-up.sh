#!/usr/bin/env bash
# Giosk 전체 테스트베드 1-커맨드 기동(멱등).
#   kind(멀티노드) + MySQL + NFS provisioner + kube-prometheus-stack + fake-gpu-operator
#   + MetalLB + 인클러스터 레지스트리(Kaniko 빌드/세션 풀) + 노드 containerd insecure 설정
# 사용: bash scripts/testbed-up.sh   (Docker Desktop 실행 필요)
# 이후: bash scripts/run-api.sh      (로컬 API 기동) + cd frontend && npm run dev
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CLUSTER=giosk
CTX="kind-$CLUSTER"
REG=giosk-registry          # 레지스트리 컨테이너명(=kind 네트워크 DNS)
REG_PORT=5000
k() { kubectl --context "$CTX" "$@"; }

docker info >/dev/null 2>&1 || { echo "✗ Docker Desktop 을 먼저 실행하세요." >&2; exit 1; }
for t in kind helm kubectl; do command -v "$t" >/dev/null 2>&1 || { echo "✗ '$t' 필요 — scripts/install-tools.sh 참고" >&2; exit 1; }; done

echo "==> 1/8 kind 클러스터"
if ! kind get clusters | grep -qx "$CLUSTER"; then
  kind create cluster --name "$CLUSTER" --config "$ROOT/deploy/kind/cluster.yaml"
fi
k cluster-info >/dev/null

echo "==> 2/8 MySQL 컨테이너(:3306)"
if ! docker ps -a --format '{{.Names}}' | grep -qx giosk-mysql; then
  docker run -d --name giosk-mysql --restart=always \
    -e MYSQL_ROOT_PASSWORD=dev -e MYSQL_DATABASE=giosk \
    -e MYSQL_USER=giosk -e MYSQL_PASSWORD=giosk -p 3306:3306 mysql:8 >/dev/null
else
  docker start giosk-mysql >/dev/null 2>&1 || true
fi

echo "==> 3/8 Helm 리포 + NFS provisioner(storageClass=nfs)"
helm repo add nfs-ganesha https://kubernetes-sigs.github.io/nfs-ganesha-server-and-external-provisioner >/dev/null 2>&1 || true
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts >/dev/null 2>&1 || true
helm repo add fake-gpu-operator https://fake-gpu-operator.storage.googleapis.com >/dev/null 2>&1 || true
helm repo update >/dev/null 2>&1 || true
helm --kube-context "$CTX" status nfs -n nfs >/dev/null 2>&1 || \
  helm --kube-context "$CTX" install nfs nfs-ganesha/nfs-server-provisioner -n nfs --create-namespace \
    --set 'storageClass.name=nfs' --set 'persistence.enabled=true' --set 'persistence.size=20Gi' --wait --timeout 5m

echo "==> 4/8 kube-prometheus-stack(모니터링) + fake-gpu-operator(가짜 GPU)"
helm --kube-context "$CTX" status kps -n monitoring >/dev/null 2>&1 || \
  helm --kube-context "$CTX" install kps prometheus-community/kube-prometheus-stack -n monitoring --create-namespace \
    --set 'grafana.enabled=false' --wait --timeout 8m
helm --kube-context "$CTX" status fakegpu -n gpu-operator >/dev/null 2>&1 || \
  helm --kube-context "$CTX" install fakegpu fake-gpu-operator/fake-gpu-operator -n gpu-operator --create-namespace --wait --timeout 5m

echo "==> 5/8 MetalLB(LoadBalancer 노출)"
SUBNET=$(docker network inspect kind -f '{{range .IPAM.Config}}{{if .Subnet}}{{.Subnet}} {{end}}{{end}}' | tr ' ' '\n' | grep -E '^[0-9]+\.' | head -1)
PREFIX=$(echo "$SUBNET" | cut -d. -f1,2)   # 예: 172.20
if ! k get ns metallb-system >/dev/null 2>&1; then
  k apply -f https://raw.githubusercontent.com/metallb/metallb/v0.14.8/config/manifests/metallb-native.yaml
  k -n metallb-system rollout status deploy/controller --timeout=180s
fi
k apply -f - <<EOF
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata: { name: giosk-pool, namespace: metallb-system }
spec: { addresses: ["${PREFIX}.255.200-${PREFIX}.255.250"] }
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata: { name: giosk-l2, namespace: metallb-system }
spec: { ipAddressPools: [giosk-pool] }
EOF

echo "==> 6/8 인클러스터 레지스트리($REG:$REG_PORT) + 노드 containerd insecure"
if ! docker ps -a --format '{{.Names}}' | grep -qx "$REG"; then
  docker run -d --restart=always --name "$REG" --net kind registry:2 >/dev/null
else
  docker network connect kind "$REG" >/dev/null 2>&1 || true
  docker start "$REG" >/dev/null 2>&1 || true
fi
for n in $(kind get nodes --name "$CLUSTER"); do
  docker exec "$n" sh -c '
    if ! grep -q "certs.d" /etc/containerd/config.toml; then
      printf "\n[plugins.\"io.containerd.grpc.v1.cri\".registry]\n  config_path = \"/etc/containerd/certs.d\"\n" >> /etc/containerd/config.toml
    fi
    mkdir -p "/etc/containerd/certs.d/'"$REG"':'"$REG_PORT"'"
    printf "server = \"http://'"$REG"':'"$REG_PORT"'\"\n\n[host.\"http://'"$REG"':'"$REG_PORT"'\"]\n  capabilities = [\"pull\", \"resolve\"]\n  skip_verify = true\n" > "/etc/containerd/certs.d/'"$REG"':'"$REG_PORT"'/hosts.toml"
    pkill -HUP containerd 2>/dev/null || true
  ' || true
done

echo "==> 7/8 fake GPU 용량 광고(노드 라벨 기반, 보조)"
bash "$ROOT/scripts/fake-gpu.sh" >/dev/null 2>&1 || true

echo "==> 8/8 Prometheus 포트포워드(:9090, 백그라운드)"
pkill -f "port-forward.*kps-kube-prometheus-stack-prometheus" 2>/dev/null || true
nohup kubectl --context "$CTX" -n monitoring port-forward svc/kps-kube-prometheus-stack-prometheus 9090:9090 >/tmp/giosk-prom-pf.log 2>&1 &

cat <<EOF

✓ 테스트베드 준비 완료.
  • kind: $CTX (control-plane + worker×3, gpu-type/physical 라벨)
  • MySQL :3306 (giosk/giosk/giosk)  • NFS storageClass=nfs  • Prometheus :9090
  • 레지스트리 $REG:$REG_PORT (Kaniko 빌드/세션 풀)  • MetalLB pool ${PREFIX}.255.200-250

  로컬 API 기동:   bash scripts/run-api.sh
  프론트(개발):    cd frontend && npm install && npm run dev
EOF
