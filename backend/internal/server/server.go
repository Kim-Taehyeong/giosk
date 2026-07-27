// Package server는 HTTP 라우터를 구성하고 도메인 라우트를 등록한다(라우팅 조립의 단일 지점).
// 도메인 로직은 각 도메인 패키지에 있고, 여기선 미들웨어 + 와이어링만 한다.
package server

import (
	"os"
	"strings"

	"giosk/internal/alert"
	"giosk/internal/announcement"
	"giosk/internal/audit"
	"giosk/internal/auth"
	"giosk/internal/authz"
	"giosk/internal/billing"
	"giosk/internal/dashboard"
	"giosk/internal/dataset"
	"giosk/internal/group"
	"giosk/internal/image"
	"giosk/internal/node"
	"giosk/internal/notify"
	"giosk/internal/org"
	"giosk/internal/policy"
	"giosk/internal/resource"
	"giosk/internal/session"
	"giosk/internal/systemconfig"
	"giosk/internal/topup"
	"giosk/internal/userdetail"
	"giosk/internal/usernotify"
	"giosk/internal/users"
	"giosk/internal/volume"
	"giosk/internal/wallet"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Deps는 서버가 등록할 도메인 핸들러/리더 모음(main 에서 조립해 주입).
type Deps struct {
	Auth            *auth.Handler
	Org             *org.Handler
	Group           *group.Handler
	Resource        *resource.Handler
	Image           *image.Handler
	Session         *session.Handler
	Wallet          *wallet.Handler
	Topup           *topup.Handler
	Volume          *volume.Handler
	Dataset         *dataset.Handler
	Node            *node.Handler
	Announcement    *announcement.Handler
	Users           *users.Handler
	UserDetail      *userdetail.Handler
	SystemConfig    *systemconfig.Handler
	Dashboard       *dashboard.Handler
	Audit           *audit.Handler
	AuditRepo       audit.Repository
	Billing         *billing.Handler
	Alert           *alert.Handler
	Notify          *notify.Handler
	UserNotify      *usernotify.Handler
	DatasetsEnabled bool // 설치 시 데이터셋 기능 on/off — off 면 데이터셋 라우트를 아예 등록하지 않는다(404)
	Policy          *policy.Handler
	OrgReader       authz.OrgReader
	AgentToken      string
	GroupReader     authz.MembershipReader
	ScopeReader     authz.ScopeReader      // 레벨 인식형 스코프 해석(group.Service)
	OrgOfGroup      authz.OrgOfGroupReader // 그룹→부모 조직(org.Repository)
}

// New는 미들웨어 + /api 라우트를 구성한 gin 엔진을 만든다.
func New(deps Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), corsMiddleware())
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

	api := r.Group("/api")
	registerPublic(api, deps)
	registerAuthed(api, deps)
	return r
}

// registerPublic — 인증 불필요.
func registerPublic(api gin.IRouter, deps Deps) {
	auth.Register(api, deps.Auth) // auth 는 내부에서 공개/인증 라우트를 함께 등록
	org.RegisterPublic(api, deps.Org)
	systemconfig.Register(api, deps.SystemConfig)       // 설치 설정 공개(/api/config)
	node.RegisterAgent(api, deps.Node, deps.AgentToken) // node-agent 전용(토큰 인증)
}

