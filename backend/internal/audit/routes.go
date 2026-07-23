package audit

import "github.com/gin-gonic/gin"

// RegisterAdmin은 감사 로그 조회 라우트.
func RegisterAdmin(admin gin.IRouter, h *Handler) {
	admin.GET("/audit", h.List)
}
