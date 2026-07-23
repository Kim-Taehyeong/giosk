package dashboard

import "github.com/gin-gonic/gin"

// RegisterUser는 인증 사용자용 대시보드.
func RegisterUser(authed gin.IRouter, h *Handler) {
	authed.GET("/dashboard", h.User)
}

// RegisterAdmin은 플랫폼 관리자 대시보드.
func RegisterAdmin(admin gin.IRouter, h *Handler) {
	admin.GET("/dashboard", h.Admin)
}