// registerAuthed — 인증 필요(+ 관리자/그룹 범위).
func registerAuthed(api gin.IRouter, deps Deps) {
	authed := api.Group("", deps.Auth.RequireAuth())
	authed.Use(audit.Middleware(deps.AuditRepo)) // 변경 요청 자동 감사 기록

	group.RegisterUser(authed, deps.Group)
	resource.RegisterUser(authed, deps.Resource)
	image.RegisterUser(authed, deps.Image)
	session.RegisterUser(authed, deps.Session)
	wallet.RegisterUser(authed, deps.Wallet)
	topup.RegisterUser(authed, deps.Topup)
	deps.Volume.Register(authed)
	if deps.DatasetsEnabled {
		dataset.RegisterUser(authed, deps.Dataset)
	}
	dashboard.RegisterUser(authed, deps.Dashboard)
	alert.RegisterUser(authed, deps.Alert)
	notify.RegisterUser(authed, deps.Notify)
	if deps.UserNotify != nil {
		usernotify.RegisterUser(authed, deps.UserNotify)
	}
	node.RegisterUser(authed, deps.Node)

	projAdmin := authz.GroupRole(deps.GroupReader, authz.RoleProjectAdmin)
	group.RegisterGroupScoped(authed, deps.Group, projAdmin)
	wallet.RegisterGroupScoped(authed, deps.Wallet, projAdmin)
	topup.RegisterGroupScoped(authed, deps.Topup, projAdmin)

	// ── 단일 거버넌스 트리(/console) — 레벨 인식형 ─────────────────────
	// platform/org/group 관리자가 같은 프론트 콘솔에서 쓰는 스코프 인식 엔드포인트.
	// /admin/* 은 마이그레이션 동안 병행 유지(플랫폼 기존 동작·팀원 브랜치 호환).
	mgmt := authed.Group("/console", authz.RequireManager(deps.ScopeReader, deps.OrgOfGroup))
	groupInScope := authz.RequireGroupInScope(deps.OrgOfGroup)
	// 운영 대시보드(스코프) — 인프라 대시보드는 /admin/dashboard(platform) 재사용.
	mgmt.GET("/dashboard/ops", deps.Dashboard.OpsScoped)
	// 거버넌스 조회(스코프) — 정책/빌링/감사. 읽기만; 하드상한 부여 등 쓰기는 /admin(platform) 유지.
	mgmt.GET("/limits", deps.Policy.ListScoped)
	// 스코프 정책 편집 — 매니저가 자기 범위 안에서 하드 제한 설정(상위 상한 초과 금지, 핸들러가 강제).
	mgmt.GET("/limits/user/:id", deps.Policy.Get(policy.LevelUser))
	mgmt.GET("/limits/group/:id", deps.Policy.Get(policy.LevelGroup))
	mgmt.GET("/limits/org/:id", deps.Policy.Get(policy.LevelOrg))
	mgmt.PUT("/limits/user/:id", deps.Policy.SetScoped(policy.LevelUser))
	mgmt.PUT("/limits/group/:id", deps.Policy.SetScoped(policy.LevelGroup))
	mgmt.PUT("/limits/org/:id", deps.Policy.SetScoped(policy.LevelOrg))
	// 설정 가능한 최대치(상위 유효 상한) — 매니저 편집 폼의 "최대 N" 힌트.
	mgmt.GET("/limits/user/:id/parent", deps.Policy.ParentLimit(policy.LevelUser))
	mgmt.GET("/limits/group/:id/parent", deps.Policy.ParentLimit(policy.LevelGroup))
	mgmt.GET("/limits/org/:id/parent", deps.Policy.ParentLimit(policy.LevelOrg))
	mgmt.GET("/billing", deps.Billing.ShowbackScoped)
	mgmt.GET("/audit", deps.Audit.ListScoped)
	// 목록(스코프로 필터)
	mgmt.GET("/orgs", deps.Org.ListScoped)
	mgmt.GET("/groups", deps.Group.ListScoped)
	mgmt.GET("/users", deps.Users.ListScoped)
	userdetail.RegisterScoped(mgmt, deps.UserDetail) // 사용자 360 상세(스코프 검증)
	deps.Volume.RegisterScoped(mgmt)                 // 볼륨 열람(스코프)
	// 조직 쓰기 — 생성/삭제/풀부여는 플랫폼, 수정은 자기 조직.
	mgmt.POST("/orgs", authz.RequirePlatform(), deps.Org.Create)
	mgmt.PUT("/orgs/:id", authz.RequireOrgInScope(), deps.Org.Update)
	mgmt.DELETE("/orgs/:id", authz.RequirePlatform(), deps.Org.Archive)
	mgmt.POST("/orgs/:id/grant", authz.RequirePlatform(), deps.Org.Grant)
	// 그룹 쓰기 — Create 는 핸들러가 스코프 강제(org→자기 조직, group→거부), 수정/삭제는 스코프 가드.
	mgmt.POST("/groups", deps.Group.Create)
	mgmt.PUT("/groups/:id", groupInScope, deps.Group.Update)
	mgmt.DELETE("/groups/:id", groupInScope, deps.Group.Delete)
	mgmt.PUT("/groups/:id/members/:userId/move", authz.RequirePlatform(), deps.Group.MoveMember)
	// 멤버·그룹지갑 — 기존 그룹범위 라우트를 in-scope 게이트로 재사용(org admin 자식 그룹 관리 포함).
	group.RegisterGroupScoped(mgmt, deps.Group, groupInScope)
	wallet.RegisterGroupScoped(mgmt, deps.Wallet, groupInScope)
	mgmt.POST("/groups/:id/wallet/grant", groupInScope, deps.Wallet.GrantGroup)

	admin := authed.Group("/admin", authz.RequireAdmin())
	org.RegisterAdmin(admin, deps.Org)
	group.RegisterAdmin(admin, deps.Group)
	resource.RegisterAdmin(admin, deps.Resource)
	image.RegisterAdmin(admin, deps.Image)
	wallet.RegisterAdmin(admin, deps.Wallet)
	topup.RegisterAdmin(admin, deps.Topup)
	if deps.DatasetsEnabled {
		dataset.RegisterAdmin(admin, deps.Dataset)
	}
	node.RegisterAdmin(admin, deps.Node)
	policy.RegisterAdmin(admin, deps.Policy)
	announcement.Register(authed, admin, mgmt, deps.Announcement)
	users.RegisterAdmin(admin, deps.Users)
	userdetail.RegisterAdmin(admin, deps.UserDetail)
	session.RegisterAdmin(admin, deps.Session)
	dashboard.RegisterAdmin(admin, deps.Dashboard)
	audit.RegisterAdmin(admin, deps.Audit)
	billing.RegisterAdmin(admin, deps.Billing)
	notify.RegisterAdmin(admin, deps.Notify)
	volume.SetScopeFn(func(c *gin.Context) (int64, int64) { return authz.CurrentScope(c).EffectiveIDs() }) // 매니저 볼륨 스코프
	deps.Volume.RegisterAdmin(admin)                                                                       // 관리자 스토리지 현황(/admin/storage)
	admin.PUT("/config", deps.SystemConfig.Update)                                                         // 운영 중 조정(유휴·기능 토글) 영속
}

// corsMiddleware는 허용 오리진을 GIOSK_CORS_ORIGINS(콤마 구분)에서 읽는다.
// 배포 기본형(프론트 nginx 가 /api 를 같은 오리진으로 프록시)에서는 CORS 자체가
// 발생하지 않으므로 불필요하지만, API 를 직접 호출하는 구성·개발을 위해 설정 가능.
//
//	미설정 → 로컬 개발 오리진(localhost:3000/5173)
//	"*"    → 모든 오리진 허용(인증은 Authorization 헤더라 credentials 불필요)
func corsMiddleware() gin.HandlerFunc {
	cfg := cors.DefaultConfig()
	cfg.AllowHeaders = []string{"Origin", "Content-Type", "Authorization"}
	cfg.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	switch origins := strings.TrimSpace(os.Getenv("GIOSK_CORS_ORIGINS")); origins {
	case "":
		cfg.AllowOrigins = []string{"http://localhost:3000", "http://localhost:5173"}
	case "*":
		cfg.AllowAllOrigins = true
	default:
		parts := strings.Split(origins, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		cfg.AllowOrigins = parts
	}
	return cors.New(cfg)
}
