#!/usr/bin/env bash
# 로컬 검수용 대규모 시드 — 조직/그룹/사용자를 실제 규모로 채운다.
#
# 왜: 사용자가 5명일 땐 안 보이던 문제(계정 검색, 동명이인, 목록 성능, 페이지네이션 부재)가
#     수백 명부터 드러난다. 특히 한국 대학 환경은 동명이인이 흔해서 일부러 많이 겹치게 만든다.
#
# 사용: ./scripts/qa-seed.sh [사용자수]   (기본 300, qa-local.sh 가 먼저 떠 있어야 함)
set -euo pipefail

API=${API:-http://localhost:8080/api}
N=${1:-300}
DB=${DB:-giosk-mysql}

key=$(curl -fsS -X POST "$API/auth/login" -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"giosk123"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["sessionkey"])')
[ -n "$key" ] || { echo "✗ 로그인 실패 — qa-local.sh 를 먼저 실행하세요" >&2; exit 1; }

echo "→ 조직/그룹/사용자 ${N}명 시드"
python3 - "$API" "$key" "$N" <<'PY'
import json, sys, urllib.request, random

api, key, n = sys.argv[1], sys.argv[2], int(sys.argv[3])
random.seed(42)  # 재현 가능하게

def call(method, path, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(api + path, data=data, method=method,
                                 headers={'Content-Type': 'application/json',
                                          'Authorization': 'Bearer ' + key})
    try:
        with urllib.request.urlopen(req) as r:
            return json.loads(r.read() or b'{}')
    except urllib.error.HTTPError as e:
        return {'_err': e.code, '_body': e.read().decode()[:120]}

ORGS = [('ai-center', 'AI응용연구센터'), ('eng-college', '공과대학'), ('med-school', '의과대학')]
GROUPS = ['비전', '자연어처리', '로보틱스', '데이터마이닝', '음성인식', '강화학습']
# 동명이인을 일부러 많이 만든다 — 성 10개 × 이름 12개면 300명에서 대량 중복.
SURNAME = list('김이박최정강조윤장임')
GIVEN = ['민준', '서연', '지우', '서준', '하윤', '도윤', '지호', '수아', '지민', '예준', '수빈', '지훈']

org_ids = {}
for name, disp in ORGS:
    r = call('POST', '/admin/orgs', {'name': name, 'displayName': disp, 'creditPool': 10000})
    if '_err' in r:
        print('  org', name, 'skip', r['_err'])
    org_ids[name] = None

for o in call('GET', '/admin/orgs').get('items', []):
    if o['name'] in org_ids:
        org_ids[o['name']] = o['id']

group_ids = []
for name, _ in ORGS:
    oid = org_ids.get(name)
    if not oid:
        continue
    for g in GROUPS:
        r = call('POST', '/admin/groups', {'orgId': oid, 'name': f'{name}-{g}', 'displayName': f'{g} 연구실'})
        if '_err' in r:
            continue
for g in call('GET', '/admin/groups').get('items', []):
    group_ids.append(g['id'])
print(f'  조직 {len([v for v in org_ids.values() if v])} · 그룹 {len(group_ids)}')

made = dups = 0
seen = {}
for i in range(n):
    nm = random.choice(SURNAME) + random.choice(GIVEN)
    seen[nm] = seen.get(nm, 0) + 1
    if seen[nm] > 1:
        dups += 1
    uname = f'u{i:04d}'
    gid = random.choice(group_ids) if group_ids else None
    r = call('POST', '/admin/users', {
        'username': uname, 'lastName': nm[0], 'firstName': nm[1:],
        'email': f'{uname}@giosk.io', 'role': 'member', 'groupId': gid,
    })
    if '_err' not in r:
        made += 1

print(f'  사용자 {made}명 생성 (동명이인 {dups}명 — 이름만으론 구분 불가)')
top = sorted(seen.items(), key=lambda x: -x[1])[:3]
print('  가장 흔한 이름:', ', '.join(f'{k}×{v}' for k, v in top))
PY

echo "✓ 시드 완료 — http://localhost:5173 에서 확인"
