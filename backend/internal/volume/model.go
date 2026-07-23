package volume

import "time"

// Volume은 volumes 테이블 매핑.
type Volume struct {
	ID            int64     `gorm:"primaryKey" json:"id"`
	Name          string    `gorm:"column:name" json:"name"`
	Kind          string    `gorm:"column:kind" json:"kind"`
	OwnerUserID   *int64    `gorm:"column:owner_user_id" json:"-"`
	GroupID       *int64    `gorm:"column:group_id" json:"groupId,omitempty"`
	SizeGiB       int       `gorm:"column:size_gib" json:"capGb"`
	UsedGiB       int       `gorm:"column:used_gib" json:"usedGb"`
	AccessMode    string    `gorm:"column:access_mode" json:"accessMode"`
	Perm          string    `gorm:"->;column:perm" json:"perm,omitempty"` // 공유 볼륨 권한(ro|rw) — ListShared 의 계산 컬럼. 읽기전용(->): INSERT/UPDATE 에서 제외(volumes 테이블엔 실제 perm 컬럼 없음).
	PVCName       string    `gorm:"column:pvc_name" json:"-"`
	PVCNamespace  string    `gorm:"column:pvc_namespace" json:"-"`
	// NFS 임포트 볼륨: 지정한 기존 NFS 경로를 정적 PV 로 바인딩(빈값=일반 동적 볼륨).
	NFSServer     string    `gorm:"column:nfs_server" json:"nfsServer,omitempty"`
	NFSPath       string    `gorm:"column:nfs_path" json:"nfsPath,omitempty"`
	Status        string    `gorm:"column:status" json:"status"`
	BilledCredits int       `gorm:"column:billed_credits" json:"-"` // 누적 차감 크레딧(스토리지 과금, 델타 회계)
	CreatedAt     time.Time `gorm:"column:created_at" json:"-"`
}

func (Volume) TableName() string { return "volumes" }

// Share는 volume_shares 테이블 매핑.
type Share struct {
	ID                int64  `gorm:"primaryKey" json:"id"`
	VolumeID          int64  `gorm:"column:volume_id" json:"-"`
	SharedWithUserID  *int64 `gorm:"column:shared_with_user_id" json:"-"`
	SharedWithGroupID *int64 `gorm:"column:shared_with_group_id" json:"-"`
	Permission        string `gorm:"column:permission" json:"perm"`
}

func (Share) TableName() string { return "volume_shares" }

const (
	StatusPending = "pending"
	StatusBound   = "bound"
	StatusFailed  = "failed"
)

// LocalHome — 물리노드 로컬 home 특수 볼륨(hostPath, 해당 노드 전용). 사용자가 대여한 적 있는 노드만 노출.
// 컨테이너 세션에서 선택하면 그 노드에 핀되고 노드 로컬 home 을 /home/work 로 마운트한다.
type LocalHome struct {
	Node string `json:"node"`
	Name string `json:"name"`
}

// ListRes — /volumes 응답(owned + shared + 로컬 Home + quota).
type ListRes struct {
	Owned      []Volume    `json:"owned"`
	Shared     []Volume    `json:"shared"`
	LocalHomes []LocalHome `json:"localHomes"`
	Quota      Quota       `json:"quota"`
}

type Quota struct {
	AllocatedGB int `json:"allocatedGb"`
	TotalGB     int `json:"totalGb"`
}

// 요청 바디.
type CreateReq struct {
	Name    string `json:"name" binding:"required"`
	SizeGiB int    `json:"sizeGib" binding:"required"`
}

type ShareReq struct {
	Target     string `json:"target"`     // user | group
	Value      string `json:"value"`      // username | groupId
	Permission string `json:"permission"` // ro | rw
}
