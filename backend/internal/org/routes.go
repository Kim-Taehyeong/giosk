package org

import "github.com/gin-gonic/gin"

// RegisterPublic은 인증 불필요 라우트(가입 폼용 공개 카탈로그).
func RegisterPublic(api gin.IRouter, h *Handler) {
	api.GET("/public/orgs", h.PublicCatalog)
}

// RegisterAdmin은 플랫폼 관리자 라우트. adminMW 는 호출측(main)에서 주입.
func RegisterAdmin(admin gin.IRouter, h *Handler) {
	admin.GET("/orgs", h.List)
	admin.POST("/orgs", h.Create)
	admin.PUT("/orgs/:id", h.Update)
	admin.DELETE("/orgs/:id", h.Archive)
	admin.POST("/orgs/:id/grant", h.Grant)
}
