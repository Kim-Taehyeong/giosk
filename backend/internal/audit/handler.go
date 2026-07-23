package audit

import (
	"giosk/internal/authz"
	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// Handler는 감사 로그 조회(관리자).
type Handler struct{ repo Repository }

func NewHandler(repo Repository) *Handler { return &Handler{repo: repo} }

func (h *Handler) List(c *gin.Context) {
	items, err := h.repo.List(200)
	if err != nil {
		httpx.Internal(c, "감사 로그 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// ListScoped는 감사 로그를 호출자 스코프로 좁힌다(단일 /console 트리).
func (h *Handler) ListScoped(c *gin.Context) {
	orgID, groupID := authz.CurrentScope(c).EffectiveIDs()
	items, err := h.repo.ListScoped(orgID, groupID, 200)
	if err != nil {
		httpx.Internal(c, "감사 로그 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}
