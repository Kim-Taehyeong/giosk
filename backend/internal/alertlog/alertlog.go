// Package alertlog는 발화된 경고 이벤트를 alert_events 표에 영속하고 조회한다(감시월 통합 피드).
// notify 엔진이 Record 로 적재하고, dashboard 가 Recent 로 읽어 운영/인프라 경고를 한 화면에 띄운다.
package alertlog

import (
	"time"

	"gorm.io/gorm"
)

// Event는 alert_events 한 행.
type Event struct {
	ID       int64     `json:"-" gorm:"column:id"`
	TS       time.Time `json:"ts" gorm:"column:ts"`
	Source   string    `json:"source" gorm:"column:source"`     // infra | ops
	Severity string    `json:"severity" gorm:"column:severity"` // info | warn | err
	Type     string    `json:"type" gorm:"column:type"`
	Target   string    `json:"target" gorm:"column:target"`
	Message  string    `json:"message" gorm:"column:message"`
}

func (Event) TableName() string { return "alert_events" }

// Store는 alert_events 읽기/쓰기.
type Store struct{ db *gorm.DB }

func New(db *gorm.DB) *Store { return &Store{db: db} }

// Record는 경고 이벤트 한 건을 적재한다(ts 는 DB 기본값). nil-safe.
func (s *Store) Record(ev Event) {
	if s == nil || s.db == nil {
		return
	}
	if ev.TS.IsZero() {
		ev.TS = time.Now() // GORM zero-value 가 DB 기본값(CURRENT_TIMESTAMP)을 덮어쓰지 않도록.
	}
	s.db.Create(&ev)
}

// Recent는 최근 경고 이벤트를 최신순으로 반환한다.
func (s *Store) Recent(limit int) []Event {
	out := []Event{}
	if s == nil || s.db == nil {
		return out
	}
	if limit <= 0 {
		limit = 50
	}
	s.db.Order("ts DESC").Limit(limit).Find(&out)
	return out
}
