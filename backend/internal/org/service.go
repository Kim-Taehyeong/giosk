package org

import (
	"errors"
	"strings"
)

// ErrDuplicateName은 같은 이름의 조직이 이미 있을 때(unique 충돌).
var ErrDuplicateName = errors.New("duplicate org name")

// ErrOrgHasUsers는 조직 산하에 아직 사용자가 있어 삭제할 수 없을 때(고아 방지) — 전원 이전/삭제 후 가능.
var ErrOrgHasUsers = errors.New("organization still has users")

func isDuplicate(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Duplicate entry")
}

// AdminGroupBootstrapper는 조직 생성 시 기본 그룹 생성 + org_admin 배정을 수행한다(group.Service 구현).
type AdminGroupBootstrapper interface {
	CreateOrgAdminGroup(orgID int64, adminAccount string) error
	ResolveAdmin(account string) error // 조직 생성 전 관리자 계정 사전 검증
}

// Service는 org 비즈니스 로직.
type Service struct {
	repo         Repository
	bootstrapper AdminGroupBootstrapper
	platInterval func() int // 플랫폼 기본 리필 주기(조직 주기 캡)
}

func NewService(repo Repository) *Service { return &Service{repo: repo} }

// WithBootstrapper는 조직 생성 시 관리자(org_admin) 그룹 부트스트랩을 주입한다.
func (s *Service) WithBootstrapper(b AdminGroupBootstrapper) *Service { s.bootstrapper = b; return s }

func (s *Service) List() ([]Summary, error) { return s.repo.List() }

func (s *Service) Create(req CreateReq) (*Organization, error) {
	// 관리자 계정을 지정했으면 조직을 만들기 전에 먼저 검증한다(잘못된 계정인데 조직만 생성되는 문제 방지 — 원자성).
	if req.AdminAccount != "" && s.bootstrapper != nil {
		if err := s.bootstrapper.ResolveAdmin(req.AdminAccount); err != nil {
			return nil, err
		}
	}
	o := &Organization{
		Name:        req.Name,
		DisplayName: orDefault(req.DisplayName, req.Name),
		CreditPool:  req.CreditPool,
		Status:      "active",
	}
	if err := s.repo.Create(o); err != nil {
		if isDuplicate(err) {
			return nil, ErrDuplicateName
		}
		return nil, err
	}
	// 기본('일반') 그룹은 관리자 지정과 무관하게 항상 만든다 — org_admin 이 멤버십 역할이라
	// 나중에 관리자를 앉히려면 그룹이 먼저 있어야 한다(계정은 위에서 이미 검증됨).
	if s.bootstrapper != nil {
		if err := s.bootstrapper.CreateOrgAdminGroup(o.ID, req.AdminAccount); err != nil {
			return o, err
		}
	}
	return o, nil
}

func (s *Service) Update(id int64, req UpdateReq) error {
	fields := map[string]any{}
	if req.DisplayName != nil {
		fields["display_name"] = *req.DisplayName
	}
	if req.DefaultGroupBudget != nil {
		fields["default_group_budget"] = *req.DefaultGroupBudget
	}
	return s.repo.Update(id, fields)
}

// Archive는 조직을 삭제한다. 데이터 무결성 가드: 산하에 활성 사용자가 하나라도 있으면 삭제 불가
// (모든 사용자를 다른 조직으로 이전하거나 삭제한 뒤에야 삭제 가능) → 고아 사용자 방지.
func (s *Service) Archive(id int64) error {
	if s.repo.ActiveUserCount(id) > 0 {
		return ErrOrgHasUsers
	}
	return s.repo.Archive(id)
}

// Grant는 조직 크레딧 풀에 가산한다(크레딧 모드 전용 — 게이팅은 라우트/상위에서).
// PlatformInterval은 플랫폼 기본 리필 주기(일)를 반환한다(조직 주기 캡). nil 이면 캡 없음.
func (s *Service) WithPlatformInterval(fn func() int) *Service { s.platInterval = fn; return s }

func (s *Service) Grant(id int64, req GrantReq) error {
	if req.Amount != 0 {
		if err := s.repo.AddCredit(id, int64(req.Amount)); err != nil {
			return err
		}
	}
	// 정기 크레딧 설정 — 매 주기 재생성 잡(RunCreditRecharge)이 이 값으로 조직 풀을 채운다.
	if req.Recurring != nil {
		if err := s.repo.SetRecurring(id, *req.Recurring); err != nil {
			return err
		}
	}
	// 리필 주기/이월 — 주기는 플랫폼 기본보다 길 수 없다(하드캡).
	if req.Interval != nil || req.Carryover != nil {
		iv := 0
		if req.Interval != nil {
			iv = *req.Interval
			if s.platInterval != nil {
				if cap := s.platInterval(); cap > 0 && iv > cap {
					iv = cap
				}
			}
		}
		carry := false
		if req.Carryover != nil {
			carry = *req.Carryover
		}
		if err := s.repo.SetRefill(id, iv, carry); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) PublicCatalog() ([]PublicOrg, error) { return s.repo.PublicCatalog() }

// DrawForGroup은 그룹의 조직 풀에서 amount 를 차감한다(할당 enforce). 조직 없으면 통과(true).
func (s *Service) DrawForGroup(groupID int64, amount int) (bool, error) {
	orgID, ok := s.repo.OrgOfGroup(groupID)
	if !ok {
		return true, nil
	}
	return s.repo.DrawPool(orgID, amount)
}

// DrawForUser은 사용자의 조직 풀에서 amount 를 차감한다(할당 enforce). 조직 없으면 통과(true: 플랫폼 직할).
func (s *Service) DrawForUser(userID int64, amount int) (bool, error) {
	orgID, ok := s.repo.OrgOfUser(userID)
	if !ok {
		return true, nil
	}
	return s.repo.DrawPool(orgID, amount)
}

// AddForGroup은 그룹의 조직 풀에 amount 를 가산한다(최고관리자 그룹 부여 시 조직 자동 부여).
func (s *Service) AddForGroup(groupID int64, amount int) error {
	orgID, ok := s.repo.OrgOfGroup(groupID)
	if !ok {
		return nil
	}
	return s.repo.AddPool(orgID, amount)
}

// AddForUser은 사용자의 조직 풀에 amount 를 가산한다(최고관리자 개인 부여 시 조직 자동 부여).
func (s *Service) AddForUser(userID int64, amount int) error {
	orgID, ok := s.repo.OrgOfUser(userID)
	if !ok {
		return nil
	}
	return s.repo.AddPool(orgID, amount)
}

// GroupOfUser는 사용자의 대표 그룹 id 를 반환한다(없으면 false).
func (s *Service) GroupOfUser(userID int64) (int64, bool) { return s.repo.GroupOfUser(userID) }

// OrgOfUser는 사용자의 대표(활성 멤버십) 조직 id 를 반환한다(없으면 false).
func (s *Service) OrgOfUser(userID int64) (int64, bool) { return s.repo.OrgOfUser(userID) }

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
