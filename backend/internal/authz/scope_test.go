package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"giosk/internal/auth"

	"github.com/gin-gonic/gin"
)

type fakeScope struct {
	level      string
	org, group int64
	ok         bool
}

func (f fakeScope) PrimaryScope(userID int64) (string, int64, int64, bool) {
	return f.level, f.org, f.group, f.ok
}

func (f fakeScope) ManagerScopes(userID int64) []Scope {
	if f.ok && (f.level == "org" || f.level == "group") {
		return []Scope{{Level: f.level, OrgID: f.org, GroupID: f.group}}
	}
	return nil
}

type fakeOrgOfGroup map[int64]int64 // groupID 별 orgID

func (f fakeOrgOfGroup) OrgOfGroup(gid int64) (int64, bool) { o, ok := f[gid]; return o, ok }

// withUser는 gin 컨텍스트에 CurrentUser(auth 미들웨어와 동일 키)를 심는 테스트 미들웨어.
func withUser(u *auth.User) gin.HandlerFunc {
	return func(c *gin.Context) { c.Set("giosk.user", u); c.Next() }
}

// withUserScope는 유저 + X-Console-Scope 헤더를 함께 심는다(스코프 채택 테스트용).
func withUserScope(u *auth.User, scope string) gin.HandlerFunc {
	return func(c *gin.Context) { c.Set("giosk.user", u); c.Request.Header.Set("X-Console-Scope", scope); c.Next() }
}

func run(mw []gin.HandlerFunc, param string) int {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	path := "/x"
	if param != "" {
		path = "/x/:id"
	}
	chain := append(mw, func(c *gin.Context) { c.Status(200) })
	r.GET(path, chain...)
	target := "/x"
	if param != "" {
		target = "/x/" + param
	}
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", target, nil)
	r.ServeHTTP(w, req)
	return w.Code
}

func TestRequireManager_Levels(t *testing.T) {
	admin := &auth.User{ID: 1, Role: auth.RoleAdmin}
	orgAdmin := &auth.User{ID: 2, Role: "member"}
	member := &auth.User{ID: 3, Role: "member"}

	// platform admin 은 통과하고 scope=platform 이다
	if c := run([]gin.HandlerFunc{withUser(admin), RequireManager(fakeScope{}, nil), func(c *gin.Context) {
		if CurrentScope(c).Level != "platform" {
			t.Errorf("admin scope=%s", CurrentScope(c).Level)
		}
	}}, ""); c != 200 {
		t.Errorf("admin code=%d", c)
	}
	// org admin 도 통과한다
	if c := run([]gin.HandlerFunc{withUser(orgAdmin), RequireManager(fakeScope{level: "org", org: 7, ok: true}, nil)}, ""); c != 200 {
		t.Errorf("org admin code=%d", c)
	}
	// 일반 멤버는 403
	if c := run([]gin.HandlerFunc{withUser(member), RequireManager(fakeScope{level: "member", ok: true}, nil)}, ""); c != 403 {
		t.Errorf("member should be 403, got %d", c)
	}
}

// 최고관리자가 X-Console-Scope 로 조직/그룹을 고르면 그 스코프를 채택한다(소속 검증 없이 전권).
func TestRequireManager_AdminAdoptsScope(t *testing.T) {
	admin := &auth.User{ID: 1, Role: auth.RoleAdmin}
	oog := fakeOrgOfGroup{200: 9} // group200 ∈ org9

	got := ""
	run([]gin.HandlerFunc{withUserScope(admin, "org:7"), RequireManager(fakeScope{}, oog), func(c *gin.Context) {
		s := CurrentScope(c)
		got = s.Level
		if s.Level != "org" || s.OrgID != 7 {
			t.Errorf("admin org:7 결과 level=%s org=%d", s.Level, s.OrgID)
		}
	}}, "")
	if got != "org" {
		t.Errorf("handler not reached, got=%q", got)
	}

	// group:200 이면 OrgID 가 부모(9)로 보강돼야 한다.
	run([]gin.HandlerFunc{withUserScope(admin, "group:200"), RequireManager(fakeScope{}, oog), func(c *gin.Context) {
		s := CurrentScope(c)
		if s.Level != "group" || s.GroupID != 200 || s.OrgID != 9 {
			t.Errorf("admin group:200 결과 level=%s grp=%d org=%d (want group/200/9)", s.Level, s.GroupID, s.OrgID)
		}
	}}, "")

	// 헤더가 없거나 platform 이면 기본 platform 을 유지한다.
	run([]gin.HandlerFunc{withUser(admin), RequireManager(fakeScope{}, oog), func(c *gin.Context) {
		if CurrentScope(c).Level != "platform" {
			t.Errorf("admin no-header 결과 %s (want platform)", CurrentScope(c).Level)
		}
	}}, "")
}

