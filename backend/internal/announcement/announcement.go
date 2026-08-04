// Package announcement은 공지 도메인(목록/배너 + 관리자 CRUD).
package announcement

import (
	"time"

	"gorm.io/gorm"
)

// Announcement는 announcements 테이블 매핑.
// TargetOrgID/TargetGroupID: 둘 다 nil=전역(전원 노출), 하나 지정 시 그 범위 멤버에게만.
type Announcement struct {
	ID            int64     `gorm:"primaryKey" json:"id"`
	Level         string    `gorm:"column:level" json:"level"`
	Title         string    `gorm:"column:title" json:"title"`
	Body          string    `gorm:"column:body" json:"body"`
	Active        bool      `gorm:"column:active" json:"active"`
	Pinned        bool      `gorm:"column:pinned" json:"pinned"`
	TargetOrgID   *int64    `gorm:"column:target_org_id" json:"targetOrgId"`
	TargetGroupID *int64    `gorm:"column:target_group_id" json:"targetGroupId"`
	CreatedBy     *int64    `gorm:"column:created_by" json:"-"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"createdAt"`
}

func (Announcement) TableName() string { return "announcements" }

// Req는 공지 생성/수정 바디. Target* 는 platform 만 자유 지정; 매니저는 핸들러가 자기 범위로 강제.
type Req struct {
	Level         string `json:"level"`
	Title         string `json:"title" binding:"required"`
	Body          string `json:"body" binding:"required"`
	Pinned        bool   `json:"pinned"`
	TargetOrgID   *int64 `json:"targetOrgId"`
	TargetGroupID *int64 `json:"targetGroupId"`
}

// Repository는 영속성 계약.
type Repository interface {
	ListActive() ([]Announcement, error)
	ListActiveFor(orgID, groupID int64) ([]Announcement, error) // 사용자 노출: 전역 + 내 조직/그룹 타겟
	ListAll() ([]Announcement, error)
	ListAllScoped(orgID, groupID int64) ([]Announcement, error) // 관리자: 내 범위 타겟만
	Create(a *Announcement) error
	Update(id int64, fields map[string]any) error
	Toggle(id int64) error
	Delete(id int64) error
	Get(id int64) (*Announcement, error)
}

type repo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &repo{db: db} }

func (r *repo) ListActive() ([]Announcement, error) {
	var out []Announcement
	return out, r.db.Where("active = ?", true).Order("pinned DESC, id DESC").Find(&out).Error
}

// ListActiveFor는 사용자에게 보일 활성 공지를 준다. 전역(타겟 없음)과 내 조직·그룹 타겟이다.
func (r *repo) ListActiveFor(orgID, groupID int64) ([]Announcement, error) {
	var out []Announcement
	q := r.db.Where("active = ?", true).
		Where("(target_org_id IS NULL AND target_group_id IS NULL) OR target_org_id = ? OR target_group_id = ?", orgID, groupID)
	return out, q.Order("pinned DESC, id DESC").Find(&out).Error
}

func (r *repo) ListAll() ([]Announcement, error) {
	var out []Announcement
	return out, r.db.Order("id DESC").Find(&out).Error
}

// ListAllScoped는 관리자 목록을 스코프로 좁힌다. org 는 자기 조직 타겟이나 산하 그룹 타겟, group 은 자기 그룹 타겟만 본다.
func (r *repo) ListAllScoped(orgID, groupID int64) ([]Announcement, error) {
	var out []Announcement
	q := r.db.Model(&Announcement{})
	if groupID > 0 {
		q = q.Where("target_group_id = ?", groupID)
	} else if orgID > 0 {
		q = q.Where("target_org_id = ? OR target_group_id IN (SELECT id FROM `groups` WHERE org_id = ?)", orgID, orgID)
	}
	return out, q.Order("id DESC").Find(&out).Error
}

func (r *repo) Get(id int64) (*Announcement, error) {
	var a Announcement
	if err := r.db.First(&a, id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *repo) Create(a *Announcement) error { return r.db.Create(a).Error }

func (r *repo) Update(id int64, fields map[string]any) error {
	return r.db.Model(&Announcement{}).Where("id = ?", id).Updates(fields).Error
}

func (r *repo) Toggle(id int64) error {
	return r.db.Model(&Announcement{}).Where("id = ?", id).
		Update("active", gorm.Expr("NOT active")).Error
}

func (r *repo) Delete(id int64) error { return r.db.Delete(&Announcement{}, id).Error }
