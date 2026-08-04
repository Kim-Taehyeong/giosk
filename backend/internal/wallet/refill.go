package wallet

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// ErrNoRecurring은 즉시 리필 대상 지갑에 정기 리필 금액(recurring_credit)이 설정돼 있지 않을 때.
var ErrNoRecurring = errors.New("no recurring refill configured")

// RefillNowUser는 (user,group) 멤버십 지갑을 즉시 리필한다(정기 주기를 기다리지 않고 지금 적용).
// 이월이면 잔액에 더하고, 아니면 recurring 으로 세팅. cycle_started_at 을 now 로 리셋해 다음 주기가 여기서 다시 센다.
func (r *gormRepo) RefillNowUser(userID, groupID int64) (int, error) {
	var w struct {
		Recurring int
		Balance   int
		Carryover bool
	}
	r.db.Raw("SELECT recurring_credit AS recurring, balance, carryover FROM user_wallets WHERE user_id=? AND group_id=?", userID, groupID).Scan(&w)
	if w.Recurring <= 0 {
		return 0, ErrNoRecurring
	}
	bal := w.Recurring
	if w.Carryover {
		bal = w.Balance + w.Recurring
	}
	now := time.Now()
	if err := r.db.Exec("UPDATE user_wallets SET balance=?, cycle_started_at=? WHERE user_id=? AND group_id=?", bal, now, userID, groupID).Error; err != nil {
		return 0, err
	}
	err := r.db.Exec(`INSERT INTO credit_transactions (user_id, group_id, type, amount, balance_after, description, created_at)
		VALUES (?, ?, 'recharge', ?, ?, '즉시 리필', NOW())`, userID, groupID, w.Recurring, bal).Error
	return w.Recurring, err
}

// RefillNowGroup은 팀(그룹) 지갑을 즉시 리필한다.
func (r *gormRepo) RefillNowGroup(groupID int64) (int, error) {
	var w struct {
		Recurring int
		Balance   int
		Carryover bool
	}
	r.db.Raw("SELECT recurring_credit AS recurring, balance, carryover FROM group_wallets WHERE group_id=?", groupID).Scan(&w)
	if w.Recurring <= 0 {
		return 0, ErrNoRecurring
	}
	bal := w.Recurring
	if w.Carryover {
		bal = w.Balance + w.Recurring
	}
	if err := r.db.Exec("UPDATE group_wallets SET balance=?, cycle_started_at=? WHERE group_id=?", bal, time.Now(), groupID).Error; err != nil {
		return 0, err
	}
	return w.Recurring, nil
}

// 계층형 정기 크레딧 리필 엔진.
//
// 플랫폼에서 조직, 팀, 개인으로 캐스케이드되며 각 풀은 자기 주기(effInterval)마다 recurring_credit 만큼 리필된다.
// 이월(carryover)이 켜지면 미사용분이 누적되고, 꺼지면 주기마다 recurring 으로 리셋된다.
// 상위가 이월불가면 그 상위가 리셋되는 순간(= cycle_started_at 이 갱신되는 순간) 하위 이월분도 전액 소멸한다.
//
// 핵심 단순화: 어떤 엔티티의 cycle_started_at 은 "마지막 리필/리셋 시각"이다. 따라서 이월불가 상위가
// 하위의 마지막 리필 이후에 리셋됐다면(= 상위 cycle > 하위 cycle) 하위는 이월분을 버리고 리셋해야 한다.
// 상위를 먼저 처리(top-down)하면 상위의 새 cycle 이 반영돼 이 비교가 정확해진다.

// PlatformRefill은 최고관리자 기본값(= 하드캡). Interval 은 리필 주기(일), Carryover 는 이월 허용.
type PlatformRefill struct {
	IntervalDays int
	Carryover    bool
}

// boundaryCrossed는 이월불가 상위가 since 이후에 리셋됐는지 본다(하위 강제 소멸 판정).
func boundaryCrossed(carryover bool, ancestorCycle *time.Time, since time.Time) bool {
	if carryover || ancestorCycle == nil {
		return false
	}
	return ancestorCycle.After(since)
}

func due(cycle *time.Time, intervalDays int, now time.Time) bool {
	if intervalDays <= 0 {
		return false
	}
	if cycle == nil {
		return true
	}
	return !now.Before(cycle.Add(time.Duration(intervalDays) * 24 * time.Hour))
}

// sinceOf는 nil cycle 을 0시각으로(항상 도래) 취급한다.
func sinceOf(cycle *time.Time) time.Time {
	if cycle == nil {
		return time.Time{}
	}
	return *cycle
}

