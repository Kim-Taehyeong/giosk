#!/usr/bin/env bash
# ════════════════════════════════════════════════════════════════════
#  Giosk 서비스 배포 (DKU Ubuntu kubeadm 클러스터, control 노드에서 실행)
#
#  방식: control 노드에서 이미지를 빌드(nerdctl)해 각 워커에 ctr import 한다(레지스트리를 쓰지 않는다).
#        그다음 helm 으로 API, 프론트, 번들 MySQL 을 배포한다. 프론트는 MetalLB LoadBalancer 다.
#        단일 오리진이라(nginx 가 /api 를 프록시) CORS 가 필요 없다.
#
#  사용:  sudo ./deploy.sh                # 빌드+배급+배포
#         sudo ./deploy.sh --skip-build   # 이미지 그대로 두고 helm 만 재적용
#         sudo TAG=v1 ./deploy.sh         # 태그 지정
#         sudo ./deploy.sh --with-node-agent   # node-agent 이미지도 빌드/배급(hybrid)
#  환경:  RELEASE(=giosk) NAMESPACE(=giosk) TAG(=latest)
#         NODE_USER(=ubuntu) NODE_PASS(=ubuntu)  # 워커 SSH(이미지 import)
# ════════════════════════════════════════════════════════════════════
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)          # Giosk/
CHART="$REPO_ROOT/charts/giosk"
VALUES="${VALUES:-$SCRIPT_DIR/values.example.yaml}"   # 자체 values 는 VALUES=... 로 지정
SECRETS_ENV="$SCRIPT_DIR/.giosk-secrets.env"     # 생성된 비밀번호 영속(gitignore)

RELEASE=${RELEASE:-giosk}
NAMESPACE=${NAMESPACE:-giosk}
TAG=${TAG:-latest}
NODE_USER=${NODE_USER:-ubuntu}
NODE_PASS=${NODE_PASS:-ubuntu}
export KUBECONFIG=${KUBECONFIG:-/etc/kubernetes/admin.conf}

WITH_NODE_AGENT=false
WITH_GATEWAY=false
SKIP_BUILD=false
MONITORING=${MONITORING:-true}     # kube-prometheus-stack(별도 릴리스) 설치 여부
for a in "$@"; do
  case "$a" in
    --with-node-agent) WITH_NODE_AGENT=true ;;
    --with-gateway)    WITH_GATEWAY=true ;;
    --skip-build)      SKIP_BUILD=true ;;
    --no-monitoring)   MONITORING=false ;;
    *) echo "unknown arg: $a" >&2; exit 1 ;;
  esac
done

log(){ echo -e "\n\033[1;36m▶ $*\033[0m"; }
die(){ echo -e "\033[1;31m✗ $*\033[0m" >&2; exit 1; }

[ "$EUID" -eq 0 ] || die "root 로 실행하세요: sudo ./deploy.sh (ctr/nerdctl 권한 필요)"
command -v kubectl >/dev/null || die "kubectl 없음"
command -v helm    >/dev/null || die "helm 없음"
command -v ctr     >/dev/null || die "ctr(containerd) 없음"
kubectl get nodes >/dev/null 2>&1 || die "클러스터 접속 불가 (KUBECONFIG=$KUBECONFIG)"

# ── 0) 빌더(nerdctl+buildkit) 준비 ──────────────────────────────────
ensure_builder(){
  command -v nerdctl >/dev/null && command -v buildctl >/dev/null && return
  log "nerdctl(full) 설치 — buildkit 포함"
  local ver=2.0.2 arch=amd64
  curl -fsSL "https://github.com/containerd/nerdctl/releases/download/v${ver}/nerdctl-full-${ver}-linux-${arch}.tar.gz" \
    | tar Cxz /usr/local
  systemctl enable --now buildkit
}

