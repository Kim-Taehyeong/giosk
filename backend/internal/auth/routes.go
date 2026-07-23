package auth

import "github.com/gin-gonic/gin"

// Register는 auth 라우트를 등록한다.
//
//	공개:  POST /auth/login, /auth/signup, GET /auth/logout
//	인증:  GET  /auth/me, PUT /auth/me/ssh-key
func Register(api gin.IRouter, h *Handler) {
	api.POST("/auth/login", h.Login)
	api.POST("/auth/signup", h.Signup)
	api.GET("/auth/logout", h.Logout)

	authed := api.Group("", h.RequireAuth())
	authed.GET("/auth/me", h.Me)
	authed.PUT("/auth/me/ssh-key", h.SetSSHKey)
	authed.PUT("/auth/me/password", h.ChangePassword)
}
