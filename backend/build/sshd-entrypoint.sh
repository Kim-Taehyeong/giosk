#!/bin/sh
# giosk-sshd 사이드카 entrypoint — 컨테이너 세션 SSH 접속용.
#
# 계약(k8s/mapper.go sshdSidecar 가 주입하는 env):
#   SESSION_UID    로그인 계정 UID(세션 컨테이너와 동일 → 홈 파일 소유 일치)
#   SESSION_USER   로그인 계정명(기본 work)
#   HOME_DIR       공유 홈 마운트 경로(/home/work)
#   GATEWAY_PUBKEY 신뢰할 공개키(OpenSSH 1줄) — 게이트웨이 관리키. 이것만 신뢰한다.
#
# 이 sshd 는 게이트웨이 SSH 프록시(<iid>.<ns>.svc:22)만 상대한다. 사용자는 게이트웨이에
# 1회 토큰으로 인증하고, 게이트웨이가 관리키로 여기에 재접속한다 → 사용자 키 불필요.
set -eu

: "${SESSION_USER:=work}"
: "${HOME_DIR:=/home/work}"

[ -n "${SESSION_UID:-}" ] || { echo "sshd: SESSION_UID 필요" >&2; exit 1; }
[ -n "${GATEWAY_PUBKEY:-}" ] || { echo "sshd: GATEWAY_PUBKEY 필요(신뢰 공개키 없음)" >&2; exit 1; }

# 계정 생성 — 홈은 이미 마운트돼 있으므로 만들지 않는다(-H).
addgroup -g "$SESSION_UID" "$SESSION_USER" 2>/dev/null || true
adduser -D -H -u "$SESSION_UID" -G "$SESSION_USER" -h "$HOME_DIR" -s /bin/sh "$SESSION_USER" 2>/dev/null || true
# busybox adduser -D 는 shadow 를 '!'(잠금)로 둔다 → sshd 가 잠긴 계정 로그인을 거부.
# '*'(비밀번호 로그인 불가, 잠금 아님)로 바꿔 공개키 인증만 허용한다.
sed -i "s|^${SESSION_USER}:!:|${SESSION_USER}:*:|" /etc/shadow

# authorized_keys 는 홈이 아닌 root 소유 경로에 둔다 — 공유 홈은 세션 사용자가 쓸 수 있어
# 홈에 두면 사용자가 신뢰 키를 바꿔치기할 수 있다(sshd_config AuthorizedKeysFile 이 여기를 가리킴).
mkdir -p /etc/ssh/authorized_keys
printf '%s\n' "$GATEWAY_PUBKEY" > "/etc/ssh/authorized_keys/$SESSION_USER"
chown -R root:root /etc/ssh/authorized_keys
# 읽기는 열되(sshd 는 privsep 후 비-root 로 authorized_keys 를 읽는다 → root-only 700/600 이면 "Permission denied"),
# 쓰기는 root 로만 제한한다. StrictModes 는 group/other '쓰기'만 금지하므로 755/644 가 안전하다.
chmod 755 /etc/ssh/authorized_keys
chmod 644 "/etc/ssh/authorized_keys/$SESSION_USER"

# 홈 마운트 소유가 다르면(사전 생성된 PVC 등) 로그인 후 쓰기가 막힌다 — 최상위만 맞춘다(재귀 X: NFS 홈이 클 수 있음).
chown "$SESSION_UID:$SESSION_UID" "$HOME_DIR" 2>/dev/null || true

# 호스트키는 파드 수명주기 동안만 유효(게이트웨이가 InsecureIgnoreHostKey 로 접속 — 인클러스터 신뢰망).
ssh-keygen -A >/dev/null

# AllowUsers 를 런타임 계정으로 고정(그 외 계정 로그인 차단).
echo "AllowUsers $SESSION_USER" > /etc/ssh/sshd_config.d/allow-user.conf

exec /usr/sbin/sshd -D -e
