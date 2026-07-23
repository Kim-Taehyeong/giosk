// Package alert는 워크로드 가용 알림(GPU/노드) 구독을 사용자별로 관리한다.
package alert

import "time"

// Alert는 workload_alerts 테이블 매핑.
type Alert struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	UserID    int64     `gorm:"column:user_id" json:"-"`
	Type      string    `gorm:"column:type" json:"type"`     // gpu | node
	Target    string    `gorm:"column:target" json:"target"`
	Enabled   bool      `gorm:"column:enabled" json:"enabled"`
	CreatedAt time.Time `gorm:"column:created_at" json:"-"`
}

func (Alert) TableName() string { return "workload_alerts" }

// CreateReq — 알림 생성 바디.
type CreateReq struct {
	Type   string `json:"type" binding:"required"`
	Target string `json:"target" binding:"required"`
}
