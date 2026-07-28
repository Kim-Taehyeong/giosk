package wallet

import "gorm.io/gorm"

// Repository는 wallet 영속성 계약(원장 정합성을 위해 트랜잭션 사용).
type Repository interface {
	UserWallet(userID, groupID int64) (*UserWallet, error)
	ApplyUserTx(userID, groupID int64, txType string, amount int, desc, ref string, actor *int64) (int, error)
	SpendBySession(userID, groupID int64) ([]SessionSpend, error) // 세션별 소비 합계(원장 ref 기준)
	UserTransactions(userID, groupID int64, limit int) ([]CreditTx, error)
	DailyConsumption(userID, groupID int64, days int) ([]DailyPoint, error) // 잔디/달력 추이
	AddMemberConsumed(userID, groupID int64, credits int) error             // showback 누적((user,group) 멤버십)

	GroupWallet(groupID int64) (*GroupWallet, error)
	ApplyGroupTx(groupID int64, txType string, amount int, actor *int64, ref string) (int, error)
	DrawGroupTx(groupID int64, amount int, actor *int64) (bool, error) // 그룹 풀 차감(부족하면 false)
	SetGroupBudget(groupID int64, cap, alertPct int) error
	SetDefaultMemberBudget(groupID int64, v int) error
	RefillDue(plat PlatformRefill) (int, error) // 계층형 정기 크레딧 리필(조직→팀→개인). 구현: refill.go
	SetGroupRefill(groupID int64, recurring, intervalDays int, carryover bool) error
	SetUserRefill(userID, groupID int64, recurring, intervalDays int, carryover bool) error
	GroupRefillCap(groupID int64, platDefault int) int
	UserRefillCap(groupID int64, platDefault int) int // 멤버십 지갑의 팀(group) 기준 리필 주기 상한
	RefillNowUser(userID, groupID int64) (int, error) // 즉시 리필(멤버) — 정기 주기 무시하고 지금 적용
	RefillNowGroup(groupID int64) (int, error)        // 즉시 리필(팀)
}

type gormRepo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

// AddMemberConsumed는 (user,group) 멤버십에 사용량을 누적한다(showback ByUser). group 미지정(0)이면
// 대표(최소 id) 멤버십에 누적(과거 호환).
func (r *gormRepo) AddMemberConsumed(userID, groupID int64, credits int) error {
	if groupID > 0 {
		return r.db.Exec(`UPDATE memberships SET consumed = consumed + ? WHERE user_id = ? AND group_id = ?`, credits, userID, groupID).Error
	}
	return r.db.Exec(`UPDATE memberships SET consumed = consumed + ? WHERE user_id = ? ORDER BY id LIMIT 1`, credits, userID).Error
}

// UserWallet은 (user,group) 지갑을 보장(없으면 생성)하고 반환한다.
func (r *gormRepo) UserWallet(userID, groupID int64) (*UserWallet, error) {
	if err := r.ensureUserWallet(userID, groupID); err != nil {
		return nil, err
	}
	var w UserWallet
	return &w, r.db.Where("user_id = ? AND group_id = ?", userID, groupID).First(&w).Error
}

func (r *gormRepo) ensureUserWallet(userID, groupID int64) error {
	return r.db.Exec(`INSERT IGNORE INTO user_wallets (user_id, group_id) VALUES (?, ?)`, userID, groupID).Error
}

// ApplyUserTx는 (user,group) 지갑 잔액을 갱신하고 거래를 기록한다(트랜잭션). 갱신 후 잔액 반환.
func (r *gormRepo) ApplyUserTx(userID, groupID int64, txType string, amount int, desc, ref string, actor *int64) (int, error) {
	var newBalance int
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT IGNORE INTO user_wallets (user_id, group_id) VALUES (?, ?)`, userID, groupID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE user_wallets SET balance = balance + ? WHERE user_id = ? AND group_id = ?`, amount, userID, groupID).Error; err != nil {
			return err
		}
		if err := tx.Raw(`SELECT balance FROM user_wallets WHERE user_id = ? AND group_id = ?`, userID, groupID).Scan(&newBalance).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO credit_transactions (user_id, group_id, type, amount, balance_after, description, ref, actor_user_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, userID, groupID, txType, amount, newBalance, desc, nullIfEmpty(ref), actor).Error
	})
	return newBalance, err
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// SpendBySession은 세션별 소비 합계를 내림차순으로 반환한다(삭제된 세션도 ref 로 표시).
// 세션 식별자는 ref 컬럼 우선, 없으면(과거 거래) 설명 "GPU 사용(<id>)" 에서 추출한다.
func (r *gormRepo) SpendBySession(userID, groupID int64) ([]SessionSpend, error) {
	var out []SessionSpend
	err := r.db.Raw(`
		SELECT k.ref AS ref, COALESCE(s.name, k.ref) AS name, CAST(SUM(k.amt) AS SIGNED) AS credit
		FROM (
			SELECT COALESCE(NULLIF(t.ref, ''),
			               SUBSTRING_INDEX(SUBSTRING_INDEX(t.description, '(', -1), ')', 1)) AS ref,
			       -t.amount AS amt
			FROM credit_transactions t
			WHERE t.user_id = ? AND t.group_id = ? AND t.type = 'consume' AND t.amount < 0
		) k
		LEFT JOIN sessions s ON s.instance_id = k.ref
		WHERE k.ref IS NOT NULL AND k.ref <> ''
		GROUP BY k.ref, COALESCE(s.name, k.ref)
		ORDER BY credit DESC`, userID, groupID).Scan(&out).Error
	return out, err
}

