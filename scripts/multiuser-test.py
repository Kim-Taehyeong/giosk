#!/usr/bin/env python3
"""다중 사용자 동시 사용 검증. 실제 클러스터에 계정을 만들고 동시에 세션을 띄운다.

혼자 눌러 보는 것으로는 안 드러나는 것들을 본다.
  - 같은 자리를 여러 명이 동시에 노릴 때 승인이 직렬화되는지(초과 배치가 안 나는지)
  - 세션이 팀 네임스페이스로 갈리는지
  - 자리가 없을 때 매달아 두지 않고 즉시 거절하는지
  - 세션마다 접속 좌표(웹/SSH)가 제대로 나오는지, IP 풀이 마르면 어떻게 되는지

사용:
  python3 multiuser-test.py --api http://10.10.0.232/api --admin-pass '...' [--users 8] [--keep]

계정과 팀은 이름이 qa- 로 시작한다. --cleanup 으로 이 테스트가 만든 세션을 지운다.
"""
import argparse
import json
import sys
import threading
import time
import urllib.error
import urllib.request

PREFIX = "qa-mu"


class API:
    def __init__(self, base, key=None):
        self.base, self.key = base.rstrip("/"), key

    def call(self, method, path, body=None, timeout=30):
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(self.base + path, data=data, method=method,
                                     headers={"Content-Type": "application/json"})
        if self.key:
            req.add_header("Authorization", "Bearer " + self.key)
        try:
            with urllib.request.urlopen(req, timeout=timeout) as r:
                return json.loads(r.read() or b"{}"), r.status
        except urllib.error.HTTPError as e:
            raw = e.read().decode()[:200]
            try:
                return json.loads(raw), e.code
            except Exception:
                return {"raw": raw}, e.code
        except Exception as e:
            return {"raw": str(e)}, 0


def login(base, username, password):
    body, code = API(base).call("POST", "/auth/login", {"username": username, "password": password})
    if code != 200:
        raise SystemExit(f"로그인 실패 {username}: {code} {body}")
    return body["sessionkey"]


