#!/usr/bin/env python3
"""랩에 대규모 계정을 시드하고 목록·검색 응답을 잰다.

사용자가 몇 명일 땐 안 보이던 것(검색, 동명이인, 페이지네이션 부재, 목록 쿼리)이
수백 명부터 드러난다. 한국 대학 환경은 동명이인이 흔하므로 일부러 많이 겹치게 만든다.

계정 아이디는 qa-s 로 시작한다. 나중에 골라내 지울 수 있게 접두사를 고정한다.
"""
import argparse
import json
import random
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

PREFIX = "qa-s"


def call(base, key, method, path, body=None, timeout=60):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(base.rstrip("/") + path, data=data, method=method,
                                 headers={"Content-Type": "application/json"})
    if key:
        req.add_header("Authorization", "Bearer " + key)
    t0 = time.time()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return json.loads(r.read() or b"{}"), r.status, (time.time() - t0) * 1000
    except urllib.error.HTTPError as e:
        raw = e.read().decode()[:160]
        try:
            return json.loads(raw), e.code, (time.time() - t0) * 1000
        except Exception:
            return {"raw": raw}, e.code, (time.time() - t0) * 1000
    except Exception as e:
        return {"raw": str(e)}, 0, (time.time() - t0) * 1000


SURNAME = list("김이박최정강조윤장임")
GIVEN = ["민준", "서연", "지우", "서준", "하윤", "도윤", "지호", "수아", "지민", "예준", "수빈", "지훈"]
ORGS = [("ai-center", "AI응용연구센터"), ("eng-college", "공과대학"), ("med-school", "의과대학")]
LABS = ["비전", "자연어처리", "로보틱스", "데이터마이닝", "음성인식", "강화학습"]


def seed(base, key, n):
    random.seed(42)  # 재현 가능하게
    org_ids = {}
    for name, disp in ORGS:
        call(base, key, "POST", "/admin/orgs", {"name": PREFIX + name, "displayName": disp, "creditPool": 1000000})
    for o in call(base, key, "GET", "/admin/orgs")[0].get("items", []):
        org_ids[o["name"]] = o["id"]

    for name, _ in ORGS:
        oid = org_ids.get(PREFIX + name)
        if not oid:
            continue
        for lab in LABS:
            call(base, key, "POST", "/admin/groups",
                 {"orgId": oid, "name": f"{PREFIX}{name}-{lab}", "displayName": f"{lab} 연구실"})
    gids = [g["id"] for g in call(base, key, "GET", "/admin/groups")[0].get("items", [])
            if str(g.get("name", "")).startswith(PREFIX)]
    print(f"  조직 {len([k for k in org_ids if k.startswith(PREFIX)])} · 팀 {len(gids)}")

    made = dup = 0
    seen = {}
    batch = []
    for i in range(n):
        nm = random.choice(SURNAME) + random.choice(GIVEN)
        seen[nm] = seen.get(nm, 0) + 1
        dup += 1 if seen[nm] > 1 else 0
        uname = f"{PREFIX}{i:04d}"
        batch.append({"username": uname, "lastName": nm[0], "firstName": nm[1:],
                      "email": f"{uname}@giosk.test", "role": "member",
                      "groupId": random.choice(gids) if gids else None})
        if len(batch) == 50:
            r, _, _ = call(base, key, "POST", "/admin/users/bulk", {"users": batch}, timeout=180)
            made += r.get("created", 0)
            batch = []
            print(f"    {made}명…", end="\r", flush=True)
    if batch:
        r, _, _ = call(base, key, "POST", "/admin/users/bulk", {"users": batch}, timeout=180)
        made += r.get("created", 0)
    top = sorted(seen.items(), key=lambda x: -x[1])[:3]
    print(f"  사용자 {made}명 (동명이인 {dup}명) · 가장 흔한 이름 " + ", ".join(f"{k}×{v}" for k, v in top))


def bench(base, key, rounds=3):
    """관리자 화면이 실제로 부르는 엔드포인트의 응답 시간(중앙값)을 잰다."""
    paths = [
        ("사용자 목록(1페이지)", "/admin/users?limit=50"),
        ("사용자 목록(전체)", "/admin/users?limit=1000"),
        ("사용자 검색(동명이인)", "/admin/users?q=" + urllib.parse.quote("김민준")),
        ("팀 목록", "/admin/groups"),
        ("조직 목록", "/admin/orgs"),
        ("세션 관제", "/admin/sessions"),
        ("대시보드", "/admin/dashboard"),
        ("가용량", "/resources/availability"),
        ("빌링 요약", "/admin/billing"),
        ("감사 로그", "/admin/audit?limit=50"),
    ]
    print(f"\n{'엔드포인트':<24}{'HTTP':<6}{'중앙 ms':>8}{'최대 ms':>9}  건수")
    for label, p in paths:
        times, code, cnt = [], 0, ""
        for _ in range(rounds):
            body, code, ms = call(base, key, "GET", p)
            times.append(ms)
            if isinstance(body, dict):
                items = body.get("items")
                if isinstance(items, list):
                    cnt = str(len(items))
                    if body.get("total"):
                        cnt = f"{len(items)}/{body['total']}"
        times.sort()
        print(f"{label:<24}{code:<6}{times[len(times)//2]:>8.0f}{times[-1]:>9.0f}  {cnt}")


def cleanup(base, key):
    n = 0
    for u in call(base, key, "GET", "/admin/users?limit=5000")[0].get("items", []):
        if str(u.get("username", "")).startswith(PREFIX):
            n += 1
    print(f"접두사 {PREFIX} 계정 {n}개 (삭제 API 는 별도 확인 필요)")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--api", required=True)
    ap.add_argument("--admin-pass", required=True)
    ap.add_argument("--users", type=int, default=300)
    ap.add_argument("--bench-only", action="store_true")
    ap.add_argument("--count-only", action="store_true")
    args = ap.parse_args()

    body, code, _ = call(args.api, None, "POST", "/auth/login",
                         {"username": "admin", "password": args.admin_pass})
    if code != 200:
        raise SystemExit(f"admin 로그인 실패: {code} {body}")
    key = body["sessionkey"]

    if args.count_only:
        cleanup(args.api, key)
        return
    if not args.bench_only:
        print(f"== 시드 {args.users}명")
        t0 = time.time()
        seed(args.api, key, args.users)
        print(f"  소요 {time.time() - t0:.1f}s")
    bench(args.api, key)


if __name__ == "__main__":
    sys.exit(main() or 0)
