# REST API 레퍼런스

라우트 등록은 `backend/internal/*/routes.go`에 모여 있다. 이 문서는 그 목록을 사람이 읽는
순서로 정리한 것이다. 새 라우트를 추가하면 이 문서도 같이 갱신한다.

## 접두사와 권한

| 접두사 | 인증 | 권한 |
|--------|------|------|
| `/api/...` | 일부 공개, 나머지는 로그인 필요 | 일반 사용자 |
| `/api/console/...` | 로그인 | 관리자(조직/팀). 요청 스코프를 미들웨어가 검증 |
| `/api/admin/...` | 로그인 | 최고 관리자(`super`) |
| `/api/agent/...` | 토큰 | node-agent 전용 |

겸직 관리자는 `X-Console-Scope` 헤더로 현재 조직/팀 컨텍스트를 지정한다.
헤더가 없으면 사용자의 대표 소속을 쓴다.

응답 형식은 JSON이다. 오류는 아래 형태로 통일한다.

```json
{ "code": "no_capacity", "message": "요청한 자원의 여유가 없습니다" }
```

프론트엔드는 `code`로 분기하고 `message`는 그대로 보여준다. 새 오류 코드를 만들면
`frontend/src/i18n/errorMap.js`에 문구를 추가해야 번역이 붙는다.

## 인증

| 메서드 | 경로 | 설명 |
|--------|------|------|
| POST | `/api/auth/login` | 로그인(세션 쿠키 발급) |
| POST | `/api/auth/signup` | 가입 요청 (`features.signupRequest`) |
| GET | `/api/auth/logout` | 로그아웃 |
| GET | `/api/auth/me` | 현재 사용자·역할·소속 |
| PUT | `/api/auth/me/password` | 비밀번호 변경 |
| PUT | `/api/auth/me/ssh-key` | SSH 공개키 등록 |
| POST | `/api/auth/me/ssh-key/generate` | 키쌍 생성(개인키는 응답에서 한 번만 내려간다) |
| GET | `/api/config` | 설치 시 고정된 설정 공개(브랜딩, 모드, 기능 토글) |
| GET | `/api/public/orgs` | 가입 화면용 조직 목록 |

## 세션

사용자 경로는 `/instances`, 관리자 경로는 `/sessions`로 이름이 다르다.

| 메서드 | 경로 | 설명 |
|--------|------|------|
| GET | `/api/instances` | 내 세션 목록(현재 팀 스코프) |
| POST | `/api/instances` | 세션 생성. 승인은 직렬화되며 자원이 없으면 즉시 거절 |
| DELETE | `/api/instances/:id` | 삭제(홈 PVC까지 정리) |
| POST | `/api/instances/:id/stop` | 중지(홈 데이터 유지) |
| POST | `/api/instances/:id/start` | 재개 |
| POST | `/api/instances/:id/reconfigure` | 중단 세션의 계산자원 변경(GPU 붙이기/떼기) |
| POST | `/api/instances/:id/extend` | 임대 연장 |
| GET | `/api/instances/:id/connection` | 접속 정보(웹 URL, SSH 명령, 토큰) |
| POST | `/api/instances/:id/access` | 접속 토큰 재발급 |
| GET | `/api/instances/:id/metrics` | 세션 사용량(GPU/CPU/VRAM) |
| GET | `/api/instances/:id/logs` | 컨테이너 로그 |
| GET | `/api/instances/:id/history` | 상태 변화 이력 |
| GET | `/api/instances/:id/audit` | 세션 관련 감사 로그 |
| GET | `/api/instances/:id/terminal` | **WebSocket.** 웹 터미널(xterm) |
| GET | `/api/metrics/sessions` | 내 사용량 요약 |

`POST /api/instances` 요청 예:

```json
{
  "name": "llm-finetune",
  "imageId": 3,
  "gpuMode": "shared",
  "offeringId": 7,
  "gpuType": "NVIDIA GeForce RTX 4090",
  "gpuCount": 1,
  "node": "gpu2-1",
  "volumeIds": [12],
  "datasetIds": [4]
}
```

`gpuMode`는 `shared` | `exclusive` | `cpu`. `shared`면 `offeringId`가 필요하고,
`exclusive`면 `gpuType` + `gpuCount`를 쓴다. `node`를 주면 그 노드에 하드 핀된다.

관리자용:

| 메서드 | 경로 | 설명 |
|--------|------|------|
| GET | `/api/admin/sessions` | 전체 세션 |
| GET | `/api/admin/sessions/metrics` | 전체 사용량 |
| GET | `/api/admin/sessions/:id/describe` | Pod describe(스케줄 실패 원인 확인용) |
| POST | `/api/admin/sessions/:id/stop` · `/terminate` | 중지 / 강제 종료 |

## 자원과 가용량

| 메서드 | 경로 | 설명 |
|--------|------|------|
| GET | `/api/resources/availability` | 전용/공유 GPU 가용량. 전용은 정수, 공유는 소수 |
| GET | `/api/offerings` | 공유 GPU 오퍼링 목록 |
| GET | `/api/presets` | CPU/메모리 프리셋 |
| GET | `/api/gpu-types` | 클러스터에 존재하는 GPU 종류(GFD 라벨 기준) |
| POST/PUT/DELETE | `/api/admin/offerings[/:id]` | 오퍼링 관리 |
| POST/PUT/DELETE | `/api/admin/presets[/:id]` | 프리셋 관리 |
| GET/PUT | `/api/admin/gpu-pricing` | GPU 종류별 크레딧 단가 |

## 노드

