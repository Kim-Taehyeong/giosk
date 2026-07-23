package audit

import "gorm.io/gorm"

// Repository는 감사 로그 영속성 계약.
type Repository interface {
	Insert(l *Log) error
	List(limit int) ([]Log, error)
	ListScoped(orgID, groupID int64, limit int) ([]Log, error)
	ListByTarget(target string, limit int) ([]Log, error)
}

type gormRepo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

func (r *gormRepo) Insert(l *Log) error { return r.db.Create(l).Error }

func (r *gormRepo) List(limit int) ([]Log, error) {
	var out []Log
	return out, r.db.Order("id DESC").Limit(limit).Find(&out).Error
}

// ListScoped는 actor 가 스코프(org/group) 소속인 로그만 반환한다.
// group=그룹 멤버가 만든 로그, org=조직 소속이 만든 로그. platform(둘 다 0)=전체.
func (r *gormRepo) ListScoped(orgID, groupID int64, limit int) ([]Log, error) {
	if orgID <= 0 && groupID <= 0 {
		return r.List(limit)
	}
	var sub string
	var arg int64
	if groupID > 0 {
		sub = "SELECT user_id FROM memberships WHERE group_id = ? AND status='active'"
		arg = groupID
	} else {
		sub = "SELECT m.user_id FROM memberships m JOIN `groups` g ON g.id=m.group_id WHERE g.org_id = ? AND m.status='active'"
		arg = orgID
	}
	var out []Log
	return out, r.db.Where("actor_id IN ("+sub+")", arg).
		Order("id DESC").Limit(limit).Find(&out).Error
}

func (r *gormRepo) ListByTarget(target string, limit int) ([]Log, error) {
	var out []Log
	return out, r.db.Where("target = ?", target).Order("id DESC").Limit(limit).Find(&out).Error
}
