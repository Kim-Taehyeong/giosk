package dashboard

import (
	"time"

	"giosk/internal/alertlog"
)

// ActiveUser — 현재 실행 세션을 가진 사용자(운영 대시보드 '사용 중 유저').
type ActiveUser struct {
	Name     string `json:"name"`
	Sessions int    `json:"sessions"`
	GpuType  string `json:"gpuType"` // 대표 GPU 모델(여러 개면 하나)
}

// SessionStats — 세션 상태 분해(감시 대시보드 공통). 실행/유휴/중단 비율.
type SessionStats struct {
	Running      int            `json:"running"`
	Idle         int            `json:"idle"`         // running 이지만 저사용(GPU util<5%)
	Provisioning int            `json:"provisioning"` // 준비 중
	Stopped      int            `json:"stopped"`      // 정지(유휴리퍼/수동/만료)
	ByMode       map[string]int `json:"byMode"`       // shared/exclusive/cpu running 수
	ByGpuType    map[string]int `json:"byGpuType"`    // gpu_type → running 수
}

// ── 사용자 대시보드 ───────────────────────────
type UserDashboard struct {
	KPIs     UserKPIs      `json:"kpis"`
	Regions  []Region      `json:"regions"`
	Sessions []SessionCard `json:"sessions"`
}

// SessionCard — 대시보드 최근 세션 카드.
type SessionCard struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Kind   string   `json:"kind"`
	Spec   string   `json:"spec"`
	Credit int      `json:"credit"`
	Conn   []string `json:"conn"`
	Status string   `json:"status"`
}

type UserKPIs struct {
	Balance        int `json:"balance"`
	Cap            int `json:"cap"`
	ActiveSessions int `json:"activeSessions"`
	MaxSessions    int `json:"maxSessions"`
	VramUsed       int `json:"vramUsed"`
	VramTotal      int `json:"vramTotal"`
	GpuHoursMonth  int `json:"gpuHoursMonth"`
	EtaDays        int `json:"etaDays"`
	Burn           int `json:"burn"`
}

// Region — 자원군별 가용량(드롭다운/가용 표시).
type Region struct {
	Name  string `json:"name"`
	Free  int    `json:"free"`
	Total int    `json:"total"`
	Queue int    `json:"queue"`
}

// ── 관리자(인프라) 대시보드 ───────────────────────────
type AdminDashboard struct {
	KPIs         AdminKPIs        `json:"kpis"`
	CreditTrend  []TrendPoint     `json:"creditTrend"`
	Alerts       []AdminAlert     `json:"alerts"`      // 현재 상태 경보(노드 NotReady/cordon 등, 라이브)
	AlertFeed    []alertlog.Event `json:"alertFeed"`   // 발화 경고 이력(시간순, 감시월 통합 피드)
	TopGroups    []NameCredit     `json:"topGroups"`
	TopUsers     []NameCredit     `json:"topUsers"`
	GpuTrend7d   []TrendPoint     `json:"gpuTrend7d"`  // 폴백(Prometheus range) — 스냅샷 없을 때
	Snapshots    []Snapshot       `json:"snapshots"`   // 최근 24h 인프라 스냅샷(우리 DB, 감시 트렌드)
	SessionStats SessionStats     `json:"sessionStats"`
	ActiveUsers  []ActiveUser     `json:"activeUsers"` // 현재 실행 세션 보유 사용자
}

type AdminKPIs struct {
	GpuUtil        int `json:"gpuUtil"`
	VramAlloc      int `json:"vramAlloc"`
	GpuTempAvg     int `json:"gpuTempAvg"`
	GpuTempMax     int `json:"gpuTempMax"`
	GpusUsed       int `json:"gpusUsed"`
	GpusTotal      int `json:"gpusTotal"`
	ActiveSessions int `json:"activeSessions"`
	MaxSessions    int `json:"maxSessions"`
	Queue          int `json:"queue"`
	MonthCredit    int `json:"monthCredit"`
	NodesUp        int `json:"nodesUp"`
	NodesTotal     int `json:"nodesTotal"`
	HealthAlerts   int `json:"healthAlerts"`
	BudgetRisk     int `json:"budgetRisk"`
}

// TrendPoint — 추이 1점(creditTrend=amount, gpuTrend=util 둘 다 수용).
type TrendPoint struct {
	Date   string `json:"date"`
	Util   int    `json:"util,omitempty"`
	Amount int    `json:"amount,omitempty"`
}

type AdminAlert struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Age    string `json:"age"`
}

// ── 운영 대시보드(사용·거버넌스) — 레벨 스코프 ────────────────
// 인프라(GPU/노드)는 기존 Admin() 재사용(platform 전용). 운영은 스코프로 필터해 전 관리 레벨이 본다.
type OpsDashboard struct {
	BillingMode  string           `json:"billingMode"` // credit | dynamic | free — 프론트 위젯 게이팅
	KPIs         OpsKPIs          `json:"kpis"`
	SessionStats SessionStats     `json:"sessionStats"` // 전 모드 공통(실행/유휴/중단·모드별)
	GpuHours     int              `json:"gpuHours"`     // 이번달 GPU 사용시간(전 모드 공통)
	CreditTrend  []TrendPoint     `json:"creditTrend"`  // credit 모드만 채움
	TopGroups    []NameCredit     `json:"topGroups"`    // credit 모드만
	TopUsers     []NameCredit     `json:"topUsers"`     // credit 모드만
	ActiveUsers  []ActiveUser     `json:"activeUsers"`  // 현재 실행 세션 보유 사용자(전 모드)
	AlertFeed    []alertlog.Event `json:"alertFeed"`    // 통합 경고 피드(최근)
}

type OpsKPIs struct {
	ActiveSessions int `json:"activeSessions"`
	MonthCredit    int `json:"monthCredit"` // 이번달 소비(스코프 내, credit 모드만 의미)
	BudgetRisk     int `json:"budgetRisk"`  // 풀 소진 그룹 수(스코프 내, credit 모드만)
}

// Snapshot은 metric_snapshots 한 행 — 인프라 시계열 샘플.
type Snapshot struct {
	ID              int64     `json:"-" gorm:"column:id"`
	TS              time.Time `json:"ts" gorm:"column:ts"`
	GpuUtil         int       `json:"gpuUtil" gorm:"column:gpu_util"`
	VramUsedPct     int       `json:"vramUsedPct" gorm:"column:vram_used_pct"`
	GpuTempAvg      int       `json:"gpuTempAvg" gorm:"column:gpu_temp_avg"`
	GpuTempMax      int       `json:"gpuTempMax" gorm:"column:gpu_temp_max"`
	NodesUp         int       `json:"nodesUp" gorm:"column:nodes_up"`
	NodesTotal      int       `json:"nodesTotal" gorm:"column:nodes_total"`
	GpusUsed        int       `json:"gpusUsed" gorm:"column:gpus_used"`
	GpusTotal       int       `json:"gpusTotal" gorm:"column:gpus_total"`
	RunningSessions int       `json:"runningSessions" gorm:"column:running_sessions"`
	IdleSessions    int       `json:"idleSessions" gorm:"column:idle_sessions"`
}

func (Snapshot) TableName() string { return "metric_snapshots" }
