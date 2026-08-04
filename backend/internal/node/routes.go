package node

import (
	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// RegisterUser는 사용자 물리 노드 조회(hybrid 노출).
func RegisterUser(authed gin.IRouter, h *Handler) {
	authed.GET("/nodes/physical", h.PhysicalNodes)
}

// RegisterAdmin은 플랫폼 관리자 노드/임대 관리.
func RegisterAdmin(admin gin.IRouter, h *Handler) {
	admin.GET("/nodes", h.List)
	admin.GET("/nodes/:name/gpus", h.GPUs)
	admin.GET("/nodes/:name/metrics", h.Metrics)
	admin.GET("/nodes/:name/mode-impact", h.ModeImpact) // 공유 모드 변경 전 영향 세션 수
	admin.PUT("/nodes/:name", h.SetConfig)
	admin.POST("/nodes/:name/cordon", h.Cordon)
	admin.POST("/nodes/:name/uncordon", h.Uncordon)
	admin.POST("/nodes/:name/lease", h.CreateLease)
	admin.POST("/node-leases/:id/extend", h.ExtendLease)
	admin.POST("/node-leases/:id/release", h.ReleaseLease)
}

// RegisterAgent는 node-agent 전용(공유 토큰 인증).
func RegisterAgent(api gin.IRouter, h *Handler, agentToken string) {
	g := api.Group("/node-agent", agentAuth(agentToken))
	g.GET("/leases", h.AgentLeases)
}

// agentAuth는 Authorization: Bearer <agentToken> 을 검사한다.
func agentAuth(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if token == "" || c.GetHeader("Authorization") != "Bearer "+token {
			httpx.Unauthorized(c, "invalid agent token")
			return
		}
		c.Next()
	}
}
