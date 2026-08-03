# Contributing to Giosk

Thanks for your interest in improving Giosk! Contributions of all kinds are welcome —
bug reports, docs, features, and reviews.

한국어 기여 안내는 아래 [기여 안내(한국어)](#기여-안내-한국어)를 참고하세요.

## Getting started

1. Read [`docs/development.md`](docs/development.md) for local setup
   (Korean: [`docs/ko/development.md`](docs/ko/development.md)).
2. Fork the repo and create a topic branch (`feat/…`, `fix/…`, `docs/…`, `chore/…`).
3. Make your change with tests where it makes sense.
4. Ensure checks pass:
   - Backend: `cd backend && go build ./... && go vet ./... && go test ./...`
   - Frontend: `cd frontend && npm run build && npm run lint`
   - Chart: `helm lint charts/giosk`
5. Open a pull request describing **what** and **why**.

## Branching

`main` is always releasable. Work happens on topic branches and lands via a **`--no-ff`
merge**, so each feature stays a visible unit in the history.

| Prefix | Use |
|--------|-----|
| `feat/` | new capability |
| `fix/` | bug fix |
| `docs/` | documentation only |
| `chore/` | build, release, cleanup |

One topic per branch. Delete the branch after it merges — the merge commit keeps the name.

## Commit messages

Commit messages are written in **Korean**, matching the code comments in this repository.

- Subject: what changed, ~50 characters, no trailing period.
- Body (only when it helps): **why**, and any trap worth warning the next reader about.
  The diff already says what changed.
- Tooling changes may use a prefix (`ci:`, `chart:`).

```
GPU 유휴 판정에 전력 보조 신호 추가

GeForce 계열 + 최신 드라이버 조합에서 DCGM 사용률이 0으로 오보고되는
사례를 확인했다. 사용률만 보면 멀쩡히 학습 중인 세션을 회수한다.
```

## Guidelines

- Keep changes focused; one logical change per PR.
- Match the surrounding code's style and comment density. Comments explain **why**, not what.
- Never commit secrets, credentials, kubeconfigs, or internal network details.
  (`.gitignore` covers the common ones — double-check `git diff --cached`.)
- Update docs and `CHANGELOG.md` (Unreleased) when behavior or config changes.
- New DB columns go in a numbered migration under
  `backend/internal/store/migrations/`. There is no down-migration — split destructive
  changes across two releases.

## Reporting bugs / requesting features

Use the GitHub issue templates. Include your environment (Kubernetes distro/version,
GPU stack) and steps to reproduce.

## License

By contributing, you agree that your contributions are licensed under the
[Apache-2.0](LICENSE) license.

---

## 기여 안내 (한국어)

### 시작하기

1. [`docs/ko/development.md`](docs/ko/development.md)에서 로컬 개발 환경을 확인한다.
2. 주제 브랜치를 만든다 (`feat/`, `fix/`, `docs/`, `chore/`).
3. 필요하면 테스트를 함께 작성한다.
4. 아래 검사를 통과시킨다.
   - 백엔드: `cd backend && go build ./... && go vet ./... && go test ./...`
   - 프론트엔드: `cd frontend && npm run build && npm run lint`
   - 차트: `helm lint charts/giosk`
5. **무엇을, 왜** 바꿨는지 적어 PR을 연다.

### 브랜치와 커밋

- `main`은 항상 배포 가능한 상태로 둔다. 직접 커밋하지 않는다.
- 주제 브랜치에서 작업하고 `--no-ff`로 머지한다. 기능이 히스토리에 덩어리로 남는다.
- 커밋 메시지는 한국어로 쓴다. 제목은 50자 안팎, 본문에는 **왜**와 함정을 적는다.
- 머지한 브랜치는 지운다.

### 지켜야 할 것

- 한 브랜치는 한 주제만 다룬다.
- 주변 코드의 스타일과 주석 밀도를 따른다. 주석은 "무엇"이 아니라 "왜"를 적는다.
- 시크릿, kubeconfig, 내부 IP·호스트명은 절대 커밋하지 않는다.
- 동작이나 설정이 바뀌면 문서와 `CHANGELOG.md`(Unreleased)를 함께 갱신한다.
- DB 변경은 번호순 마이그레이션으로만 한다. 롤백 스크립트가 없으므로 파괴적 변경은
  두 릴리스에 나눠서 한다.
