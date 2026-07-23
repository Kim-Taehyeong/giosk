package auth

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// ErrNotFound는 조회 실패 sentinel.
var ErrNotFound = errors.New("not found")

// Repository는 auth 도메인의 영속성 계약. 함수 1개 = 단일 쿼리.
type Repository interface {
	FindUserByUsername(username string) (*User, error)
	FindUserByID(id int64) (*User, error)
	CreateUser(u *User) error
	UpdateSSHKey(userID int64, key string) error
	UpdatePassword(userID int64, hash string) error
	CreateJoinRequest(userID, groupID int64) error
	CreateSession(s *Session) error
	FindSession(key string) (*Session, error)
	DeleteSession(key string) error
}

// gormRepo는 Repository의 GORM 구현.
type gormRepo struct{ db *gorm.DB }

// NewRepository는 GORM 기반 Repository를 만든다.
func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

func (r *gormRepo) FindUserByUsername(username string) (*User, error) {
	return r.firstUser("username = ?", username)
}

func (r *gormRepo) FindUserByID(id int64) (*User, error) {
	return r.firstUser("id = ?", id)
}

// firstUser는 조건 1건 조회 공통 헬퍼(ErrNotFound 정규화).
func (r *gormRepo) firstUser(cond string, args ...any) (*User, error) {
	var u User
	err := r.db.Where(cond, args...).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *gormRepo) CreateUser(u *User) error { return r.db.Create(u).Error }

func (r *gormRepo) UpdateSSHKey(userID int64, key string) error {
	return r.db.Model(&User{}).Where("id = ?", userID).Update("ssh_public_key", key).Error
}

func (r *gormRepo) UpdatePassword(userID int64, hash string) error {
	return r.db.Model(&User{}).Where("id = ?", userID).Update("password_hash", hash).Error
}

func (r *gormRepo) CreateJoinRequest(userID, groupID int64) error {
	return r.db.Exec(
		`INSERT INTO group_join_requests (user_id, group_id, status) VALUES (?, ?, 'pending')`,
		userID, groupID,
	).Error
}

func (r *gormRepo) CreateSession(s *Session) error { return r.db.Create(s).Error }

func (r *gormRepo) FindSession(key string) (*Session, error) {
	var s Session
	err := r.db.Where("session_key = ? AND expires_at > ?", key, time.Now().UTC()).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *gormRepo) DeleteSession(key string) error {
	return r.db.Where("session_key = ?", key).Delete(&Session{}).Error
}
