// Package usernotify는 사용자 인앱 알림 수신함을 담당한다.
//
// 알림 엔진(notify.Engine)이 사용자 규칙(크레딧/예산/볼륨/유휴) 위반을 감지하면 Record 로 여기에 적재하고,
// 사용자는 알림센터에서 목록을 보고 토픽바 종 배지로 미읽음 수를 확인한다. 채널(이메일/웹훅)과 별개다.
package usernotify

import (
	"strconv"
	"time"

	"giosk/internal/auth"
	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Note는 user_notifications 행.
type Note struct {
	ID       int64  `gorm:"primaryKey" json:"id"`
	UserID   int64  `gorm:"column:user_id" json:"-"`
	Severity string `gorm:"column:severity" json:"severity"`
	Metric   string `gorm:"column:metric" json:"metric"`
	// 신규 알림은 렌더된 문자열 대신 metric+파라미터만 저장 → 프론트가 현지화 렌더(i18n).
	// 구 알림은 title/body(한국어)가 남아 있어 프론트가 그대로 폴백 표시한다.
	Value     float64    `gorm:"column:value" json:"value"`
	Threshold int        `gorm:"column:threshold" json:"threshold"`
	Title     string     `gorm:"column:title" json:"title"` // legacy(구 알림)
	Body      string     `gorm:"column:body" json:"body"`   // legacy(구 알림)
	ReadAt    *time.Time `gorm:"column:read_at" json:"-"`
	CreatedAt time.Time  `gorm:"column:created_at" json:"createdAt"`
	Read      bool       `gorm:"-" json:"read"`
}

func (Note) TableName() string { return "user_notifications" }

// Store는 인앱 알림 수신함 영속.
type Store struct{ db *gorm.DB }

func New(db *gorm.DB) *Store { return &Store{db: db} }

// Record는 사용자 알림 한 건을 적재한다(엔진 발화 시). metric+value+threshold 만 저장하고
// 표시 문자열은 프론트가 현지화 렌더한다(i18n). title/body 는 저장하지 않는다(구 알림 폴백 전용).
func (s *Store) Record(userID int64, severity, metric string, value float64, threshold int) error {
	if s == nil {
		return nil
	}
	return s.db.Create(&Note{UserID: userID, Severity: severity, Metric: metric, Value: value, Threshold: threshold}).Error
}

// ListByUser는 사용자의 최근 알림(최신순, 최대 100)을 반환한다.
func (s *Store) ListByUser(userID int64) ([]Note, error) {
	var out []Note
	if err := s.db.Where("user_id = ?", userID).Order("id DESC").Limit(100).Find(&out).Error; err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Read = out[i].ReadAt != nil
	}
	return out, nil
}

// UnreadCount는 미읽음 수(토픽바 배지).
func (s *Store) UnreadCount(userID int64) (int64, error) {
	var n int64
	err := s.db.Model(&Note{}).Where("user_id = ? AND read_at IS NULL", userID).Count(&n).Error
	return n, err
}

// MarkRead는 알림 1건을 읽음 처리(본인 것만).
func (s *Store) MarkRead(id, userID int64) error {
	return s.db.Model(&Note{}).Where("id = ? AND user_id = ?", id, userID).Update("read_at", time.Now()).Error
}

// MarkAllRead는 사용자의 미읽음 전체를 읽음 처리.
func (s *Store) MarkAllRead(userID int64) error {
	return s.db.Model(&Note{}).Where("user_id = ? AND read_at IS NULL", userID).Update("read_at", time.Now()).Error
}

// ── HTTP ────────────────────────────────────────────────────────────

type Handler struct{ store *Store }

func NewHandler(s *Store) *Handler { return &Handler{store: s} }

func (h *Handler) List(c *gin.Context) {
	items, err := h.store.ListByUser(uid(c))
	if err != nil {
		httpx.Internal(c, "알림 조회 실패")
		return
	}
	unread := 0
	for _, n := range items {
		if !n.Read {
			unread++
		}
	}
	httpx.OK(c, gin.H{"items": items, "unread": unread})
}

func (h *Handler) Unread(c *gin.Context) {
	n, err := h.store.UnreadCount(uid(c))
	if err != nil {
		httpx.Internal(c, "미읽음 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"unread": n})
}

func (h *Handler) Read(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.store.MarkRead(id, uid(c)); err != nil {
		httpx.Internal(c, "읽음 처리 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) ReadAll(c *gin.Context) {
	if err := h.store.MarkAllRead(uid(c)); err != nil {
		httpx.Internal(c, "읽음 처리 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// RegisterUser는 사용자 인앱 알림 라우트(authed 그룹).
func RegisterUser(authed gin.IRouter, h *Handler) {
	authed.GET("/inbox", h.List)
	authed.GET("/inbox/unread", h.Unread)
	authed.POST("/inbox/:id/read", h.Read)
	authed.POST("/inbox/read-all", h.ReadAll)
}

func uid(c *gin.Context) int64 { return auth.CurrentUser(c).ID }
