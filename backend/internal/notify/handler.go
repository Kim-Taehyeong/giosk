package notify

import (
	"giosk/internal/auth"
	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// Handler는 알림 설정 조회/저장(사용자·관리자 공용, scope 로 분기).
type Handler struct{ repo Repository }

func NewHandler(repo Repository) *Handler { return &Handler{repo: repo} }

// ── 사용자 scope ──────────────────────────
func (h *Handler) GetUser(c *gin.Context) { h.get(c, ScopeUser, auth.CurrentUser(c).ID) }
func (h *Handler) PutUser(c *gin.Context) { h.put(c, ScopeUser, auth.CurrentUser(c).ID) }

// ── 관리자 scope(owner=0 전역) ──────────────
func (h *Handler) GetAdmin(c *gin.Context) { h.get(c, ScopeAdmin, 0) }
func (h *Handler) PutAdmin(c *gin.Context) { h.put(c, ScopeAdmin, 0) }

func (h *Handler) get(c *gin.Context, scope string, owner int64) {
	cfg, err := h.repo.Get(scope, owner)
	if err != nil {
		httpx.Internal(c, "알림 설정 조회 실패")
		return
	}
	httpx.OK(c, cfg)
}

func (h *Handler) put(c *gin.Context, scope string, owner int64) {
	var req PutReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "본문 오류")
		return
	}
	if err := h.repo.Replace(scope, owner, req); err != nil {
		httpx.Internal(c, "알림 설정 저장 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}
