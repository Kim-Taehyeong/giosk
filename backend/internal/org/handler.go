package org

import (
	"errors"
	"strconv"

	"giosk/internal/authz"
	"giosk/internal/group"
	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// Handler는 org HTTP 핸들러(얇게).
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) List(c *gin.Context) {
	items, err := h.svc.List()
	if err != nil {
		httpx.Internal(c, "조직 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// ListScoped는 호출자 스코프로 조직 목록을 좁힌다(단일 /console 트리).
// platform=전체, org=자기 조직 1개, group=빈 목록(조직 리스트 탭은 프론트에서도 숨김).
func (h *Handler) ListScoped(c *gin.Context) {
	items, err := h.svc.List()
	if err != nil {
		httpx.Internal(c, "조직 조회 실패")
		return
	}
	s := authz.CurrentScope(c)
	switch s.Level {
	case "platform":
		// 전체
	case "org", "group":
		// org·group 모두 자기 소속 조직 1개만(group 은 부모 조직 읽기용).
		out := make([]Summary, 0, 1)
		for _, o := range items {
			if o.ID == s.OrgID {
				out = append(out, o)
			}
		}
		items = out
	default:
		items = []Summary{}
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) Create(c *gin.Context) {
	var req CreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "name 필요")
		return
	}
	o, err := h.svc.Create(req)
	if errors.Is(err, group.ErrUserUnknown) {
		httpx.Err(c, 400, "admin_unknown", "지정한 조직 관리자 계정을 찾을 수 없습니다")
		return
	}
	if errors.Is(err, ErrDuplicateName) {
		httpx.Err(c, 409, "duplicate_name", "이미 존재하는 조직 이름입니다")
		return
	}
	if err != nil {
		httpx.Internal(c, "조직 생성 실패")
		return
	}
	httpx.Created(c, o)
}

func (h *Handler) Update(c *gin.Context) {
	var req UpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "본문 오류")
		return
	}
	if err := h.svc.Update(idParam(c), req); err != nil {
		httpx.Internal(c, "조직 수정 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) Archive(c *gin.Context) {
	if err := h.svc.Archive(idParam(c)); err != nil {
		if errors.Is(err, ErrOrgHasUsers) {
			httpx.Err(c, 409, "org_has_users", "조직에 아직 사용자가 있어 삭제할 수 없습니다. 모든 사용자를 다른 조직으로 옮기거나 삭제한 뒤 삭제하세요.")
			return
		}
		httpx.Internal(c, "조직 보관 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) Grant(c *gin.Context) {
	var req GrantReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "amount 필요")
		return
	}
	if err := h.svc.Grant(idParam(c), req); err != nil {
		httpx.Internal(c, "크레딧 부여 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) PublicCatalog(c *gin.Context) {
	items, err := h.svc.PublicCatalog()
	if err != nil {
		httpx.Internal(c, "공개 조직 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func idParam(c *gin.Context) int64 {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	return id
}
