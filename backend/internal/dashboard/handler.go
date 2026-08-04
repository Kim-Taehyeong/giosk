package dashboard

import (
	"strconv"
	"strings"

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
	httpx.OK(c, h.svc.User(c.Request.Context(), uid, scopeGroup(c)))
}

// scopeGroup은 X-Console-Scope("group:N")에서 활성 팀 id 를 읽는다(없으면 0=전 팀 합산).
// 대시보드 크레딧을 배지/알림과 같은 활성 팀 기준으로 맞추기 위함.
func scopeGroup(c *gin.Context) int64 {
	if sel := c.GetHeader("X-Console-Scope"); strings.HasPrefix(sel, "group:") {
		id, _ := strconv.ParseInt(sel[len("group:"):], 10, 64)
		return id
	}
	return 0
}

func (h *Handler) Admin(c *gin.Context) {
	httpx.OK(c, h.svc.Admin(c.Request.Context()))
}

// OpsScoped는 운영 대시보드를 호출자 스코프로 반환한다(단일 /console 트리).
func (h *Handler) OpsScoped(c *gin.Context) {
	orgID, groupID := authz.CurrentScope(c).EffectiveIDs()
	httpx.OK(c, h.svc.Ops(orgID, groupID)) // platform 은 orgID/groupID 가 0 이라 전체를 본다
}
