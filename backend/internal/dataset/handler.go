package dataset

import (
	"errors"
	"strconv"

	"giosk/internal/auth"
	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// Handler는 dataset HTTP 핸들러(얇게).
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// ── 사용자 ──────────────────────────────
func (h *Handler) List(c *gin.Context) {
	res, err := h.svc.List(c.Request.Context(), auth.CurrentUser(c).ID)
	if err != nil {
		httpx.Internal(c, "데이터셋 조회 실패")
		return
	}
	httpx.OK(c, res)
}

func (h *Handler) Register(c *gin.Context) {
	var req RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "name 필요")
		return
	}
	if err := h.svc.Register(auth.CurrentUser(c).ID, req); err != nil {
		if errors.Is(err, ErrNameTaken) {
			httpx.Err(c, 409, "name_taken", "이미 같은 이름의 데이터셋이 있습니다")
			return
		}
		httpx.Internal(c, "등록 신청 실패")
		return
	}
	httpx.Created(c, gin.H{"ok": true})
}

// ── 관리자 데이터셋 등록: ① NFS 인박스  ② URL(wget) ──

// Inbox는 SCP 안내 경로 + 인박스에 올라온 아카이브 목록을 반환한다.
func (h *Handler) Inbox(c *gin.Context) {
	if !h.svc.UploadEnabled() {
		httpx.Err(c, 503, "upload_disabled", "데이터셋 NFS 마운트가 설정되지 않았습니다")
		return
	}
	files, err := h.svc.InboxList()
	if err != nil {
		httpx.Internal(c, "인박스 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"scpTarget": h.svc.InboxTarget(), "files": files})
}

// RegisterNFS는 인박스 파일을 데이터셋으로 등록한다(등록 후 인박스에서 제거·자동 해제). {name, filename, scope}.
func (h *Handler) RegisterNFS(c *gin.Context) {
	var req struct{ Name, Filename, Scope, SizeClass string }
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" || req.Filename == "" {
		httpx.BadRequest(c, "name·filename 필요")
		return
	}
	u := auth.CurrentUser(c)
	if err := h.svc.RegisterNFS(c.Request.Context(), u.ID, req.Name, req.Scope, u.Username, req.Filename, req.SizeClass); err != nil {
		h.registerErr(c, err)
		return
	}
	httpx.Created(c, gin.H{"ok": true})
}

// RegisterURL은 URL(wget)로 데이터셋을 등록한다. {name, url, scope}.
func (h *Handler) RegisterURL(c *gin.Context) {
	var req struct{ Name, Url, Scope, SizeClass string }
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" || req.Url == "" {
		httpx.BadRequest(c, "name·url 필요")
		return
	}
	u := auth.CurrentUser(c)
	if err := h.svc.RegisterURL(c.Request.Context(), u.ID, req.Name, req.Scope, u.Username, req.Url, req.SizeClass); err != nil {
		h.registerErr(c, err)
		return
	}
	httpx.Created(c, gin.H{"ok": true})
}

func (h *Handler) registerErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNameTaken):
		httpx.Err(c, 409, "name_taken", "이미 같은 이름의 데이터셋이 있습니다")
	case errors.Is(err, ErrUploadDisabled):
		httpx.Err(c, 503, "upload_disabled", "데이터셋 NFS/저장소가 설정되지 않았습니다")
	default:
		httpx.Internal(c, "등록 실패: "+err.Error())
	}
}

func (h *Handler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), idParam(c)); err != nil {
		httpx.Internal(c, "삭제 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// ── 관리자 ──────────────────────────────

// UpdateDescription은 데이터셋 설명을 저장한다(관리자 편집).
func (h *Handler) UpdateDescription(c *gin.Context) {
	var req struct {
		Description string `json:"description"`
		SizeClass   string `json:"sizeClass"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "본문 오류")
		return
	}
	if err := h.svc.UpdateMeta(idParam(c), req.Description, req.SizeClass); err != nil {
		httpx.Internal(c, "저장 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) PendingRequests(c *gin.Context) {
	items, err := h.svc.PendingRequests()
	if err != nil {
		httpx.Internal(c, "요청 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) Approve(c *gin.Context) {
	u := auth.CurrentUser(c)
	if err := h.svc.Approve(c.Request.Context(), reqIDParam(c), u.ID, u.Username); err != nil {
		httpx.Internal(c, "승인 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) Reject(c *gin.Context) {
	if err := h.svc.Reject(reqIDParam(c), auth.CurrentUser(c).ID); err != nil {
		httpx.Internal(c, "거절 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) ToggleCache(c *gin.Context) {
	var req CacheReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "node 필요")
		return
	}
	if err := h.svc.ToggleCache(c.Request.Context(), idParam(c), req.Node); err != nil {
		httpx.Internal(c, "캐시 배치 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func idParam(c *gin.Context) int64    { id, _ := strconv.ParseInt(c.Param("id"), 10, 64); return id }
func reqIDParam(c *gin.Context) int64 { id, _ := strconv.ParseInt(c.Param("reqId"), 10, 64); return id }
