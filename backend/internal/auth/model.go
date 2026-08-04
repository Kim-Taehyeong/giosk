package auth

import "time"

// User는 users 테이블 매핑. password_hash 는 JSON 비노출.
type User struct {
	ID              int64      `gorm:"primaryKey" json:"id"`
	Username        string     `gorm:"column:username" json:"username"`
	PasswordHash    string     `gorm:"column:password_hash" json:"-"`
	Email           string     `gorm:"column:email" json:"email"`
	FirstName       string     `gorm:"column:first_name" json:"firstName"`
	LastName        string     `gorm:"column:last_name" json:"lastName"`
	FirstNameEn     string     `gorm:"column:first_name_en" json:"firstNameEn"`
	LastNameEn      string     `gorm:"column:last_name_en" json:"lastNameEn"`
	SponsorName     string     `gorm:"column:sponsor_name" json:"sponsorName"`
	SSHPublicKey    string     `gorm:"column:ssh_public_key" json:"-"`
	Role            string     `gorm:"column:role" json:"role"`
	Status          string     `gorm:"column:status" json:"status"`
	SignupGroupID   *int64     `gorm:"column:signup_group_id" json:"-"` // 가입신청 시 선택 그룹(승인 시 멤버십 생성)
	TermsAcceptedAt *time.Time `gorm:"column:terms_accepted_at" json:"-"`
	CreatedAt       time.Time  `gorm:"column:created_at" json:"createdAt"`
}

func (User) TableName() string { return "users" }

// Session은 sessions 테이블 매핑(세션키로 user 를 찾는다).
type Session struct {
	SessionKey string    `gorm:"primaryKey;column:session_key" json:"sessionkey"`
	UserID     int64     `gorm:"column:user_id" json:"-"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"-"`
	ExpiresAt  time.Time `gorm:"column:expires_at" json:"-"`
}

func (Session) TableName() string { return "auth_sessions" }

// 사용자 상태 enum.
const (
	StatusPending   = "pending"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
	StatusSuspended = "suspended"

	RoleAdmin  = "admin"
	RoleMember = "member"
)

// LoginReq 와 SignupReq 는 요청 바디.
type LoginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type SignupReq struct {
	Username      string `json:"username" binding:"required"`
	Password      string `json:"password" binding:"required"`
	Email         string `json:"email" binding:"required"`
	FirstName     string `json:"firstName"`                   // 선택 항목이다. 단일 이름은 LastName 에 저장한다
	LastName      string `json:"lastName" binding:"required"` // 표시 이름(성+이름 합본 또는 전체 이름)
	FirstNameEn   string `json:"firstNameEn"`
	LastNameEn    string `json:"lastNameEn"`
	SponsorName   string `json:"sponsor"`
	GroupID       *int64 `json:"groupId"`
	TermsAccepted bool   `json:"termsAccepted"`
}

// LoginRes는 로그인 성공 응답(프론트 user 객체와 호환).
type LoginRes struct {
	SessionKey string `json:"sessionkey"`
	User       *User  `json:"user"`
}
