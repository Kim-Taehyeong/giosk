package users

import (
	"strconv"
	"strings"

	"giosk/internal/authz"
	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// Handler는 users 관리자 HTTP 핸들러(얇게).
type Handler struct{ repo Repository }

func NewHandler(repo Repository) *Handler { return &Handler{repo: repo} }

// List는 서버측 검색/필터/페이징으로 사용자 목록을 반환한다.
// 예전엔 전체를 내려주고 브라우저가 걸렀다 — 300명에 3.3초로, 실사용 규모(수천 명)에선 못 쓴다.
//
//	?q=&status=&group=&org=&page=1&size=50  → { items, total, page, size }
func (h *Handler) List(c *gin.Context) {
	page := atoiDefault(c.Query("page"), 1)
	if page < 1 {
		page = 1
	}
	size := atoiDefault(c.Query("size"), defaultPageSize)
	if size < 1 || size > maxPageSize {
		size = defaultPageSize // 과도한 size 로 전체를 긁어가지 못하게 한다.
	}
	items, total, err := h.repo.List(ListFilter{
		Q:      c.Query("q"),
		Status: c.Query("status"),
		Group:  c.Query("group"),
		Org:    c.Query("org"),
		Limit:  size,
		Offset: (page - 1) * size,
	})
	if err != nil {
		httpx.Internal(c, "사용자 조회 실패")
		return
	}
	if items == nil {
		items = []Summary{}
	}
	httpx.OK(c, gin.H{"items": items, "total": total, "page": page, "size": size})
}

// ListScoped는 호출자 스코프로 사용자 목록을 강제 필터한다(단일 /console 트리).
// org=자기 조직 소속만, group=자기 그룹 소속만. platform 은 기존 List 와 동일.
func (h *Handler) ListScoped(c *gin.Context) {
	page := atoiDefault(c.Query("page"), 1)
	if page < 1 {
		page = 1
	}
	size := atoiDefault(c.Query("size"), defaultPageSize)
	if size < 1 || size > maxPageSize {
		size = defaultPageSize
	}
	f := ListFilter{
		Q:      c.Query("q"),
		Status: c.Query("status"),
		Group:  c.Query("group"),
		Org:    c.Query("org"),
		Limit:  size,
		Offset: (page - 1) * size,
	}
	// 스코프로 강제 — 클라이언트가 보낸 org/group 표시명 필터보다 우선(교차 조직 조회 차단).
	s := authz.CurrentScope(c)
	switch s.Level {
	case "org":
		f.OrgID = s.OrgID
	case "group":
		f.GroupID = s.GroupID
	}
	items, total, err := h.repo.List(f)
	if err != nil {
		httpx.Internal(c, "사용자 조회 실패")
		return
	}
	if items == nil {
		items = []Summary{}
	}
	httpx.OK(c, gin.H{"items": items, "total": total, "page": page, "size": size})
}

const (
	defaultPageSize = 50
	maxPageSize     = 200
)

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

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

func (h *Handler) Create(c *gin.Context) {
	var req CreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "username 필요")
		return
	}
	id, err := h.repo.Create(req)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			httpx.Err(c, 409, "duplicate_user", "이미 존재하는 아이디 또는 이메일입니다")
			return
		}
		httpx.Internal(c, "사용자 생성 실패: "+err.Error())
		return
	}
	httpx.Created(c, gin.H{"id": id})
}

func (h *Handler) BulkCreate(c *gin.Context) {
	var req BulkReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "users 배열 필요")
		return
	}
	var created, failed int
	for _, u := range req.Users {
		if _, err := h.repo.Create(u); err != nil {
			failed++
		} else {
			created++
		}
	}
	httpx.OK(c, gin.H{"created": created, "failed": failed})
}

// RegisterAdmin은 플랫폼 관리자 사용자 관리 라우트.
func RegisterAdmin(admin gin.IRouter, h *Handler) {
	admin.GET("/users", h.List)
	admin.POST("/users", h.Create)
	admin.POST("/users/bulk", h.BulkCreate)
	admin.PUT("/users/:id/status", h.SetStatus)
}

func idParam(c *gin.Context) int64 { id, _ := strconv.ParseInt(c.Param("id"), 10, 64); return id }
