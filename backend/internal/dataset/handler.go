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

// ── 청크(이어올리기) 업로드 — Cloudflare 100MB 리밋 우회 + 새로고침 재개 ──

// UploadInit: {name, filename} → {offset} (재개 지점).
func (h *Handler) UploadInit(c *gin.Context) {
	var req struct{ Name, Filename string }
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		httpx.BadRequest(c, "name·filename 필요")
		return
	}
	off, err := h.svc.UploadInit(strings.TrimSpace(req.Name), req.Filename)
	if err != nil {
		h.uploadErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"offset": off})
}

// UploadChunk: 본문=청크 바이트, 쿼리 name·filename·offset → {offset} (새 크기). offset 불일치면 409+서버 offset.
func (h *Handler) UploadChunk(c *gin.Context) {
	name := strings.TrimSpace(c.Query("name"))
	filename := c.Query("filename")
	offset, _ := strconv.ParseInt(c.Query("offset"), 10, 64)
	newOff, err := h.svc.UploadChunk(name, filename, offset, c.Request.Body)
	if errors.Is(err, ErrChunkOffset) {
		httpx.Err(c, 409, "offset_mismatch", strconv.FormatInt(newOff, 10)) // 클라이언트가 이 offset 부터 재전송
		return
	}
	if err != nil {
		h.uploadErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"offset": newOff})
}

// UploadStatus: ?name=&filename= → {offset} (재개용, 새로고침 후 조회).
func (h *Handler) UploadStatus(c *gin.Context) {
	httpx.OK(c, gin.H{"offset": h.svc.UploadStatus(strings.TrimSpace(c.Query("name")), c.Query("filename"))})
}

// UploadFinish: {name, scope, filename, size} → 데이터셋 확정(해제 시작).
func (h *Handler) UploadFinish(c *gin.Context) {
	var req struct {
		Name, Scope, Filename string
		Size                  int64
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		httpx.BadRequest(c, "name 필요")
		return
	}
	u := auth.CurrentUser(c)
	if err := h.svc.UploadFinish(c.Request.Context(), u.ID, strings.TrimSpace(req.Name), req.Scope, u.Username, req.Filename, req.Size); err != nil {
		h.uploadErr(c, err)
		return
	}
	httpx.Created(c, gin.H{"ok": true})
}

func (h *Handler) uploadErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNameTaken):
		httpx.Err(c, 409, "name_taken", "이미 같은 이름의 데이터셋이 있습니다")
	case errors.Is(err, ErrUploadDisabled):
		httpx.Err(c, 503, "upload_disabled", "파일 업로드가 비활성화되어 있습니다(데이터셋 NFS 마운트 필요)")
	default:
		httpx.Internal(c, "업로드 실패: "+err.Error())
	}
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
