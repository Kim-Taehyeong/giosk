#!/usr/bin/env bash
# 프론트 전용 빠른 반영 — 이미지 재빌드/9노드 배급/rollout 없이 dist 를 실행 중 파드에 직접 복사.
# 테스트 환경 전용(파드 재시작 시 사라짐 = ephemeral). 백엔드 변경엔 못 씀(그건 full deploy).
#
# 사용:  ./scripts/fast-frontend.sh
# 소요:  ~10초 (npm build 제외)
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SSH=gk-control
NS=giosk

echo "▶ 빌드 (VITE_API_URL=/api — 같은 오리진; MSYS_NO_PATHCONV 필수: Git Bash 가 /api 를 C:/Program Files/Git/api 로 바꿔버림)"
( cd "$ROOT/frontend" && MSYS_NO_PATHCONV=1 MSYS2_ARG_CONV_EXCL='*' VITE_API_URL=/api npm run build >/dev/null 2>&1 )

echo "▶ 전송"
tar czf /tmp/giosk-dist.tgz -C "$ROOT/frontend/dist" .
scp -q /tmp/giosk-dist.tgz "$SSH":/tmp/giosk-dist.tgz

echo "▶ 파드에 복사(rollout 없음)"
ssh "$SSH" '
  NS=giosk
  POD=$(kubectl -n $NS get pod --no-headers | grep frontend | awk "{print \$1}" | head -1)
  kubectl -n $NS cp /tmp/giosk-dist.tgz "$POD":/tmp/giosk-dist.tgz
  kubectl -n $NS exec "$POD" -- sh -c "cd /usr/share/nginx/html && tar xzf /tmp/giosk-dist.tgz && rm /tmp/giosk-dist.tgz"
  echo "✓ 반영됨: $POD"
'
echo "✓ 완료 — 브라우저 새로고침(캐시 강제: Ctrl+Shift+R)"
