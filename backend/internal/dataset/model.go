package dataset

import "time"

// Dataset은 datasets 테이블 매핑.
type Dataset struct {
	ID           int64     `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"column:name" json:"name"`
	Scope        string    `gorm:"column:scope" json:"scope"`
	Owner        string    `gorm:"column:owner" json:"owner"`
	OwnerUserID  *int64    `gorm:"column:owner_user_id" json:"-"`
	SizeClass    string    `gorm:"column:size_class" json:"sizeClass"`
	SizeGB       int       `gorm:"column:size_gb" json:"sizeGb"`
	SizeBytes    int64     `gorm:"column:size_bytes" json:"sizeBytes"` // 실제 용량(소스 URL Content-Length)
	Hash         string    `gorm:"column:hash" json:"hash"`
	Format       string    `gorm:"column:format" json:"format"`
	Description  string    `gorm:"column:description" json:"desc"`
	Status       string    `gorm:"column:status" json:"status"`
	LoadStatus   string    `gorm:"column:load_status" json:"loadStatus"` // loading|ready|failed (NFS 정규경로 적재 상태)
	SourceURL    string    `gorm:"column:source_url" json:"-"` // 승인 시 NFS /dataset/<name> 로 적재할 원본 URL(빈값=빈 디렉터리)
	PVCName      string    `gorm:"column:pvc_name" json:"-"`    // RWX NFS 실체(세션 RO 마운트 원천)
	PVCNamespace string    `gorm:"column:pvc_namespace" json:"-"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"-"`
}

func (Dataset) TableName() string { return "datasets" }

// Request는 dataset_requests 테이블 매핑.
type Request struct {
	ID              int64      `gorm:"primaryKey" json:"id"`
	Name            string     `gorm:"column:name" json:"name"`
	RequesterUserID int64      `gorm:"column:requester_user_id" json:"-"`
	SizeClass       string     `gorm:"column:size_class" json:"sizeClass"`
	SizeGB          int        `gorm:"column:size_gb" json:"sizeGb"`
	SizeBytes       int64      `gorm:"column:size_bytes" json:"sizeBytes"`
	Hash            string     `gorm:"column:hash" json:"hash"`
	SourceURL       string     `gorm:"column:source_url" json:"sourceUrl"`
	TargetScope     string     `gorm:"column:target_scope" json:"targetScope"`
	Status          string     `gorm:"column:status" json:"status"`
	ReviewerUserID  *int64     `gorm:"column:reviewer_user_id" json:"-"`
	CreatedAt       time.Time  `gorm:"column:created_at" json:"createdAt"`
	ReviewedAt      *time.Time `gorm:"column:reviewed_at" json:"-"`
}

func (Request) TableName() string { return "dataset_requests" }

// GlobalItem은 전역 레지스트리 표시(노드 로컬 캐시 상태 포함).
type GlobalItem struct {
	Dataset
	Nodes    []string       `gorm:"-" json:"nodes"`    // 캐시 완료(cached) 노드 이름(노드별 매칭/개수)
	Caches   []DatasetCache `gorm:"-" json:"caches"`   // 노드별 캐시 상태(caching/cached/failed). 토글과 진행 표시에 쓴다
	Progress   int    `gorm:"-" json:"progress"`   // 현재 단계 진행률 %(loading 일 때만)
	Phase      string `gorm:"-" json:"phase"`      // download 나 extract (loading 세부 단계). 프론트 라벨용
	EtaSec     int    `gorm:"-" json:"etaSec"`     // 예상 남은 시간(초). 속도 미측정이면 0
	Downloaded int64  `gorm:"-" json:"downloaded"` // 현재까지 받은 바이트(loading)
}

// DatasetCache는 노드 로컬 캐시 1건(노드, 상태, 진행률).
type DatasetCache struct {
	Node     string `json:"node"`
	Status   string `json:"status"`   // caching|cached|failed
	Progress int    `json:"progress"` // 현재 단계 진행률 %(caching 일 때만)
	Phase    string `json:"phase"`    // copy 나 extract (캐시 세부 단계). 프론트 라벨용
}

// ListRes는 /datasets 응답.
type ListRes struct {
	Global []GlobalItem `json:"global"`
	Mine   []Dataset    `json:"mine"`
}

const (
	ScopeGlobal   = "global"
	ScopePersonal = "personal"

	StatusActive  = "active"
	StatusPending = "pending"
	StatusPrivate = "private"
)

// RegisterReq는 등록 신청.
type RegisterReq struct {
	Name        string `json:"name" binding:"required"`
	SizeClass   string `json:"sizeClass"`
	SizeGB      int    `json:"sizeGb"`
	Hash        string `json:"hash"`
	SourceURL   string `json:"sourceUrl"`   // 승인 시 PVC 로 받을 다운로드 URL(빈값=빈 PVC)
	TargetScope string `json:"targetScope"` // global | personal
}

// CacheReq는 노드 캐시 배치 토글.
type CacheReq struct {
	Node string `json:"node" binding:"required"`
}
