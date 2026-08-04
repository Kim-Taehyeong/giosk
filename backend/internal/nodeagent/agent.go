// Package nodeagent는 물리노드 DaemonSet 에이전트의 reconcile 로직이다.
// API 에서 자기 노드의 활성 임대를 받아, 임대별 리눅스 계정/SSH/NFS 마운트를 보장(Provision)하고
// 사라진 임대는 정리(Deprovision)한다. 실제 OS 작업은 Executor 로 추상화(테스트=log, 운영=shell).
package nodeagent

import (
	"log"
	"time"
)

// Lease는 API 가 주는 임대 1건(계정/홈 마운트 + 추가/공유 볼륨 마운트).
type Lease struct {
	LeaseID      int64      `json:"leaseId"`
	Username     string     `json:"username"`
	UID          int        `json:"uid"` // 전역 안정 UID(useradd -u; 재사용 안 함)
	SSHPublicKey string     `json:"sshPublicKey"`
	NFSServer    string     `json:"nfsServer"`
	NFSPath      string     `json:"nfsPath"`
	MountPath    string     `json:"mountPath"`
	Volumes      []VolMount `json:"volumes,omitempty"`
	Scratch      string     `json:"scratch,omitempty"` // 노드로컬 스크래치 계정폴더(빈값=비활성)
}

// VolMount는 물리노드에 직접 NFS 마운트할 볼륨 1건(RW/RO).
type VolMount struct {
	NFSServer string `json:"nfsServer"`
	NFSPath   string `json:"nfsPath"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly"`
}

// Executor는 노드 OS 작업 계약(계정/SSH/마운트).
type Executor interface {
	Provision(l Lease) error
	Deprovision(username string) error
}

// LeaseSource는 자기 노드의 활성 임대를 가져온다(API 클라이언트가 구현).
type LeaseSource interface {
	ActiveLeases() ([]Lease, error)
}

// Reconciler는 주기적으로 임대 상태를 노드에 수렴시킨다.
type Reconciler struct {
	src     LeaseSource
	exec    Executor
	applied map[int64]string // leaseID 별 username (현재 프로비저닝된 것)
}

func NewReconciler(src LeaseSource, exec Executor) *Reconciler {
	return &Reconciler{src: src, exec: exec, applied: map[int64]string{}}
}

// Run은 interval 마다 reconcile 한다(블로킹).
func (r *Reconciler) Run(interval time.Duration) {
	for {
		if err := r.reconcileOnce(); err != nil {
			log.Printf("[node-agent] reconcile error: %v", err)
		}
		time.Sleep(interval)
	}
}

// reconcileOnce는 1회 수렴: 신규 임대 Provision, 사라진 임대 Deprovision.
func (r *Reconciler) reconcileOnce() error {
	leases, err := r.src.ActiveLeases()
	if err != nil {
		return err
	}
	seen := map[int64]bool{}
	for _, l := range leases {
		seen[l.LeaseID] = true
		if _, ok := r.applied[l.LeaseID]; ok {
			continue // 이미 적용됨
		}
		if err := r.exec.Provision(l); err != nil {
			log.Printf("[node-agent] provision %s 실패: %v", l.Username, err)
			continue
		}
		r.applied[l.LeaseID] = l.Username
	}
	r.deprovisionStale(seen)
	return nil
}

func (r *Reconciler) deprovisionStale(seen map[int64]bool) {
	for id, username := range r.applied {
		if seen[id] {
			continue
		}
		if err := r.exec.Deprovision(username); err != nil {
			log.Printf("[node-agent] deprovision %s 실패: %v", username, err)
			continue
		}
		delete(r.applied, id)
	}
}