# ── 1) 스토리지 프로비저너(local-path) + 기본 StorageClass ───────────
ensure_storage(){
  if kubectl get sc local-path >/dev/null 2>&1; then
    log "local-path StorageClass 이미 존재"
  else
    log "local-path-provisioner 설치"
    kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.30/deploy/local-path-storage.yaml
    kubectl -n local-path-storage rollout status deploy/local-path-provisioner --timeout=120s
  fi
  # 기본 StorageClass 로 지정(PVC 가 storageClassName 생략해도 동작)
  kubectl patch storageclass local-path \
    -p '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}' >/dev/null 2>&1 || true
}

# ── 2) 비밀번호 생성/로드(최초 1회 생성 후 재사용) ──────────────────
load_secrets(){
  if [ -f "$SECRETS_ENV" ]; then
    # shellcheck disable=SC1090
    source "$SECRETS_ENV"
  else
    # openssl 사용(파이프 SIGPIPE+pipefail 로 set -e 가 죽는 문제 회피)
    ADMIN_PASSWORD=$(openssl rand -hex 12)
    DB_PASSWORD=$(openssl rand -hex 16)
    umask 077
    cat > "$SECRETS_ENV" <<EOF
# Giosk 배포 비밀번호. 자동 생성하며 절대 커밋하지 않는다(.gitignore). 바꾸면 재배포해야 한다.
ADMIN_PASSWORD=$ADMIN_PASSWORD
DB_PASSWORD=$DB_PASSWORD
EOF
    log "비밀번호 생성 → $SECRETS_ENV"
  fi
  # 게이트웨이 비밀과 SSH 키는 고정이다. drift 를 막으려 최초 1회 생성 후 재사용한다.
  if [ -z "${GATEWAY_SECRET:-}" ]; then
    GATEWAY_SECRET=$(openssl rand -hex 20)
    local gkey; gkey=$(mktemp -u /tmp/giosk-gw-key.XXXX)
    ssh-keygen -t ed25519 -N '' -C giosk-gateway -f "$gkey" >/dev/null
    GATEWAY_SSH_PRIVATE=$(cat "$gkey")
    GATEWAY_SSH_PUBLIC=$(cat "$gkey.pub")
    rm -f "$gkey" "$gkey.pub"
    umask 077
    cat >> "$SECRETS_ENV" <<EOF
GATEWAY_SECRET=$GATEWAY_SECRET
GATEWAY_SSH_PRIVATE=$(printf '%q' "$GATEWAY_SSH_PRIVATE")
GATEWAY_SSH_PUBLIC=$(printf '%q' "$GATEWAY_SSH_PUBLIC")
EOF
    log "게이트웨이 비밀/SSH 키 생성 → $SECRETS_ENV"
  fi
}

# ── 3) 이미지 빌드 ──────────────────────────────────────────────────
build_images(){
  ensure_builder
  log "이미지 빌드 (api, frontend${WITH_NODE_AGENT:+, node-agent})"
  nerdctl build --target api -t "giosk-api:$TAG" \
    -f "$REPO_ROOT/backend/Dockerfile" "$REPO_ROOT/backend"
  nerdctl build -t "giosk-frontend:$TAG" --build-arg VITE_API_URL=/api \
    -f "$REPO_ROOT/frontend/Dockerfile" "$REPO_ROOT/frontend"
  if $WITH_NODE_AGENT; then
    nerdctl build --target node-agent -t "giosk-node-agent:$TAG" \
      -f "$REPO_ROOT/backend/Dockerfile" "$REPO_ROOT/backend"
  fi
  if $WITH_GATEWAY; then
    nerdctl build --target gateway -t "giosk-gateway:$TAG" \
      -f "$REPO_ROOT/backend/Dockerfile" "$REPO_ROOT/backend"
    # 세션 SSH 사이드카(컨테이너 SSH 탭). 게이트웨이 SSH 프록시가 이 sshd 로 재접속한다.
    nerdctl build --target sshd -t "giosk-sshd:$TAG" \
      -f "$REPO_ROOT/backend/Dockerfile" "$REPO_ROOT/backend"
  fi
}

