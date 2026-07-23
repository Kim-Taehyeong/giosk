package resource

import "time"

// Offering은 offerings 테이블 매핑(GPU 슬라이스 카탈로그).
type Offering struct {
	ID           int64     `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"column:name" json:"name"`
	GpuType      string    `gorm:"column:gpu_type" json:"gpuType"`
	VramMB       int       `gorm:"column:vram_mb" json:"vramMb"`
	CorePercent  int       `gorm:"column:core_percent" json:"corePercent"`
	Mode         string    `gorm:"column:mode" json:"mode"`
	PricePerHour int       `gorm:"column:price_per_hour" json:"pricePerHour"`
	IsActive     bool      `gorm:"column:is_active" json:"isActive"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"-"`
}

func (Offering) TableName() string { return "offerings" }

// Preset은 presets 테이블 매핑(세션 마법사 프리셋).
type Preset struct {
	ID          int64     `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"column:name" json:"name"`
	DisplayName string    `gorm:"column:display_name" json:"displayName"`
	Icon        string    `gorm:"column:icon" json:"icon"`
	VramMB      int       `gorm:"column:vram_mb" json:"vramMb"`
	CorePercent int       `gorm:"column:core_percent" json:"corePercent"`
	SortOrder   int       `gorm:"column:sort_order" json:"sortOrder"`
	IsActive    bool      `gorm:"column:is_active" json:"isActive"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"-"`
}

func (Preset) TableName() string { return "presets" }
