package node

import "time"

// Config는 nodes 테이블(라이브 K8s 노드에 대한 DB 오버레이).
type Config struct {
	Name           string `gorm:"primaryKey;column:name" json:"name"`
	Cluster        string `gorm:"column:cluster" json:"cluster"`
	ShareMode      string `gorm:"column:share_mode" json:"shareMode"`
	SplitCount     int    `gorm:"column:split_count" json:"splitCount"`
	DatasetQuotaGB int    `gorm:"column:dataset_quota_gb" json:"datasetQuotaGb"`
	SSHEnabled     bool   `gorm:"column:ssh_enabled" json:"sshEnabled"`
	PhysicalLease  bool   `gorm:"column:physical_lease" json:"physicalLease"`
	ScratchEnabled bool   `gorm:"column:scratch_enabled" json:"scratchEnabled"`
	// 물리노드는 독점 전용(대여자 1인, cordon으로 강제). 로컬홈 용량은 ~/nfs 개인볼륨 쿼터로 통합.
	// temp_invites·local_accounts·home_quota_gb 스텁은 폐기(설계 정립).
}

func (Config) TableName() string { return "nodes" }

// Lease는 node_leases 테이블 매핑.
type Lease struct {
	ID             int64      `gorm:"primaryKey" json:"id"`
	Node           string     `gorm:"column:node" json:"node"`
	UserID         int64      `gorm:"column:user_id" json:"userId"`
	InstanceID     string     `gorm:"column:instance_id" json:"instanceId"`
	Status         string     `gorm:"column:status" json:"status"`
	Shared         bool       `gorm:"column:shared" json:"-"` // 자유=공유(노드당 사용자별 1건), 비-자유=전용(노드당 1건). lease_key 유니크로 강제
	LeaseHours     int        `gorm:"column:lease_hours" json:"leaseHours"`
	ExtensionsUsed int        `gorm:"column:extensions_used" json:"extensionsUsed"`
	StartedAt      time.Time  `gorm:"column:started_at" json:"startedAt"`
	ExpiresAt      time.Time  `gorm:"column:expires_at" json:"expiresAt"`
	ReleasedAt     *time.Time `gorm:"column:released_at" json:"-"`
}

func (Lease) TableName() string { return "node_leases" }

const (
	LeaseActive   = "active"
	LeaseExpired  = "expired"
	LeaseReleased = "released"
)

// 노드 GPU 공유 전략(share_mode) — 노드당 하나만 선택(상호 배타).
//   exclusive   : GPU 카드를 통째로 점유.
//   hami        : HAMi 분할 — VRAM(GB)+코어(%) 단위, 메모리 격리 있음.
//   timeslicing : NVIDIA device plugin 타임슬라이싱 — GPU 1개를 split_count 슬롯으로 광고.
//                 메모리 격리 없음(한 파드 OOM 이 같은 GPU 의 다른 파드에 전파) → HAMi 미설치 노드용 대안.
const (
	ShareExclusive   = "exclusive"
	ShareHami        = "hami"
	ShareTimeslicing = "timeslicing"
)

// ShareModeLabel은 노드의 공유 전략을 나타내는 K8s 라벨(세션 배치 steering 용).
const ShareModeLabel = "giosk.io/share-mode"

// View는 라이브(K8s) + 오버레이(DB) + 라이브 지표 + 임대를 머지한 관리자 노드 뷰.
type View struct {
	Name        string `json:"name"`
	GpuType     string `json:"gpuType"`
	GpuCapacity string `json:"gpuCapacity"`
	CudaVersion string `json:"cudaVersion"` // 노드 CUDA(드라이버) 버전(GFD 라벨). 없으면 빈 문자열.
	Ready       bool   `json:"ready"`
	Cordoned    bool   `json:"cordoned"`
	Physical    bool   `json:"physical"`
	ShareMode   string `json:"shareMode"`
	SplitCount  int    `json:"splitCount"`
	SSHEnabled  bool   `json:"sshEnabled"`
	LeasedTo    string `json:"leasedTo,omitempty"`

	// 사양(K8s capacity)
	CPUCores int `json:"cpuCores"`
	MemGB    int `json:"memGb"`

	// 라이브 모니터링(DCGM/Prometheus, 미연동 시 0)
	Util      int `json:"util"`      // GPU 평균 사용률 %
	VramUsed  int `json:"vramUsed"`  // VRAM 사용 GB(노드 합)
	VramTotal int `json:"vramTotal"` // VRAM 총 GB(노드 합)
	Temp      int `json:"temp"`      // GPU 최고 온도 ℃
	CpuUtil   int `json:"cpuUtil"`   // CPU 사용률 %
	MemUtil   int `json:"memUtil"`   // 메모리 사용률 %
	Sessions  int `json:"sessions"`  // running 세션 수

	// DB 오버레이(설정)
	DatasetQuotaGB int  `json:"datasetQuotaGb"`
	PhysicalLease  bool `json:"physicalLease"`
	ScratchEnabled bool `json:"scratchEnabled"`
	Hami           bool `json:"hami"`
}