# ── 4) 이미지를 모든 노드에 배급(ctr import, 레지스트리 없음) ─────────
distribute_images(){
  local imgs=("giosk-api:$TAG" "giosk-frontend:$TAG")
  $WITH_NODE_AGENT && imgs+=("giosk-node-agent:$TAG")
  $WITH_GATEWAY && imgs+=("giosk-gateway:$TAG" "giosk-sshd:$TAG")

  log "이미지 tar 저장"
  local tars=()
  for img in "${imgs[@]}"; do
    local f="/tmp/${img//[:\/]/_}.tar"
    nerdctl save "$img" -o "$f"
    tars+=("$f")
  done

  # 노드 목록(이름 InternalIP). control(자기 자신)은 로컬 import, 워커는 SSH import.
  # 루프 입력은 fd3 으로 읽는다. fd0(stdin)으로 읽으면 내부 ssh 가 노드 목록을
  #    먹어버려 첫 워커만 처리되고 끝난다(전형적 함정). ssh 도 -n 으로 stdin 차단.
  local self; self=$(hostname)
  while read -r name ip <&3; do
    [ -n "$ip" ] || continue
    if [ "$name" = "$self" ] || [ "$name" = "control" ]; then
      log "[$name] 로컬 import"
      for f in "${tars[@]}"; do ctr -n k8s.io images import "$f" >/dev/null; done
    else
      log "[$name $ip] SSH import"
      for f in "${tars[@]}"; do
        sshpass -p "$NODE_PASS" scp -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
          "$f" "$NODE_USER@$ip:/tmp/" >/dev/null
        sshpass -p "$NODE_PASS" ssh -n -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
          "$NODE_USER@$ip" "echo $NODE_PASS | sudo -S ctr -n k8s.io images import /tmp/$(basename "$f") >/dev/null && rm -f /tmp/$(basename "$f")"
      done
    fi
  done 3< <(kubectl get nodes -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.status.addresses[?(@.type=="InternalIP")].address}{"\n"}{end}')

  rm -f "${tars[@]}"
}

# ── 5a) 모니터링: kube-prometheus-stack 을 '별도 릴리스'로 설치 ───────
# 서브차트로 넣으면 CRD(Prometheus/ServiceMonitor/PodMonitor/PrometheusRule)가
#    매니페스트보다 먼저 안 깔려 "ensure CRDs are installed first" 로 실패한다.
#    직접(top-level) 설치하면 CRD 가 정상 적용된다. giosk DCGM PodMonitor 보다 먼저.
install_monitoring(){
  $MONITORING || { log "monitoring 비활성(--no-monitoring) — Prometheus 설치 생략"; return 0; }
  vbool monitoring.install || { log "monitoring.install=false — 외부 Prometheus 사용, kps 설치 생략"; return 0; }
  log "kube-prometheus-stack 설치 (별도 릴리스 'kps', CRD 포함 — 무거워서 수 분)"
  helm repo add prometheus-community https://prometheus-community.github.io/helm-charts >/dev/null 2>&1 || true
  helm repo update >/dev/null
  helm upgrade --install kps prometheus-community/kube-prometheus-stack \
    --version "${KPS_VERSION:-65.5.1}" \
    -n "$NAMESPACE" --create-namespace \
    -f "$SCRIPT_DIR/kps-values.yaml" \
    --wait --timeout 12m
}

