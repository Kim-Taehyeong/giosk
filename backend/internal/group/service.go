package group

import (
	"errors"

	"giosk/internal/auth"
	"giosk/internal/authz"
)

var (
	ErrJoinClosed   = errors.New("group not accepting join requests")
	ErrUserUnknown  = errors.New("user not found")
	ErrGroupUnknown = errors.New("target group not found")
	ErrSameGroup    = errors.New("source and target group are identical")
	// 삭제 가드로 고아 멤버를 막는다.
	ErrDefaultGroup    = errors.New("default group cannot be deleted") // '일반'(general)은 이전 대상이라 삭제 불가
	ErrGroupHasMembers = errors.New("group has active members")        // 멤버를 다른 팀으로 옮기거나 제거해야 삭제 가능
)

// GroupWalletSeeder는 팀 생성 시 초기 크레딧/리필을 세팅한다(wallet.Service 구현; import 사이클 방지 위해 로컬 인터페이스).
type GroupWalletSeeder interface {
	CreditGroup(groupID int64, amount int, actor *int64) error
	SetGroupRefillVals(groupID int64, recurring, intervalDays int, carryover bool) error
}

// Service는 group 비즈니스 로직.
type Service struct {
	repo   Repository
	wallet GroupWalletSeeder // nil 이면 비크레딧이거나 미주입이라 초기크레딧과 리필을 생략한다
}

func NewService(repo Repository) *Service { return &Service{repo: repo} }

// WithWallet은 팀 생성 시 초기 크레딧/리필 세팅용 지갑 시더를 주입한다.
func (s *Service) WithWallet(w GroupWalletSeeder) *Service { s.wallet = w; return s }

func (s *Service) ListAll() ([]Summary, error) { return s.repo.ListAll() }

func (s *Service) Create(req CreateReq) (*Group, error) {
	// 팀 관리자 계정이 지정되면 먼저 검증(없는 계정이면 그룹 생성 전에 실패).
	var adminUID int64
	if req.AdminAccount != "" {
		uid, err := s.repo.ResolveUserID(req.AdminAccount)
		if errors.Is(err, ErrNotFound) {
			return nil, ErrUserUnknown
		}
		if err != nil {
			return nil, err
		}
		adminUID = uid
	}
	g := &Group{
		OrgID:       req.OrgID,
		Name:        req.Name,
		DisplayName: orDefault(req.DisplayName, req.Name),
		AcceptsJoin: true,
		Status:      "active",
	}
	if err := s.repo.Create(g); err != nil {
		return nil, err
	}
	if adminUID > 0 { // 지정 계정을 팀 관리자(project_admin)로 배정
		if err := s.repo.UpsertMembership(g.ID, adminUID, RoleProjectAdmin, MemberActive); err != nil {
			return g, err
		}
	}
	// 초기 크레딧/정기 리필(크레딧 모드·시더 주입 시). 실패해도 팀은 이미 생성됐으니 로그성 반환.
	if s.wallet != nil {
		if req.InitialCredit > 0 {
			if err := s.wallet.CreditGroup(g.ID, req.InitialCredit, nil); err != nil {
				return g, err
			}
		}
		if req.Recurring > 0 {
			if err := s.wallet.SetGroupRefillVals(g.ID, req.Recurring, req.Interval, req.Carryover); err != nil {
				return g, err
			}
		}
	}
	return g, nil
}

// ResolveAdmin은 관리자 계정 존재 여부만 검증한다(조직 생성 전 사전 검증이라 원자성을 지킨다). 없으면 ErrUserUnknown.
func (s *Service) ResolveAdmin(account string) error {
	_, err := s.repo.ResolveUserID(account)
	if errors.Is(err, ErrNotFound) {
		return ErrUserUnknown
	}
	return err
}

