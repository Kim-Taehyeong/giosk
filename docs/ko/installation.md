# 설치

## 사전 요구사항

설치 전에 아래가 **이미 있어야 한다**. Giosk가 대신 만들어 주지 않는다.

- **쿠버네티스 클러스터**: 모든 스케줄 가능한 노드에서 CNI가 정상 동작해야 한다.
- **NFS 서버**: 쓰기 가능한 export 하나. 어떤 모드에서도 필요하다(영속 홈 `~/nfs`가 RWX).
- **GPU 노드** (GPU 세션을 쓸 경우): NVIDIA 드라이버, 컨테이너 런타임, device-plugin,
  GPU Feature Discovery 라벨(`nvidia.com/gpu.product`). 이게 없으면 CPU 세션만 뜬다.
- **LoadBalancer**: 베어메탈이면 MetalLB에 줄 유휴 IP 대역, 클라우드면 provider LB.
- `helm`, `kubectl`. 자체 호스팅 설치 스크립트를 쓸 경우 컨트롤 노드에 `ctr`.

나머지(MetalLB, NFS provisioner, kube-prometheus-stack, DCGM, HAMi, 레지스트리)는 Giosk가
**번들 설치**할 수 있다. values의 `install: true/false` 토글로 켜고 끈다.

> 실제로 가장 많이 막히는 지점은 MetalLB에 줄 **유휴 IP 대역 확보**다. 사내망에서
> 할당받지 못했다면 `metallb.install: false` + `session.expose: nodeport`로 먼저 띄우고
> 나중에 전환하는 편이 빠르다.

## 설정 파일 준비

```bash
cp deploy/values.example.yaml my-values.yaml
$EDITOR my-values.yaml     # NFS 서버/경로, MetalLB 대역, 모드, GPU 라벨
```

필수 값은 설치 시점에 강제된다. NFS StorageClass가 비었거나 `metallb.install=true`인데
`ipRange`가 없으면 차트가 렌더 자체를 거부한다. 설치 후에 발견하는 것보다 낫다.

## 설치 경로 두 가지

### A. 자체 호스팅 설치 스크립트 (폐쇄망 친화, 레지스트리 불필요)

컨트롤 노드에서 이미지를 직접 빌드하고 `ctr import`로 각 노드에 배포한다.

```bash
sudo VALUES=./my-values.yaml ./deploy/deploy.sh
# 옵션: --with-gateway  --with-node-agent  --skip-build  --no-monitoring
```

### B. Helm 직접 설치 (이미지를 레지스트리에서 받을 수 있는 경우)

```bash
helm install giosk charts/giosk -f my-values.yaml --set admin.password=<PW>
```

이 경로에서는 `image.*.repository`를 본인 레지스트리(예: `ghcr.io/<org>/giosk-api`)로
바꿔야 한다.

## k3s에서 설치할 때

k3s는 flannel·ServiceLB·local-path가 내장이라 오히려 간단하다. 두 가지만 다르다.

- kubeconfig: `export KUBECONFIG=/etc/rancher/k3s/k3s.yaml`
- 이미지 임포트: `k3s ctr images import` (k3s는 자체 containerd 소켓을 쓴다)
- LoadBalancer: ServiceLB로 대체할 수 있다(`metallb.install: false`)
- 스토리지: local-path는 RWO다. 영속 홈은 RWX가 필요하므로
  `nfsProvisioner.install: true`는 그대로 둔다.

## 설치 확인

```bash
kubectl -n giosk get pods
kubectl -n giosk get svc giosk-giosk-frontend      # 콘솔 접속 주소
```

확인 순서:

1. `giosk-api` Pod가 Running인지, 로그에 마이그레이션 완료가 찍혔는지
2. 콘솔에 `admin` 계정으로 로그인되는지
3. 관리자 콘솔의 노드 화면에서 GPU 노드와 GPU 개수가 보이는지
   (안 보이면 GFD 라벨부터 확인한다. `kubectl get nodes --show-labels | grep gpu.product`)
4. CPU 세션 하나를 띄워 웹 터미널까지 열리는지
5. 그 다음에 GPU 세션

## 처음 설치할 때 권장 순서

한 번에 전부 켜면 어디가 문제인지 알 수 없다. 실제 구축에서 효과가 있었던 순서다.

1. `billing.mode: free`, `session.expose: nodeport`, 번들 인프라는 전부 `install: false`로
   최소 구성으로 설치하고 세션이 뜨는 것까지 확인
2. `monitoring.install: true` 와 DCGM 을 켜고 노드·세션 지표가 채워지는지 확인
3. HAMi 를 켜고 공유 GPU 오퍼링 가용량이 소수로 잡히는지 확인
4. MetalLB 와 게이트웨이로 외부 접속 경로 정리
5. 마지막에 `billing.mode: credit`으로 전환하고 조직·팀·크레딧을 구성

## 업그레이드

```bash
helm upgrade giosk charts/giosk -f my-values.yaml
```

DB 마이그레이션은 API 기동 시 자동 적용된다. 되돌리는 스크립트는 없으므로 업그레이드 전
MySQL 덤프를 받아 두는 것을 권한다([operations.md](operations.md#백업)).