func (r *gormRepo) UserTransactions(userID, groupID int64, limit int) ([]CreditTx, error) {
	var out []CreditTx
	return out, r.db.Where("user_id = ? AND group_id = ?", userID, groupID).Order("id DESC").Limit(limit).Find(&out).Error
}

// DailyConsumption은 최근 days일간 일자별 소비 합계(양수)를 오래된→최근 순으로 반환.
// 소비/정산(amount<0)만 집계하며, 원장 전체를 일자별로 GROUP BY 한다(거래 limit 무관).
func (r *gormRepo) DailyConsumption(userID, groupID int64, days int) ([]DailyPoint, error) {
	var out []DailyPoint
	err := r.db.Raw(`
		SELECT DATE_FORMAT(created_at, '%Y-%m-%d') AS day, CAST(SUM(-amount) AS SIGNED) AS amount
		FROM credit_transactions
		WHERE user_id = ? AND group_id = ? AND amount < 0
		  AND created_at >= DATE_SUB(CURDATE(), INTERVAL ? DAY)
		GROUP BY DATE_FORMAT(created_at, '%Y-%m-%d')
		ORDER BY day ASC`, userID, groupID, days).Scan(&out).Error
	return out, err
}

func (r *gormRepo) GroupWallet(groupID int64) (*GroupWallet, error) {
	if err := r.ensureGroupWallet(groupID); err != nil {
		return nil, err
	}
	var w GroupWallet
	return &w, r.db.Where("group_id = ?", groupID).First(&w).Error
}

func (r *gormRepo) ensureGroupWallet(groupID int64) error {
	return r.db.Exec(`INSERT IGNORE INTO group_wallets (group_id) VALUES (?)`, groupID).Error
}

func (r *gormRepo) ApplyGroupTx(groupID int64, txType string, amount int, actor *int64, ref string) (int, error) {
	var newBalance, reserved int
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT IGNORE INTO group_wallets (group_id) VALUES (?)`, groupID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE group_wallets SET balance = balance + ? WHERE group_id = ?`, amount, groupID).Error; err != nil {
			return err
		}
		row := struct{ Balance, Reserved int }{}
		if err := tx.Raw(`SELECT balance, reserved FROM group_wallets WHERE group_id = ?`, groupID).Scan(&row).Error; err != nil {
			return err
		}
		newBalance, reserved = row.Balance, row.Reserved
		return tx.Exec(`INSERT INTO group_credit_transactions (group_id, type, amount, balance_after, reserved_after, actor_user_id, session_ref)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, groupID, txType, amount, newBalance, reserved, actor, ref).Error
	})
	return newBalance, err
}

// DrawGroupTx는 그룹 풀(balance)에서 amount 를 원자적으로 차감한다(부족하면 차감 없이 false).
// 그룹→개인 하위 배분 시 그룹 풀 한도를 강제한다.
func (r *gormRepo) DrawGroupTx(groupID int64, amount int, actor *int64) (bool, error) {
	var ok bool
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT IGNORE INTO group_wallets (group_id) VALUES (?)`, groupID).Error; err != nil {
			return err
		}
		res := tx.Exec(`UPDATE group_wallets SET balance = balance - ? WHERE group_id = ? AND balance >= ?`, amount, groupID, amount)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil // 잔여 부족
		}
		ok = true
		var row struct{ Balance, Reserved int }
		if err := tx.Raw(`SELECT balance, reserved FROM group_wallets WHERE group_id = ?`, groupID).Scan(&row).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO group_credit_transactions (group_id, type, amount, balance_after, reserved_after, actor_user_id, session_ref)
			VALUES (?, 'allocate', ?, ?, ?, ?, '')`, groupID, -amount, row.Balance, row.Reserved, actor).Error
	})
	return ok, err
}

func (r *gormRepo) SetGroupBudget(groupID int64, cap, alertPct int) error {
	if err := r.ensureGroupWallet(groupID); err != nil {
		return err
	}
	return r.db.Exec(`UPDATE group_wallets SET budget_cap = ?, alert_threshold_pct = ? WHERE group_id = ?`,
		cap, alertPct, groupID).Error
}

func (r *gormRepo) SetDefaultMemberBudget(groupID int64, v int) error {
	if err := r.ensureGroupWallet(groupID); err != nil {
		return err
	}
	return r.db.Exec(`UPDATE group_wallets SET default_member_budget = ? WHERE group_id = ?`, v, groupID).Error
}
