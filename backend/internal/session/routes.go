package session

import "github.com/gin-gonic/gin"

// RegisterUser는 인증 사용자용 세션 라우트(authed 그룹에 등록).
func RegisterUser(authed gin.IRouter, h *Handler) {
	authed.GET("/instances", h.List)
	authed.POST("/instances", h.Create)
	authed.DELETE("/instances/:id", h.Delete)
	authed.POST("/instances/:id/stop", h.Stop)
	authed.POST("/instances/:id/start", h.Start)
	authed.POST("/instances/:id/reconfigure", h.Reconfigure) // 중단 세션 자원 변경(GPU 붙이기/떼기)
	authed.POST("/instances/:id/extend", h.Extend)
	authed.GET("/instances/:id/connection", h.Connection)
	authed.POST("/instances/:id/access", h.Access)
	// 벌크는 /instances 하위에 둘 수 없다 — gin 은 같은 자리의 정적 세그먼트(metrics)와
	// 와일드카드(:id)를 함께 등록하면 패닉한다.
	authed.GET("/metrics/sessions", h.MyUsage)
	authed.GET("/instances/:id/metrics", h.Usage)
	authed.GET("/instances/:id/logs", h.Logs)
	authed.GET("/instances/:id/audit", h.Audit)
	authed.GET("/instances/:id/history", h.History)
	authed.GET("/instances/:id/terminal", h.Terminal) // 웹터미널 websocket(브라우저 xterm)
}

// RegisterAdmin은 플랫폼 관리자 세션 관제 라우트.
func RegisterAdmin(admin gin.IRouter, h *Handler) {
	admin.GET("/sessions", h.AdminList)
	admin.GET("/sessions/metrics", h.AdminUsage)
	admin.GET("/sessions/:id", h.AdminGet)
	admin.GET("/sessions/:id/audit", h.AdminAudit)
	admin.GET("/sessions/:id/history", h.AdminHistory)
	admin.GET("/sessions/:id/logs", h.AdminLogs)
	admin.GET("/sessions/:id/describe", h.AdminDescribe)
	admin.POST("/sessions/:id/terminate", h.AdminTerminate)
	admin.POST("/sessions/:id/stop", h.AdminStop)
	admin.DELETE("/sessions/:id", h.AdminDelete)
}