func TestRequireGroupInScope(t *testing.T) {
	oog := fakeOrgOfGroup{100: 7, 200: 9} // group100∈org7, group200∈org9
	orgAdmin := &auth.User{ID: 2, Role: "member"}

	// org7 admin 은 자기 조직 산하 group100 을 통과한다
	if c := run([]gin.HandlerFunc{withUser(orgAdmin), RequireManager(fakeScope{level: "org", org: 7, ok: true}, nil), RequireGroupInScope(oog)}, "100"); c != 200 {
		t.Errorf("org7 의 group100 은 통과해야 하는데 %d", c)
	}
	// org7 admin 은 타 조직 group200 을 거부한다(403)
	if c := run([]gin.HandlerFunc{withUser(orgAdmin), RequireManager(fakeScope{level: "org", org: 7, ok: true}, nil), RequireGroupInScope(oog)}, "200"); c != 403 {
		t.Errorf("org7 의 group200 은 403 이어야 하는데 %d", c)
	}
	// group admin 은 자기 그룹만 본다
	if c := run([]gin.HandlerFunc{withUser(orgAdmin), RequireManager(fakeScope{level: "group", org: 7, group: 100, ok: true}, nil), RequireGroupInScope(oog)}, "100"); c != 200 {
		t.Errorf("group100 admin 의 group100 은 통과해야 하는데 %d", c)
	}
	if c := run([]gin.HandlerFunc{withUser(orgAdmin), RequireManager(fakeScope{level: "group", org: 7, group: 100, ok: true}, nil), RequireGroupInScope(oog)}, "200"); c != 403 {
		t.Errorf("group100 admin 의 group200 은 403 이어야 하는데 %d", c)
	}
	// platform 은 무조건 통과
	admin := &auth.User{ID: 1, Role: auth.RoleAdmin}
	if c := run([]gin.HandlerFunc{withUser(admin), RequireManager(fakeScope{}, nil), RequireGroupInScope(oog)}, "200"); c != 200 {
		t.Errorf("platform 은 어느 그룹이든 통과해야 하는데 %d", c)
	}
}

func TestRequireOrgInScope(t *testing.T) {
	orgAdmin := &auth.User{ID: 2, Role: "member"}
	if c := run([]gin.HandlerFunc{withUser(orgAdmin), RequireManager(fakeScope{level: "org", org: 7, ok: true}, nil), RequireOrgInScope()}, "7"); c != 200 {
		t.Errorf("org7 의 org7 은 통과해야 하는데 %d", c)
	}
	if c := run([]gin.HandlerFunc{withUser(orgAdmin), RequireManager(fakeScope{level: "org", org: 7, ok: true}, nil), RequireOrgInScope()}, "9"); c != 403 {
		t.Errorf("org7 의 org9 는 403 이어야 하는데 %d", c)
	}
	// group admin 은 조직 라우트를 거부당한다
	if c := run([]gin.HandlerFunc{withUser(orgAdmin), RequireManager(fakeScope{level: "group", org: 7, group: 100, ok: true}, nil), RequireOrgInScope()}, "7"); c != 403 {
		t.Errorf("group admin 의 org 라우트는 403 이어야 하는데 %d", c)
	}
}
