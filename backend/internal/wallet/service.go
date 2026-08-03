package wallet

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"
)

// RechargeCfg는 플랫폼 기본 리필 설정(런타임 = 계층 하드캡). 금액은 조직/팀/개인 단위로 설정된다.
type RechargeCfg struct {
	Enabled      bool
	IntervalDays int  // 플랫폼 기본 리필 주기(일) = 하위 캡
	Carryover    bool // 플랫폼 이월 허용(= 하위 이월 상한 경계)
}

// RunCreditRecharge는 주기적으로(tick) 재충전 대상 지갑을 리필한다. cfgFn 은 호출 시점 설정을 읽는다
// (관리자가 런타임에 켜고/금액 바꾸면 바로 반영). 비활성/양수 아님/주기 0 이면 스킵.
func (s *Service) RunCreditRecharge(ctx context.Context, tick time.Duration, cfgFn func() RechargeCfg) {
	log.Printf("[credit-recharge] started (check interval=%s)", tick)
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c := cfgFn()
			if !c.Enabled || c.IntervalDays <= 0 {
				continue
			}
			if n, err := s.repo.RefillDue(PlatformRefill{IntervalDays: c.IntervalDays, Carryover: c.Carryover}); err != nil {
				log.Printf("[credit-recharge] error: %v", err)
			} else if n > 0 {
				log.Printf("[credit-recharge] refilled %d pools (platform interval %dd, carryover=%v)", n, c.IntervalDays, c.Carryover)
			}
		}
	}
}

// ErrInsufficientPool은 조직 크레딧 풀 잔여 부족(할당 불가).
var ErrInsufficientPool = errors.New("insufficient org credit pool")

// Pool은 계층형 크레딧 배분의 조직 풀 연산이다(org.Service 가 구현). 조직 없으면 Draw 는 (true,nil).
type Pool interface {
	DrawForUser(userID int64, amount int) (bool, error)
	DrawForGroup(groupID int64, amount int) (bool, error)
	AddForGroup(groupID int64, amount int) error // 최고관리자 그룹 부여 시 조직 풀 자동 가산
	AddForUser(userID int64, amount int) error   // 최고관리자 개인 부여 시 조직 풀 자동 가산
	GroupOfUser(userID int64) (int64, bool)      // 개인의 대표 그룹(그룹 풀 차감 대상)
}

// ErrNoGroup은 개인 배분 대상의 그룹이 없을 때(그룹 풀 차감 불가).
var ErrNoGroup = errors.New("user has no group")

// ErrInsufficientGroupPool은 그룹 풀 잔여 부족(개인 배분 불가).
var ErrInsufficientGroupPool = errors.New("insufficient group credit pool")

// Service는 wallet 비즈니스 로직. topup.Crediter(개인/그룹) 도 구현.
type Service struct {
	repo         Repository
	pool         Pool       // 할당 시 조직 풀 차감(nil=enforce 비활성)
	platInterval func() int // 플랫폼 기본 리필 주기(계층 캡 최상단)
}

func NewService(repo Repository) *Service { return &Service{repo: repo} }

// WithPlatformInterval은 팀/개인 리필 주기 캡의 최상단(플랫폼 기본)을 주입한다.
func (s *Service) WithPlatformInterval(fn func() int) *Service { s.platInterval = fn; return s }

func (s *Service) platDefault() int {
	if s.platInterval != nil {
		if v := s.platInterval(); v > 0 {
			return v
		}
	}
	return 30
}

// RefillSpec은 팀/개인 정기 리필 설정 요청(주기는 상위보다 길 수 없다 — 서비스에서 클램프).
type RefillSpec struct {
	Recurring    int  `json:"recurring"`
	IntervalDays int  `json:"interval"`
	Carryover    bool `json:"carryover"`
}

// SetGroupRefillVals는 원시값으로 팀 리필을 설정한다(group.Service 시더용 — RefillSpec import 회피).
func (s *Service) SetGroupRefillVals(groupID int64, recurring, intervalDays int, carryover bool) error {
	return s.SetGroupRefill(groupID, RefillSpec{Recurring: recurring, IntervalDays: intervalDays, Carryover: carryover})
}

