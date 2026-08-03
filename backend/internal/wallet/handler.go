package wallet

import (
	"errors"
	"strconv"
	"strings"

	"giosk/internal/auth"
	"giosk/internal/authz"
	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// Handler는 wallet HTTP 핸들러(얇게).
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// ── 사용자 ──────────────────────────────
func (h *Handler) MyWallet(c *gin.Context) {
	// 활성 스코프(팀) 기준 지갑 — 프론트가 X-Console-Scope: group:N 을 보낸다. 팀 아니면 0(대표 팀).
	w, err := h.svc.MyWallet(auth.CurrentUser(c).ID, scopeGroup(c))
	if err != nil {
		httpx.Internal(c, "지갑 조회 실패")
		return
	}
	httpx.OK(c, w)
}

// ── 그룹 범위(project_admin) ──────────────
func (h *Handler) GroupWallet(c *gin.Context) {
	w, err := h.svc.GroupWallet(gid(c))
	if err != nil {
		httpx.Internal(c, "그룹 지갑 조회 실패")
		return
	}
	httpx.OK(c, w)
}

func (h *Handler) SetBudget(c *gin.Context) {
	var req BudgetReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "본문 오류")
		return
	}
	if err := h.svc.SetGroupBudget(gid(c), req); err != nil {
		httpx.Internal(c, "예산 설정 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) SetDefaultMemberBudget(c *gin.Context) {
	var req DefaultBudgetReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "본문 오류")
		return
	}
	if err := h.svc.SetDefaultMemberBudget(gid(c), req.DefaultBudget); err != nil {
		httpx.Internal(c, "기본 예산 설정 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// ── 최고관리자(super) grant — 예외 부여 ───────
// 그룹/개인에 부여하면 무결성을 위해 상위 풀(조직·그룹)에도 자동 가산된다.
func (h *Handler) GrantUser(c *gin.Context) {
	var req GrantReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "amount 필요")
		return
	}
	actor := auth.CurrentUser(c).ID
	// super grant 대상 팀: 요청 스코프(group:N)면 그 팀, 아니면 대표 팀(0→resolve). gid(c)=대상 사용자.
	if err := h.svc.AllocateUser(gid(c), scopeGroup(c), req.Amount, true, req.Reason, &actor); err != nil {
		httpx.Internal(c, "크레딧 부여 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// GrantGroup은 그룹 풀에 크레딧을 부여한다. 레벨에 따라 회계가 다르다:
//   - 최고관리자(platform): 예외 부여 — 상위 조직 풀에도 동일 금액 자동 가산(무결성).
//   - 조직관리자(org): 자기 조직 풀에서 차감해 그룹 풀에 가산(실 이체; 부족 시 400).
func (h *Handler) GrantGroup(c *gin.Context) {
	var req GrantReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "amount 필요")
		return
	}
	actor := auth.CurrentUser(c).ID
	super := authz.CurrentScope(c).Level == "platform"
	if err := h.svc.AllocateGroup(gid(c), req.Amount, super, &actor); err != nil {
		if errors.Is(err, ErrInsufficientPool) {
			httpx.Err(c, 400, "insufficient_org_pool", "조직 크레딧 풀 잔여가 부족합니다")
			return
		}
		httpx.Internal(c, "그룹 크레딧 부여 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// ── 그룹 담당자(project_admin) 멤버 배분 — 그룹 풀에서 차감 ──
// 그룹 :id 풀에서 amount 만큼 차감하여 멤버(userId) 개인 지갑에 배분한다.
func (h *Handler) AllocateMember(c *gin.Context) {
	var req MemberAllocReq
	if err := c.ShouldBindJSON(&req); err != nil || req.UserID <= 0 {
		httpx.BadRequest(c, "userId·amount 필요")
		return
	}
	actor := auth.CurrentUser(c).ID
	// 대상 팀 = 경로의 그룹(:id). 그 팀 풀에서 차감해 (member, 그 팀) 지갑에 배분.
	if err := h.svc.AllocateUser(req.UserID, gid(c), req.Amount, false, req.Reason, &actor); err != nil {
		switch {
		case errors.Is(err, ErrInsufficientGroupPool):
			httpx.Err(c, 400, "insufficient_group_pool", "그룹 크레딧 풀 잔여가 부족합니다")
		case errors.Is(err, ErrNoGroup):
			httpx.Err(c, 400, "no_group", "대상 사용자의 그룹을 찾을 수 없습니다")
		default:
			httpx.Internal(c, "멤버 배분 실패")
		}
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// SetGroupRefill은 팀 정기 리필(양·주기·이월)을 설정한다(주기는 조직 상한 클램프).
func (h *Handler) SetGroupRefill(c *gin.Context) {
	var req RefillSpec
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "recurring/interval/carryover 필요")
		return
	}
	if err := h.svc.SetGroupRefill(gid(c), req); err != nil {
		httpx.Internal(c, "팀 리필 설정 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// SetUserRefill은 개인 정기 리필을 설정한다(주기는 팀 상한 클램프).
func (h *Handler) SetUserRefill(c *gin.Context) {
	var req RefillSpec
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "recurring/interval/carryover 필요")
		return
	}
	uid, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.SetUserRefill(uid, scopeGroup(c), req); err != nil {
		httpx.Internal(c, "개인 리필 설정 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func gid(c *gin.Context) int64 { id, _ := strconv.ParseInt(c.Param("id"), 10, 64); return id }

// scopeGroup은 X-Console-Scope 헤더가 group:N 이면 N 을, 아니면 0(대표 팀 폴백)을 반환한다.
func scopeGroup(c *gin.Context) int64 {
	if sel := c.GetHeader("X-Console-Scope"); strings.HasPrefix(sel, "group:") {
		id, _ := strconv.ParseInt(sel[len("group:"):], 10, 64)
		return id
	}
	return 0
}