def ensure_offering(admin, name, vram, core):
    """작은 공유 오퍼링을 보장한다. 큰 것만 있으면 카드 하나에 두 명밖에 못 들어간다."""
    items = admin.call("GET", "/admin/offerings")[0].get("items", [])
    for o in items:
        if o.get("name") == name:
            return o["id"]
    gpu = next((o.get("gpuType") for o in items if o.get("gpuType")), "")
    body, code = admin.call("POST", "/admin/offerings", {
        "name": name, "gpuType": gpu, "vramMb": vram, "corePercent": core,
        "mode": "fractional", "pricePerHour": max(1, core // 2), "isActive": True,
    })
    if code >= 300:
        raise SystemExit(f"오퍼링 생성 실패: {code} {body}")
    for o in admin.call("GET", "/admin/offerings")[0].get("items", []):
        if o.get("name") == name:
            return o["id"]
    raise SystemExit("오퍼링을 다시 찾지 못했다")


def ensure_env(admin, n, password, credit):
    """조직·팀·계정을 보장하고 (username, userId) 목록을 준다."""
    orgs = admin.call("GET", "/admin/orgs")[0].get("items", [])
    org = next((o for o in orgs if o["name"] == PREFIX + "-org"), None)
    if not org:
        admin.call("POST", "/admin/orgs", {"name": PREFIX + "-org", "displayName": "동시성 검증", "creditPool": 1000000})
        orgs = admin.call("GET", "/admin/orgs")[0].get("items", [])
        org = next((o for o in orgs if o["name"] == PREFIX + "-org"), None)
    if not org:
        raise SystemExit("조직 생성 실패")

    groups = admin.call("GET", "/admin/groups")[0].get("items", [])
    grp = next((g for g in groups if g["name"] == PREFIX + "-team"), None)
    if not grp:
        admin.call("POST", "/admin/groups", {"orgId": org["id"], "name": PREFIX + "-team", "displayName": "동시성 검증팀"})
        groups = admin.call("GET", "/admin/groups")[0].get("items", [])
        grp = next((g for g in groups if g["name"] == PREFIX + "-team"), None)
    if not grp:
        raise SystemExit("팀 생성 실패")

    # 팀 지갑에 크레딧을 넣는다. 크레딧 모드에서는 지갑이 비면 세션 생성 자체가 막힌다.
    admin.call("POST", f"/admin/groups/{grp['id']}/wallet/grant", {"amount": credit * n * 4})

    users = []
    for i in range(n):
        uname = f"{PREFIX}{i:02d}"
        body, code = admin.call("POST", "/admin/users", {
            "username": uname, "password": password, "email": f"{uname}@giosk.test",
            "lastName": "검증", "firstName": f"{i:02d}", "role": "member", "groupId": grp["id"],
        })
        if code >= 300 and code != 409:
            print(f"  ! {uname} 생성 실패 {code} {body}")
        users.append(uname)
    # 개인 지갑도 채운다(계층 배분에서 개인 풀을 쓰는 설정이면 필요하다).
    listed = {u["username"]: u["id"] for u in admin.call("GET", "/admin/users?limit=1000")[0].get("items", [])}
    for uname in users:
        uid = listed.get(uname)
        if uid:
            admin.call("POST", f"/admin/users/{uid}/grant-credit", {"amount": credit})
    return grp["id"], users


def pick_image(api):
    items = api.call("GET", "/images")[0].get("items", [])
    ok = [i for i in items if i.get("status") in (None, "", "ready", "cached")]
    if not ok:
        ok = items
    if not ok:
        raise SystemExit("사용 가능한 이미지가 없다")
    return ok[0]["id"], ok[0].get("name", "?")


def create_one(base, uname, password, gid, image_id, spec, results, idx, barrier):
    """계정 하나가 세션 하나를 만든다. 전원이 같은 순간에 요청하도록 barrier 로 맞춘다."""
    rec = {"user": uname, "spec": spec["label"]}
    try:
        key = login(base, uname, password)
        api = API(base, key)
        body = {
            "instancename": f"{PREFIX}-{idx:02d}",
            "groupId": gid, "imageId": image_id,
            "gpuMode": spec["mode"], "cpuCores": spec["cpu"], "memGb": spec["mem"],
        }
        body.update(spec.get("extra", {}))
        barrier.wait(timeout=60)
        t0 = time.time()
        r, code = api.call("POST", "/instances", body, timeout=60)
        rec["code"] = code
        rec["ms"] = int((time.time() - t0) * 1000)
        rec["id"] = r.get("id") or r.get("instanceId") or ""
        if code >= 300:
            rec["err"] = (r.get("code") or r.get("message") or r.get("raw") or "")[:60]
        rec["key"] = key
    except Exception as e:
        rec["code"], rec["err"] = 0, str(e)[:60]
    results[idx] = rec


def wait_phase(base, recs, timeout):
    """세션이 running 또는 실패로 확정될 때까지 기다린다."""
    deadline = time.time() + timeout
    live = [r for r in recs if r.get("id")]
    while time.time() < deadline:
        pending = 0
        for r in live:
            api = API(base, r["key"])
            body, _ = api.call("GET", "/instances")
            for s in body.get("items", []):
                if s.get("id") == r["id"]:
                    r["phase"] = s.get("status")
                    r["node"] = s.get("node") or ""
                    r["offering"] = s.get("offering") or ""
            if r.get("phase") in (None, "provisioning", "pending"):
                pending += 1
        if pending == 0:
            return
        time.sleep(5)


def check_conn(base, recs):
    for r in recs:
        if r.get("phase") != "running":
            continue
        api = API(base, r["key"])
        body, code = api.call("GET", f"/instances/{r['id']}/connection")
        if code >= 300:
            r["conn"] = f"실패({code})"
            continue
        chans = [k for k in ("vscode", "jupyter", "web") if body.get(k)]
        ssh = body.get("ssh") or {}
        r["conn"] = "+".join(chans) or "없음"
        r["ssh"] = (ssh.get("cmd") or "").replace("ssh ", "")[:34] or "없음"


def cleanup(base, admin, password):
    """이 테스트가 만든 세션만 지운다. 계정·팀은 남긴다(다시 쓰기 위해)."""
    n = 0
    for s in admin.call("GET", "/admin/sessions")[0].get("items", []):
        if str(s.get("name", "")).startswith(PREFIX):
            _, code = admin.call("DELETE", f"/admin/sessions/{s['id']}")
            n += 1 if code < 300 else 0
    print(f"세션 {n}개 삭제")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--api", required=True)
    ap.add_argument("--admin-pass", required=True)
    ap.add_argument("--users", type=int, default=8)
    ap.add_argument("--password", default="giosk-qa-1234")
    ap.add_argument("--credit", type=int, default=5000)
    ap.add_argument("--wait", type=int, default=300)
    ap.add_argument("--cleanup", action="store_true", help="세션만 지우고 종료")
    args = ap.parse_args()

    admin = API(args.api, login(args.api, "admin", args.admin_pass))
    if args.cleanup:
        cleanup(args.api, admin, args.password)
        return

    print("== 준비")
    small = ensure_offering(admin, "4090-4g", 4000, 20)
    print(f"  작은 공유 오퍼링 id={small} (4000MB / 코어 20%)")
    gid, users = ensure_env(admin, args.users, args.password, args.credit)
    print(f"  팀 id={gid} · 계정 {len(users)}개")
    image_id, image_name = pick_image(admin)
    print(f"  이미지 {image_name} (id={image_id})")

    avail = admin.call("GET", "/resources/availability")[0]
    for n in avail.get("byNode", []):
        if n.get("fractional"):
            print(f"  {n['node']} 분할 여유: VRAM {n['fracVramFreeMb']}MB · 코어 {n['fracCoresFree']}% · 슬롯 {n['fracSlotsFree']}")
        else:
            print(f"  {n['node']} 전용 여유: {n['gpuFree']}/{n['gpuTotal']}")

    # 절반은 공유 GPU, 절반은 CPU. 공유는 자리 경합을, CPU 는 그 와중에도 되는지를 본다.
    specs = []
    for i in range(args.users):
        if i % 2 == 0:
            specs.append({"label": "공유GPU 4G/20%", "mode": "shared", "cpu": 4, "mem": 8,
                          "extra": {"offeringId": small, "vramMb": 4000, "corePercent": 20}})
        else:
            specs.append({"label": "CPU", "mode": "cpu", "cpu": 2, "mem": 4})

    print(f"\n== 동시 생성 {args.users}건 (전원 같은 순간에 요청)")
    results = [None] * args.users
    barrier = threading.Barrier(args.users)
    ts = []
    for i, (u, sp) in enumerate(zip(users, specs)):
        t = threading.Thread(target=create_one,
                             args=(args.api, u, args.password, gid, image_id, sp, results, i, barrier))
        t.start()
        ts.append(t)
    for t in ts:
        t.join()
    recs = [r for r in results if r]

    ok = [r for r in recs if r.get("code", 0) < 300]
    print(f"  수락 {len(ok)} · 거절 {len(recs) - len(ok)}")
    for r in recs:
        if r.get("code", 0) >= 300:
            print(f"    거절 {r['user']:>8} {r['spec']:<14} {r['code']} {r.get('err','')}")

    print(f"\n== 기동 대기(최대 {args.wait}s)")
    wait_phase(args.api, recs, args.wait)
    check_conn(args.api, recs)

    print(f"\n{'계정':<9}{'요청':<15}{'응답':<6}{'ms':>6}  {'상태':<13}{'노드':<9}{'오퍼링':<22}{'접속':<16}{'SSH'}")
    for r in recs:
        print(f"{r['user']:<9}{r['spec']:<15}{r.get('code',0):<6}{r.get('ms',0):>6}  "
              f"{str(r.get('phase') or '-'):<13}{str(r.get('node') or '-'):<9}"
              f"{str(r.get('offering') or '-'):<22}{str(r.get('conn') or '-'):<16}{r.get('ssh','-')}")

    running = [r for r in recs if r.get("phase") == "running"]
    print(f"\n동시 running {len(running)}개")
    ips = [r.get("ssh", "") for r in running if r.get("ssh") and r["ssh"] != "없음"]
    print(f"SSH 접속 좌표가 나온 세션 {len(ips)}개")
    return 0


if __name__ == "__main__":
    sys.exit(main() or 0)
