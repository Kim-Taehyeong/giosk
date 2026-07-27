package dataset

import (
	"errors"
	"strconv"
	"strings"

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

// Upload는 최고관리자가 zip/tar 아카이브(또는 단일 파일)를 직접 업로드해 데이터셋으로 등록한다(multipart).
// form: file(파일), name(데이터셋 이름), scope(global|personal, 기본 global).
func (h *Handler) Upload(c *gin.Context) {
	if !h.svc.UploadEnabled() {
		httpx.Err(c, 503, "upload_disabled", "파일 업로드가 비활성화되어 있습니다(데이터셋 NFS 마운트 필요)")
		return
	}
	name := strings.TrimSpace(c.PostForm("name"))
	if name == "" {
		httpx.BadRequest(c, "데이터셋 이름(name)이 필요합니다")
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		httpx.BadRequest(c, "업로드 파일(file)이 필요합니다")
		return
	}
	f, err := fh.Open()
	if err != nil {
		httpx.Internal(c, "업로드 파일 열기 실패")
		return
	}
	defer f.Close()
	u := auth.CurrentUser(c)
	if err := h.svc.Upload(c.Request.Context(), u.ID, name, c.PostForm("scope"), u.Username, fh.Filename, fh.Size, f); err != nil {
		if errors.Is(err, ErrNameTaken) {
			httpx.Err(c, 409, "name_taken", "이미 같은 이름의 데이터셋이 있습니다")
			return
		}
		httpx.Internal(c, "업로드 실패: "+err.Error())
		return
	}
	httpx.Created(c, gin.H{"ok": true})
}

func (h *Handler) Delete(c *gin.Context) {
	if err := h.svc.Delete(idParam(c)); err != nil {
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
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "description 필요")
		return
	}
	if err := h.svc.UpdateDescription(idParam(c), req.Description); err != nil {
		httpx.Internal(c, "설명 저장 실패")
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
