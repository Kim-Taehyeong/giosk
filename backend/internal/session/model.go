package session

import "time"

// Session은 sessions 테이블 매핑.
type Session struct {
	ID          int64     `gorm:"primaryKey" json:"-"`
	InstanceID  string    `gorm:"column:instance_id" json:"id"`
	UserID      int64     `gorm:"column:user_id" json:"-"`
	GroupID     *int64    `gorm:"column:group_id" json:"groupId,omitempty"`
	Name        string    `gorm:"column:name" json:"name"`
	Env         string    `gorm:"column:env" json:"env"`
	GpuMode     string    `gorm:"column:gpu_mode" json:"gpuMode"`
	OfferingID  *int64    `gorm:"column:offering_id" json:"offeringId,omitempty"`
	ImageID     *int64    `gorm:"column:image_id" json:"imageId,omitempty"`
	VramMB      int       `gorm:"column:vram_mb" json:"vramMb"`
	CorePercent int       `gorm:"column:core_percent" json:"corePercent"`
	CPUCores    int       `gorm:"column:cpu_cores" json:"cpuCores"`
	MemGB       int       `gorm:"column:mem_gb" json:"memGb"`
	GpuCount    int       `gorm:"column:gpu_count" json:"gpuCount"`
	GpuType     string    `gorm:"column:gpu_type" json:"gpuType"`
	Node        string    `gorm:"column:node" json:"node"`
	IPAddress   string    `gorm:"column:ip_address" json:"ip"`
	Phase       string    `gorm:"column:phase" json:"status"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"createdAt"`

	// 소비 회계(크레딧 모드)
	PricePerHour   int        `gorm:"column:price_per_hour" json:"pricePerHour"`
	StartedAt      *time.Time `gorm:"column:started_at" json:"startedAt,omitempty"`
	BilledCredits  int        `gorm:"column:billed_credits" json:"billedCredits"`
	ExtensionsUsed int        `gorm:"column:extensions_used" json:"extensionsUsed"` // 선착순(dynamic) 임대 연장 횟수

	// 웹 접속(code-server/jupyter) 랜덤 시크릿(비밀번호/토큰) — 소유자에게만 노출.
	WebPassword string `gorm:"column:web_password" json:"-"`

	// 제공 접속 채널(웹: vscode/jupyter, 물리: ssh) — 이미지 channels 에서 도출(전송용, 비영속).
	Channels []string `gorm:"-" json:"channels,omitempty"`
}

func (Session) TableName() string { return "sessions" }

// 세션 phase.
const (
	PhaseProvisioning = "provisioning"
	PhaseRunning      = "running"
	PhaseStopped      = "stopped"
	PhaseTerminated   = "terminated"
)

// VolMount — 볼륨 마운트 입력.
type VolMount struct {
	ID        int64  `json:"id"`
	MountPath string `json:"mountPath"`
	Perm      string `json:"perm"`
}

// CreateReq — 세션 생성 요청(프론트 NewSession payload).
type CreateReq struct {
	Name          string     `json:"instancename"`
	Env           string     `json:"env"`          // ""(컨테이너) | "ssh"(물리노드 임대)
	Node          string     `json:"node"`         // ssh: 임대할 물리노드
	SSHPublicKey  string     `json:"sshpublickey"` // ssh: 사용자 공개키(node-agent 주입용)
	GroupID       *int64     `json:"groupId"`
	ImageID       int64      `json:"imageId"`
	OfferingID    *int64     `json:"offeringId"`
	GpuMode       string     `json:"gpuMode"` // shared|exclusive|cpu
	GpuType       string     `json:"gpuType"`
	GpuCount      int        `json:"gpuCount"`
	VramMB        int        `json:"vramMb"`
	CorePercent   int        `json:"corePercent"`
	CpuCores      int        `json:"cpuCores"`
	MemGB         int        `json:"memGb"`
	PricePerHour  int        `json:"pricePerHour"` // 시간당 크레딧(과금 기준; 프론트 계산값)
	Volumes       []VolMount `json:"volumes"`
	Datasets      []int64    `json:"datasets"`
	LocalHomeNode string     `json:"localHomeNode"` // 선택 시 그 물리노드 로컬 디스크 home 을 /home/work 로 hostPath 마운트 + 노드 핀(기본은 emptyDir 로컬 home)
}

// Connection — 연결정보(게이트웨이 URL/토큰).
type Connection struct {
	VSCode  map[string]string `json:"vscode,omitempty"`
	Jupyter map[string]string `json:"jupyter,omitempty"`
	SSH     map[string]string `json:"ssh,omitempty"`
	Web     map[string]string `json:"web,omitempty"` // 커스텀 포트 웹 채널(제네릭 앱). 비번 없음.
}

// AccessInfo — 게이트웨이 단기 접속 정보(토큰 포함 URL·SSH 명령). 열 때마다 새로 발급.
type AccessInfo struct {
	VSCode    map[string]string `json:"vscode,omitempty"`
	Jupyter   map[string]string `json:"jupyter,omitempty"`
	SSH       map[string]string `json:"ssh,omitempty"`
	Web       map[string]string `json:"web,omitempty"` // 커스텀 포트 웹 채널
	ExpiresAt time.Time         `json:"expiresAt"`
}
