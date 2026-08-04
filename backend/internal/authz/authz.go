// Package authz는 권한 미들웨어를 제공한다.
//   RequireAdmin: 전역 플랫폼 관리자(users.role=admin)
//   GroupRole:    :id 그룹에서 호출자의 멤버십 역할이 최소 등급 이상
// 의존성 역전: 멤버십 조회는 MembershipReader 인터페이스로 주입(패키지 순환 회피).
package authz

import (
	"strconv"
	"strings"

	"giosk/internal/auth"
	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// MembershipReader는 그룹 내 사용자 역할을 조회한다(group.Repository 가 구현).
type MembershipReader interface {
	RoleInGroup(userID, groupID int64) (string, bool)
}

// 그룹 역할 서열이다. 높을수록 권한이 크다. 플랫폼 admin 은 비교에서 빼고 무조건 통과시킨다.
//
// 역할은 member(일반) / project_admin(팀장) / org_admin(조직 관리자) 3개뿐이다.
// 과거의 billing_admin 과 guest 는 제거했다. billing_admin 은 project_admin 과 등급이 같아
// 정산 담당에게 멤버 관리 권한까지 주고 있었고(이름과 다른 권한), guest 는 member 와
// 구분하는 검사가 한 곳도 없었다. 맵에 없는 역할은 rank 0(권한 없음)으로 떨어지므로,
// 남아 있는 옛 데이터는 관리 권한을 얻지 못한다(안전한 방향).
var groupRoleRank = map[string]int{
	"member":        1,
	"project_admin": 2,
	"org_admin":     3,
}

// 그룹 역할 상수(다른 패키지에서 GroupRole 호출 시 사용).
const (
	RoleMember       = "member"
	RoleProjectAdmin = "project_admin"
	RoleOrgAdmin     = "org_admin"
)

// RequireAdmin은 전역 플랫폼 관리자만 통과시킨다.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil || u.Role != auth.RoleAdmin {
			httpx.Forbidden(c, "platform admin only")
			return
		}
		c.Next()
	}
}

// OrgReader는 조직 관리자 여부를 조회한다(org.Repository 가 구현).
type OrgReader interface {
	IsOrgAdmin(userID, orgID int64) bool
}

// OrgRole은 :id 조직의 org_admin 인지 검사한다(플랫폼 admin 우회).
func OrgRole(reader OrgReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			httpx.Unauthorized(c, "auth required")
			return
		}
		if u.Role == auth.RoleAdmin {
			c.Next()
			return
		}
		orgID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || !reader.IsOrgAdmin(u.ID, orgID) {
			httpx.Forbidden(c, "org admin only")
			return
		}
		c.Next()
	}
}

// ── 레벨 인식형 거버넌스(단일 /console 트리) ──────────────────────
//
// 관리자 콘솔을 하나로 합치기 위해, 요청당 호출자의 스코프(platform/org/group)를 한 번 해석해
// 컨텍스트에 넣고, 각 핸들러가 그 스코프로 데이터를 필터·인가한다.

// Scope는 호출자의 거버넌스 범위. platform=전체, org=자기 조직, group=자기 그룹.
type Scope struct {
	Level   string // platform | org | group
	OrgID   int64  // org/group 스코프에서 소속 조직
	GroupID int64  // group 스코프에서 소속 그룹
}

// EffectiveIDs는 스코프 레벨에 맞는 (orgID, groupID) 를 반환한다.
// 주의: org 관리자도 자기 홈 그룹(GroupID)이 채워져 있으므로, 스코프 판별은 반드시 Level 로 한다.
// org 레벨=조직만(groupID 0), group 레벨=그룹만(orgID 0), platform=0,0(전역).
func (s Scope) EffectiveIDs() (orgID, groupID int64) {
	switch s.Level {
	case "org":
		return s.OrgID, 0
	case "group":
		return 0, s.GroupID
	}
	return 0, 0
}

// ScopeReader는 사용자의 관리 스코프를 조회한다(group.Service 가 구현).
// PrimaryScope=최우선 1개(하위 호환), ManagerScopes=전체(멀티롤 전환기·검증).
type ScopeReader interface {
	PrimaryScope(userID int64) (level string, orgID, groupID int64, ok bool)
	ManagerScopes(userID int64) []Scope
}

// OrgOfGroupReader는 그룹의 부모 조직을 조회한다(org.Repository 가 구현).
type OrgOfGroupReader interface {
	OrgOfGroup(groupID int64) (int64, bool)
}

const scopeKey = "giosk.scope"