// UserNodeView는 사용자 화면용 물리 노드(사양/쿼터/가용). 캐시 데이터셋·실사용량은
// node-agent 가 보고하기 전까지 0/빈 값(점진적 채움).
type UserNodeView struct {
	Node           string          `json:"node"`
	Gpu            string          `json:"gpu"`
	Cpu            string          `json:"cpu"`
	Mem            string          `json:"mem"`
	Disk           string          `json:"disk"`
	Available      bool            `json:"available"`
	GpuTotal       int             `json:"gpuTotal"`
	GpuFree        int             `json:"gpuFree"`
	HomeUsedGb     int             `json:"homeUsedGb"`
	DatasetUsedGb  int             `json:"datasetUsedGb"`
	DatasetQuotaGb int             `json:"datasetQuotaGb"`
	Cached         []CachedDataset `json:"cached"`
}

// CachedDataset — 노드 로컬 캐시 데이터셋(agent 보고 전까지 빈 목록).
type CachedDataset struct {
	Name      string `json:"name"`
	SizeClass string `json:"sizeClass"`
	SizeGb    int    `json:"sizeGb"`
	Format    string `json:"format"`
	Files     string `json:"files"`
	UpdatedAt string `json:"updatedAt"`
	Owner     string `json:"owner"`
	Desc      string `json:"desc"`
	Path      string `json:"path"`
}

// AgentLease는 node-agent 가 reconcile 할 임대 1건(계정/마운트 정보 포함).
// repo.AgentLeases 는 스칼라 컬럼만 Scan 하고, 나머지(NFS/마운트/스크래치)는 service 가 채운다.
type AgentLease struct {
	InstanceID   string          `gorm:"column:instance_id" json:"-"` // 볼륨 조회용 내부 키(agent 전송 제외)
	LeaseID      int64           `gorm:"column:lease_id" json:"leaseId"`
	UserID       int64           `gorm:"column:user_id" json:"-"` // UID 산출용(agent 전송 제외)
	Username     string          `gorm:"column:username" json:"username"`
	UID          int             `gorm:"-" json:"uid"` // 전역 안정 UID(uidBase+userID; 재사용 안 함) — useradd -u
	SSHPublicKey string          `gorm:"column:ssh_public_key" json:"sshPublicKey"`
	NFSServer    string          `gorm:"-" json:"nfsServer"`
	NFSPath      string          `gorm:"-" json:"nfsPath"`
	MountPath    string          `gorm:"-" json:"mountPath"`
	Volumes      []AgentVolMount `gorm:"-" json:"volumes,omitempty"` // 추가/공유 볼륨(NFS 직접 마운트, RW/RO)
	Scratch      string          `gorm:"-" json:"scratch,omitempty"` // 노드로컬 스크래치 계정폴더(빈값=비활성)
}

// AgentVolMount는 물리노드에 직접 NFS 마운트할 볼륨 1건.
type AgentVolMount struct {
	NFSServer string `json:"nfsServer"`
	NFSPath   string `json:"nfsPath"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly"`
}

// 요청 바디.
type ConfigReq struct {
	ShareMode      *string `json:"shareMode"`
	SplitCount     *int    `json:"splitCount"`
	SSHEnabled     *bool   `json:"sshEnabled"`
	PhysicalLease  *bool   `json:"physicalLease"`
	ScratchEnabled *bool   `json:"scratchEnabled"`
	DatasetQuotaGB *int    `json:"datasetQuotaGb"`
}

type LeaseReq struct {
	UserID     int64 `json:"userId" binding:"required"`
	LeaseHours int   `json:"leaseHours" binding:"required"`
}
