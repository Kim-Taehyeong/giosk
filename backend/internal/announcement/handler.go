package announcement

import (
	"strconv"

	"giosk/internal/auth"
	"giosk/internal/authz"
	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// MemberScope는 사용자 대표 멤버십(조직/그룹)을 해석한다(group.Service 구현).
type MemberScope interface {
	PrimaryScope(userID int64) (level string, orgID, groupID int64, ok bool)
}

// Handler는 announcement HTTP 핸들러(얇게).
type Handler struct {
	repo  Repository
	scope MemberScope            // 사용자 노출 필터(전역 + 내 조직/그룹 타겟)
	oog   authz.OrgOfGroupReader // org admin 의 그룹 타겟 검증(그 그룹이 내 조직 소속인지)
}

func NewHandler(repo Repository, scope MemberScope, oog authz.OrgOfGroupReader) *Handler {
	return &Handler{repo: repo, scope: scope, oog: oog}
}

// List는 활성 공지 — 전역 + 로그인 사용자의 조직/그룹 타겟만.
func (h *Handler) List(c *gin.Context) {
	var orgID, groupID int64
	if u := auth.CurrentUser(c); u != nil && h.scope != nil {
		_, orgID, groupID, _ = h.scope.PrimaryScope(u.ID)
	}
	items, err := h.repo.ListActiveFor(orgID, groupID)
	if err != nil {
		httpx.Internal(c, "공지 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// AdminList는 전체 공지(플랫폼 /admin).
func (h *Handler) AdminList(c *gin.Context) {
	items, err := h.repo.ListAll()
	if err != nil {
		httpx.Internal(c, "공지 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// AdminListScoped는 관리자 공지 목록을 호출자 스코프로 좁힌다(/console).
func (h *Handler) AdminListScoped(c *gin.Context) {
	orgID, groupID := scopeIDs(c)
	items, err := h.repo.ListAllScoped(orgID, groupID)
	if err != nil {
		httpx.Internal(c, "공지 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) Create(c *gin.Context) {
	var req Req
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "title, body 필요")
		return
	}
	orgT, grpT, ok := h.resolveTarget(c, req)
	if !ok {
		httpx.Forbidden(c, "허용되지 않은 공지 대상")
		return
	}
	actor := auth.CurrentUser(c).ID
	a := &Announcement{Level: orStr(req.Level, "info"), Title: req.Title, Body: req.Body, Active: true, Pinned: req.Pinned, TargetOrgID: orgT, TargetGroupID: grpT, CreatedBy: &actor}
	if err := h.repo.Create(a); err != nil {
		httpx.Internal(c, "공지 생성 실패")
		return
	}
	httpx.Created(c, a)
}

func (h *Handler) Update(c *gin.Context) {
	var req Req
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "본문 오류")
		return
	}
	if !h.canManageID(c, idParam(c)) {
		httpx.Forbidden(c, "이 공지를 수정할 권한이 없습니다")
		return
	}
	orgT, grpT, ok := h.resolveTarget(c, req)
	if !ok {
		httpx.Forbidden(c, "허용되지 않은 공지 대상")
		return
	}
	fields := map[string]any{"level": orStr(req.Level, "info"), "title": req.Title, "body": req.Body, "pinned": req.Pinned, "target_org_id": orgT, "target_group_id": grpT}
	if err := h.repo.Update(idParam(c), fields); err != nil {
		httpx.Internal(c, "공지 수정 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) Toggle(c *gin.Context) {
	if !h.canManageID(c, idParam(c)) {
		httpx.Forbidden(c, "권한이 없습니다")
		return
	}
	if err := h.repo.Toggle(idParam(c)); err != nil {
		httpx.Internal(c, "공지 토글 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) Delete(c *gin.Context) {
	if !h.canManageID(c, idParam(c)) {
		httpx.Forbidden(c, "권한이 없습니다")
		return
	}
	if err := h.repo.Delete(idParam(c)); err != nil {
		httpx.Internal(c, "공지 삭제 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// resolveTarget은 호출자 스코프에 맞춰 공지 타겟을 강제/검증한다.
// platform=자유(요청 그대로), org=자기 조직 전체 또는 산하 그룹, group=자기 그룹 강제.
func (h *Handler) resolveTarget(c *gin.Context, req Req) (orgT, grpT *int64, ok bool) {
	s := authz.CurrentScope(c)
	switch s.Level {
	case "platform":
		return req.TargetOrgID, req.TargetGroupID, true
	case "org":
		if req.TargetGroupID != nil {
			if o, found := h.oog.OrgOfGroup(*req.TargetGroupID); !found || o != s.OrgID {
				return nil, nil, false
			}
			return nil, req.TargetGroupID, true
		}
		oid := s.OrgID
		return &oid, nil, true
	case "group":
		gid := s.GroupID
		return nil, &gid, true
	}
	return nil, nil, false
}

// canManageID는 대상 공지가 호출자 스코프에 속하는지 확인한다(수정/토글/삭제 가드).
func (h *Handler) canManageID(c *gin.Context, id int64) bool {
	s := authz.CurrentScope(c)
	if s.Level == "platform" {
		return true
	}
	a, err := h.repo.Get(id)
	if err != nil || a == nil {
		return false
	}
	switch s.Level {
	case "org":
		if a.TargetOrgID != nil && *a.TargetOrgID == s.OrgID {
			return true
		}
		if a.TargetGroupID != nil {
			if o, ok := h.oog.OrgOfGroup(*a.TargetGroupID); ok && o == s.OrgID {
				return true
			}
		}
		return false
	case "group":
		return a.TargetGroupID != nil && *a.TargetGroupID == s.GroupID
	}
	return false
}

// scopeIDs는 매니저 스코프의 org/group id 를 반환한다(platform=0,0; org 관리자는 홈 그룹 무시).
func scopeIDs(c *gin.Context) (orgID, groupID int64) {
	return authz.CurrentScope(c).EffectiveIDs()
}

// Register는 공지 라우트 — 사용자 목록(authed) + 플랫폼 CRUD(/admin) + 스코프 CRUD(/console).
func Register(authed gin.IRouter, admin gin.IRouter, mgmt gin.IRouter, h *Handler) {
	authed.GET("/announcements", h.List)

	admin.GET("/announcements", h.AdminList)
	admin.POST("/announcements", h.Create)
	admin.PUT("/announcements/:id", h.Update)
	admin.POST("/announcements/:id/toggle", h.Toggle)
	admin.DELETE("/announcements/:id", h.Delete)

	// 매니저 트리 — 스코프 강제(org/group). platform 도 RequireManager 통과라 여기로 통일 가능.
	mgmt.GET("/announcements", h.AdminListScoped)
	mgmt.POST("/announcements", h.Create)
	mgmt.PUT("/announcements/:id", h.Update)
	mgmt.POST("/announcements/:id/toggle", h.Toggle)
	mgmt.DELETE("/announcements/:id", h.Delete)
}

func idParam(c *gin.Context) int64 { id, _ := strconv.ParseInt(c.Param("id"), 10, 64); return id }
func orStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
