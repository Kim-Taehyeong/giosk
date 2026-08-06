#!/usr/bin/env python3
"""다중 사용자 격리·쿼터 점검. multiuser-test.py 로 띄운 세션이 있는 상태에서 돌린다.

보는 것
  - 남의 세션이 내 목록에 섞이지 않는지
  - 남의 세션을 id 로 직접 조작(조회·중지·삭제)할 수 있는지
  - 일반 사용자가 관리자 API 에 닿는지
  - 동시 세션 상한과 중단 세션 상한이 걸리는지
  - 크레딧이 실제로 깎이는지
"""
import argparse
import json
import sys
import urllib.error
import urllib.request

PREFIX = "qa-mu"


def call(base, key, method, path, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(base.rstrip("/") + path, data=data, method=method,
                                 headers={"Content-Type": "application/json"})
    if key:
        req.add_header("Authorization", "Bearer " + key)
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            return json.loads(r.read() or b"{}"), r.status
    except urllib.error.HTTPError as e:
        raw = e.read().decode()[:160]
        try:
            return json.loads(raw), e.code
        except Exception:
            return {"raw": raw}, e.code
    except Exception as e:
        return {"raw": str(e)}, 0


def login(base, u, p):
    b, c = call(base, None, "POST", "/auth/login", {"username": u, "password": p})
    if c != 200:
        raise SystemExit(f"로그인 실패 {u}: {c} {b}")
    return b["sessionkey"]


def check(name, ok, detail=""):
    print(f"  [{'통과' if ok else '실패'}] {name}{('  — ' + detail) if detail else ''}")
    return ok


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--api", required=True)
    ap.add_argument("--admin-pass", required=True)
    ap.add_argument("--password", default="giosk-qa-1234")
    args = ap.parse_args()
    base = args.api
    admin = login(base, "admin", args.admin_pass)

    # 세션을 가진 계정 두 개를 고른다. 관제 행의 owner 는 표시 이름이라 로그인에 못 쓴다.
    # userId 로 계정 목록과 맞춰 username 을 얻는다.
    names = {u["id"]: u["username"] for u in call(base, admin, "GET", "/admin/users?limit=1000")[0].get("items", [])}
    owners = []
    for s in call(base, admin, "GET", "/admin/sessions")[0].get("items", []):
        if str(s.get("name", "")).startswith(PREFIX) and s.get("status") == "running":
            uname = names.get(s.get("userId"))
            if uname:
                owners.append((uname, s["id"]))
    if len(owners) < 2:
        raise SystemExit("검증용 실행 세션이 2개 미만이다. multiuser-test.py 를 먼저 돌려라")
    (ua, sa), (ub, sb) = owners[0], owners[1]
    print(f"대상: {ua}({sa}) vs {ub}({sb})")

    ka, kb = login(base, ua, args.password), login(base, ub, args.password)
    fails = 0

    print("\n== 세션 격리")
    mine = [s["id"] for s in call(base, ka, "GET", "/instances")[0].get("items", [])]
    fails += not check("남의 세션이 목록에 안 섞인다", sb not in mine, f"내 목록 {len(mine)}건")

    _, c = call(base, ka, "GET", f"/instances/{sb}/connection")
    fails += not check("남의 세션 접속정보 조회 거부", c >= 400, f"HTTP {c}")
    _, c = call(base, ka, "GET", f"/instances/{sb}/logs")
    fails += not check("남의 세션 로그 조회 거부", c >= 400, f"HTTP {c}")
    _, c = call(base, ka, "POST", f"/instances/{sb}/stop", {})
    fails += not check("남의 세션 중지 거부", c >= 400, f"HTTP {c}")
    _, c = call(base, ka, "DELETE", f"/instances/{sb}")
    fails += not check("남의 세션 삭제 거부", c >= 400, f"HTTP {c}")

    print("\n== 관리자 경계")
    for path in ("/admin/sessions", "/admin/users", "/admin/nodes", "/admin/config"):
        _, c = call(base, ka, "GET", path)
        fails += not check(f"일반 사용자 {path} 거부", c >= 400, f"HTTP {c}")

    print("\n== 쿼터")
    img = call(base, ka, "GET", "/images")[0].get("items", [{}])[0].get("id")
    grp = (call(base, ka, "GET", "/me/groups")[0].get("items") or [{}])[0].get("id")
    made, last = [], None
    for i in range(6):
        b, c = call(base, ka, "POST", "/instances", {
            "instancename": f"{PREFIX}-quota{i}", "groupId": grp, "imageId": img,
            "gpuMode": "cpu", "cpuCores": 1, "memGb": 2})
        last = (c, (b.get("code") or b.get("message") or b.get("raw") or "")[:50])
        if c >= 300:
            break
        made.append(b.get("id"))
    fails += not check("동시 세션 상한이 걸린다", last[0] >= 300,
                       f"{len(made)}개 추가 후 HTTP {last[0]} {last[1]}")
    for sid in made:
        call(base, ka, "DELETE", f"/instances/{sid}")

    print("\n== 크레딧")
    wal, _ = call(base, ka, "GET", "/me/wallet")
    print(f"  지갑 응답: {json.dumps(wal, ensure_ascii=False)[:160]}")

    print(f"\n실패 {fails}건")
    return 1 if fails else 0


if __name__ == "__main__":
    sys.exit(main())