// CreateOrgAdminGroup은 조직 생성 시 기본 그룹을 만들고 지정 계정을 org_admin 으로 배정한다(조직 관리자 부트스트랩).
// CreateOrgAdminGroup은 조직의 기본('일반') 그룹을 만들고, 계정이 주어지면 org_admin 으로 배정한다.
//
// adminAccount 가 비어도 그룹은 만든다. org_admin 은 독립 역할이 아니라 멤버십 역할이라
// 역할을 걸어둘 그룹이 먼저 있어야 한다. 그래야 "조직 먼저 만들고 관리자는 나중에" 가 가능하다
// (나중에 그룹 상세에서 멤버 추가 + 역할=조직 관리자).
func (s *Service) CreateOrgAdminGroup(orgID int64, adminAccount string) error {
	var uid int64
	if adminAccount != "" {
		resolved, err := s.repo.ResolveUserID(adminAccount)
		if errors.Is(err, ErrNotFound) {
			return ErrUserUnknown
		}
		if err != nil {
			return err
		}
		uid = resolved
	}
	// 조직 기본(관리자) 그룹은 가입 신청 대상이 아니다(AcceptsJoin=false). 관리자가 멤버를 직접 추가하거나 그룹을 새로 만든다.
	g := &Group{OrgID: orgID, Name: "general", DisplayName: "일반", AcceptsJoin: false, Status: "active"}
	if err := s.repo.Create(g); err != nil {
		return err
	}
	if uid == 0 {
		return nil // 관리자는 나중에 지정
	}
	return s.repo.UpsertMembership(g.ID, uid, RoleOrgAdmin, MemberActive)
}

// UpdateGroup은 그룹 표시명·가입수락 여부를 수정한다(관리자).
func (s *Service) UpdateGroup(id int64, displayName *string, acceptsJoin *bool) error {
	fields := map[string]any{}
	if displayName != nil {
		fields["display_name"] = *displayName
	}
	if acceptsJoin != nil {
		fields["accepts_join"] = *acceptsJoin
	}
	return s.repo.Update(id, fields)
}

// ArchiveGroup은 그룹을 소프트 삭제한다(관리자).
// ArchiveGroup은 팀을 삭제한다. 데이터 무결성 가드: '일반'(general)은 이전 대상이라 절대 삭제 불가,
// 활성 멤버가 있는 팀도 삭제할 수 없다. 멤버를 다른 팀으로 옮기거나 제거해야 지울 수 있고, 그래야 고아 멤버가 안 생긴다.
func (s *Service) ArchiveGroup(id int64) error {
	g, err := s.repo.Find(id)
	if err != nil {
		return err
	}
	if g.Name == "general" {
		return ErrDefaultGroup
	}
	if s.repo.ActiveMemberCount(id) > 0 {
		return ErrGroupHasMembers
	}
	return s.repo.Archive(id)
}

// CancelJoinRequest는 본인의 대기중 가입신청을 취소한다.
func (s *Service) CancelJoinRequest(userID, reqID int64) error {
	return s.repo.CancelJoinRequest(userID, reqID)
}

// RoleInGroup은 authz.MembershipReader 구현.
func (s *Service) RoleInGroup(userID, groupID int64) (string, bool) {
	return s.repo.RoleInGroup(userID, groupID)
}

// Profile은 auth.Enricher 구현이다. 주 멤버십에서 콘솔 라우팅 컨텍스트와 전체 관리 스코프(멀티롤)를 만든다.
func (s *Service) Profile(userID int64) auth.ProfileExtra {
	p, err := s.repo.PrimaryMembership(userID)
	if err != nil || p == nil {
		return auth.ProfileExtra{}
	}
	membershipRole, consoleLevel := mapConsole(p.Role)
	ex := auth.ProfileExtra{
		MembershipRole: membershipRole, ConsoleLevel: consoleLevel,
		OrgID: &p.OrgID, GroupID: &p.GroupID, OrgName: p.OrgName, GroupName: p.GroupName,
	}
	// 전환기용이다. 관리 멤버십이 2개 이상이면 프론트가 스코프 스위처를 띄운다.
	if mm, err := s.repo.ManagerMemberships(userID); err == nil {
		for _, m := range mm {
			ex.Scopes = append(ex.Scopes, scopeView(m))
		}
	}
	return ex
}