// RequireManager는 플랫폼/조직/그룹 관리자를 통과시키고 Scope 를 컨텍스트에 넣는다.
// 일반 멤버는 403. 이후 핸들러는 authz.CurrentScope(c) 로 범위를 읽는다.
func RequireManager(sr ScopeReader, og OrgOfGroupReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			httpx.Unauthorized(c, "auth required")
			return
		}
		if u.Role == auth.RoleAdmin {
			// 최고관리자는 전권이라 기본 platform. 단 콘솔에서 특정 조직/그룹을 "역할"로 고르면
			// (X-Console-Scope) 그 스코프로 내려가서 그 담당자 시점으로 화면을 좁힌다.
			// admin 은 ManagerScopes 에 없으므로 소속 검증 없이 헤더를 신뢰(전권), 부모 조직만 보강.
			chosen := Scope{Level: "platform"}
			if sel := c.GetHeader("X-Console-Scope"); sel != "" && sel != "platform" {
				if s, ok := parseScopeSel(sel); ok {
					if s.Level == "group" && s.OrgID == 0 && og != nil {
						if orgID, found := og.OrgOfGroup(s.GroupID); found {
							s.OrgID = orgID
						}
					}
					chosen = s
				}
			}
			c.Set(scopeKey, chosen)
			c.Next()
			return
		}
		scopes := sr.ManagerScopes(u.ID)
		if len(scopes) == 0 {
			httpx.Forbidden(c, "manager only")
			return
		}
		// 멀티롤: 요청이 특정 스코프를 지정하면(X-Console-Scope) 보유 여부 검증 후 채택.
		// 없으면 기본값=최우선 스코프(scopes[0], PrimaryScope 와 동일).
		// 헤더는 UI 힌트일 뿐 권한 경계가 아니다(경계=보유 스코프 집합). 그래서 헤더가
		// 낡았거나(이전 세션·다른 로그인) 이 사용자가 안 가진 스코프면 403 대신 기본 스코프로
		// 폴백한다. 안 그러면 stale 헤더 하나가 목록 전체(사용자, 정책, 빌링)를 깨뜨린다.
		chosen := scopes[0]
		if sel := c.GetHeader("X-Console-Scope"); sel != "" {
			if s, ok := matchScope(scopes, sel); ok {
				chosen = s
			}
		}
		c.Set(scopeKey, chosen)
		c.Next()
	}
}

// parseScopeSel은 "org:10" 이나 "group:2" 를 소속 검증 없이 Scope 로 파싱한다(최고관리자 전용이라 전권이고
// 보유 집합 대조가 필요 없다). 형식 오류면 (zero, false). group 의 OrgID 는 호출측에서 보강.
func parseScopeSel(sel string) (Scope, bool) {
	i := strings.IndexByte(sel, ':')
	if i <= 0 {
		return Scope{}, false
	}
	level := sel[:i]
	id, err := strconv.ParseInt(sel[i+1:], 10, 64)
	if err != nil || id <= 0 {
		return Scope{}, false
	}
	switch level {
	case "org":
		return Scope{Level: "org", OrgID: id}, true
	case "group":
		return Scope{Level: "group", GroupID: id}, true
	}
	return Scope{}, false
}

// matchScope는 "org:10" / "group:2" 선택 문자열을 보유 스코프 집합에서 찾는다.
func matchScope(scopes []Scope, sel string) (Scope, bool) {
	i := strings.IndexByte(sel, ':')
	if i <= 0 {
		return Scope{}, false
	}
	level := sel[:i]
	id, err := strconv.ParseInt(sel[i+1:], 10, 64)
	if err != nil {
		return Scope{}, false
	}
	for _, s := range scopes {
		if s.Level != level {
			continue
		}
		if level == "org" && s.OrgID == id {
			return s, true
		}
		if level == "group" && s.GroupID == id {
			return s, true
		}
	}
	return Scope{}, false
}

// CurrentScope는 RequireManager 가 넣은 스코프를 반환한다(없으면 zero-value).
func CurrentScope(c *gin.Context) Scope {
	if v, ok := c.Get(scopeKey); ok {
		if s, ok := v.(Scope); ok {
			return s
		}
	}
	return Scope{}
}

// RequirePlatform은 RequireManager 로 진입한 뒤 플랫폼 관리자만 통과시킨다(조직 생성·조직풀 부여 등).
func RequirePlatform() gin.HandlerFunc {
	return func(c *gin.Context) {
		if CurrentScope(c).Level != "platform" {
			httpx.Forbidden(c, "platform admin only")
			return
		}
		c.Next()
	}
}

// RequireOrgInScope는 /orgs/:id 라우트를 스코프로 가드한다.
// platform=통과, org=자기 조직 id 만, group=거부.
func RequireOrgInScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		s := CurrentScope(c)
		if s.Level == "platform" {
			c.Next()
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			httpx.BadRequest(c, "invalid org id")
			return
		}
		if s.Level == "org" && s.OrgID == id {
			c.Next()
			return
		}
		httpx.Forbidden(c, "out of scope")
	}
}

// RequireGroupInScope는 /groups/:id 라우트를 스코프로 가드한다.
// platform=통과, org=그 그룹의 부모 조직이 내 조직이면 통과(org admin 은 자식 그룹 전부 관리),
// group=자기 그룹 id 만.
func RequireGroupInScope(or OrgOfGroupReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		s := CurrentScope(c)
		if s.Level == "platform" {
			c.Next()
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			httpx.BadRequest(c, "invalid group id")
			return
		}
		if s.Level == "group" {
			if s.GroupID == id {
				c.Next()
				return
			}
			httpx.Forbidden(c, "out of scope")
			return
		}
		// org 스코프: 그 그룹의 부모 조직이 내 조직이어야.
		if org, ok := or.OrgOfGroup(id); ok && org == s.OrgID {
			c.Next()
			return
		}
		httpx.Forbidden(c, "out of scope")
	}
}

// GroupRole은 :id 그룹에서 최소 등급 이상인지 검사한다.
func GroupRole(reader MembershipReader, min string) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			httpx.Unauthorized(c, "auth required")
			return
		}
		if u.Role == auth.RoleAdmin { // 플랫폼 관리자 우회
			c.Next()
			return
		}
		groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			httpx.BadRequest(c, "invalid group id")
			return
		}
		role, ok := reader.RoleInGroup(u.ID, groupID)
		if !ok || groupRoleRank[role] < groupRoleRank[min] {
			httpx.Forbidden(c, "insufficient group role")
			return
		}
		c.Next()
	}
}
