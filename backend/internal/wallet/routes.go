package wallet

import "github.com/gin-gonic/gin"

// RegisterUser는 인증 사용자용 개인 지갑.
func RegisterUser(authed gin.IRouter, h *Handler) {
	authed.GET("/me/wallet", h.MyWallet)
}

// RegisterGroupScoped은 그룹 관리자(project_admin)용 그룹 지갑/예산.
func RegisterGroupScoped(authed gin.IRouter, h *Handler, projAdmin gin.HandlerFunc) {
	g := authed.Group("/groups/:id", projAdmin)
	g.GET("/wallet", h.GroupWallet)
	g.PUT("/wallet/budget", h.SetBudget)
	g.PUT("/default-member-budget", h.SetDefaultMemberBudget)
	g.POST("/wallet/allocate", h.AllocateMember)             // 멤버에게 그룹 풀에서 배분
	g.PUT("/wallet/refill", h.SetGroupRefill)                // 팀 정기 리필(그룹관리자)
	g.POST("/wallet/refill/now", h.RefillGroupNow)           // 팀 즉시 리필
	g.PUT("/members/:userId/refill", h.SetMemberRefill)      // 멤버별 정기 리필 금액 개별 설정
	g.POST("/members/:userId/refill/now", h.RefillMemberNow) // 멤버 즉시 리필
}

// RegisterAdmin은 플랫폼 관리자 grant(개인/그룹).
func RegisterAdmin(admin gin.IRouter, h *Handler) {
	admin.POST("/users/:id/grant-credit", h.GrantUser)
	admin.POST("/groups/:id/wallet/grant", h.GrantGroup)
	admin.PUT("/groups/:id/wallet/budget", h.SetBudget)
	admin.PUT("/groups/:id/wallet/refill", h.SetGroupRefill)      // 팀 정기 리필
	admin.PUT("/users/:id/refill", h.SetUserRefill)               // 개인 정기 리필
	admin.POST("/groups/:id/wallet/refill/now", h.RefillGroupNow) // 팀 즉시 리필
	admin.POST("/users/:id/refill/now", h.RefillUserNow)          // 개인 즉시 리필
}
