package resource

import "gorm.io/gorm"

// Repository는 오퍼링/프리셋 영속성 계약.
type Repository interface {
	ListOfferings(activeOnly bool) ([]Offering, error)
	SaveOffering(o *Offering) error
	DeleteOffering(id int64) error

	ListPresets(activeOnly bool) ([]Preset, error)
	SavePreset(p *Preset) error
	DeletePreset(id int64) error

	ListGpuPricing() ([]GpuPrice, error)
	SetGpuPricing(p GpuPrice) error
	GpuTypePrice(gpuType string) int

	TimeslicingNodes() map[string]int // 타임슬라이싱 노드별 슬롯 수(split_count)
}

// GpuPrice는 gpuType별 단가다. 전용(시간)과 HAMi 분할 단위(VRAM GB/시간, 코어%/시간)를 갖는다.
type GpuPrice struct {
	GpuType      string `gorm:"column:gpu_type" json:"gpuType"`
	PricePerHour int    `gorm:"column:price_per_hour" json:"pricePerHour"` // 전용 1 GPU/시간
	PricePerGB   int    `gorm:"column:price_per_gb" json:"pricePerGb"`     // 분할 VRAM 1GB/시간
	PricePerCore int    `gorm:"column:price_per_core" json:"pricePerCore"` // 분할 코어 1%/시간
}

func (GpuPrice) TableName() string { return "gpu_pricing" }

type gormRepo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

func (r *gormRepo) ListOfferings(activeOnly bool) ([]Offering, error) {
	var out []Offering
	q := r.db.Order("id")
	if activeOnly {
		q = q.Where("is_active = ?", true)
	}
	return out, q.Find(&out).Error
}

// SaveOffering은 id 유무로 생성/수정(upsert)한다.
// Omit("created_at"): PUT 본문엔 created_at 가 없어(JSON 비노출) Save 가 0000-00-00 으로 덮어써
// MySQL strict 모드가 거부(Error 1292)한다. 생성=DB default, 수정=기존값 유지 위해 항상 제외.
func (r *gormRepo) SaveOffering(o *Offering) error { return r.db.Omit("created_at").Save(o).Error }

func (r *gormRepo) DeleteOffering(id int64) error { return r.db.Delete(&Offering{}, id).Error }

func (r *gormRepo) ListPresets(activeOnly bool) ([]Preset, error) {
	var out []Preset
	q := r.db.Order("sort_order, id")
	if activeOnly {
		q = q.Where("is_active = ?", true)
	}
	return out, q.Find(&out).Error
}

func (r *gormRepo) SavePreset(p *Preset) error { return r.db.Omit("created_at").Save(p).Error }

func (r *gormRepo) DeletePreset(id int64) error { return r.db.Delete(&Preset{}, id).Error }

func (r *gormRepo) ListGpuPricing() ([]GpuPrice, error) {
	var out []GpuPrice
	return out, r.db.Order("gpu_type").Find(&out).Error
}

// SetGpuPricing은 gpuType 단가(전용/GB/코어)를 멱등 upsert 한다.
func (r *gormRepo) SetGpuPricing(p GpuPrice) error {
	return r.db.Exec(`INSERT INTO gpu_pricing (gpu_type, price_per_hour, price_per_gb, price_per_core) VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE price_per_hour = VALUES(price_per_hour), price_per_gb = VALUES(price_per_gb), price_per_core = VALUES(price_per_core)`,
		p.GpuType, p.PricePerHour, p.PricePerGB, p.PricePerCore).Error
}

// TimeslicingNodes는 타임슬라이싱으로 설정된 노드와 그 슬롯 수를 반환한다(없으면 빈 맵).
// GPU 타입별 타임셰어 가능 여부는 이 목록을 라이브 노드(타입 라벨)와 조인해 판정한다.
func (r *gormRepo) TimeslicingNodes() map[string]int {
	out := map[string]int{}
	var rows []struct {
		Name       string
		SplitCount int
	}
	r.db.Raw(`SELECT name, split_count FROM nodes WHERE share_mode = 'timeslicing'`).Scan(&rows)
	for _, x := range rows {
		n := x.SplitCount
		if n < 1 {
			n = 1
		}
		out[x.Name] = n
	}
	return out
}

func (r *gormRepo) GpuTypePrice(gpuType string) int {
	var p int
	r.db.Raw(`SELECT price_per_hour FROM gpu_pricing WHERE gpu_type = ?`, gpuType).Scan(&p)
	return p
}