# ── 5b) Helm 배포(giosk) ────────────────────────────────────────────
helm_deploy(){
  # giosk 차트는 subchart 없음(모니터링은 별도 릴리스 kps). dependencies 블록 제거 후 설치.
  local tmpchart=/tmp/giosk-chart
  rm -rf "$tmpchart"; cp -r "$CHART" "$tmpchart"
  sed -i '/^dependencies:/,$d' "$tmpchart/Chart.yaml"
  rm -rf "$tmpchart/charts" "$tmpchart/Chart.lock"

  # 게이트웨이 키와 비밀은 멀티라인 PEM 이라 임시 파일로 --set-file 주입한다(고정해서 drift 를 막는다).
  local gwargs=()
  if $WITH_GATEWAY || grep -q 'enabled: *true' <(sed -n '/^gateway:/,/^[a-z]/p' "$VALUES" 2>/dev/null); then
    local gpriv gpub
    gpriv=$(mktemp); gpub=$(mktemp)
    printf '%s' "${GATEWAY_SSH_PRIVATE:-}" > "$gpriv"
    printf '%s' "${GATEWAY_SSH_PUBLIC:-}"  > "$gpub"
    gwargs=(
      --set gateway.enabled=true
      --set image.gateway.tag="$TAG"
      --set gateway.secret="$GATEWAY_SECRET"
      --set-file gateway.sshPrivateKey="$gpriv"
      --set-file gateway.sshPublicKey="$gpub"
    )
    # 세션 SSH 사이드카 이미지는 이 배포의 TAG 로 고정(values 의 :latest 를 덮어씀).
    $WITH_GATEWAY && gwargs+=(--set gateway.sshdImage="giosk-sshd:$TAG")
    # 게이트웨이 오버레이(domain, scheme, LB IP)를 겹쳐 쓴다. base values 뒤에 적용되어 이긴다.
    $WITH_GATEWAY && [ -f "$SCRIPT_DIR/values-gateway-testbed.yaml" ] && gwargs+=(-f "$SCRIPT_DIR/values-gateway-testbed.yaml")
  fi

  log "helm upgrade --install $RELEASE (ns=$NAMESPACE, tag=$TAG)"
  helm upgrade --install "$RELEASE" "$tmpchart" \
    -n "$NAMESPACE" --create-namespace \
    -f "$VALUES" \
    --set image.api.tag="$TAG" \
    --set image.frontend.tag="$TAG" \
    --set image.nodeAgent.tag="$TAG" \
    --set admin.password="$ADMIN_PASSWORD" \
    --set database.password="$DB_PASSWORD" \
    "${gwargs[@]}" \
    --wait --timeout 6m
}

# ── 인프라 번들: values 토글로 각 인프라를 별도 릴리스로 설치(install:true) ↔ 기존 사용(false) ──
# vget: values 파일에서 중첩 스칼라를 뽑는다(2-space 들여쓰기, stdlib 만 쓴다. 랩 python 에 yaml 모듈이 없다).
vget(){
  VF="$VALUES" python3 - "$1" <<'PY'
import os,sys
path=sys.argv[1].split('.'); lines=open(os.environ['VF']).read().splitlines()
ti=0; i=0
for pi,part in enumerate(path):
    hit=False
    while i<len(lines):
        ln=lines[i]; s=ln.strip()
        if not s or s.startswith('#'): i+=1; continue
        ind=len(ln)-len(ln.lstrip())
        if ind<ti: sys.exit(0)
        if ind==ti and s.split(':',1)[0].strip()==part:
            if pi==len(path)-1:
                v=ln.split(':',1)[1].strip().split(' #')[0].strip().strip('\'"'); print(v); sys.exit(0)
            ti+=2; i+=1; hit=True; break
        i+=1
    if not hit: sys.exit(0)
PY
}
vbool(){ [ "$(vget "$1")" = "true" ]; }

install_metallb(){
  vbool metallb.install || { log "MetalLB 번들 off(metallb.install=false) — 기존 LB 사용"; return 0; }
  local range; range=$(vget metallb.ipRange)
  [ -n "$range" ] || die "metallb.install=true 인데 metallb.ipRange 가 비었습니다"
  log "MetalLB 설치 + IPAddressPool($range)"
  helm repo add metallb https://metallb.github.io/metallb >/dev/null 2>&1 || true; helm repo update >/dev/null
  helm upgrade --install metallb metallb/metallb -n metallb-system --create-namespace --wait --timeout 5m
  local ok=false; for _ in 1 2 3 4 5 6; do
    if kubectl apply -f - >/dev/null 2>&1 <<YAML
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata: { name: giosk-pool, namespace: metallb-system }
spec: { addresses: [ "$range" ] }
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata: { name: giosk-l2, namespace: metallb-system }
spec: { ipAddressPools: [ giosk-pool ] }
YAML
    then ok=true; break; fi; sleep 5   # 웹훅 준비 전이면 재시도
  done
  $ok || die "MetalLB IPAddressPool 적용 실패(웹훅 미준비?)"
}

