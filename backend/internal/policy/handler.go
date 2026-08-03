package policy

import (
	"strconv"

	"giosk/internal/authz"
	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// Handler는 하드 제한 관리자 API(얇게).
type Handler struct {
	repo     Repository
	resolver *Resolver
	// setGlobal 은 전역 상한을 런타임 저장소에 영속한다(nil = 전역 편집 불가).
	setGlobal  func(Resolved) error
	orgOfGroup authz.OrgOfGroupReader // 그룹→부모조직(스코프 검증용)
}

func NewHandler(repo Repository, resolver *Resolver) *Handler {
	return &Handler{repo: repo, resolver: resolver}
}

// WithGlobalSetter는 전역 상한 런타임 편집을 활성화한다.
func (h *Handler) WithGlobalSetter(fn func(Resolved) error) *Handler { h.setGlobal = fn; return h }

// WithOrgOfGroup은 그룹→조직 해석기를 주입한다(스코프 정책 편집 인가에 필요).
func (h *Handler) WithOrgOfGroup(r authz.OrgOfGroupReader) *Handler { h.orgOfGroup = r; return h }

// limitsBody는 4개 하드 제한을 통째로 받는다. 각 필드 nil(JSON null/생략) = 이 레벨 미지정(상위 폴백).
type limitsBody struct {
	MaxGpu                *int `json:"maxGpu"`
	MaxVramGB             *int `json:"maxVramGb"`
	MaxVolumeGiB          *int `json:"maxVolumeGib"`
	MaxConcurrentSessions *int `json:"maxConcurrentSessions"`
	MaxStoppedSessions    *int `json:"maxStoppedSessions"`
	MaxEphemeralGiB       *int `json:"maxEphemeralGib"`
}

func (b limitsBody) toLimits() Limits {
	return Limits{MaxGpu: b.MaxGpu, MaxVramGB: b.MaxVramGB, MaxVolumeGiB: b.MaxVolumeGiB, MaxConcurrentSessions: b.MaxConcurrentSessions, MaxStoppedSessions: b.MaxStoppedSessions, MaxEphemeralGiB: b.MaxEphemeralGiB}
}

// Set은 지정 계층(user/group/org)의 하드 제한을 설정한다.
func (h *Handler) Set(level Level) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			httpx.Err(c, 400, "bad_id", "잘못된 id")
			return
		}
		var body limitsBody
		if err := c.ShouldBindJSON(&body); err != nil {
			httpx.Err(c, 400, "bad_body", "잘못된 요청")
			return
		}
		if err := h.repo.SetLimits(level, id, body.toLimits()); err != nil {
			httpx.Internal(c, "제한 설정 실패")
			return
		}
		httpx.OK(c, gin.H{"ok": true})
	}
}

// SetScoped은 매니저(조직/그룹 관리자)가 자기 스코프 안의 하드 제한을 설정한다(단일 /console 트리).
// 두 가지를 강제한다: (1) 대상이 내 스코프에 속할 것, (2) 어떤 값도 상위 유효 상한을 초과하지 못할 것.
func (h *Handler) SetScoped(level Level) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			httpx.Err(c, 400, "bad_id", "잘못된 id")
			return
		}
		var body limitsBody
		if err := c.ShouldBindJSON(&body); err != nil {
			httpx.Err(c, 400, "bad_body", "잘못된 요청")
			return
		}
		s := authz.CurrentScope(c)
		parent, ok := h.scopeParent(s, level, id)
		if !ok {
			httpx.Err(c, 403, "out_of_scope", "권한 범위를 벗어난 대상입니다")
			return
		}
		// 상위 하드캡 초과 금지 — 상속 시 부모보다 큰 값을 둘 수 없다.
		if over := exceedsParent(body, parent); over != "" {
			httpx.Err(c, 400, "exceeds_parent", "상위 제한("+over+")을 초과할 수 없습니다")
			return
		}
		if err := h.repo.SetLimits(level, id, body.toLimits()); err != nil {
			httpx.Internal(c, "제한 설정 실패")
			return
		}
		httpx.OK(c, gin.H{"ok": true})
	}
}

// ParentLimit은 대상의 상위 유효 상한(설정 가능한 최대치)을 반환한다 — 매니저 편집 UI 힌트용.
func (h *Handler) ParentLimit(level Level) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			httpx.Err(c, 400, "bad_id", "잘못된 id")
			return
		}
		parent, ok := h.scopeParent(authz.CurrentScope(c), level, id)
		if !ok {
			httpx.Err(c, 403, "out_of_scope", "권한 범위를 벗어난 대상입니다")
			return
		}
		httpx.OK(c, gin.H{
			"maxGpu": parent.MaxGpu, "maxVramGb": parent.MaxVramGB,
			"maxVolumeGib": parent.MaxVolumeGiB, "maxConcurrentSessions": parent.MaxConcurrentSessions,
			"maxStoppedSessions": parent.MaxStoppedSessions,
			"maxEphemeralGib":    parent.MaxEphemeralGiB,
		})
	}
}

