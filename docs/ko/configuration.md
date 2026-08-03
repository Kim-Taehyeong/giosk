# 설정

설치 시점 설정은 전부 Helm values로 준다. 시작점은
[`deploy/values.example.yaml`](../../deploy/values.example.yaml), 전체 항목과 주석은
[`charts/giosk/values.yaml`](../../charts/giosk/values.yaml)에 있다.

설치 후에 바꿀 수 있는 것과 고정되는 것을 구분해 두는 게 중요하다.

- **설치 시 고정** — 배포 모드, 스토리지 클래스, 번들 인프라 토글, 게이트웨이 도메인.
  바꾸려면 `helm upgrade`가 필요하다.
- **런타임 변경** — 정책 한도, GPU 단가, 알림 규칙, 브랜딩, 기능 토글 일부.
  콘솔의 시스템 설정 화면에서 바꾸며 DB에 저장된다.

## 운영 모드

| 키 | 값 | 의미 |
|----|-----|------|
| `deployment.mode` | `container` \| `hybrid` | `hybrid`면 물리 노드 SSH 임대 세션이 추가로 열린다 |
| `billing.mode` | `credit` \| `dynamic` \| `free` | 크레딧 차감 / 선착순 임대 / 무제한 |

- **credit** — 세션 실행 시간 × GPU 단가만큼 차감한다. 잔액이 떨어지면 세션을 자동 중지한다.
  조직 → 팀 → 멤버 순으로 크레딧을 배분한다.
- **dynamic** — 크레딧 없이 선착순으로 빌려 쓰고 만료되면 회수한다.
  `billing.dynamic.maxLeaseHours`, `cooldownHours`로 제한한다.
- **free** — 제한 없음. 초기 구축·검증 단계에 쓴다.

## 인프라 번들 토글

각 의존 인프라는 Giosk가 설치하거나(`install: true`) 이미 있다고 가정한다(`install: false`).

| 키 | 기본값 | 비고 |
|----|--------|------|
| `monitoring.install` | `true` | kube-prometheus-stack. `false`면 `monitoring.prometheusURL` 필요 |
| `monitoring.dcgm.install` | `true` | dcgm-exporter. GPU Operator가 이미 제공하면 `false` |
| `metallb.install` | `true` | 베어메탈 LB. `metallb.ipRange` 필수. 클라우드면 `false` |
| `nfsProvisioner.install` | `true` | RWX StorageClass 생성. `server`/`path` 필수 |
| `hami.install` | `false` | 공유(분할) GPU 스케줄러. 선택 |
| `registry.install` | `false` | 이미지 빌드용 인클러스터 레지스트리. 선택 |

## 스토리지

```yaml
storage:
  localClass: local-path              # 세션 홈(노드 로컬 영속) StorageClass
  persistence:
    storageClass: nfs                 # 영속 홈 ~/nfs (RWX) — 모든 모드에서 필수
  datasets:
    enabled: true
    nfs: { server: ..., path: ... }   # 공유 데이터셋 export
  scratch:
    enabled: false                    # 노드 로컬 고속 임시 공간(/scratch)
```

`storage.persistence.storageClass`는 번들 설치 시 `nfsProvisioner.storageClassName`과
반드시 일치해야 한다. 불일치하면 세션이 PVC 바인딩 대기에서 멈춘다.

디스크가 차는 것을 막는 정리 DaemonSet도 있다.

```yaml
nodeCleaner:
  enabled: true
  scratchThreshold: 85     # scratch 는 계약상 임시 공간 → 바로 정리
  homeThreshold: 92        # 세션 홈은 더 늦게, 최근 파일은 보호
  homeMinAgeDays: 3
```

## GPU 라벨

```yaml
k8s:
  gpuTypeLabel: nvidia.com/gpu.product        # GPU 종류 (GFD)
  cudaLabel: nvidia.com/cuda.driver-version   # CUDA 버전 (GFD)
```

공유 오퍼링의 `gpu_type`은 **노드의 `nvidia.com/gpu.product` 라벨 값과 정확히 같아야 한다.**
다르면 전용 GPU는 잡히는데 공유만 "가용 없음"으로 보이는, 원인을 찾기 어려운 증상이 난다.

## 정책 한도

한도는 전역 → 조직 → 팀 → 사용자 순으로 계층 적용된다. 아래는 전역 기본값(하드 상한)이고,
실제 값은 콘솔 정책 화면에서 조직·팀·사용자별로 덮어쓴다.

```yaml
quota:
  maxGpuCount: 64
  maxVramGb: 512
  maxConcurrentSessions: 50
  maxStoppedSessions: 5    # 0 = 무제한. 중단 세션도 로컬 홈 PVC 로 디스크를 점유한다
  volumeQuotaGb: 2000
```

`maxStoppedSessions`와 임시 디스크 상한은 **0이 "무제한"이라는 유효한 설정**이다.
1 이상을 강제하지 않는다.

## 세션 노출

```yaml
session:
  expose: nodeport         # nodeport | loadbalancer
  ssh: { ... }             # 컨테이너 직접 SSH
idle:
  timeoutMin: 30           # 유휴 회수 기준
```

## 접속 게이트웨이 (선택)

`gateway.enabled: true`면 세션별 서브도메인 웹 접속과 SSH 프록시를 단일 접점으로 묶는다.
`*.<gateway.domain>` 와일드카드 DNS가 필요하고, HTTPS면 TLS 시크릿도 필요하다.
비밀값(공유 토큰키, SSH 개인키)은 비워 두면 `deploy.sh`가 생성한다.

## 브랜딩·기능 토글

```yaml
branding: { name: ..., accent: ... }
features:
  signupRequest: true      # 가입 요청
  datasetRegister: true    # 사용자 데이터셋 등록 요청
  workloadAlerts: true     # 세션 사용량 알림
  groupJoinRequest: true
  creditRequest: true
```

## 환경변수 대응

차트가 만드는 ConfigMap/Secret은 `GIOSK_*` 환경변수로 API에 전달된다. 차트 없이 직접
띄울 때 참고할 이름들은 `backend/internal/config/config.go`에 모여 있다
(예: `GIOSK_BILLING_MODE`, `GIOSK_NFS_CLASS`, `GIOSK_GPU_TYPE_LABEL`, `GIOSK_IDLE_TIMEOUT_MIN`).
