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

type fakeOrgOfGroup map[int64]int64 // groupID → orgID

func (f fakeOrgOfGroup) OrgOfGroup(gid int64) (int64, bool) { o, ok := f[gid]; return o, ok }

// withUser는 gin 컨텍스트에 CurrentUser(auth 미들웨어와 동일 키)를 심는 테스트 미들웨어.
func withUser(u *auth.User) gin.HandlerFunc {
	return func(c *gin.Context) { c.Set("giosk.user", u); c.Next() }
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

	// platform admin → 통과, scope=platform
	if c := run([]gin.HandlerFunc{withUser(admin), RequireManager(fakeScope{}), func(c *gin.Context) {
		if CurrentScope(c).Level != "platform" {
			t.Errorf("admin scope=%s", CurrentScope(c).Level)
		}
	}}, ""); c != 200 {
		t.Errorf("admin code=%d", c)
	}
	// org admin → 통과
	if c := run([]gin.HandlerFunc{withUser(orgAdmin), RequireManager(fakeScope{level: "org", org: 7, ok: true})}, ""); c != 200 {
		t.Errorf("org admin code=%d", c)
	}
	// 일반 멤버 → 403
	if c := run([]gin.HandlerFunc{withUser(member), RequireManager(fakeScope{level: "member", ok: true})}, ""); c != 403 {
		t.Errorf("member should be 403, got %d", c)
	}
}

func TestRequireGroupInScope(t *testing.T) {
	oog := fakeOrgOfGroup{100: 7, 200: 9} // group100∈org7, group200∈org9
	orgAdmin := &auth.User{ID: 2, Role: "member"}

	// org7 admin → 자기 조직 산하 group100 통과
	if c := run([]gin.HandlerFunc{withUser(orgAdmin), RequireManager(fakeScope{level: "org", org: 7, ok: true}), RequireGroupInScope(oog)}, "100"); c != 200 {
		t.Errorf("org7 → group100 should pass, got %d", c)
	}
	// org7 admin → 타 조직 group200 거부(403)
	if c := run([]gin.HandlerFunc{withUser(orgAdmin), RequireManager(fakeScope{level: "org", org: 7, ok: true}), RequireGroupInScope(oog)}, "200"); c != 403 {
		t.Errorf("org7 → group200 should 403, got %d", c)
	}
	// group admin → 자기 그룹만
	if c := run([]gin.HandlerFunc{withUser(orgAdmin), RequireManager(fakeScope{level: "group", org: 7, group: 100, ok: true}), RequireGroupInScope(oog)}, "100"); c != 200 {
		t.Errorf("group100 admin → group100 should pass, got %d", c)
	}
	if c := run([]gin.HandlerFunc{withUser(orgAdmin), RequireManager(fakeScope{level: "group", org: 7, group: 100, ok: true}), RequireGroupInScope(oog)}, "200"); c != 403 {
		t.Errorf("group100 admin → group200 should 403, got %d", c)
	}
	// platform → 무조건 통과
	admin := &auth.User{ID: 1, Role: auth.RoleAdmin}
	if c := run([]gin.HandlerFunc{withUser(admin), RequireManager(fakeScope{}), RequireGroupInScope(oog)}, "200"); c != 200 {
		t.Errorf("platform → any group should pass, got %d", c)
	}
}

func TestRequireOrgInScope(t *testing.T) {
	orgAdmin := &auth.User{ID: 2, Role: "member"}
	if c := run([]gin.HandlerFunc{withUser(orgAdmin), RequireManager(fakeScope{level: "org", org: 7, ok: true}), RequireOrgInScope()}, "7"); c != 200 {
		t.Errorf("org7 → org7 should pass, got %d", c)
	}
	if c := run([]gin.HandlerFunc{withUser(orgAdmin), RequireManager(fakeScope{level: "org", org: 7, ok: true}), RequireOrgInScope()}, "9"); c != 403 {
		t.Errorf("org7 → org9 should 403, got %d", c)
	}
	// group admin → 조직 라우트 거부
	if c := run([]gin.HandlerFunc{withUser(orgAdmin), RequireManager(fakeScope{level: "group", org: 7, group: 100, ok: true}), RequireOrgInScope()}, "7"); c != 403 {
		t.Errorf("group admin → org route should 403, got %d", c)
	}
}
