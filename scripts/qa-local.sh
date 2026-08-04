#!/usr/bin/env bash
# 로컬 검수 베드다. MySQL(도커)과 API(호스트), 프론트(Vite)를 띄운다.
#
# 왜: 실 랩 배포는 tar 로 묶어 scp 하고 이미지를 빌드해 9노드에 배급한 뒤 rollout 까지 5~10분이 걸린다.
#     화면·API 수준의 기능(정책·역할·멤버·피커 등)은 k8s 가 필요 없어 여기서 30초면 확인된다.
#     k8s 가 실제로 필요한 것(세션 스케줄·GPU·hostPath·DaemonSet)만 랩에서 검증한다.
#
# 사용:  ./scripts/qa-local.sh          # 기동(MySQL+API) 후 프론트 실행법 안내
#        ./scripts/qa-local.sh --down   # 정리
#
# 로그인: admin / giosk123  (아래에서 해시를 강제 세팅)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DB=giosk-mysql
API_LOG=/tmp/giosk-api-local.log

if [ "${1:-}" = "--down" ]; then
  pkill -f 'cmd/api' 2>/dev/null || true
  docker rm -f "$DB" >/dev/null 2>&1 || true
  echo "✓ 정리 완료"
  exit 0
fi

docker info >/dev/null 2>&1 || { echo "✗ Docker 를 먼저 실행하세요." >&2; exit 1; }

# 1) MySQL 은 있으면 재사용하고(데이터 보존) 없으면 만든다.
if docker ps -a --format '{{.Names}}' | grep -qx "$DB"; then
  docker start "$DB" >/dev/null
else
  docker run -d --name "$DB" \
    -e MYSQL_ROOT_PASSWORD=dev -e MYSQL_DATABASE=giosk \
    -e MYSQL_USER=giosk -e MYSQL_PASSWORD=giosk \
    -p 3306:3306 mysql:8 >/dev/null
fi
printf '→ MySQL 대기'
for _ in $(seq 1 40); do
  docker exec "$DB" mysqladmin ping -ugiosk -pgiosk >/dev/null 2>&1 && break
  printf '.'; sleep 2
done
echo ' ready'

# 2) API 는 기존 프로세스를 정리한 뒤 기동한다. k8s 에 연결하지 않으므로 세션과 노드 기능은 degrade 된다(정상).
pkill -f 'cmd/api' 2>/dev/null || true
cd "$ROOT/backend"
GIOSK_DB_HOST=127.0.0.1 GIOSK_DB_USER=giosk GIOSK_DB_PASS=giosk GIOSK_DB_NAME=giosk \
GIOSK_CORS_ORIGINS=http://localhost:5173 \
  go run ./cmd/api > "$API_LOG" 2>&1 &

printf '→ API 대기'
for _ in $(seq 1 45); do
  curl -fsS -m 2 http://localhost:8080/api/config >/dev/null 2>&1 && break
  printf '.'; sleep 2
done
echo ' ready'

# 3) admin 비밀번호를 고정한다. 부트스트랩 값이 환경마다 달라 검수 때 매번 막힌다.
#    bcrypt("giosk123", cost 10).
docker exec -i "$DB" mysql -ugiosk -pgiosk giosk >/dev/null 2>&1 <<'SQL'
UPDATE users SET password_hash='$2a$10$5FW6JMAgFIt51aIQUfw.4udnglaCZgmnyB9cE7Cm3/bKAMVHWCHSW',
                 status='approved'
 WHERE username='admin';
SQL

cat <<EOF

✓ 로컬 검수 베드 준비됨
    API   : http://localhost:8080/api   (로그 tail -f $API_LOG)
    로그인 : admin / giosk123

  프론트(다른 터미널):
    cd frontend && npm run dev     → http://localhost:5173

  주의: k8s 미연결이라 세션 생성·노드·GPU·DaemonSet 은 여기서 검증 불가(랩에서 확인).
        거버넌스/정책/역할/멤버/피커/빌링 화면은 전부 여기서 확인 가능.
EOF
