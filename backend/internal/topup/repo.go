package topup

import (
	"errors"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("not found")

// Repository는 topup 영속성 계약.
type Repository interface {
	Create(r *Request) error
	Get(id int64) (*Request, error)
	MyRequests(userID int64) ([]Item, error)
	PendingForGroup(groupID int64) ([]Item, error) // target=user, group 멤버
	PendingForOrg(orgID int64) ([]Item, error)     // target=group, org 산하
	PendingAll() ([]Item, error)                    // target=group|org (플랫폼 큐)
	MarkReviewed(id, reviewer int64, status string) error
}

type gormRepo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

func (r *gormRepo) Create(req *Request) error { return r.db.Create(req).Error }

func (r *gormRepo) Get(id int64) (*Request, error) {
	var req Request
	err := r.db.Where("id = ?", id).First(&req).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &req, err
}

func (r *gormRepo) MyRequests(userID int64) ([]Item, error) {
	var out []Item
	err := r.db.Raw(`
		SELECT t.id, u.username, TRIM(CONCAT(COALESCE(u.last_name,''),COALESCE(u.first_name,''))) AS requester_name,
		       t.target_type, t.target_id, t.amount, t.reason, t.created_at
		FROM topup_requests t JOIN users u ON u.id = t.requester_user_id
		WHERE t.requester_user_id = ? ORDER BY t.id DESC`, userID).Scan(&out).Error
	return out, err
}

// PendingForGroup — 해당 그룹 멤버를 대상(target=user)으로 한 대기 요청.
func (r *gormRepo) PendingForGroup(groupID int64) ([]Item, error) {
	var out []Item
	err := r.db.Raw(`
		SELECT t.id, u.username, TRIM(CONCAT(COALESCE(u.last_name,''),COALESCE(u.first_name,''))) AS requester_name,
		       t.target_type, t.target_id, t.amount, t.reason, t.created_at
		FROM topup_requests t JOIN users u ON u.id = t.requester_user_id
		WHERE t.status = 'pending' AND t.target_type = 'user'
		  AND t.target_id IN (SELECT user_id FROM memberships WHERE group_id = ? AND status='active')
		ORDER BY t.id`, groupID).Scan(&out).Error
	return out, err
}

// PendingForOrg — 조직 승인 큐(target=group, 해당 org 산하 그룹).
func (r *gormRepo) PendingForOrg(orgID int64) ([]Item, error) {
	var out []Item
	err := r.db.Raw(`
		SELECT t.id, u.username, TRIM(CONCAT(COALESCE(u.last_name,''),COALESCE(u.first_name,''))) AS requester_name,
		       t.target_type, t.target_id, t.amount, t.reason, t.created_at
		FROM topup_requests t JOIN users u ON u.id = t.requester_user_id
		WHERE t.status = 'pending' AND t.target_type = 'group'
		  AND t.target_id IN (SELECT id FROM `+"`groups`"+` WHERE org_id = ?)
		ORDER BY t.id`, orgID).Scan(&out).Error
	return out, err
}

// PendingAll — 플랫폼 승인 큐(target=group 또는 org).
func (r *gormRepo) PendingAll() ([]Item, error) {
	var out []Item
	err := r.db.Raw(`
		SELECT t.id, u.username, TRIM(CONCAT(COALESCE(u.last_name,''),COALESCE(u.first_name,''))) AS requester_name,
		       t.target_type, t.target_id, t.amount, t.reason, t.created_at
		FROM topup_requests t JOIN users u ON u.id = t.requester_user_id
		WHERE t.status = 'pending' AND t.target_type IN ('group','org') ORDER BY t.id`).Scan(&out).Error
	return out, err
}

func (r *gormRepo) MarkReviewed(id, reviewer int64, status string) error {
	return r.db.Exec(
		`UPDATE topup_requests SET status=?, reviewer_user_id=?, reviewed_at=NOW() WHERE id=? AND status='pending'`,
		status, reviewer, id).Error
}
