package nodeagent

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
)

// ShellExecutor는 호스트에서 실제 계정/SSH/마운트 작업을 수행한다(운영, privileged DaemonSet).
// nsenter -t 1 로 호스트 PID1 네임스페이스에 진입해 호스트의 useradd/mount.nfs/userdel 을 실행한다.
//
// home 모델: home(l.MountPath = <homeBase>/<user>)은 노드 로컬 디스크(useradd -m -d). 개인 영속은
// l.NFSServer:l.NFSPath/<user> 를 ~/nfs 로 마운트(없으면 부모 export 에 서브디렉터리 생성). 데이터셋·
// 추가 볼륨은 l.Volumes(NFS). 반납 시 마운트 해제 후 userdel(홈 보존 — 안정 UID 라 재임대 시 소유권 회복).
type ShellExecutor struct {
	node       string
	homeBase   string // 물리 로컬 home 루트(/home/giosk)
	nsenter    bool   // true=nsenter 로 호스트 실행(DaemonSet), false=직접(호스트에서 직접 구동 시)
	gatewayKey string // 게이트웨이 SSH 관리키의 공개키(authorized_keys 에 사용자 키와 병기 → 게이트웨이 프록시 접속)
}

func NewShellExecutor(node, homeBase string, nsenter bool) *ShellExecutor {
	if homeBase == "" {
		homeBase = "/home/giosk"
	}
	return &ShellExecutor{node: node, homeBase: homeBase, nsenter: nsenter}
}

// WithGatewayKey는 게이트웨이 프록시가 계정으로 로그인할 수 있도록 신뢰할 공개키를 설정한다.
func (e *ShellExecutor) WithGatewayKey(pub string) *ShellExecutor {
	e.gatewayKey = strings.TrimSpace(pub)
	return e
}

// run은 호스트에서 명령을 실행한다(nsenter 옵션). CombinedOutput 반환.
func (e *ShellExecutor) run(args ...string) (string, error) {
	if e.nsenter {
		args = append([]string{"-t", "1", "-m", "-u", "-i", "-n", "-p", "--"}, args...)
		out, err := exec.Command("nsenter", args...).CombinedOutput()
		return string(out), err
	}
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	return string(out), err
}

// sh는 호스트에서 sh -c 로 스크립트를 실행한다(파이프/조건 필요 시).
func (e *ShellExecutor) sh(script string) (string, error) {
	return e.run("/bin/sh", "-c", script)
}

func (e *ShellExecutor) Provision(l Lease) error {
	home := l.MountPath
	if home == "" {
		home = e.homeBase + "/" + l.Username
	}
	// 1) 계정 생성(멱등). 전역 안정 UID·노드 로컬 home.
	if _, err := e.run("id", l.Username); err != nil {
		if _, err := e.sh("mkdir -p " + shq(e.homeBase)); err != nil {
			return fmt.Errorf("mkdir homeBase: %w", err)
		}
		// -N: 동명 사용자그룹 생성 안 함(호스트에 같은 이름 그룹이 이미 있어도 충돌 없음). 기본그룹=users.
		if out, err := e.run("useradd", "-u", strconv.Itoa(l.UID), "-m", "-d", home, "-s", "/bin/bash", "-N", l.Username); err != nil {
			return fmt.Errorf("useradd %s: %v (%s)", l.Username, err, strings.TrimSpace(out))
		}
		log.Printf("[node-agent:%s] useradd %s uid=%d home=%s", e.node, l.Username, l.UID, home)
	}
	// 2) SSH authorized_keys — 사용자 키 + 게이트웨이 관리키(복붙 프록시 접속용)를 병기.
	keys := joinKeys(l.SSHPublicKey, e.gatewayKey)
	if keys != "" {
		script := strings.Join([]string{
			"set -e",
			"mkdir -p " + shq(home+"/.ssh"),
			"printf '%s\\n' " + shq(keys) + " > " + shq(home+"/.ssh/authorized_keys"),
			"chown -R " + shq(l.Username) + " " + shq(home+"/.ssh"),
			"chmod 700 " + shq(home+"/.ssh"),
			"chmod 600 " + shq(home+"/.ssh/authorized_keys"),
		}, "; ")
		if out, err := e.sh(script); err != nil {
			log.Printf("[node-agent:%s] authorized_keys %s 실패: %v (%s)", e.node, l.Username, err, strings.TrimSpace(out))
		}
	}
	// 3) 개인 영속 ~/nfs — NFSServer:NFSPath/<user>. 서버측 서브디렉터리 없으면 부모 export 에 생성.
	if l.NFSServer != "" && l.NFSPath != "" {
		e.ensureNFSHome(l, home)
	}
	// 4) 추가 NFS 마운트(데이터셋 RO·공유 볼륨 RW/RO)
	for _, v := range l.Volumes {
		opt := "rw"
		if v.ReadOnly {
			opt = "ro"
		}
		script := fmt.Sprintf("mkdir -p %s; mountpoint -q %s || mount -t nfs -o %s %s:%s %s",
			shq(v.MountPath), shq(v.MountPath), opt, v.NFSServer, shq(v.NFSPath), shq(v.MountPath))
		if out, err := e.sh(script); err != nil {
			log.Printf("[node-agent:%s] mount %s 실패: %v (%s)", e.node, v.MountPath, err, strings.TrimSpace(out))
		}
	}
	// 5) 노드로컬 스크래치(RWX 본인만) + 홈에 심링크
	if l.Scratch != "" {
		script := fmt.Sprintf("mkdir -p %s; chown %s %s; chmod 700 %s; ln -sfn %s %s",
			shq(l.Scratch), shq(l.Username), shq(l.Scratch), shq(l.Scratch), shq(l.Scratch), shq(home+"/scratch"))
		_, _ = e.sh(script)
	}
	return nil
}

