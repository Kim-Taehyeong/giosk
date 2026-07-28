package org

import "time"

// Organization은 organizations 테이블 매핑.
type Organization struct {
	ID                 int64     `gorm:"primaryKey" json:"id"`
	Name               string    `gorm:"column:name" json:"name"`
	DisplayName        string    `gorm:"column:display_name" json:"displayName"`
	Description        string    `gorm:"column:description" json:"description"`
	CreditPool         int       `gorm:"column:credit_pool" json:"creditPool"`
	RecurringCredit    int       `gorm:"column:recurring_credit" json:"recurringCredit"`        // 매 주기 재생성될 정기 크레딧(0=없음)
	RefillIntervalDays int       `gorm:"column:refill_interval_days" json:"refillIntervalDays"` // 리필 주기(일). 0=플랫폼 기본
	Carryover          bool      `gorm:"column:carryover" json:"carryover"`                     // 이월 여부
	DefaultGroupBudget int       `gorm:"column:default_group_budget" json:"defaultGroupBudget"`
	Status             string    `gorm:"column:status" json:"status"`
	CreatedAt          time.Time `gorm:"column:created_at" json:"createdAt"`
}

func (Organization) TableName() string { return "organizations" }

// Summary는 관리자 목록용(집계 카운트 포함).
type Summary struct {
	Organization
	GroupCount int `json:"groupCount"`
	UserCount  int `json:"userCount"`
}

// PublicGroup / PublicOrg — 가입 폼용 공개 카탈로그(민감정보 제외).
type PublicGroup struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

type PublicOrg struct {
	ID          int64         `json:"id"`
	Name        string        `json:"name"`
	DisplayName string        `json:"displayName"`
	Groups      []PublicGroup `json:"groups"`
}

// Overview — 조직 관리자 대시보드 집계.
type Overview struct {
	OrgName            string        `json:"orgName"`
	Pool               int           `json:"pool"`
	Consumed           int           `json:"consumed"`
	GroupCount         int           `json:"groupCount"`
	MemberCount        int           `json:"memberCount"`
	PendingReq         int           `json:"pendingReq"`
	DefaultGroupBudget int           `json:"defaultGroupBudget"`
	TopGroups          []GroupCredit `gorm:"-" json:"topGroups"`
}

type GroupCredit struct {
	Name   string `json:"name"`
	Credit int    `json:"credit"`
}

// GroupBudget — 조직 내 그룹 목록 행.
type GroupBudget struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Status      string `json:"status"`
	MemberCount int    `json:"memberCount"`
	Balance     int    `json:"balance"`
	BudgetCap   int    `json:"budgetCap"`
}

// CreateReq / GrantReq — 요청 바디.
type CreateReq struct {
	Name         string `json:"name" binding:"required"`
	DisplayName  string `json:"displayName"`
	CreditPool   int    `json:"creditPool"`
	AdminAccount string `json:"adminAccount"` // 조직 관리자(org_admin)로 지정할 계정. 지정 시 기본 그룹 자동생성 후 배정.
}

type GrantReq struct {
	Amount    int    `json:"amount"`              // 일회성 부여(즉시 풀에 가산)
	Recurring *int   `json:"recurring,omitempty"` // 정기 크레딧 설정(매 주기 재생성될 양). nil=변경 안 함
	Interval  *int   `json:"interval,omitempty"`  // 리필 주기(일). 플랫폼 기본보다 길 수 없다(캡). nil=변경 안 함
	Carryover *bool  `json:"carryover,omitempty"` // 이월 on/off. nil=변경 안 함
	Reason    string `json:"reason"`
}

type UpdateReq struct {
	DisplayName        *string `json:"displayName"`
	DefaultGroupBudget *int    `json:"defaultGroupBudget"`
}