// SetGroupRefill은 팀 리필을 설정한다(주기는 부모 조직 상한으로 클램프).
func (s *Service) SetGroupRefill(groupID int64, spec RefillSpec) error {
	iv := spec.IntervalDays
	if cap := s.repo.GroupRefillCap(groupID, s.platDefault()); cap > 0 && iv > cap {
		iv = cap
	}
	return s.repo.SetGroupRefill(groupID, spec.Recurring, iv, spec.Carryover)
}

// SetUserRefill은 (user,group) 멤버십 지갑의 리필을 설정한다(주기는 그 팀 상한으로 클램프).
func (s *Service) SetUserRefill(userID, groupID int64, spec RefillSpec) error {
	iv := spec.IntervalDays
	if cap := s.repo.UserRefillCap(groupID, s.platDefault()); cap > 0 && iv > cap {
		iv = cap
	}
	return s.repo.SetUserRefill(userID, groupID, spec.Recurring, iv, spec.Carryover)
}

// resolveGroup은 정산/조회에 쓸 팀을 정한다: 명시된 groupID 우선, 없으면 사용자의 대표 팀(pool).
// 팀 없는 세션은 금지하므로 세션 정산 경로엔 항상 실제 팀이 온다 — 이 폴백은 groupID 미전달 방어용.
func (s *Service) resolveGroup(userID, groupID int64) int64 {
	if groupID > 0 {
		return groupID
	}
	if s.pool != nil {
		if g, ok := s.pool.GroupOfUser(userID); ok {
			return g
		}
	}
	return 0
}

// WithPool은 할당(grant/topup) 시 조직 풀 차감 enforce 를 활성화한다.
func (s *Service) WithPool(p Pool) *Service { s.pool = p; return s }

// MyWallet은 (user,group) 멤버십 지갑 + 최근 거래를 반환한다. groupID=활성 스코프(팀); 0이면 대표 팀.
func (s *Service) MyWallet(userID, groupID int64) (*MyWalletRes, error) {
	g := s.resolveGroup(userID, groupID)
	w, err := s.repo.UserWallet(userID, g)
	if err != nil {
		return nil, err
	}
	hist, err := s.repo.UserTransactions(userID, g, 500) // 페이지네이션은 프론트(클라)에서
	if err != nil {
		return nil, err
	}
	trend, err := s.repo.DailyConsumption(userID, g, 210)
	if err != nil {
		return nil, err
	}
	bySession, err := s.repo.SpendBySession(userID, g)
	if err != nil {
		return nil, err
	}
	return &MyWalletRes{UserWallet: *w, History: hist, Trend: trend, BySession: bySession}, nil
}

func (s *Service) GroupWallet(groupID int64) (*GroupWallet, error) {
	return s.repo.GroupWallet(groupID)
}

func (s *Service) SetGroupBudget(groupID int64, req BudgetReq) error {
	return s.repo.SetGroupBudget(groupID, req.BudgetCap, orPct(req.AlertPct))
}

func (s *Service) SetDefaultMemberBudget(groupID int64, v int) error {
	return s.repo.SetDefaultMemberBudget(groupID, v)
}

// Balance는 (user,group) 멤버십 지갑 잔액을 반환한다(지갑 없음/오류=0). 세션 생성 전 잔액 검사용.
func (s *Service) Balance(userID, groupID int64) int {
	w, err := s.repo.UserWallet(userID, s.resolveGroup(userID, groupID))
	if err != nil || w == nil {
		return 0
	}
	return w.Balance
}

// CreditUser는 개인 지갑에 가산(topup.Crediter / 관리자 grant).
// 조직 풀 enforce 활성 시: 사용자의 조직 풀에서 먼저 차감하고, 잔여 부족이면 ErrInsufficientPool.
func (s *Service) CreditUser(userID, groupID int64, amount int, desc string, actor *int64) error {
	if s.pool != nil && amount > 0 {
		ok, err := s.pool.DrawForUser(userID, amount)
		if err != nil {
			return err
		}
		if !ok {
			return ErrInsufficientPool
		}
	}
	_, err := s.repo.ApplyUserTx(userID, s.resolveGroup(userID, groupID), TxTopup, amount, desc, "", actor)
	return err
}

