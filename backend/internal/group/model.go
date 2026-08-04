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

// Summary는 관리자 그룹 목록(집계와 지갑 포함).
type Summary struct {
	Group
	OrgName            string `json:"orgName"`
	MemberCount        int    `json:"memberCount"`
	Balance            int    `json:"balance"`            // group_wallets.balance
	BudgetCap          int    `json:"budgetCap"`          // group_wallets.budget_cap
	AlertPct           int    `json:"alertPct"`           // group_wallets.alert_threshold_pct
	RecurringCredit    int    `json:"recurringCredit"`    // 정기 리필 양
	RefillIntervalDays int    `json:"refillIntervalDays"` // 리필 주기(일)
	Carryover          bool   `json:"carryover"`          // 이월 여부
	Consumed           int    `json:"consumed"`           // 멤버 누적 사용량 합(showback)
}

// Member는 그룹 멤버(memberships 와 users 조인).
type Member struct {
	UserID          int64       `json:"userId"`
	Username        string      `json:"username"`
	Name            string      `json:"name"`
	Email           string      `json:"email"`
	Role            string      `json:"role"`
	Status          string      `json:"status"`
	Budget          int         `json:"budget"`
	Balance         int         `json:"balance"`         // user_wallets.balance(배분된 잔여 크레딧)
	RecurringCredit int         `json:"recurringCredit"` // 이 멤버의 정기 리필 금액(user_wallets.recurring_credit)
	Consumed        int         `json:"consumed"`
	Roles           []RoleBadge `json:"roles" gorm:"-"` // 이 사용자가 가진 전체 관리 역할(멀티롤 배지). gorm:"-" 라 스캔에서 빼고 서비스에서 채운다
}

// RoleBadge는 사용자가 가진 관리 역할 하나다(어느 조직이나 그룹의 무슨 관리자인지). 멀티롤 표시용.
type RoleBadge struct {
	Role      string `json:"role"` // org_admin | project_admin
	GroupID   int64  `json:"groupId"`
	GroupName string `json:"groupName"`
	OrgName   string `json:"orgName"`
}

// JoinRequest는 그룹 관리자 큐(가입 신청과 users 조인).
type JoinRequest struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"userId"`
	Name        string    `json:"name"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	Status      string    `json:"status"`
	RequestedAt time.Time `json:"requestedAt"`
}

// PrimaryMembership은 사용자의 최고 권한 멤버십(콘솔 라우팅용).
type PrimaryMembership struct {
	Role      string `json:"role"`
	GroupID   int64  `json:"groupId"`
	GroupName string `json:"groupName"`
	OrgID     int64  `json:"orgId"`
	OrgName   string `json:"orgName"`
}

// GroupRef는 사용자 소속 그룹(스위처와 컨텍스트용).
type GroupRef struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	OrgID       int64  `json:"orgId"`
	OrgName     string `json:"orgName"`
}

// DirItem은 가입 가능한 그룹 디렉터리.
type DirItem struct {
	GroupRef
	Joined      bool `json:"joined"`
	AcceptsJoin bool `json:"acceptsJoin"`
}

// ShareTargets는 볼륨 공유 대상(내 그룹과 동료 사용자).
type ShareTargets struct {
	Users  []ShareUser `json:"users"`
	Groups []GroupRef  `json:"groups"`
}

// ShareUser는 공유 대상 사용자(username 으로 공유한다).
type ShareUser struct {
	Username string `json:"username"`
	Name     string `json:"name"`
}

// UsageRow는 그룹 사용량(멤버별 소모).
type UsageRow struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Credit   int    `json:"credit"`
}

// UsageTrendPoint는 그룹 일자별 GPU 사용시간(시간).
type UsageTrendPoint struct {
	Date  string `json:"date"`
	Hours int    `json:"hours"`
}

// SessionUsage는 그룹 세션별 GPU 사용시간이다(gpu_usage 원장 기반이라 세션을 지워도 누적이 남는다).
type SessionUsage struct {
	Session  string `json:"session"`
	User     string `json:"user"`
	GpuHours int    `json:"gpuHours"`
}

// MyJoinRequest는 내 가입 신청 내역.
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
	// 생성 시 초기 크레딧/정기 리필(선택). 크레딧 모드에서 팀 지갑을 곧바로 세팅한다.
	InitialCredit int  `json:"initialCredit"` // 조직 풀에서 팀 풀로 즉시 배분(0=생략)
	Recurring     int  `json:"recurring"`     // 정기 리필 금액(0=없음)
	Interval      int  `json:"interval"`      // 리필 주기(일)
	Carryover     bool `json:"carryover"`     // 미사용분 이월 여부
}

type AddMemberReq struct {
	Account string `json:"account" binding:"required"` // username 또는 email
	Role    string `json:"role"`
}

// MoveMemberReq는 그룹 이동 요청(관리자). Role 이 비면 기존 역할을 유지한다.
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