// SetGroupRefill은 팀(그룹) 지갑의 정기 리필(양·주기·이월)을 설정한다. 지갑 없으면 생성.
func (r *gormRepo) SetGroupRefill(groupID int64, recurring, intervalDays int, carryover bool) error {
	r.db.Exec("INSERT IGNORE INTO group_wallets (group_id) VALUES (?)", groupID)
	return r.db.Exec("UPDATE group_wallets SET recurring_credit=?, refill_interval_days=?, carryover=? WHERE group_id=?",
		recurring, intervalDays, carryover, groupID).Error
}

// SetUserRefill은 (user,group) 멤버십 지갑의 정기 리필(양·주기·이월)을 설정한다. 지갑 없으면 생성.
func (r *gormRepo) SetUserRefill(userID, groupID int64, recurring, intervalDays int, carryover bool) error {
	r.db.Exec("INSERT IGNORE INTO user_wallets (user_id, group_id) VALUES (?, ?)", userID, groupID)
	return r.db.Exec("UPDATE user_wallets SET recurring_credit=?, refill_interval_days=?, carryover=? WHERE user_id=? AND group_id=?",
		recurring, intervalDays, carryover, userID, groupID).Error
}

// GroupRefillCap은 팀 주기 상한(부모 조직의 실효 주기). 없으면 platDefault.
func (r *gormRepo) GroupRefillCap(groupID int64, platDefault int) int {
	var iv int
	r.db.Raw("SELECT o.refill_interval_days FROM `groups` g JOIN organizations o ON o.id=g.org_id WHERE g.id=?", groupID).Scan(&iv)
	if iv > 0 {
		return iv
	}
	return platDefault
}

// UserRefillCap은 멤버십 지갑의 주기 상한(그 지갑의 팀 실효 주기 = 팀>0 ? 팀 : 조직>0 ? 조직 : platDefault).
func (r *gormRepo) UserRefillCap(groupID int64, platDefault int) int {
	var row struct{ GroupIV, OrgIV int }
	r.db.Raw("SELECT COALESCE(gw.refill_interval_days,0) AS group_iv, o.refill_interval_days AS org_iv "+
		"FROM `groups` g LEFT JOIN group_wallets gw ON gw.group_id=g.id JOIN organizations o ON o.id=g.org_id "+
		"WHERE g.id=?", groupID).Scan(&row)
	if row.GroupIV > 0 {
		return row.GroupIV
	}
	if row.OrgIV > 0 {
		return row.OrgIV
	}
	return platDefault
}

