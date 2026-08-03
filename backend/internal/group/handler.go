package group

import (
	"errors"
	"strconv"

	"giosk/internal/auth"
	"giosk/internal/authz"
	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// Handler는 group HTTP 핸들러(얇게).
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// ── 관리자: 그룹 CRUD ───────────────────────────────
func (h *Handler) ListAll(c *gin.Context) {
	items, err := h.svc.ListAll()
	if err != nil {
		httpx.Internal(c, "그룹 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// ListScoped는 호출자 스코프로 그룹 목록을 좁힌다(단일 /console 트리).
// platform=전체, org=자기 조직 산하 그룹, group=자기 그룹 1개.
func (h *Handler) ListScoped(c *gin.Context) {
	items, err := h.svc.ListAll()
	if err != nil {
		httpx.Internal(c, "그룹 조회 실패")
		return
	}
	s := authz.CurrentScope(c)
	if s.Level != "platform" {
		out := make([]Summary, 0, len(items))
		for _, g := range items {
			if (s.Level == "org" && g.OrgID == s.OrgID) || (s.Level == "group" && g.ID == s.GroupID) {
				out = append(out, g)
			}
		}
		items = out
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) Create(c *gin.Context) {
	var req CreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "orgId, name 필요")
		return
	}
	// 스코프 강제(단일 /console 트리): org admin 은 자기 조직에만, group admin 은 생성 불가.
	// 플랫폼(스코프 미설정 /admin 경로 포함)은 요청 orgId 그대로.
	switch authz.CurrentScope(c).Level {
	case "org":
		req.OrgID = authz.CurrentScope(c).OrgID
	case "group":
		httpx.Forbidden(c, "group admin cannot create groups")
		return
	}
	g, err := h.svc.Create(req)
	if errors.Is(err, ErrUserUnknown) {
		httpx.Err(c, 400, "admin_unknown", "지정한 팀 관리자 계정을 찾을 수 없습니다")
		return
	}
	if err != nil {
		httpx.Internal(c, "그룹 생성 실패")
		return
	}
	httpx.Created(c, g)
}

// Update는 그룹 표시명·가입수락 여부를 수정한다(플랫폼 관리자).
func (h *Handler) Update(c *gin.Context) {
	var body struct {
		DisplayName *string `json:"displayName"`
		AcceptsJoin *bool   `json:"acceptsJoin"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BadRequest(c, "본문 오류")
		return
	}
	if err := h.svc.UpdateGroup(gid(c), body.DisplayName, body.AcceptsJoin); err != nil {
		httpx.Internal(c, "그룹 수정 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// Delete는 그룹을 소프트 삭제(archived)한다(플랫폼 관리자).
func (h *Handler) Delete(c *gin.Context) {
	if err := h.svc.ArchiveGroup(gid(c)); err != nil {
		switch {
		case errors.Is(err, ErrDefaultGroup):
			httpx.Err(c, 409, "default_group", "'일반' 팀은 삭제할 수 없습니다(다른 팀 삭제 시 멤버 이전 대상).")
		case errors.Is(err, ErrGroupHasMembers):
			httpx.Err(c, 409, "group_has_members", "팀에 멤버가 있어 삭제할 수 없습니다. 멤버를 다른 팀으로 옮기거나 제거한 뒤 삭제하세요.")
		default:
			httpx.Internal(c, "그룹 삭제 실패")
		}
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// ── 그룹 범위: 멤버/정책/가입승인 ──────────────────────
func (h *Handler) Members(c *gin.Context) {
	items, err := h.svc.Members(gid(c))
	if err != nil {
		httpx.Internal(c, "멤버 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) AddMember(c *gin.Context) {
	var req AddMemberReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "account 필요")
		return
	}
	if err := h.svc.AddMember(gid(c), req); err != nil {
		if errors.Is(err, ErrUserUnknown) {
			httpx.NotFound(c, "사용자를 찾을 수 없습니다")
			return
		}
		httpx.Internal(c, "멤버 추가 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// MoveMember — 사용자를 이 그룹에서 다른 그룹으로 옮긴다(관리자).
func (h *Handler) MoveMember(c *gin.Context) {
	var req MoveMemberReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "toGroupId 필요")
		return
	}
	err := h.svc.MoveMember(gid(c), req.ToGroupID, uidParam(c), req.Role)
	switch {
	case errors.Is(err, ErrSameGroup):
		httpx.BadRequest(c, "같은 그룹으로는 옮길 수 없습니다")
	case errors.Is(err, ErrGroupUnknown):
		httpx.NotFound(c, "대상 그룹을 찾을 수 없습니다")
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(c, "이 그룹의 멤버가 아닙니다")
	case err != nil:
		httpx.Internal(c, "그룹 이동 실패")
	default:
		httpx.OK(c, gin.H{"ok": true})
	}
}

func (h *Handler) RemoveMember(c *gin.Context) {
	if err := h.svc.RemoveMember(gid(c), uidParam(c)); err != nil {
		httpx.Internal(c, "멤버 제거 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) SetMemberBudget(c *gin.Context) {
	var req struct {
		Budget int `json:"budget"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "budget 필요")
		return
	}
	if err := h.svc.SetMemberBudget(gid(c), uidParam(c), req.Budget); err != nil {
		httpx.Internal(c, "예산 변경 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) Usage(c *gin.Context) {
	rows, err := h.svc.Usage(gid(c))
	if err != nil {
		httpx.Internal(c, "사용량 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"byUser": rows, "bySession": h.svc.UsageBySession(gid(c)), "trend": h.svc.UsageTrend(gid(c))})
}

func (h *Handler) SetMemberRole(c *gin.Context) {
	var req RoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "role 필요")
		return
	}
	if err := h.svc.SetMemberRole(gid(c), uidParam(c), req.Role); err != nil {
		httpx.Internal(c, "역할 변경 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) GetJoinPolicy(c *gin.Context) {
	v, err := h.svc.JoinPolicy(gid(c))
	if err != nil {
		httpx.Internal(c, "정책 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"acceptsJoin": v})
}

func (h *Handler) SetJoinPolicy(c *gin.Context) {
	var req JoinPolicyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "acceptsJoin 필요")
		return
	}
	if err := h.svc.SetJoinPolicy(gid(c), req.AcceptsJoin); err != nil {
		httpx.Internal(c, "정책 변경 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) JoinRequests(c *gin.Context) {
	items, err := h.svc.PendingJoinRequests(gid(c))
	if err != nil {
		httpx.Internal(c, "가입 신청 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) ApproveJoin(c *gin.Context) {
	if err := h.svc.ApproveJoin(reqIDParam(c)); err != nil {
		httpx.Internal(c, "가입 승인 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) RejectJoin(c *gin.Context) {
	if err := h.svc.RejectJoin(reqIDParam(c)); err != nil {
		httpx.Internal(c, "가입 거절 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// ── 사용자: 내 소속/디렉터리/가입신청 ────────────────────
func (h *Handler) MyGroups(c *gin.Context) {
	items, err := h.svc.MyGroups(auth.CurrentUser(c).ID)
	if err != nil {
		httpx.Internal(c, "내 그룹 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) Directory(c *gin.Context) {
	items, err := h.svc.Directory(auth.CurrentUser(c).ID)
	if err != nil {
		httpx.Internal(c, "디렉터리 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// ShareTargets는 볼륨 공유 대상(내 그룹 + 동료 사용자).
func (h *Handler) ShareTargets(c *gin.Context) {
	res, err := h.svc.ShareTargets(auth.CurrentUser(c).ID)
	if err != nil {
		httpx.Internal(c, "공유 대상 조회 실패")
		return
	}
	httpx.OK(c, res)
}

func (h *Handler) MembershipContext(c *gin.Context) {
	uid := auth.CurrentUser(c).ID
	myGroups, err := h.svc.MyGroups(uid)
	if err != nil {
		httpx.Internal(c, "컨텍스트 조회 실패")
		return
	}
	dir, _ := h.svc.Directory(uid)
	reqs, _ := h.svc.MyJoinRequests(uid)
	httpx.OK(c, gin.H{"org": homeOrg(myGroups), "myGroups": myGroups, "directory": dir, "joinRequests": reqs})
}

func (h *Handler) CreateJoinRequest(c *gin.Context) {
	var body struct {
		GroupID int64 `json:"groupId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BadRequest(c, "groupId 필요")
		return
	}
	err := h.svc.RequestJoin(auth.CurrentUser(c).ID, body.GroupID)
	if errors.Is(err, ErrJoinClosed) {
		httpx.Err(c, 409, "join_closed", "이 그룹은 가입 신청을 받지 않습니다")
		return
	}
	if err != nil {
		httpx.Internal(c, "가입 신청 실패")
		return
	}
	httpx.Created(c, gin.H{"ok": true})
}

func (h *Handler) MyJoinRequests(c *gin.Context) {
	items, err := h.svc.MyJoinRequests(auth.CurrentUser(c).ID)
	if err != nil {
		httpx.Internal(c, "신청 내역 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// CancelJoinRequest는 본인의 대기중 가입신청을 취소한다.
func (h *Handler) CancelJoinRequest(c *gin.Context) {
	if err := h.svc.CancelJoinRequest(auth.CurrentUser(c).ID, reqIDParam(c)); err != nil {
		httpx.Internal(c, "가입 신청 취소 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// homeOrg는 첫 소속 그룹의 조직을 홈 조직으로 본다.
func homeOrg(groups []GroupRef) gin.H {
	if len(groups) == 0 {
		return nil
	}
	return gin.H{"id": groups[0].OrgID, "name": groups[0].OrgName}
}

func gid(c *gin.Context) int64      { v, _ := strconv.ParseInt(c.Param("id"), 10, 64); return v }
func uidParam(c *gin.Context) int64 { v, _ := strconv.ParseInt(c.Param("userId"), 10, 64); return v }
func reqIDParam(c *gin.Context) int64 {
	v, _ := strconv.ParseInt(c.Param("reqId"), 10, 64)
	return v
}
