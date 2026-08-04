package nodeagent

import "log"

// LogExecutor는 실제 OS 작업 대신 로그만 남긴다(개발/kind 테스트용).
type LogExecutor struct{ node string }

func NewLogExecutor(node string) *LogExecutor { return &LogExecutor{node: node} }

func (e *LogExecutor) Provision(l Lease) error {
	log.Printf("[node-agent:%s] PROVISION user=%s uid=%d useradd -u %d, authorized_keys(%d bytes), mount %s:%s on %s",
		e.node, l.Username, l.UID, l.UID, len(l.SSHPublicKey), l.NFSServer, l.NFSPath, l.MountPath)
	for _, v := range l.Volumes {
		opt := "rw"
		if v.ReadOnly {
			opt = "ro"
		}
		log.Printf("[node-agent:%s]   volume mount -o %s %s:%s on %s",
			e.node, opt, v.NFSServer, v.NFSPath, v.MountPath)
	}
	if l.Scratch != "" { // 노드로컬 스크래치 계정폴더(RWX 본인만) + 홈에 심링크
		log.Printf("[node-agent:%s]   scratch mkdir -p %s · chown %s · chmod 700 · ln -s %s %s/scratch",
			e.node, l.Scratch, l.Username, l.Scratch, l.MountPath)
	}
	return nil
}

func (e *LogExecutor) Deprovision(username string) error {
	log.Printf("[node-agent:%s] DEPROVISION user=%s umount, userdel", e.node, username)
	return nil
}

// NOTE(운영): ShellExecutor 가 useradd/usermod·authorized_keys 기록·mount.nfs 를 exec.Command 로 수행.
// 계정 생성은 useradd -u <l.UID> -m <user> 로 한다(전역 안정 UID 라 재사용하지 않는다). 같은 사용자는 모든 노드에서
// 동일 UID 라 NFS 권한이 일관되고, 삭제 후 재임대해도 본인 스크래치/홈 소유권을 그대로 회복한다.
// 홈은 RW 로 마운트하고, l.Volumes 각각은 mount -o ro,rw -t nfs server:path mountPath 로 처리한다(ReadOnly 면 ro).
// l.Scratch 는 `mkdir -p $S; chown $user $S; chmod 700 $S; ln -sfn $S $home/scratch`(RWX 본인만, 노드로컬).
// Deprovision 은 umount 후 userdel 이다. 스크래치는 보존한다(재임대 시 데이터가 남고 UID 가 안정이라 안전하다).
// hostPath + host 네임스페이스 권한 필요. kind 컨테이너 노드엔 도구가 없어 개발은 LogExecutor 사용.
