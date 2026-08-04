package image

import (
	"errors"
	"strconv"

	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// Handler는 image HTTP 핸들러(얇게). CRUD 는 repo, 빌드 파이프라인은 svc.
type Handler struct {
	repo Repository
	svc  *Service
}

func NewHandler(repo Repository, svc *Service) *Handler { return &Handler{repo: repo, svc: svc} }

// Build는 가이드형 입력(베이스+패키지)으로 Kaniko 빌드를 시작한다(POST /admin/images).
func (h *Handler) Build(c *gin.Context) {
	var req BuildReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "name, base 필요")
		return
	}
	if err := h.svc.Build(c.Request.Context(), req); err != nil {
		httpx.Internal(c, "이미지 빌드 시작 실패: "+err.Error())
		return
	}
	httpx.Created(c, gin.H{"ok": true})
}

// RegisterExternal은 외부 이미지를 빌드 없이 등록한다(기본 경로, 항상 가용).
func (h *Handler) RegisterExternal(c *gin.Context) {
	var req ExternalReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "ref 필요")
		return
	}
	if err := h.svc.RegisterExternal(req); err != nil {
		httpx.Internal(c, "외부 이미지 등록 실패: "+err.Error())
		return
	}
	httpx.Created(c, gin.H{"ok": true})
}

// Logs는 빌드/스캔 Job 로그를 반환한다(실패 진단).
func (h *Handler) Logs(c *gin.Context) {
	httpx.OK(c, gin.H{"logs": h.svc.ImageLogs(c.Request.Context(), idParam(c))})
}

// CacheList는 이미지의 노드별 캐시 상태를 반환한다.
func (h *Handler) CacheList(c *gin.Context) {
	httpx.OK(c, gin.H{"items": h.svc.CacheList(idParam(c))})
}

// CacheOn은 이미지를 특정 노드에 프리페치한다(body: {node}).
func (h *Handler) CacheOn(c *gin.Context) {
	var body struct {
		Node string `json:"node"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Node == "" {
		httpx.BadRequest(c, "node 필요")
		return
	}
	if err := h.svc.CacheOnNode(c.Request.Context(), idParam(c), body.Node); err != nil {
		httpx.Internal(c, "캐시 시작 실패: "+err.Error())
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// CacheOff는 노드 캐시를 해제한다(?node=).
func (h *Handler) CacheOff(c *gin.Context) {
	node := c.Query("node")
	if node == "" {
		httpx.BadRequest(c, "node 필요")
		return
	}
	if err := h.svc.UncacheNode(c.Request.Context(), idParam(c), node); err != nil {
		httpx.Internal(c, "캐시 해제 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// Rebuild는 기존 이미지를 태그를 올려 다시 빌드한다.
func (h *Handler) Rebuild(c *gin.Context) {
	if err := h.svc.Rebuild(c.Request.Context(), idParam(c)); err != nil {
		if errors.Is(err, ErrNotBuildable) {
			httpx.BadRequest(c, "외부/기본 이미지는 재빌드할 수 없습니다(가이드형으로 만든 이미지만 가능)")
			return
		}
		httpx.Internal(c, "재빌드 실패: "+err.Error())
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) List(c *gin.Context) {
	items, err := h.repo.List(c.Query("active") == "true")
	if err != nil {
		httpx.Internal(c, "이미지 조회 실패")
		return
	}
	cached := h.repo.CachedNodesMap() // 이미지별 캐시 완료 노드. 빠른 생성 배지에 쓴다
	for i := range items {
		items[i].CachedNodes = cached[items[i].ID]
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) Save(c *gin.Context) {
	var img Image
	if err := c.ShouldBindJSON(&img); err != nil {
		httpx.BadRequest(c, "본문 오류")
		return
	}
	img.ID = idParam(c)
	if err := h.repo.Save(&img); err != nil {
		httpx.Internal(c, "이미지 저장 실패")
		return
	}
	httpx.OK(c, img)
}

// SetStatus는 draft 에서 active 로 올리는 publish 같은 상태 전이를 처리한다.
func (h *Handler) SetStatus(c *gin.Context) {
	var req StatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "status 필요")
		return
	}
	if err := h.repo.SetStatus(idParam(c), req.Status); err != nil {
		httpx.Internal(c, "상태 변경 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) Delete(c *gin.Context) {
	if err := h.repo.Delete(idParam(c)); err != nil {
		httpx.Internal(c, "이미지 삭제 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func idParam(c *gin.Context) int64 {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	return id
}