// scopeView는 관리 멤버십을 전환기 표시용 ProfileScope 로 변환한다.
func scopeView(m PrimaryMembership) auth.ProfileScope {
	if m.Role == "org_admin" {
		return auth.ProfileScope{Level: "org", OrgID: m.OrgID, OrgName: m.OrgName}
	}
	return auth.ProfileScope{Level: "group", OrgID: m.OrgID, GroupID: m.GroupID, OrgName: m.OrgName, GroupName: m.GroupName}
}

// PrimaryScope는 authz.ScopeReader 구현이다. 주 멤버십에서 거버넌스 스코프(level, orgID, groupID)를 만든다.
// org_admin 은 자기 조직, project_admin 은 자기 그룹, 그 외는 member(권한 없음)가 된다.
func (s *Service) PrimaryScope(userID int64) (level string, orgID, groupID int64, ok bool) {
	p, err := s.repo.PrimaryMembership(userID)
	if err != nil || p == nil {
		return "", 0, 0, false
	}
	switch p.Role {
	case "org_admin":
		return "org", p.OrgID, p.GroupID, true
	case RoleProjectAdmin:
		return "group", p.OrgID, p.GroupID, true
	default:
		return "member", p.OrgID, p.GroupID, true
	}
}

// ManagerScopes는 authz.ScopeReader 구현이다. 사용자의 모든 관리 스코프(멀티롤)를 준다.
// 우선순위 순(org 먼저); [0] 은 PrimaryScope 와 동일(하위 호환 기본값).
func (s *Service) ManagerScopes(userID int64) []authz.Scope {
	mm, err := s.repo.ManagerMemberships(userID)
	if err != nil {
		return nil
	}
	out := make([]authz.Scope, 0, len(mm))
	for _, m := range mm {
		if m.Role == "org_admin" {
			out = append(out, authz.Scope{Level: "org", OrgID: m.OrgID, GroupID: m.GroupID})
		} else {
			out = append(out, authz.Scope{Level: "group", OrgID: m.OrgID, GroupID: m.GroupID})
		}
	}
	return out
}

// mapConsole은 멤버십 role 을 프론트 membershipRole 과 consoleLevel 로 옮긴다.
func mapConsole(role string) (string, string) {
	switch role {
	case "org_admin":
		return "org_admin", "org"
	case RoleProjectAdmin:
		return "group_admin", "group"
	default:
		return "member", "user"
	}
}

func (s *Service) Members(groupID int64) ([]Member, error) {
	members, err := s.repo.ListMembers(groupID)
	if err != nil {
		return nil, err
	}
	// 멀티롤 배지다. 각 멤버가 다른 그룹이나 조직에서 갖는 관리 역할까지 함께 보여준다.
	ids := make([]int64, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.UserID)
	}
	if badges, err := s.repo.ManagerRoleBadges(ids); err == nil {
		for i := range members {
			members[i].Roles = badges[members[i].UserID]
		}
	}
	return members, nil
}

func (s *Service) SetMemberBudget(groupID, userID int64, budget int) error {
	return s.repo.SetMemberBudget(groupID, userID, budget)
}

func (s *Service) Usage(groupID int64) ([]UsageRow, error) { return s.repo.Usage(groupID) }

// UsageTrend는 그룹 일자별 GPU 사용시간 추이(최근 14일).
func (s *Service) UsageTrend(groupID int64) []UsageTrendPoint { return s.repo.UsageTrend(groupID, 14) }

// UsageBySession은 그룹 세션별 GPU 사용시간.
func (s *Service) UsageBySession(groupID int64) []SessionUsage { return s.repo.UsageBySession(groupID) }

