package billing

import "github.com/gin-gonic/gin"

// RegisterAdmin은 빌링 showback 라우트.
func RegisterAdmin(admin gin.IRouter, h *Handler) {
	admin.GET("/billing", h.Showback)
}
