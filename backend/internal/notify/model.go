// Package notify는 알림 규칙/채널(이메일·웹훅)을 scope(user|admin)별로 영속한다.
//
// 규칙이 하나도 없으면 scope 별 기본 규칙을 반환한다(기본값은 백엔드 단일 출처).
// 실제 발송(SMTP/웹훅)은 별도 인프라이며 여기서는 정책 저장/조회만 담당한다.
package notify

const (
	ScopeUser  = "user"
	ScopeAdmin = "admin"
)

// Rule은 notify_rules 행.
type Rule struct {
	ID      int64  `gorm:"primaryKey" json:"id"`
	Scope   string `gorm:"column:scope" json:"-"`
	OwnerID int64  `gorm:"column:owner_id" json:"-"`
	Metric  string `gorm:"column:metric" json:"metric"`
	Op      string `gorm:"column:op" json:"op"`
	Value   int    `gorm:"column:value" json:"value"`
	Channel string `gorm:"column:channel" json:"channel"`
	Enabled bool   `gorm:"column:enabled" json:"on"`
}

func (Rule) TableName() string { return "notify_rules" }

// Channel은 notify_channels 행.
type Channel struct {
	ID      int64  `gorm:"primaryKey" json:"id"`
	Scope   string `gorm:"column:scope" json:"-"`
	OwnerID int64  `gorm:"column:owner_id" json:"-"`
	Kind    string `gorm:"column:kind" json:"kind"`
	Target  string `gorm:"column:target" json:"target"`
}

func (Channel) TableName() string { return "notify_channels" }

// Config는 한 scope/owner 의 알림 설정 전체(프론트 응답).
type Config struct {
	Rules    []Rule   `json:"rules"`
	Emails   []string `json:"emails"`
	Webhooks []string `json:"webhooks"`
}

// PutReq는 설정 전체 교체 요청(프론트가 전체 상태를 보냄).
type PutReq struct {
	Rules    []Rule   `json:"rules"`
	Emails   []string `json:"emails"`
	Webhooks []string `json:"webhooks"`
}

// defaultRules는 규칙이 없을 때 scope 별 기본 정책(백엔드 단일 출처).
func defaultRules(scope string) []Rule {
	if scope == ScopeAdmin {
		// 엔진이 실제 평가하는 메트릭(gpu_util/gpu_temp/node_down/disk_usage)만 기본값으로 제공.
		return []Rule{
			{Metric: "node_down", Op: "gte", Value: 1, Channel: "email", Enabled: true},
			// 정리 DaemonSet 의 Home 임계(92%)보다 낮게 — 파일이 지워지기 전에 먼저 알린다.
			{Metric: "disk_usage", Op: "gte", Value: 85, Channel: "email", Enabled: true},
			// 볼륨(PVC)이 꽉 차기 전에 — 쓰기 실패/세션 중단 예방.
			{Metric: "volume_usage", Op: "gte", Value: 85, Channel: "email", Enabled: true},
			// 클러스터 GPU 가 거의 소진되면 증설/대기열 신호.
			{Metric: "capacity", Op: "gte", Value: 85, Channel: "webhook", Enabled: false},
			// GPU 를 점유만 하고 놀리는 유휴 노드 감지(가장 한가한 노드가 5% 이하).
			{Metric: "node_idle", Op: "lte", Value: 5, Channel: "webhook", Enabled: false},
			{Metric: "gpu_temp", Op: "gte", Value: 82, Channel: "webhook", Enabled: true},
			{Metric: "gpu_util", Op: "gte", Value: 95, Channel: "webhook", Enabled: false},
		}
	}
	return []Rule{
		{Metric: "budget_pct", Op: "gte", Value: 80, Channel: "email", Enabled: true},
		{Metric: "credit_balance", Op: "lte", Value: 50, Channel: "email", Enabled: true},
		{Metric: "session_idle", Op: "gte", Value: 30, Channel: "webhook", Enabled: false},
	}
}
