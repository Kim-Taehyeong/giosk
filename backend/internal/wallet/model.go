package wallet

import "time"

// 거래 type.
const (
	TxTopup      = "topup"
	TxHold       = "hold"
	TxConsume    = "consume"
	TxSettle     = "settle"
	TxRefund     = "refund"
	TxAdminGrant = "admin_grant"
)

// UserWallet은 user_wallets 테이블 매핑. 지갑은 멤버십(유저×팀) 단위 — PK=(user_id, group_id).
type UserWallet struct {
	UserID             int64      `gorm:"primaryKey;column:user_id" json:"-"`
	GroupID            int64      `gorm:"primaryKey;column:group_id" json:"groupId"` // 팀(그룹) id — 지갑이 귀속된 팀
	Balance            int        `gorm:"column:balance" json:"balance"`
	Reserved           int        `gorm:"column:reserved" json:"reserved"`
	MonthlyCap         int        `gorm:"column:monthly_cap" json:"cap"`
	CycleStartedAt     *time.Time `gorm:"column:cycle_started_at" json:"cycleStartedAt,omitempty"`
	RecurringCredit    int        `gorm:"column:recurring_credit" json:"recurringCredit"`
	RefillIntervalDays int        `gorm:"column:refill_interval_days" json:"refillIntervalDays"`
	Carryover          bool       `gorm:"column:carryover" json:"carryover"`
}

func (UserWallet) TableName() string { return "user_wallets" }

// GroupWallet은 group_wallets 테이블 매핑.
type GroupWallet struct {
	GroupID             int64 `gorm:"primaryKey;column:group_id" json:"-"`
	Balance             int   `gorm:"column:balance" json:"balance"`
	Reserved            int   `gorm:"column:reserved" json:"reserved"`
	BudgetCap           int   `gorm:"column:budget_cap" json:"budgetCap"`
	AlertThresholdPct   int   `gorm:"column:alert_threshold_pct" json:"alertPct"`
	DefaultMemberBudget int   `gorm:"column:default_member_budget" json:"defaultBudget"`
	RecurringCredit     int   `gorm:"column:recurring_credit" json:"recurringCredit"`
	RefillIntervalDays  int   `gorm:"column:refill_interval_days" json:"refillIntervalDays"`
	Carryover           bool  `gorm:"column:carryover" json:"carryover"`
}

func (GroupWallet) TableName() string { return "group_wallets" }

// CreditTx — 개인 거래 내역.
type CreditTx struct {
	ID           int64     `gorm:"primaryKey" json:"-"`
	Type         string    `gorm:"column:type" json:"type"`
	Amount       int       `gorm:"column:amount" json:"amount"`
	BalanceAfter int       `gorm:"column:balance_after" json:"balance"`
	Description  string    `gorm:"column:description" json:"desc"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"createdAt"`
}

func (CreditTx) TableName() string { return "credit_transactions" }

// DailyPoint — 일자별 소비 합계(양수). 잔디/달력 추이용.
type DailyPoint struct {
	Day    string `gorm:"column:day" json:"day"` // YYYY-MM-DD
	Amount int    `gorm:"column:amount" json:"amount"`
}

// SessionSpend — 세션별 소비 합계(원장 ref 기준).
type SessionSpend struct {
	Ref    string `gorm:"column:ref" json:"ref"`
	Name   string `gorm:"column:name" json:"name"`
	Credit int    `gorm:"column:credit" json:"credit"`
}

// MyWalletRes — /me/wallet 응답.
type MyWalletRes struct {
	UserWallet
	History   []CreditTx     `json:"history"`
	Trend     []DailyPoint   `json:"trend"`     // 일자별 소비 합계(오래된→최근)
	BySession []SessionSpend `json:"bySession"` // 세션별 소비 합계(많은 순)
}

// 요청 바디.
type GrantReq struct {
	Amount int    `json:"amount" binding:"required"`
	Reason string `json:"reason"`
}

type BudgetReq struct {
	BudgetCap int `json:"budgetCap"`
	AlertPct  int `json:"alertPct"`
}

type DefaultBudgetReq struct {
	DefaultBudget int `json:"defaultBudget"`
}

// MemberAllocReq는 그룹 담당자가 멤버에게 그룹 풀에서 배분할 때.
type MemberAllocReq struct {
	UserID int64  `json:"userId" binding:"required"`
	Amount int    `json:"amount" binding:"required"`
	Reason string `json:"reason"`
}