// scopeParent는 (호출자 스코프, 대상 레벨/id)가 허용되는지 확인하고, 허용되면 부모 유효 상한을 돌려준다.
func (h *Handler) scopeParent(s authz.Scope, level Level, id int64) (Resolved, bool) {
	switch level {
	case LevelOrg:
		// 조직 상한: platform, 또는 자기 조직인 org_admin.
		if s.Level == "platform" || (s.Level == "org" && s.OrgID == id) {
			return h.resolver.Global(), true
		}
	case LevelGroup:
		// 그룹 상한: platform / 그 그룹이 속한 조직의 org_admin / 그 그룹의 group_admin.
		orgID := int64(0)
		if h.orgOfGroup != nil {
			orgID, _ = h.orgOfGroup.OrgOfGroup(id)
		}
		switch s.Level {
		case "platform":
			return h.resolver.ResolveOrg(orgID), true
		case "org":
			if orgID == s.OrgID {
				return h.resolver.ResolveOrg(orgID), true
			}
		case "group":
			if s.GroupID == id {
				return h.resolver.ResolveOrg(s.OrgID), true
			}
		}
	case LevelUser:
		// 사용자 상한: platform / 대상의 조직 org_admin / 대상의 그룹 group_admin.
		uOrg, uGroup := h.resolver.HierOfUser(id)
		switch s.Level {
		case "platform":
			return h.resolver.ParentOfUser(id), true
		case "org":
			if uOrg == s.OrgID {
				return h.resolver.ParentOfUser(id), true
			}
		case "group":
			if uGroup == s.GroupID {
				return h.resolver.ParentOfUser(id), true
			}
		}
	}
	return Resolved{}, false
}

// exceedsParent는 지정된 필드 중 부모 상한(>0)을 넘는 게 있으면 그 항목명을 반환한다(없으면 "").
func exceedsParent(b limitsBody, p Resolved) string {
	if b.MaxGpu != nil && p.MaxGpu > 0 && *b.MaxGpu > p.MaxGpu {
		return "GPU"
	}
	if b.MaxVramGB != nil && p.MaxVramGB > 0 && *b.MaxVramGB > p.MaxVramGB {
		return "VRAM"
	}
	if b.MaxVolumeGiB != nil && p.MaxVolumeGiB > 0 && *b.MaxVolumeGiB > p.MaxVolumeGiB {
		return "Volume"
	}
	if b.MaxConcurrentSessions != nil && p.MaxConcurrentSessions > 0 && *b.MaxConcurrentSessions > p.MaxConcurrentSessions {
		return "Sessions"
	}
	if b.MaxStoppedSessions != nil && p.MaxStoppedSessions > 0 && *b.MaxStoppedSessions > p.MaxStoppedSessions {
		return "StoppedSessions"
	}
	if b.MaxEphemeralGiB != nil && p.MaxEphemeralGiB > 0 && *b.MaxEphemeralGiB > p.MaxEphemeralGiB {
		return "EphemeralDisk"
	}
	return ""
}

// List는 "정책" 화면용 — 전역(설치 고정) + 상한이 설정된 모든 범위를 한 목록으로 반환한다.
// 범위별 화면을 돌아다니지 않고 여기서 전부 보고 관리한다.
func (h *Handler) List(c *gin.Context) {
	rows, err := h.repo.ListPolicies()
	if err != nil {
		httpx.Internal(c, "정책 조회 실패")
		return
	}
	if rows == nil {
		rows = []PolicyRow{}
	}
	g := h.resolver.Global() // 전역 폴백(설치 env + 런타임 오버라이드)
	httpx.OK(c, gin.H{
		"items": rows,
		"global": gin.H{
			"maxGpu": g.MaxGpu, "maxVramGb": g.MaxVramGB,
			"maxVolumeGib": g.MaxVolumeGiB, "maxConcurrentSessions": g.MaxConcurrentSessions,
			"maxStoppedSessions": g.MaxStoppedSessions,
			"maxEphemeralGib":    g.MaxEphemeralGiB,
		},
	})
}

// ListScoped는 정책 목록을 호출자 스코프로 좁힌다(단일 /console 트리).
func (h *Handler) ListScoped(c *gin.Context) {
	s := authz.CurrentScope(c)
	var rows []PolicyRow
	var err error
	if s.Level == "platform" {
		rows, err = h.repo.ListPolicies()
	} else {
		orgID, groupID := s.EffectiveIDs()
		rows, err = h.repo.ListPoliciesScoped(orgID, groupID)
	}
	if err != nil {
		httpx.Internal(c, "정책 조회 실패")
		return
	}
	if rows == nil {
		rows = []PolicyRow{}
	}
	g := h.resolver.Global()
	httpx.OK(c, gin.H{
		"items": rows,
		"global": gin.H{
			"maxGpu": g.MaxGpu, "maxVramGb": g.MaxVramGB,
			"maxVolumeGib": g.MaxVolumeGiB, "maxConcurrentSessions": g.MaxConcurrentSessions,
			"maxStoppedSessions": g.MaxStoppedSessions,
			"maxEphemeralGib":    g.MaxEphemeralGiB,
		},
	})
}

