package policy

import "github.com/gin-gonic/gin"

// RegisterAdmin은 하드 제한 관리 라우트를 등록한다(플랫폼 관리자).
//
//	PUT /admin/limits/user/:id | group/:id | org/:id  — 계층별 하드 제한 설정
//	GET /admin/limits/user/:id/effective              — 계층 해석된 최종 상한
func RegisterAdmin(admin gin.IRouter, h *Handler) {
	admin.GET("/limits", h.List)             // 정책 화면 — 전 범위 한 목록
	admin.PUT("/limits/global", h.SetGlobal) // 전역 상한 런타임 조정
	admin.GET("/limits/user/:id", h.Get(LevelUser))
	admin.GET("/limits/group/:id", h.Get(LevelGroup))
	admin.GET("/limits/org/:id", h.Get(LevelOrg))
	admin.PUT("/limits/user/:id", h.Set(LevelUser))
	admin.PUT("/limits/group/:id", h.Set(LevelGroup))
	admin.PUT("/limits/org/:id", h.Set(LevelOrg))
	admin.GET("/limits/user/:id/effective", h.Effective)
}