// CreditGroup은 그룹 지갑에 가산(topup.Crediter / 관리자 grant). 조직 풀에서 차감(enforce).
func (s *Service) CreditGroup(groupID int64, amount int, actor *int64) error {
	if s.pool != nil && amount > 0 {
		ok, err := s.pool.DrawForGroup(groupID, amount)
		if err != nil {
			return err
		}
		if !ok {
			return ErrInsufficientPool
		}
	}
	_, err := s.repo.ApplyGroupTx(groupID, TxTopup, amount, actor, "")
	return err
}

// AllocateGroup은 조직→그룹 하위 배분이다(계층형).
//   - super(최고관리자): 예외 부여 — 그룹에 가산하고 무결성을 위해 조직 풀에도 자동 가산(조직 150·그룹 50).
//   - 일반(조직 담당자): 조직 풀에서 amount 차감 후 그룹에 가산(잔여 부족이면 ErrInsufficientPool).
func (s *Service) AllocateGroup(groupID int64, amount int, super bool, actor *int64) error {
	if amount <= 0 {
		return nil
	}
	if super {
		if s.pool != nil {
			if err := s.pool.AddForGroup(groupID, amount); err != nil { // 조직 풀 자동 가산
				return err
			}
		}
	} else if s.pool != nil {
		ok, err := s.pool.DrawForGroup(groupID, amount)
		if err != nil {
			return err
		}
		if !ok {
			return ErrInsufficientPool
		}
	}
	_, err := s.repo.ApplyGroupTx(groupID, TxTopup, amount, actor, "")
	return err
}

// AllocateUser은 그룹→개인 하위 배분이다(계층형).
//   - super(최고관리자): 예외 부여 — 개인에 가산하고 무결성을 위해 그룹·조직 풀에도 자동 가산.
//   - 일반(그룹 담당자): 사용자의 그룹 풀에서 amount 차감 후 개인에 가산(부족이면 ErrInsufficientGroupPool).
func (s *Service) AllocateUser(userID, groupID int64, amount int, super bool, desc string, actor *int64) error {
	if amount <= 0 {
		return nil
	}
	if groupID <= 0 { // 팀 미지정이면 대표 팀으로
		groupID = s.resolveGroup(userID, 0)
		if groupID <= 0 {
			return ErrNoGroup
		}
	}
	if super {
		if s.pool != nil {
			if err := s.pool.AddForUser(userID, amount); err != nil { // 조직 풀 자동 가산
				return err
			}
			if _, err := s.repo.ApplyGroupTx(groupID, TxTopup, amount, actor, ""); err != nil { // 그룹 풀 자동 가산
				return err
			}
		}
	} else if s.pool != nil {
		drawn, err := s.repo.DrawGroupTx(groupID, amount, actor)
		if err != nil {
			return err
		}
		if !drawn {
			return ErrInsufficientGroupPool
		}
	}
	_, err := s.repo.ApplyUserTx(userID, groupID, TxTopup, amount, desc, "", actor)
	return err
}

// Consume은 사용량(credits)을 개인 지갑에서 차감한다(세션 소비 회계).
// 잔액 부족이면 (false, nil). 멤버 누적 사용량(showback)도 갱신.
func (s *Service) Consume(userID, groupID int64, credits int, ref string) (bool, error) {
	if credits <= 0 {
		return true, nil
	}
	g := s.resolveGroup(userID, groupID)
	w, err := s.repo.UserWallet(userID, g)
	if err != nil {
		return false, err
	}
	if w.Balance < credits {
		return false, nil
	}
	// 소비 내역 설명은 리소스 종류에 맞춘다(ref 접두사: vol-=스토리지, 그 외 ses-=GPU 세션).
	// 스토리지 과금이 "GPU 사용"으로 오표기되던 문제 수정.
	label := "GPU 사용(" + ref + ")"
	if strings.HasPrefix(ref, "vol-") {
		label = "스토리지(" + ref + ")"
	}
	if _, err := s.repo.ApplyUserTx(userID, g, TxConsume, -credits, label, ref, nil); err != nil {
		return false, err
	}
	_ = s.repo.AddMemberConsumed(userID, g, credits)
	return true, nil
}

func orPct(v int) int {
	if v <= 0 {
		return 80
	}
	return v
}
