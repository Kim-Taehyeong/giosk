package alert

import (
	"errors"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("not found")

// Repository는 워크로드 알림 영속성 계약(사용자 스코프).
type Repository interface {
	ListByUser(userID int64) ([]Alert, error)
	Create(a *Alert) error
	Toggle(id, userID int64) error
	Delete(id, userID int64) error
}

type gormRepo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

func (r *gormRepo) ListByUser(userID int64) ([]Alert, error) {
	var out []Alert
	return out, r.db.Where("user_id = ?", userID).Order("id DESC").Find(&out).Error
}

func (r *gormRepo) Create(a *Alert) error { return r.db.Create(a).Error }

func (r *gormRepo) Toggle(id, userID int64) error {
	res := r.db.Exec(`UPDATE workload_alerts SET enabled = NOT enabled WHERE id = ? AND user_id = ?`, id, userID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepo) Delete(id, userID int64) error {
	res := r.db.Where("id = ? AND user_id = ?", id, userID).Delete(&Alert{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