// RefillDue는 주기 도래한 조직/팀/개인 풀을 계층 규칙대로 리필한다. 리필된 풀 수를 반환한다.
func (r *gormRepo) RefillDue(plat PlatformRefill) (int, error) {
	var n int
	now := time.Now()
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// 0) 플랫폼 주기 앵커. 도래하면 전진하며, 이 순간이 조직 이월분 소멸 경계다.
		var platCycle *time.Time
		{
			var row struct{ PlatformCycleStartedAt *time.Time }
			tx.Raw(`SELECT platform_cycle_started_at FROM refill_state WHERE id=1`).Scan(&row)
			platCycle = row.PlatformCycleStartedAt
			if due(platCycle, plat.IntervalDays, now) {
				tx.Exec(`UPDATE refill_state SET platform_cycle_started_at=? WHERE id=1`, now)
				platCycle = &now
			}
		}

		// 1) 조직. 상위는 플랫폼이다.
		type orgRow struct {
			ID           int64
			Recurring    int
			IntervalDays int
			Carryover    bool
			Cycle        *time.Time
			Pool         int
		}
		var orgs []orgRow
		tx.Raw(`SELECT id, recurring_credit AS recurring, refill_interval_days AS interval_days, carryover, recurring_cycle_started_at AS cycle, credit_pool AS pool
			FROM organizations WHERE recurring_credit > 0 AND status='active'`).Scan(&orgs)
		orgCycle := map[int64]*time.Time{} // 처리 후 갱신된 조직 cycle(하위 판정용)
		orgCarry := map[int64]bool{}
		orgEff := map[int64]int{} // 조직 실효 리필 주기(하위 기본값)
		for _, o := range orgs {
			iv := o.IntervalDays
			if iv <= 0 {
				iv = plat.IntervalDays
			}
			orgCarry[o.ID] = o.Carryover
			orgEff[o.ID] = iv
			if !due(o.Cycle, iv, now) {
				orgCycle[o.ID] = o.Cycle
				continue
			}
			wipe := !o.Carryover || boundaryCrossed(plat.Carryover, platCycle, sinceOf(o.Cycle))
			pool := o.Recurring
			if !wipe {
				pool = o.Pool + o.Recurring
			}
			tx.Exec(`UPDATE organizations SET credit_pool=?, recurring_cycle_started_at=? WHERE id=?`, pool, now, o.ID)
			orgCycle[o.ID] = &now
			n++
		}

		// 2) 팀(그룹). 상위는 조직과 플랫폼이다.
		type grpRow struct {
			GroupID      int64
			OrgID        int64
			Recurring    int
			IntervalDays int
			Carryover    bool
			Cycle        *time.Time
			Balance      int
		}
		var grps []grpRow
		tx.Raw(`SELECT gw.group_id, g.org_id, gw.recurring_credit AS recurring, gw.refill_interval_days AS interval_days, gw.carryover, gw.cycle_started_at AS cycle, gw.balance
			FROM group_wallets gw JOIN ` + "`groups`" + ` g ON g.id=gw.group_id WHERE gw.recurring_credit > 0`).Scan(&grps)
		grpCycle := map[int64]*time.Time{}
		grpCarry := map[int64]bool{}
		grpOrg := map[int64]int64{}
		for _, gr := range grps {
			iv := gr.IntervalDays
			if iv <= 0 {
				if oi := orgEff[gr.OrgID]; oi > 0 {
					iv = oi
				} else {
					iv = plat.IntervalDays
				}
			}
			grpCarry[gr.GroupID] = gr.Carryover
			grpOrg[gr.GroupID] = gr.OrgID
			if !due(gr.Cycle, iv, now) {
				grpCycle[gr.GroupID] = gr.Cycle
				continue
			}
			since := sinceOf(gr.Cycle)
			wipe := !gr.Carryover ||
				boundaryCrossed(plat.Carryover, platCycle, since) ||
				boundaryCrossed(orgCarry[gr.OrgID], orgCycle[gr.OrgID], since)
			bal := gr.Recurring
			if !wipe {
				bal = gr.Balance + gr.Recurring
			}
			tx.Exec(`UPDATE group_wallets SET balance=?, cycle_started_at=? WHERE group_id=?`, bal, now, gr.GroupID)
			grpCycle[gr.GroupID] = &now
			n++
		}

		// 3) 개인. 상위는 팀, 조직, 플랫폼이다.
		type usrRow struct {
			UserID       int64
			GroupID      int64
			OrgID        int64
			Recurring    int
			IntervalDays int
			Carryover    bool
			Cycle        *time.Time
			Balance      int
		}
		var usrs []usrRow
		// 멤버십 지갑((user,group))마다 그 지갑의 팀(group)으로 상위(조직)를 결정한다.
		// group_id=0(미귀속) 지갑은 groups 조인이 없어 제외(리필 대상 아님).
		tx.Raw(`SELECT uw.user_id, uw.group_id, g.org_id, uw.recurring_credit AS recurring, uw.refill_interval_days AS interval_days, uw.carryover, uw.cycle_started_at AS cycle, uw.balance
			FROM user_wallets uw
			JOIN ` + "`groups`" + ` g ON g.id = uw.group_id
			WHERE uw.recurring_credit > 0`).Scan(&usrs)
		for _, us := range usrs {
			iv := us.IntervalDays
			if iv <= 0 {
				iv = plat.IntervalDays
			}
			if !due(us.Cycle, iv, now) {
				continue
			}
			since := sinceOf(us.Cycle)
			wipe := !us.Carryover ||
				boundaryCrossed(plat.Carryover, platCycle, since) ||
				boundaryCrossed(orgCarry[us.OrgID], orgCycle[us.OrgID], since) ||
				boundaryCrossed(grpCarry[us.GroupID], grpCycle[us.GroupID], since)
			bal := us.Recurring
			if !wipe {
				bal = us.Balance + us.Recurring
			}
			tx.Exec(`UPDATE user_wallets SET balance=?, cycle_started_at=? WHERE user_id=? AND group_id=?`, bal, now, us.UserID, us.GroupID)
			// 개인(멤버십) 리필은 크레딧 원장에 남긴다(팀 귀속).
			tx.Exec(`INSERT INTO credit_transactions (user_id, group_id, type, amount, balance_after, description, created_at)
				VALUES (?, ?, 'recharge', ?, ?, '정기 리필', NOW())`, us.UserID, us.GroupID, us.Recurring, bal)
			n++
		}
		return nil
	})
	return n, err
}
