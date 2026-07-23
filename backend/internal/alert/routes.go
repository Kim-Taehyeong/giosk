package alert

import "github.com/gin-gonic/gin"

// RegisterUser는 사용자 워크로드 알림 라우트.
func RegisterUser(authed gin.IRouter, h *Handler) {
	authed.GET("/alerts", h.List)
	authed.POST("/alerts", h.Create)
	authed.POST("/alerts/:id/toggle", h.Toggle)
	authed.DELETE("/alerts/:id", h.Delete)
}
