package notify

import "github.com/gin-gonic/gin"

// RegisterUser는 사용자 알림 설정.
func RegisterUser(authed gin.IRouter, h *Handler) {
	authed.GET("/notify", h.GetUser)
	authed.PUT("/notify", h.PutUser)
}

// RegisterAdmin은 관리자(전역) 알림 설정.
func RegisterAdmin(admin gin.IRouter, h *Handler) {
	admin.GET("/notify", h.GetAdmin)
	admin.PUT("/notify", h.PutAdmin)
}