| 메서드 | 경로 | 설명 |
|--------|------|------|
| GET | `/api/nodes/physical` | 임대 가능한 물리 노드(사용자용) |
| GET | `/api/admin/nodes` | 노드 목록·상태·디스크 |
| GET | `/api/admin/nodes/:name/gpus` · `/metrics` | 노드 GPU·지표 |
| PUT | `/api/admin/nodes/:name` | 노드 설정(공유 모드, scratch 허용 등) |
| POST | `/api/admin/nodes/:name/cordon` · `/uncordon` | 스케줄 차단·해제 |
| POST | `/api/admin/nodes/:name/lease` | 물리 노드 임대 생성 |
| POST | `/api/admin/node-leases/:id/extend` · `/release` | 임대 연장·반납 |
| GET | `/api/agent/leases` | node-agent가 자기 노드의 임대를 조회(토큰 인증) |

## 조직 · 팀 · 사용자

| 메서드 | 경로 | 설명 |
|--------|------|------|
| GET | `/api/me/groups` · `/me/membership-context` | 내 소속과 현재 컨텍스트 |
| GET/POST/DELETE | `/api/me/join-requests[/:reqId]` | 팀 가입 요청 |
| GET | `/api/console/members` · POST | 팀 멤버 조회·추가 |
| PUT | `/api/console/members/:userId/role` | 멤버 역할 변경 |
| GET/POST/PUT/DELETE | `/api/admin/orgs[/:id]` | 조직 관리 |
| GET/POST/PUT/DELETE | `/api/admin/groups[/:id]` | 팀 관리 |
| PUT | `/api/admin/groups/:id/members/:userId/move` | 멤버 팀 이동 |
| GET/PUT | `/api/admin/join-policy` | 가입 승인 정책 |

조직에 사용자가 남아 있거나 팀에 멤버가 남아 있으면 삭제가 거절된다.
`users.status = approved`(가입 승인)와 `memberships.status = active`(소속 활성)는
서로 다른 값이다. 혼동하기 쉬우니 쿼리 작성 시 주의한다.

## 크레딧 · 과금

크레딧은 **(사용자, 팀) 단위 지갑**이다. 겸직 사용자는 팀별로 잔액이 따로 있다.

| 메서드 | 경로 | 설명 |
|--------|------|------|
| GET | `/api/me/wallet` | 내 지갑(현재 팀) |
| GET | `/api/console/wallet` | 팀 지갑 |
| POST | `/api/console/wallet/allocate` | 팀 풀에서 멤버에게 배분 |
| PUT | `/api/console/wallet/refill` · POST `/wallet/refill/now` | 팀 정기 리필 · 즉시 리필 |
| PUT | `/api/console/members/:userId/refill` | 멤버별 정기 리필 금액 |
| POST | `/api/console/members/:userId/refill/now` | 멤버 즉시 리필 |
| POST | `/api/admin/orgs/:id/grant` · `/groups/:id/wallet/grant` | 상위 풀 부여 |
| POST | `/api/admin/users/:id/grant-credit` | 개인 예외 부여(상위 풀 백필 포함) |
| GET/POST | `/api/me/topup-requests`, `/api/admin/topup-requests/:reqId/approve` | 충전 요청·승인 |
| GET | `/api/admin/billing` | 사용량·비용 집계(기간·팀 스코프) |

크레딧 부여는 상위를 자동으로 채운다. 조직이 100인데 팀에 50을 주면 조직 풀이 부족할 때
조직을 150으로 올리는 식으로 백필한다.

## 데이터셋 · 이미지 · 볼륨

| 메서드 | 경로 | 설명 |
|--------|------|------|
| GET | `/api/datasets` | 데이터셋 목록(노드별 캐시 상태 포함) |
| POST | `/api/datasets/register` | 등록 요청(사용자) |
| GET | `/api/admin/datasets/inbox` | NFS 반입함 조회 |
| POST | `/api/admin/datasets/register-nfs` · `register-url` | 반입함/URL로 등록 |
| POST | `/api/admin/datasets/:id/cache` | 노드 로컬 캐시 토글 |
| GET/POST/PUT/DELETE | `/api/images...` | 세션 이미지 |
| POST | `/api/admin/images` · `/images/external` | 빌드(Kaniko) · 외부 이미지 등록 |
| POST | `/api/admin/images/:id/rebuild` | 재빌드 (Dockerfile이 있는 자작 이미지만) |
| GET | `/api/admin/images/:id/logs` | 빌드 로그 |
| GET/POST/DELETE | `/api/volumes[/:id]` | 볼륨 목록·생성·삭제 |
| POST | `/api/volumes/:id/share` | 볼륨 공유. RW/RO 권한은 서버가 강제한다 |
| PUT | `/api/volumes/:id/team` | 볼륨 소속 팀 변경(쿼터·과금 귀속이 따라간다) |
| GET | `/api/admin/storage` | 노드 디스크·NFS·사용자별 할당 현황 |

## 정책 · 알림 · 감사

| 메서드 | 경로 | 설명 |
|--------|------|------|
| GET | `/api/admin/limits` | 전 범위 정책 한 목록 |
| PUT | `/api/admin/limits/global` | 전역 상한 |
| GET/PUT | `/api/admin/limits/{user,group,org}/:id` | 계층별 한도 |
| GET | `/api/admin/limits/user/:id/effective` | 실제 적용되는 유효 한도 |
| GET/POST | `/api/alerts`, `/api/alerts/:id/toggle` | 알림 규칙 |
| GET/PUT | `/api/notify` | 알림 수신 설정(웹훅/메일) |
| GET | `/api/admin/audit` | 감사 로그 |
| GET | `/api/dashboard`, `/api/admin/dashboard` | 사용자·운영 대시보드 |