install_nfs_provisioner(){
  vbool nfsProvisioner.install || { log "NFS provisioner 번들 off — 기존 RWX StorageClass 사용"; return 0; }
  local server path sc; server=$(vget nfsProvisioner.server); path=$(vget nfsProvisioner.path); sc=$(vget nfsProvisioner.storageClassName)
  [ -n "$server" ] && [ -n "$path" ] || die "nfsProvisioner.install=true 인데 server/path 가 비었습니다"
  sc=${sc:-nfs}
  log "NFS provisioner 설치 (server=$server path=$path sc=$sc, RWX)"
  helm repo add nfs-subdir https://kubernetes-sigs.github.io/nfs-subdir-external-provisioner >/dev/null 2>&1 || true; helm repo update >/dev/null
  helm upgrade --install nfs-provisioner nfs-subdir/nfs-subdir-external-provisioner \
    -n nfs-provisioner --create-namespace \
    --set nfs.server="$server" --set nfs.path="$path" \
    --set storageClass.name="$sc" --set storageClass.accessModes=ReadWriteMany \
    --wait --timeout 5m
}

install_dcgm(){
  vbool monitoring.dcgm.install || { log "dcgm-exporter 번들 off — GPU Operator 제공 가정"; return 0; }
  log "dcgm-exporter 설치"
  helm repo add gpu-helm-charts https://nvidia.github.io/dcgm-exporter/helm-charts >/dev/null 2>&1 || true; helm repo update >/dev/null
  helm upgrade --install dcgm-exporter gpu-helm-charts/dcgm-exporter -n "$NAMESPACE" --wait --timeout 5m
}

install_hami(){
  vbool hami.install || return 0
  log "HAMi(분할 GPU 스케줄러) 설치"
  helm repo add hami https://project-hami.github.io/HAMi/ >/dev/null 2>&1 || true; helm repo update >/dev/null
  helm upgrade --install hami hami/hami -n "$NAMESPACE" --wait --timeout 5m
}

# ── 실행 ────────────────────────────────────────────────────────────
ensure_storage
install_metallb            # LB IP 공급자(프론트/게이트웨이/레지스트리 LoadBalancer)
install_nfs_provisioner    # RWX StorageClass(영속 home) — NFS 는 모든 모드 필수
load_secrets
if ! $SKIP_BUILD; then
  build_images
  distribute_images
fi
install_monitoring   # CRD 먼저 깔리도록 giosk 보다 앞
install_dcgm         # GPU 메트릭(monitoring.dcgm.install)
install_hami         # 분할 GPU(hami.install)
helm_deploy

# ── 접속 정보 ───────────────────────────────────────────────────────
log "배포 완료"
LB=$(kubectl -n "$NAMESPACE" get svc "${RELEASE}-giosk-frontend" -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)
echo "  프론트엔드 : http://${LB:-<pending: kubectl -n $NAMESPACE get svc>}"
echo "  관리자     : admin / $ADMIN_PASSWORD"
echo "  파드 상태  : kubectl -n $NAMESPACE get pods"
GWLB=$(kubectl -n "$NAMESPACE" get svc "${RELEASE}-giosk-gateway" -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)
if [ -n "$GWLB" ]; then
  echo "  게이트웨이 : $GWLB (웹 80/443, SSH 2222)"
  echo "             와일드카드 DNS *.$(helm -n "$NAMESPACE" get values "$RELEASE" -o json 2>/dev/null | grep -o '"domain":"[^"]*"' | head -1 | cut -d'"' -f4) → $GWLB 로 지정"
  echo "             접속 예: ssh -p 2222 <콘솔에서 발급한 토큰>@$GWLB"
fi
