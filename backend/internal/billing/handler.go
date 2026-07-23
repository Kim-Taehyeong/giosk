package billing

import (
	"giosk/internal/authz"
	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// Handler는 빌링 showback 조회(관리자).
type Handler struct{ repo Repository }

func NewHandler(repo Repository) *Handler { return &Handler{repo: repo} }

func (h *Handler) Showback(c *gin.Context) {
	groups := h.repo.ByGroup()
	total := 0
	budget := 0
	for _, g := range groups {
		total += g.Consumed
	}
	orgs := h.repo.ByOrg()
	for _, o := range orgs {
		budget += o.CreditPool
	}
	httpx.OK(c, Showback{
		TotalConsumed: total,
		TotalBudget:   budget,
		ByGroup:       groups,
		ByUser:        h.repo.ByUser(),
		ByOrg:         orgs,
	})
}

// ShowbackScoped는 빌링 집계를 호출자 스코프로 좁힌다(단일 /console 트리).
func (h *Handler) ShowbackScoped(c *gin.Context) {
	orgID, groupID := authz.CurrentScope(c).EffectiveIDs()
	groups := h.repo.ByGroupScoped(orgID, groupID)
	total := 0
	for _, g := range groups {
		total += g.Consumed
	}
	orgs := h.repo.ByOrgScoped(orgID, groupID)
	budget := 0
	for _, o := range orgs {
		budget += o.CreditPool
	}
	httpx.OK(c, Showback{
		TotalConsumed: total,
		TotalBudget:   budget,
		ByGroup:       groups,
		ByUser:        h.repo.ByUserScoped(orgID, groupID),
		ByOrg:         orgs,
	})
}
