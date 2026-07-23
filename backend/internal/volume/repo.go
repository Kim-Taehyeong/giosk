package volume

import (
	"errors"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("not found")

// Repository는 volume 영속성 계약.
type Repository interface {
	Create(v *Volume) error
	SetBound(id int64, pvcName, ns, status string) error
	ListOwned(userID int64) ([]Volume, error)
	ListShared(userID int64) ([]Volume, error)
	Get(id int64) (*Volume, error)
	Delete(id int64) error
	AllocatedGiB(userID int64) (int, error)
	AddShare(volumeID int64, userID, groupID *int64, perm string) error
	ResolveUserID(username string) (*int64, error)
	ListAll(orgID, groupID int64) ([]AdminVolume, error) // 전체 볼륨(관리자 목록; org/group>0=스코프 필터)
	UsersAllocated() []UserAlloc       // 사용자별 볼륨 할당 합(관리자 스토리지 뷰)
	LeasedNodes(userID int64) []string // 사용자가 대여한 적 있는 물리노드(로컬 Home 노출 대상)
	ListBillable() ([]Volume, error)   // 과금 대상 볼륨(bound + 소유자 있음)
	SetBilled(id int64, credits int) error
}

// UserAlloc — 사용자별 볼륨 할당 합계.
type UserAlloc struct {
	UserID      int64  `gorm:"column:user_id"`
	Name        string `gorm:"column:name"`
	AllocatedGB int    `gorm:"column:allocated_gb"`
}

type gormRepo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

func (r *gormRepo) Create(v *Volume) error { return r.db.Create(v).Error }

// LeasedNodes는 사용자가 대여한 적 있는(이력 포함) 물리노드 목록을 반환한다 — 로컬 Home 볼륨 노출 대상.
func (r *gormRepo) LeasedNodes(userID int64) []string {
	var out []string
	r.db.Raw(`SELECT DISTINCT node FROM node_leases WHERE user_id = ? AND node <> '' ORDER BY node`, userID).Scan(&out)
	return out
}

func (r *gormRepo) SetBound(id int64, pvcName, ns, status string) error {
	return r.db.Model(&Volume{}).Where("id = ?", id).
		Updates(map[string]any{"pvc_name": pvcName, "pvc_namespace": ns, "status": status}).Error
}

func (r *gormRepo) ListOwned(userID int64) ([]Volume, error) {
	var out []Volume
	return out, r.db.Where("owner_user_id = ?", userID).Order("id").Find(&out).Error
}

// ListShared — 나에게 공유된 볼륨(직접 공유 + 내 활성 그룹 공유). 내가 소유한 건 제외.
func (r *gormRepo) ListShared(userID int64) ([]Volume, error) {
	var out []Volume
	err := r.db.Raw(`
		SELECT v.*, CASE WHEN SUM(s.permission = 'rw') > 0 THEN 'rw' ELSE 'ro' END AS perm
		FROM volumes v
		JOIN volume_shares s ON s.volume_id = v.id
		WHERE (
			s.shared_with_user_id = ?
			OR s.shared_with_group_id IN (
				SELECT group_id FROM memberships WHERE user_id = ? AND status = 'active'
			)
		)
		AND (v.owner_user_id IS NULL OR v.owner_user_id <> ?)
		GROUP BY v.id
		ORDER BY v.id`, userID, userID, userID).Scan(&out).Error
	return out, err
}

func (r *gormRepo) Get(id int64) (*Volume, error) {
	var v Volume
	err := r.db.Where("id = ?", id).First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &v, err
}

func (r *gormRepo) Delete(id int64) error { return r.db.Delete(&Volume{}, id).Error }

// ListBillable은 과금 대상(bound 상태 + 소유자 존재) 볼륨을 반환한다.
func (r *gormRepo) ListBillable() ([]Volume, error) {
	var out []Volume
	return out, r.db.Where("status = ? AND owner_user_id IS NOT NULL", StatusBound).Find(&out).Error
}

func (r *gormRepo) SetBilled(id int64, credits int) error {
	return r.db.Model(&Volume{}).Where("id = ?", id).Update("billed_credits", credits).Error
}

func (r *gormRepo) AllocatedGiB(userID int64) (int, error) {
	var sum int
	err := r.db.Raw(`SELECT COALESCE(SUM(size_gib),0) FROM volumes WHERE owner_user_id = ?`, userID).Scan(&sum).Error
	return sum, err
}

func (r *gormRepo) AddShare(volumeID int64, userID, groupID *int64, perm string) error {
	// 같은 대상(사용자/그룹) 기존 공유는 제거 후 재생성 → 권한 변경·다운그레이드(rw→ro) 반영 + 중복 방지.
	q := r.db.Where("volume_id = ?", volumeID)
	switch {
	case userID != nil:
		q = q.Where("shared_with_user_id = ?", *userID)
	case groupID != nil:
		q = q.Where("shared_with_group_id = ?", *groupID)
	}
	if err := q.Delete(&Share{}).Error; err != nil {
		return err
	}
	return r.db.Create(&Share{VolumeID: volumeID, SharedWithUserID: userID, SharedWithGroupID: groupID, Permission: perm}).Error
}

// UsersAllocated는 사용자별 볼륨 할당 합(GiB)을 내림차순으로 반환한다(과사용 식별용).
// AdminVolume — 관리자 볼륨 목록 한 행(소유자·조직·그룹 포함).
type AdminVolume struct {
	ID          int64  `gorm:"column:id" json:"id"`
	Name        string `gorm:"column:name" json:"name"`
	Kind        string `gorm:"column:kind" json:"kind"`
	OwnerUserID int64  `gorm:"column:owner_user_id" json:"ownerUserId"`
	Owner       string `gorm:"column:owner" json:"owner"`
	Org         string `gorm:"column:org" json:"org"`
	Group       string `gorm:"column:group" json:"group"`
	CapGb       int    `gorm:"column:cap_gb" json:"capGb"`
	UsedGb      int    `gorm:"column:used_gb" json:"usedGb"`
	AccessMode  string `gorm:"column:access_mode" json:"accessMode"`
	Status      string `gorm:"column:status" json:"status"`
	NFSServer   string `gorm:"column:nfs_server" json:"nfsServer"`
	NFSPath     string `gorm:"column:nfs_path" json:"nfsPath"`
}

// ListAll은 전체 볼륨을 소유자/조직/그룹과 함께 반환한다. orgID/groupID>0 이면 소유자의
// 대표 멤버십 기준으로 스코프 필터(매니저용).
func (r *gormRepo) ListAll(orgID, groupID int64) ([]AdminVolume, error) {
	where, args := "", []any{}
	if groupID > 0 {
		where, args = " WHERE g.id = ?", []any{groupID}
	} else if orgID > 0 {
		where, args = " WHERE g.org_id = ?", []any{orgID}
	}
	var out []AdminVolume
	err := r.db.Raw(`
		SELECT v.id, v.name, v.kind, COALESCE(v.owner_user_id,0) AS owner_user_id,
		       COALESCE(NULLIF(TRIM(CONCAT(COALESCE(u.last_name,''),COALESCE(u.first_name,''))),''), u.username, '') AS owner,
		       COALESCE(o.display_name,'') AS org, COALESCE(g.display_name,'') AS `+"`group`"+`,
		       v.size_gib AS cap_gb, v.used_gib AS used_gb, v.access_mode, v.status,
		       COALESCE(v.nfs_server,'') AS nfs_server, COALESCE(v.nfs_path,'') AS nfs_path
		FROM volumes v
		LEFT JOIN users u ON u.id = v.owner_user_id
		LEFT JOIN memberships m ON m.id = (
		    SELECT m2.id FROM memberships m2 WHERE m2.user_id=v.owner_user_id AND m2.status='active'
		    ORDER BY FIELD(m2.role,'org_admin','project_admin','billing_admin','member','guest') LIMIT 1)
		LEFT JOIN `+"`groups`"+` g ON g.id = m.group_id
		LEFT JOIN organizations o ON o.id = g.org_id`+where+`
		ORDER BY v.id DESC`, args...).Scan(&out).Error
	return out, err
}

func (r *gormRepo) UsersAllocated() []UserAlloc {
	var out []UserAlloc
	r.db.Raw(`
		SELECT u.id AS user_id,
		       COALESCE(NULLIF(TRIM(CONCAT(COALESCE(u.last_name,''),COALESCE(u.first_name,''))),''), u.username) AS name,
		       COALESCE(SUM(v.size_gib),0) AS allocated_gb
		FROM volumes v JOIN users u ON u.id = v.owner_user_id
		GROUP BY u.id, u.last_name, u.first_name, u.username
		HAVING allocated_gb > 0
		ORDER BY allocated_gb DESC`).Scan(&out)
	return out
}

func (r *gormRepo) ResolveUserID(username string) (*int64, error) {
	var id int64
	if err := r.db.Raw(`SELECT id FROM users WHERE username = ? LIMIT 1`, username).Scan(&id).Error; err != nil {
		return nil, err
	}
	if id == 0 {
		return nil, ErrNotFound
	}
	return &id, nil
}
