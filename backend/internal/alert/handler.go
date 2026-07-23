package alert

import (
	"errors"
	"strconv"

	"giosk/internal/auth"
	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// Handler는 워크로드 알림 HTTP 핸들러(사용자 스코프).
type Handler struct{ repo Repository }

func NewHandler(repo Repository) *Handler { return &Handler{repo: repo} }

func (h *Handler) List(c *gin.Context) {
	items, err := h.repo.ListByUser(uid(c))
	if err != nil {
		httpx.Internal(c, "알림 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) Create(c *gin.Context) {
	var req CreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "type, target 필요")
		return
	}
	a := Alert{UserID: uid(c), Type: req.Type, Target: req.Target, Enabled: true}
	if err := h.repo.Create(&a); err != nil {
		httpx.Internal(c, "알림 생성 실패")
		return
	}
	httpx.Created(c, a)
}

func (h *Handler) Toggle(c *gin.Context) { h.mutate(c, h.repo.Toggle) }
func (h *Handler) Delete(c *gin.Context) { h.mutate(c, h.repo.Delete) }

func (h *Handler) mutate(c *gin.Context, fn func(id, userID int64) error) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := fn(id, uid(c)); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.NotFound(c, "알림을 찾을 수 없습니다")
			return
		}
		httpx.Internal(c, "처리 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func uid(c *gin.Context) int64 { return auth.CurrentUser(c).ID }
