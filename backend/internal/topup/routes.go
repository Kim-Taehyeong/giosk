package topup

import "github.com/gin-gonic/gin"

// RegisterUser는 인증 사용자용 충전 요청 생성/내역.
func RegisterUser(authed gin.IRouter, h *Handler) {
	authed.POST("/me/topup-requests", h.Create)
	authed.GET("/me/topup-requests", h.MyRequests)
}

// RegisterGroupScoped은 그룹 관리자(project_admin)용 멤버 충전 승인 큐.
func RegisterGroupScoped(authed gin.IRouter, h *Handler, projAdmin gin.HandlerFunc) {
	g := authed.Group("/groups/:id", projAdmin)
	g.GET("/topup-requests", h.GroupQueue)
	g.POST("/topup-requests/:reqId/approve", h.Approve)
	g.POST("/topup-requests/:reqId/reject", h.Reject)
}

// RegisterAdmin은 플랫폼 관리자 승인 큐(그룹→조직, 조직→플랫폼).
func RegisterAdmin(admin gin.IRouter, h *Handler) {
	admin.GET("/topup-requests", h.PlatformQueue)
	admin.POST("/topup-requests/:reqId/approve", h.Approve)
	admin.POST("/topup-requests/:reqId/reject", h.Reject)
}
