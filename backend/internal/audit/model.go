// Package audit는 인증된 변경 요청(POST/PUT/DELETE)을 자동 기록·조회한다.
//
// 서비스 코드를 침범하지 않도록 미들웨어에서 actor/action/target/result 를
// 도출해 audit_logs 에 적재한다(읽기 요청은 제외). 관리자만 조회 가능.
package audit

import "time"

// Log는 audit_logs 테이블 매핑.
type Log struct {
	ID            int64     `gorm:"primaryKey" json:"-"`
	ActorID       *int64    `gorm:"column:actor_id" json:"-"`
	ActorUsername string    `gorm:"column:actor_username" json:"actorUsername"`
	Action        string    `gorm:"column:action" json:"action"`
	Target        string    `gorm:"column:target" json:"targetUsername"`
	Result        string    `gorm:"column:result" json:"result"`
	IP            string    `gorm:"column:ip" json:"-"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"createdAt"`
}

func (Log) TableName() string { return "audit_logs" }
