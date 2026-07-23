// Package billing은 크레딧 소비 showback(그룹/사용자/조직 집계)을 제공한다.
//
// 소비 원천: memberships.consumed(사용자/그룹/조직 모두 멤버 누적 합), gpuHours 는 gpu_usage 원장(초→시간).
// 조직 소비도 산하 그룹 멤버 consumed 합으로 일관화. credit 모드 전용 화면(프론트가 모드로 노출 제어).
package billing

// Showback은 빌링 화면 전체 응답.
type Showback struct {
	TotalConsumed int        `json:"totalConsumed"`
	TotalBudget   int        `json:"totalBudget"`
	ByGroup       []GroupRow `json:"byGroup"`
	ByUser        []UserRow  `json:"byUser"`
	ByOrg         []OrgRow   `json:"byOrg"`
}

type GroupRow struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	OrgName   string `json:"orgName"`
	Sessions  int    `json:"sessions"`
	GpuHours  float64 `json:"gpuHours"`
	Consumed  int    `json:"consumed"`
	BudgetCap int    `json:"budgetCap"`
	UsagePct  int    `json:"usagePct"`
}

type UserRow struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Group    string `json:"group"`
	Sessions int    `json:"sessions"`
	GpuHours  float64 `json:"gpuHours"`
	Consumed int    `json:"consumed"`
}

type OrgRow struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Groups     int    `json:"groups"`
	Users      int    `json:"users"`
	GpuHours  float64 `json:"gpuHours"`
	Consumed   int    `json:"consumed"`
	CreditPool int    `json:"creditPool"`
}