// ensureNFSHome은 ~/nfs(개인 영속)를 마운트한다. 서버에 <base>/<user> 가 없으면 부모를 잠깐 마운트해 생성.
func (e *ShellExecutor) ensureNFSHome(l Lease, home string) {
	base := strings.TrimRight(l.NFSPath, "/")
	userPath := base + "/" + l.Username
	target := home + "/nfs"
	// 서브디렉터리 보장(부모 export 임시 마운트 → mkdir → 해제). $T 확장 위해 user 는 쉘 변수로.
	ensure := fmt.Sprintf(
		"U=%s; T=$(mktemp -d); mount -t nfs %s:%s \"$T\" && mkdir -p \"$T/$U\" && umount \"$T\"; rmdir \"$T\" 2>/dev/null || true",
		shq(l.Username), l.NFSServer, shq(base))
	if out, err := e.sh(ensure); err != nil {
		log.Printf("[node-agent:%s] ~/nfs 서브디렉터리 생성 %s: %v (%s)", e.node, l.Username, err, strings.TrimSpace(out))
	}
	mountScript := fmt.Sprintf("mkdir -p %s; mountpoint -q %s || mount -t nfs -o rw %s:%s %s",
		shq(target), shq(target), l.NFSServer, shq(userPath), shq(target))
	if out, err := e.sh(mountScript); err != nil {
		log.Printf("[node-agent:%s] ~/nfs mount %s 실패: %v (%s)", e.node, l.Username, err, strings.TrimSpace(out))
		return
	}
	// ~/nfs 디렉터리를 사용자 소유로(no_root_squash NFS) → 컨테이너(runAsUser=UID)와 동일 UID 권한.
	_, _ = e.run("chown", strconv.Itoa(l.UID)+":"+strconv.Itoa(l.UID), target)
}

func (e *ShellExecutor) Deprovision(username string) error {
	home := e.homeBase + "/" + username
	// home 하위 모든 마운트 해제(역순, lazy) — 데이터셋·~/nfs·볼륨.
	e.sh(fmt.Sprintf("for m in $(mount | awk '{print $3}' | grep '^%s/' | sort -r); do umount -l \"$m\" 2>/dev/null; done", home))
	// 계정 삭제(홈은 보존: -r 미사용). 안정 UID 라 재임대 시 소유권 회복.
	if out, err := e.run("userdel", username); err != nil {
		log.Printf("[node-agent:%s] userdel %s: %v (%s)", e.node, username, err, strings.TrimSpace(out))
	} else {
		log.Printf("[node-agent:%s] userdel %s (home 보존)", e.node, username)
	}
	return nil
}

// shq는 쉘 단일따옴표 인용(내부 ' 이스케이프).
func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// joinKeys는 비어있지 않은 authorized_keys 항목들을 개행으로 합친다.
func joinKeys(keys ...string) string {
	var out []string
	for _, k := range keys {
		if k = strings.TrimSpace(k); k != "" {
			out = append(out, k)
		}
	}
	return strings.Join(out, "\n")
}
