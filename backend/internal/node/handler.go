package node

import (
	"strconv"

	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// Handler는 node HTTP 핸들러(얇게).
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) List(c *gin.Context) {
	items, err := h.svc.List(c.Request.Context())
	if err != nil {
		httpx.Internal(c, "노드 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// GPUs는 노드 내 GPU별 실측 지표(DCGM).
func (h *Handler) GPUs(c *gin.Context) {
	httpx.OK(c, gin.H{"items": h.svc.NodeGPUs(c.Request.Context(), c.Param("name"))})
}

// Metrics는 노드 상세 페이지용 시계열 + 인사이트(?hours=24).
func (h *Handler) Metrics(c *gin.Context) {
	hours, _ := strconv.Atoi(c.Query("hours"))
	points, insights := h.svc.NodeMetrics(c.Request.Context(), c.Param("name"), hours)
	httpx.OK(c, gin.H{"points": points, "insights": insights})
}

// PhysicalNodes는 사용자 화면용 물리 노드 목록.
func (h *Handler) PhysicalNodes(c *gin.Context) {
	items, err := h.svc.PhysicalNodes(c.Request.Context())
	if err != nil {
		httpx.Internal(c, "노드 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"nodes": items})
}

func (h *Handler) Cordon(c *gin.Context)   { h.cordon(c, true) }
func (h *Handler) Uncordon(c *gin.Context) { h.cordon(c, false) }

func (h *Handler) cordon(c *gin.Context, on bool) {
	if err := h.svc.Cordon(c.Request.Context(), c.Param("name"), on); err != nil {
		httpx.Internal(c, "cordon 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) SetConfig(c *gin.Context) {
	var req ConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "본문 오류")
		return
	}
	if err := h.svc.SetConfig(c.Param("name"), req); err != nil {
		httpx.Internal(c, "노드 설정 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) CreateLease(c *gin.Context) {
	var req LeaseReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "userId, leaseHours 필요")
		return
	}
	l, err := h.svc.CreateLease(c.Request.Context(), c.Param("name"), req)
	if err != nil {
		httpx.Internal(c, "임대 생성 실패")
		return
	}
	httpx.Created(c, l)
}

func (h *Handler) ExtendLease(c *gin.Context) {
	var body struct {
		Hours int64 `json:"hours"`
	}
	_ = c.ShouldBindJSON(&body)
	if err := h.svc.Extend(leaseID(c), body.Hours); err != nil {
		httpx.Internal(c, "연장 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) ReleaseLease(c *gin.Context) {
	if err := h.svc.Release(c.Request.Context(), leaseID(c)); err != nil {
		httpx.Internal(c, "임대 해제 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// AgentLeases는 node-agent 전용(agent 토큰 인증).
func (h *Handler) AgentLeases(c *gin.Context) {
	items, err := h.svc.AgentLeases(c.Request.Context(), c.Query("node"))
	if err != nil {
		httpx.Internal(c, "에이전트 임대 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func leaseID(c *gin.Context) int64 { id, _ := strconv.ParseInt(c.Param("id"), 10, 64); return id }
