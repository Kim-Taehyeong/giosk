# Giosk (GPU Kiosk)

**쿠버네티스 네이티브 오픈소스 GPU 클라우드.**

Giosk는 쿠버네티스 클러스터를 셀프서비스 GPU 클라우드로 바꾼다. 사용자는 콘솔에서
전용 또는 공유 GPU 위에 VSCode / Jupyter / 터미널 세션을 직접 띄우고, 관리자는 크레딧,
쿼터, 조직·팀 계층으로 자원을 통제한다.

> 세션은 **실제 쿠버네티스 Pod**로 뜬다. 네이티브 스케줄링, RBAC, PVC 스토리지, GPU 스택
> (device-plugin, GFD, HAMi, DCGM)을 그대로 쓴다. 별도 VM도, 자체 스케줄러도 없다.

[English README](README.md) · [한국어 문서](docs/ko/)

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
![Kubernetes](https://img.shields.io/badge/Kubernetes-native-326CE5?logo=kubernetes&logoColor=white)

---

## 왜 만들었나

대학·연구실에서 GPU 서버는 대개 이렇게 쓰인다. 관리자가 계정을 만들어 주고, 사용자는 SSH로
들어가서 눈치껏 `nvidia-smi`를 쳐 보고 빈 GPU를 찾는다. 누가 얼마나 썼는지는 아무도 모르고,
잡아만 두고 안 쓰는 세션은 방치된다. 쓸 만한 상용 솔루션은 학내 예산으로 감당하기 어렵다.

Giosk는 이미 있는 쿠버네티스 클러스터 위에 얹어서, 그 흐름을 셀프서비스로 바꾼다.

## 주요 기능

- **셀프서비스 GPU 세션**: VSCode(code-server), Jupyter, 브라우저 터미널을 사용자별 Pod로.
- **유연한 GPU 모드**: 전용(카드 통째), 공유(HAMi vGPU로 VRAM·코어 분할), CPU 전용,
  물리 노드 SSH 임대(hybrid).
- **데이터셋과 볼륨**: NFS(RWX) 데이터셋과 노드 로컬 캐시, 영속 홈(`~/nfs`), 공유 볼륨.
- **과금과 거버넌스**: 크레딧 회계(GPU 종류별 단가, 정기 리필), 조직에서 팀, 사용자로 이어지는
  계층 배분, 정책 한도, 감사 로그.
- **관측과 알림**: Prometheus + DCGM 기반 대시보드(GPU 사용률/VRAM/온도), 규칙 기반 알림
  (노드 다운, 디스크, GPU 온도, 크레딧 잔액, 세션 사용량)을 메일·웹훅·인앱으로.
- **접속 게이트웨이(선택)**: 세션별 서브도메인 웹 접속과 복사·붙여넣기 SSH를 단일 접점으로.

## 구성

- **백엔드** (`backend/`): Go(Gin) API + `node-agent`. client-go로 Pod/Job/PVC/Service/exec을
  제어한다. 도메인 상태(사용자, 세션, 조직, 지갑)는 **MySQL**에 있다.
- **프론트엔드** (`frontend/`): React 콘솔(관리자 + 사용자).
- **차트** (`charts/giosk/`): 플랫폼 전체 Helm 차트. 인프라 의존성(MetalLB, NFS provisioner,
  Prometheus, DCGM, HAMi)은 `install: true/false`로 켜고 끈다.

## 빠른 시작

**사전 요구사항** (설치 전에 있어야 한다)

- 모든 노드에 CNI가 동작하는 쿠버네티스 클러스터
- 쓰기 가능한 **NFS 서버**. 어떤 모드에서도 필요하다(영속 홈이 RWX)
- GPU를 쓰려면: NVIDIA 드라이버 + 컨테이너 런타임 + device-plugin + GFD 라벨
- 베어메탈에서 LoadBalancer를 쓰려면 MetalLB에 줄 IP 대역

```bash
# 1) 예시 values 를 복사해 NFS 서버, LB 대역 등을 채운다
cp deploy/values.example.yaml my-values.yaml
$EDITOR my-values.yaml

# 2a) 자체 호스팅 설치 스크립트 (노드에서 직접 빌드, 레지스트리 불필요)
sudo VALUES=./my-values.yaml ./deploy/deploy.sh

# 2b) 또는 Helm 직접 설치 (이미지를 레지스트리에서 받을 수 있을 때)
helm install giosk charts/giosk -f my-values.yaml --set admin.password=<PW>
```

자세한 절차와 처음 설치할 때 권장 순서는 [설치 문서](docs/ko/installation.md)에 있다.

## 문서

| 문서 | 내용 |
|------|------|
| [아키텍처](docs/ko/architecture.md) | 구성 요소, 세션 수명주기, 자원 모델 |
| [설치](docs/ko/installation.md) | 사전 요구사항, 설치, 확인 |
| [설정](docs/ko/configuration.md) | Helm values 레퍼런스, 운영 모드, 정책 |
| [운영](docs/ko/operations.md) | 모니터링, 알림, 자원 회수, 백업, 업그레이드 |
| [문제 해결](docs/ko/troubleshooting.md) | 실제로 겪은 장애와 원인 |
| [API](docs/ko/api.md) | REST API 레퍼런스 |
| [개발](docs/ko/development.md) | 로컬 환경, 브랜치·커밋 규약 |
| [설계 결정(ADR)](docs/ko/adr/) | 주요 결정과 그 이유 |

## 기여

이슈와 PR을 환영한다. [CONTRIBUTING.md](CONTRIBUTING.md)와
[행동 강령](CODE_OF_CONDUCT.md)을 먼저 봐 달라. 보안 취약점은 [SECURITY.md](SECURITY.md)의
절차를 따른다.

## 라이선스

[Apache-2.0](LICENSE)
