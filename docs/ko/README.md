# Giosk 문서 (한국어)

Giosk는 쿠버네티스 클러스터를 **GPU 셀프서비스 클라우드**로 바꾸는 컨트롤 플레인이다.
사용자는 콘솔에서 VSCode / Jupyter / 웹 터미널 세션을 띄우고, 관리자는 크레딧·쿼터·조직
단위로 자원을 통제한다.

영문 문서는 [`docs/`](../) 아래에 있다. 이 디렉터리가 더 자세하다.

## 읽는 순서

| 문서 | 대상 | 내용 |
|------|------|------|
| [architecture.md](architecture.md) | 개발자 | 구성 요소, 세션 수명주기, 자원 모델, 데이터 흐름 |
| [installation.md](installation.md) | 운영자 | 사전 요구사항, 설치 두 가지 경로, 설치 확인 |
| [configuration.md](configuration.md) | 운영자 | Helm values 레퍼런스, 운영 모드, 정책 계층 |
| [operations.md](operations.md) | 운영자 | 일상 운영, 모니터링·알림, 백업, 업그레이드 |
| [troubleshooting.md](troubleshooting.md) | 운영자 | 실제로 겪은 장애와 원인·해결 |
| [api.md](api.md) | 개발자 | REST API 레퍼런스 |
| [development.md](development.md) | 기여자 | 로컬 개발 환경, 테스트, 브랜치·커밋 규약 |
| [adr/](adr/) | 개발자 | 주요 설계 결정과 그 이유 |
| [roadmap.md](roadmap.md) | 모두 | 알려진 한계와 다음에 할 것 |

## 용어

프로젝트 전반에서 아래 용어를 일관되게 쓴다.

- **세션(session)**: 사용자가 쓰는 작업 환경 하나. 컨테이너 세션은 Pod, 물리 세션은
  노드 임대 + 리눅스 계정이다.
- **전용(exclusive) GPU**: GPU 카드를 통째로 한 세션에 붙인다.
- **공유(shared) GPU**: HAMi vGPU로 VRAM/코어를 쪼개 여러 세션이 나눠 쓴다.
- **오퍼링(offering)**: 공유 GPU를 얼마나 떼어 줄지 미리 정의해 둔 규격(GPU 타입 + VRAM + 코어 비율).
- **조직(org) / 팀(group)**: 2단 계층. 크레딧과 정책이 조직에서 팀으로, 팀에서 사용자로 내려온다.
- **크레딧(credit)**: 세션 실행 시간에 GPU 단가를 곱해 차감하는 내부 화폐.
- **임대(lease)**: 물리 노드를 통째로 한 사용자에게 빌려주는 상태.
