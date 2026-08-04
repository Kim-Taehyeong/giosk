package topup

import "time"

// target_type / status.
const (
	TargetUser  = "user"
	TargetGroup = "group"
	TargetOrg   = "org"

	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
)

// Request는 topup_requests 테이블 매핑.
type Request struct {
	ID              int64      `gorm:"primaryKey" json:"id"`
	RequesterUserID int64      `gorm:"column:requester_user_id" json:"requesterUserId"`
	TargetType      string     `gorm:"column:target_type" json:"targetType"`
	TargetID        int64      `gorm:"column:target_id" json:"targetId"`
	Amount          int        `gorm:"column:amount" json:"amount"`
	Reason          string     `gorm:"column:reason" json:"reason"`
	Status          string     `gorm:"column:status" json:"status"`
	ReviewerUserID  *int64     `gorm:"column:reviewer_user_id" json:"-"`
	CreatedAt       time.Time  `gorm:"column:created_at" json:"createdAt"`
	ReviewedAt      *time.Time `gorm:"column:reviewed_at" json:"-"`
}

func (Request) TableName() string { return "topup_requests" }

// Item은 승인 큐 표시용(요청자 정보 조인).
type Item struct {
	ID            int64     `json:"id"`
	RequesterName string    `json:"requesterName"`
	Username      string    `json:"username"`
	TargetType    string    `json:"targetType"`
	TargetID      int64     `json:"targetId"`
	Amount        int       `json:"amount"`
	Reason        string    `json:"reason"`
	CreatedAt     time.Time `json:"createdAt"`
}

// CreateReq는 충전 요청 생성.
type CreateReq struct {
	TargetType string `json:"targetType" binding:"required"` // user|group|org
	TargetID   int64  `json:"targetId"`                      // user 가 자기충전할 때 0 이면 본인이다
	Amount     int    `json:"amount" binding:"required"`
	Reason     string `json:"reason"`
}
