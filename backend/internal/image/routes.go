package image

import "github.com/gin-gonic/gin"

// RegisterUser는 인증 사용자용(세션 마법사 이미지 카탈로그 — active).
func RegisterUser(authed gin.IRouter, h *Handler) {
	authed.GET("/images", h.List)
}

// RegisterAdmin은 플랫폼 관리자 이미지 빌드/관리.
func RegisterAdmin(admin gin.IRouter, h *Handler) {
	admin.GET("/images", h.List)
	admin.POST("/images", h.Build)                  // 가이드형 빌드(베이스+패키지→Kaniko)
	admin.POST("/images/external", h.RegisterExternal) // 외부 이미지 등록(빌드 없음, 기본 경로)
	admin.PUT("/images/:id", h.Save)             // 메타 수정
	admin.POST("/images/:id/rebuild", h.Rebuild) // 재빌드(태그 +1)
	admin.POST("/images/:id/status", h.SetStatus)
	admin.GET("/images/:id/logs", h.Logs)         // 빌드/스캔 실패 로그
	admin.GET("/images/:id/cache", h.CacheList)   // 노드 캐시 상태
	admin.POST("/images/:id/cache", h.CacheOn)    // 노드 프리페치
	admin.DELETE("/images/:id/cache", h.CacheOff) // 노드 캐시 해제
	admin.DELETE("/images/:id", h.Delete)
}