// SetGlobal은 전역 하드 상한을 런타임 조정한다(정책 탭의 '전역' 행).
//
// 전역은 원래 설치 시 고정이었는데, 조직/그룹/개인 상한은 화면에서 바로 바꾸면서
// 정작 모두에게 적용되는 전역만 Helm upgrade 를 해야 하는 게 이상했다.
// 세션 생성 때 숫자 비교만 하는 값이라 인프라와 무관하고 재시작도 필요 없다.
// 0 이하는 거부 — 전역은 폴백이라 "미지정"이 없다(비우면 상한이 사라져 버린다).
func (h *Handler) SetGlobal(c *gin.Context) {
	if h.setGlobal == nil {
		httpx.Err(c, 501, "not_supported", "런타임 설정 저장소가 없습니다")
		return
	}
	var body limitsBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Err(c, 400, "bad_body", "잘못된 요청")
		return
	}
	cur := h.resolver.Global()
	next := Resolved{
		MaxGpu:                pick(body.MaxGpu, cur.MaxGpu),
		MaxVramGB:             pick(body.MaxVramGB, cur.MaxVramGB),
		MaxVolumeGiB:          pick(body.MaxVolumeGiB, cur.MaxVolumeGiB),
		MaxConcurrentSessions: pick(body.MaxConcurrentSessions, cur.MaxConcurrentSessions),
		// 아래 둘을 빠뜨리면 Resolved 제로값(0)으로 저장돼 화면에서 바꿔도 반영되지 않고,
		// 오히려 기존 상한이 0(무제한)으로 조용히 풀린다.
		MaxStoppedSessions: pick(body.MaxStoppedSessions, cur.MaxStoppedSessions),
		MaxEphemeralGiB:    pick(body.MaxEphemeralGiB, cur.MaxEphemeralGiB),
	}
	// 1 이상 강제는 "미지정이 없어야 하는" 상한에만. 중단 세션 수·임시 디스크는 0=무제한이 유효한 설정이라
	// 여기서 막으면 관리자가 제한을 해제할 방법이 없어진다.
	if next.MaxGpu <= 0 || next.MaxVramGB <= 0 || next.MaxVolumeGiB <= 0 || next.MaxConcurrentSessions <= 0 {
		httpx.Err(c, 400, "bad_value", "전역 상한은 1 이상이어야 합니다")
		return
	}
	if next.MaxStoppedSessions < 0 || next.MaxEphemeralGiB < 0 {
		httpx.Err(c, 400, "bad_value", "중단 세션·임시 디스크 상한은 0 이상이어야 합니다(0=무제한)")
		return
	}
	if err := h.setGlobal(next); err != nil {
		httpx.Internal(c, "전역 정책 저장 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// pick은 값이 오면 그 값, 없으면 기존 값(부분 수정 허용).
func pick(v *int, cur int) int {
	if v == nil {
		return cur
	}
	return *v
}

// Get은 지정 계층(user/group/org)에 직접 설정된(미해석) 하드 제한을 반환한다.
// 미지정 항목은 JSON null(상위 폴백 대상). 편집 폼 초기값 로딩용.
func (h *Handler) Get(level Level) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			httpx.Err(c, 400, "bad_id", "잘못된 id")
			return
		}
		var l Limits
		switch level {
		case LevelUser:
			l, err = h.repo.UserLimits(id)
		case LevelGroup:
			l, err = h.repo.GroupLimits(id)
		case LevelOrg:
			l, err = h.repo.OrgLimits(id)
		}
		if err != nil {
			httpx.Internal(c, "제한 조회 실패")
			return
		}
		httpx.OK(c, limitsBody{MaxGpu: l.MaxGpu, MaxVramGB: l.MaxVramGB, MaxVolumeGiB: l.MaxVolumeGiB, MaxConcurrentSessions: l.MaxConcurrentSessions, MaxStoppedSessions: l.MaxStoppedSessions, MaxEphemeralGiB: l.MaxEphemeralGiB})
	}
}

// Effective는 한 사용자에게 최종 적용되는(계층 해석된) 하드 상한을 반환한다(관리자 확인용).
func (h *Handler) Effective(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Err(c, 400, "bad_id", "잘못된 id")
		return
	}
	httpx.OK(c, h.resolver.Resolve(id))
}
