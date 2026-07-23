package dashboard

import (
	"giosk/internal/auth"
	"giosk/internal/authz"
	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// Handler는 dashboard HTTP 핸들러(얇게).
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) User(c *gin.Context) {
	uid := auth.CurrentUser(c).ID
	httpx.OK(c, h.svc.User(c.Request.Context(), uid))
}

func (h *Handler) Admin(c *gin.Context) {
	httpx.OK(c, h.svc.Admin(c.Request.Context()))
}

// OpsScoped는 운영 대시보드를 호출자 스코프로 반환한다(단일 /console 트리).
func (h *Handler) OpsScoped(c *gin.Context) {
	orgID, groupID := authz.CurrentScope(c).EffectiveIDs()
	httpx.OK(c, h.svc.Ops(orgID, groupID)) // platform 은 orgID/groupID=0 → 전체
}
