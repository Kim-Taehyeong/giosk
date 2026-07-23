package topup

import (
	"strconv"

	"giosk/internal/auth"
	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// Handler는 topup HTTP 핸들러(얇게).
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Create(c *gin.Context) {
	var req CreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "targetType, targetId, amount 필요")
		return
	}
	if err := h.svc.Create(auth.CurrentUser(c).ID, req); err != nil {
		httpx.Internal(c, "충전 요청 실패")
		return
	}
	httpx.Created(c, gin.H{"ok": true})
}

func (h *Handler) MyRequests(c *gin.Context) {
	items, err := h.svc.MyRequests(auth.CurrentUser(c).ID)
	if err != nil {
		httpx.Internal(c, "요청 내역 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) GroupQueue(c *gin.Context) {
	items, err := h.svc.PendingForGroup(gid(c))
	if err != nil {
		httpx.Internal(c, "승인 큐 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) PlatformQueue(c *gin.Context) {
	items, err := h.svc.PendingAll()
	if err != nil {
		httpx.Internal(c, "승인 큐 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) Approve(c *gin.Context) { h.review(c, h.svc.Approve, "승인 실패") }
func (h *Handler) Reject(c *gin.Context)  { h.review(c, h.svc.Reject, "거절 실패") }

func (h *Handler) review(c *gin.Context, fn func(id, reviewer int64) error, msg string) {
	reqID, _ := strconv.ParseInt(c.Param("reqId"), 10, 64)
	if err := fn(reqID, auth.CurrentUser(c).ID); err != nil {
		httpx.Internal(c, msg)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func gid(c *gin.Context) int64 { id, _ := strconv.ParseInt(c.Param("id"), 10, 64); return id }
