# 개발

## 디렉터리

```
backend/
  cmd/{api,node-agent,gateway}   실행 바이너리
  internal/<도메인>/             도메인별 패키지 (routes.go / handler.go / service.go / repo.go / model.go)
  internal/store/migrations/     SQL 마이그레이션 (번호 순 적용)
frontend/src/
  pages/console/{user,admin}/    화면
  components/console/            공용 컴포넌트
  api/                           백엔드 호출
  locales/                       번역 리소스
charts/giosk/                    Helm 차트
deploy/                          설치 스크립트, values 예시
scripts/                         개발·로컬 QA 헬퍼
```

백엔드는 도메인 패키지마다 같은 파일 구성을 반복한다. 새 도메인을 추가할 때도 이 구조를
따르고, 라우트 등록은 `backend/internal/server/server.go`에서 한다.

## 빌드와 검사

```bash
cd backend
go build ./... && go vet ./... && go test ./...

cd ../frontend
npm ci && npm run build && npm run lint

helm lint charts/giosk
```

CI(`.github/workflows/ci.yml`)가 push와 PR에서 위 셋을 그대로 돌린다. 로컬에서 통과시키고
올리는 것이 빠르다.

## 로컬 QA 베드

클러스터 없이 MySQL + API + Vite만 띄워서 화면·API 흐름을 확인한다. 30초면 뜬다.

```bash
scripts/qa-local.sh      # MySQL + API + 프론트엔드
scripts/qa-seed.sh       # 데모 조직·팀·사용자 시드
scripts/run-api.sh       # 로컬 DB 상대로 API만
scripts/fast-frontend.sh # 프론트엔드만 빠르게
```

쿠버네티스가 필요한 기능(세션 생성, 노드, 지표)은 여기서 확인할 수 없다. 그 경우에만
실제 클러스터에 배포한다. **변경할 때마다 배포하지 말고 로컬에서 검수한 뒤 모아서 한 번에
올린다.**

## 차트 확인

```bash
helm template giosk charts/giosk -f deploy/values.example.yaml \
  --set metallb.ipRange=10.0.0.200-10.0.0.210 \
  --set nfsProvisioner.server=10.0.0.5 --set nfsProvisioner.path=/export \
  --set admin.password=x
```

차트는 안전하지 않은 설정을 렌더 단계에서 거부한다. 새 필수값을 추가하면 그 검증도 함께
넣는다.

## 마이그레이션

`backend/internal/store/migrations/NNNN_이름.sql` 형식으로 번호를 이어서 추가한다.
API 기동 시 순서대로 적용된다. **되돌리는 스크립트는 없으므로** 파괴적인 변경(컬럼 삭제,
타입 축소)은 두 단계로 나눈다 — 먼저 새 컬럼을 추가해 양쪽을 쓰고, 다음 릴리스에서 제거한다.

## 브랜치 전략

```
main                 항상 배포 가능한 상태
  feat/<주제>        기능 추가
  fix/<주제>         버그 수정
  docs/<주제>        문서
  chore/<주제>       빌드·릴리스·정리
```

- `main`에 직접 커밋하지 않는다. 주제 브랜치에서 작업하고 **`--no-ff`로 머지**해서
  기능 단위가 히스토리에 덩어리로 남게 한다.
- 브랜치 하나는 하나의 주제만 다룬다. 다른 게 눈에 띄면 별도 브랜치로 뺀다.
- 머지한 브랜치는 지운다. 머지 커밋에 이름이 남는다.

```bash
git switch -c feat/session-reconfigure
# ... 작업 ...
git switch main
git merge --no-ff feat/session-reconfigure
git branch -d feat/session-reconfigure
```

## 커밋 메시지

한국어로 쓴다. 코드 주석도 한국어이므로 맞춘다.

- **제목** — 50자 안팎, 무엇을 했는지. 마침표 없음.
- **본문** — 필요할 때만. **왜** 그렇게 했는지, 어떤 함정이 있었는지를 적는다.
  무엇을 바꿨는지는 diff에 이미 있다.
- 도구·설정 변경은 접두사를 붙여도 된다 (`ci:`, `chart:`).

```
GPU 유휴 판정에 전력 보조 신호 추가

GeForce 계열 + 최신 드라이버 조합에서 DCGM 사용률이 0으로 오보고되는
사례를 확인했다. 사용률만 보면 멀쩡히 학습 중인 세션을 회수한다.
```

## 코딩 규약

- 주변 코드의 스타일과 주석 밀도를 따른다. 새 파일이라고 다른 스타일을 쓰지 않는다.
- 주석은 "무엇"이 아니라 "왜"를 적는다. 함정이 있는 곳에는 반드시 남긴다.
- 시크릿, kubeconfig, 내부 IP·호스트명을 커밋에 넣지 않는다.
- 오류는 `code` + `message` 형태로 반환하고, 새 코드를 만들면
  `frontend/src/i18n/errorMap.js`에 문구를 추가한다.
- 권한 판정은 `authz.Scope.EffectiveIDs()`로 한다. `org_admin`도 `GroupID`가 채워질 수
  있어서 필드 존재 여부로 레벨을 판단하면 틀린다.
