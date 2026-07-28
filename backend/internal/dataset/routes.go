package dataset

import "github.com/gin-gonic/gin"

// RegisterUser는 인증 사용자용 데이터셋 라우트.
func RegisterUser(authed gin.IRouter, h *Handler) {
	authed.GET("/datasets", h.List)
	authed.POST("/datasets/register", h.Register)
	authed.DELETE("/datasets/:id", h.Delete)
}

// RegisterAdmin은 플랫폼 관리자 데이터셋 요청/캐시 관리.
func RegisterAdmin(admin gin.IRouter, h *Handler) {
	admin.GET("/dataset-requests", h.PendingRequests)
	admin.POST("/dataset-requests/:reqId/approve", h.Approve)
	admin.POST("/dataset-requests/:reqId/reject", h.Reject)
	admin.POST("/datasets/:id/cache", h.ToggleCache)
	// 관리자 데이터셋 등록: ① NFS 인박스(SCP 복사 후 선택 등록) ② URL(wget). 웹 파일 업로드는 폐지.
	admin.GET("/datasets/inbox", h.Inbox)
	admin.POST("/datasets/register-nfs", h.RegisterNFS)
	admin.POST("/datasets/register-url", h.RegisterURL)
	admin.PATCH("/datasets/:id", h.UpdateDescription) // 데이터셋 설명 수정
}