// AddMember는 account(username/email)로 사용자를 찾아 그룹에 배정한다.
func (s *Service) AddMember(groupID int64, req AddMemberReq) error {
	uid, err := s.repo.ResolveUserID(req.Account)
	if errors.Is(err, ErrNotFound) {
		return ErrUserUnknown
	}
	if err != nil {
		return err
	}
	return s.repo.UpsertMembership(groupID, uid, orDefault(req.Role, RoleMember), MemberActive)
}

func (s *Service) RemoveMember(groupID, userID int64) error {
	return s.repo.RemoveMember(groupID, userID)
}

// MoveMember는 사용자를 from 그룹에서 to 그룹으로 옮긴다.
// role 이 비면 원 그룹에서의 역할을 유지한다(팀장을 옮겨도 팀장).
func (s *Service) MoveMember(fromGroupID, toGroupID, userID int64, role string) error {
	if fromGroupID == toGroupID {
		return ErrSameGroup
	}
	if _, err := s.repo.Find(toGroupID); err != nil {
		return ErrGroupUnknown // 대상 그룹이 없으면 원본 멤버십을 건드리기 전에 멈춘다.
	}
	if role == "" {
		if cur, ok := s.repo.RoleInGroup(userID, fromGroupID); ok {
			role = cur
		} else {
			role = RoleMember
		}
	}
	return s.repo.MoveMember(fromGroupID, toGroupID, userID, role)
}
func (s *Service) SetMemberRole(groupID, userID int64, role string) error {
	return s.repo.SetMemberRole(groupID, userID, role)
}

func (s *Service) JoinPolicy(groupID int64) (bool, error) { return s.repo.GetAcceptsJoin(groupID) }
func (s *Service) SetJoinPolicy(groupID int64, v bool) error {
	return s.repo.SetAcceptsJoin(groupID, v)
}

func (s *Service) PendingJoinRequests(groupID int64) ([]JoinRequest, error) {
	return s.repo.ListJoinRequests(groupID, "pending")
}
func (s *Service) ApproveJoin(reqID int64) error { return s.repo.ApproveJoin(reqID) }
func (s *Service) RejectJoin(reqID int64) error  { return s.repo.RejectJoin(reqID) }

func (s *Service) MyGroups(userID int64) ([]GroupRef, error) { return s.repo.MyGroups(userID) }
func (s *Service) Directory(userID int64) ([]DirItem, error) { return s.repo.Directory(userID) }
func (s *Service) MyJoinRequests(userID int64) ([]MyJoinRequest, error) {
	return s.repo.MyJoinRequests(userID)
}

// ShareTargets는 볼륨 공유 대상(내 그룹 + 그 그룹의 동료 사용자, 본인 제외).
func (s *Service) ShareTargets(userID int64) (ShareTargets, error) {
	groups, err := s.repo.MyGroups(userID)
	if err != nil {
		return ShareTargets{}, err
	}
	if groups == nil {
		groups = []GroupRef{}
	}
	out := ShareTargets{Users: []ShareUser{}, Groups: groups}
	seen := map[int64]bool{userID: true}
	for _, g := range groups {
		members, err := s.repo.ListMembers(g.ID)
		if err != nil {
			continue
		}
		for _, m := range members {
			if seen[m.UserID] || m.Status != "active" {
				continue
			}
			seen[m.UserID] = true
			out.Users = append(out.Users, ShareUser{Username: m.Username, Name: m.Name})
		}
	}
	return out, nil
}

// RequestJoin은 그룹별 accepts_join 게이팅 후 가입 신청을 만든다.
func (s *Service) RequestJoin(userID, groupID int64) error {
	ok, err := s.repo.GetAcceptsJoin(groupID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrJoinClosed
	}
	return s.repo.CreateJoinRequest(userID, groupID)
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
