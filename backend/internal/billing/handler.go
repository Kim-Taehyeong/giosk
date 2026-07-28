package billing

import (
	"giosk/internal/authz"
	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// Handler는 빌링 showback 조회(관리자).
type Handler struct{ repo Repository }

func NewHandler(repo Repository) *Handler { return &Handler{repo: repo} }

// rangeOf는 ?from=&to= 쿼리(YYYY-MM-DD 또는 ISO)를 빌링 집계 기간으로 읽는다(빈 값=전체 기간).
func rangeOf(c *gin.Context) Range { return Range{From: c.Query("from"), To: c.Query("to")} }

func (h *Handler) Showback(c *gin.Context) {
	rng := rangeOf(c)
	groups := h.repo.ByGroup(rng)
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
		ByUser:        h.repo.ByUser(rng),
		ByOrg:         orgs,
	})
}

// ShowbackScoped는 빌링 집계를 호출자 스코프로 좁힌다(단일 /console 트리).
func (h *Handler) ShowbackScoped(c *gin.Context) {
	rng := rangeOf(c)
	orgID, groupID := authz.CurrentScope(c).EffectiveIDs()
	groups := h.repo.ByGroupScoped(orgID, groupID, rng)
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
		ByUser:        h.repo.ByUserScoped(orgID, groupID, rng),
		ByOrg:         orgs,
	})
}
