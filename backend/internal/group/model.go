package group

import "time"

// Group은 groups 테이블 매핑(백틱 테이블명 주의).
type Group struct {
	ID          int64     `gorm:"primaryKey" json:"id"`
	OrgID       int64     `gorm:"column:org_id" json:"orgId"`
	Name        string    `gorm:"column:name" json:"name"`
	DisplayName string    `gorm:"column:display_name" json:"displayName"`
	Cluster     string    `gorm:"column:cluster" json:"cluster"`
	AcceptsJoin bool      `gorm:"column:accepts_join" json:"acceptsJoin"`
	Status      string    `gorm:"column:status" json:"status"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"createdAt"`
}

func (Group) TableName() string { return "`groups`" }

// 멤버십 상태/역할.
const (
	MemberActive  = "active"
	MemberInvited = "invited"
	MemberPending = "pending"
	MemberRemoved = "removed"

	RoleMember       = "member"
	RoleProjectAdmin = "project_admin"
	RoleOrgAdmin     = "org_admin"
)

// Summary — 관리자 그룹 목록(집계 + 지갑 포함).
type Summary struct {
	Group
	OrgName     string `json:"orgName"`
	MemberCount int    `json:"memberCount"`
	Balance            int  `json:"balance"`            // group_wallets.balance
	BudgetCap          int  `json:"budgetCap"`          // group_wallets.budget_cap
	AlertPct           int  `json:"alertPct"`           // group_wallets.alert_threshold_pct
	RecurringCredit    int  `json:"recurringCredit"`    // 정기 리필 양
	RefillIntervalDays int  `json:"refillIntervalDays"` // 리필 주기(일)
	Carryover          bool `json:"carryover"`          // 이월 여부
	Consumed           int  `json:"consumed"`           // 멤버 누적 사용량 합(showback)
}

// Member — 그룹 멤버(memberships ⨝ users).
type Member struct {
	UserID   int64       `json:"userId"`
	Username string      `json:"username"`
	Name     string      `json:"name"`
	Email    string      `json:"email"`
	Role     string      `json:"role"`
	Status   string      `json:"status"`
	Budget   int         `json:"budget"`
	Balance  int         `json:"balance"` // user_wallets.balance(배분된 잔여 크레딧)
	Consumed int         `json:"consumed"`
	Roles    []RoleBadge `json:"roles" gorm:"-"` // 이 사용자가 가진 전체 관리 역할(멀티롤 배지). gorm:"-" — 서비스에서 별도 채움(스캔 제외)
}

// RoleBadge — 사용자가 가진 관리 역할 하나(어느 조직/그룹의 무슨 관리자인지). 멀티롤 표시용.
type RoleBadge struct {
	Role      string `json:"role"` // org_admin | project_admin
	GroupID   int64  `json:"groupId"`
	GroupName string `json:"groupName"`
	OrgName   string `json:"orgName"`
}

// JoinRequest — 그룹 관리자 큐(가입 신청 ⨝ users).
type JoinRequest struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"userId"`
	Name        string    `json:"name"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	Status      string    `json:"status"`
	RequestedAt time.Time `json:"requestedAt"`
}

// PrimaryMembership — 사용자의 최고 권한 멤버십(콘솔 라우팅용).
type PrimaryMembership struct {
	Role      string `json:"role"`
	GroupID   int64  `json:"groupId"`
	GroupName string `json:"groupName"`
	OrgID     int64  `json:"orgId"`
	OrgName   string `json:"orgName"`
}

// GroupRef — 사용자 소속 그룹(스위처/컨텍스트).
type GroupRef struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	OrgID       int64  `json:"orgId"`
	OrgName     string `json:"orgName"`
}

// DirItem — 가입 가능한 그룹 디렉터리.
type DirItem struct {
	GroupRef
	Joined      bool `json:"joined"`
	AcceptsJoin bool `json:"acceptsJoin"`
}

// ShareTargets — 볼륨 공유 대상(내 그룹 + 동료 사용자).
type ShareTargets struct {
	Users  []ShareUser `json:"users"`
	Groups []GroupRef  `json:"groups"`
}

// ShareUser — 공유 대상 사용자(username 으로 공유).
type ShareUser struct {
	Username string `json:"username"`
	Name     string `json:"name"`
}

// UsageRow — 그룹 사용량(멤버별 소모).
type UsageRow struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Credit   int    `json:"credit"`
}

// UsageTrendPoint — 그룹 일자별 GPU 사용시간(시간).
type UsageTrendPoint struct {
	Date  string `json:"date"`
	Hours int    `json:"hours"`
}

// SessionUsage — 그룹 세션별 GPU 사용시간(gpu_usage 원장 기반; 세션 삭제돼도 누적 유지).
type SessionUsage struct {
	Session  string `json:"session"`
	User     string `json:"user"`
	GpuHours int    `json:"gpuHours"`
}

// MyJoinRequest — 내 가입 신청 내역.
type MyJoinRequest struct {
	ID          int64     `json:"id"`
	GroupID     int64     `json:"groupId"`
	GroupName   string    `json:"groupName"`
	OrgName     string    `json:"orgName"`
	Status      string    `json:"status"`
	RequestedAt time.Time `json:"requestedAt"`
}

// 요청 바디.
type CreateReq struct {
	OrgID        int64  `json:"orgId" binding:"required"`
	Name         string `json:"name" binding:"required"`
	DisplayName  string `json:"displayName"`
	AdminAccount string `json:"adminAccount"` // 팀 관리자(project_admin)로 지정할 계정(username/email). 필수 권장.
}

type AddMemberReq struct {
	Account string `json:"account" binding:"required"` // username 또는 email
	Role    string `json:"role"`
}

// MoveMemberReq — 그룹 이동(관리자). Role 이 비면 기존 역할 유지.
type MoveMemberReq struct {
	ToGroupID int64  `json:"toGroupId" binding:"required"`
	Role      string `json:"role"`
}

type RoleReq struct {
	Role string `json:"role" binding:"required"`
}

type JoinPolicyReq struct {
	AcceptsJoin bool `json:"acceptsJoin"`
}
