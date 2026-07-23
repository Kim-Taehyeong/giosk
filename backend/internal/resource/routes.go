package resource

import "github.com/gin-gonic/gin"

// RegisterUser는 인증 사용자용 읽기(세션 마법사 카탈로그).
func RegisterUser(authed gin.IRouter, h *Handler) {
	authed.GET("/offerings", h.ListOfferings)
	authed.GET("/presets", h.ListPresets)
	authed.GET("/gpu-types", h.GpuTypes)
	authed.GET("/resources/availability", h.Availability)
}

// RegisterAdmin은 플랫폼 관리자 CRUD.
func RegisterAdmin(admin gin.IRouter, h *Handler) {
	admin.GET("/offerings", h.ListOfferings)
	admin.POST("/offerings", h.SaveOffering)
	admin.PUT("/offerings/:id", h.SaveOffering)
	admin.DELETE("/offerings/:id", h.DeleteOffering)
	admin.GET("/presets", h.ListPresets)
	admin.POST("/presets", h.SavePreset)
	admin.PUT("/presets/:id", h.SavePreset)
	admin.DELETE("/presets/:id", h.DeletePreset)
	admin.GET("/gpu-types", h.GpuTypes)
	admin.GET("/gpu-pricing", h.GpuPricing)
	admin.PUT("/gpu-pricing", h.SetGpuPricing)
}
