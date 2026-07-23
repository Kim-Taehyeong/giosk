package image

import (
	"encoding/json"
	"time"
)

// Image는 images 테이블 매핑(관리자 빌드 파이프라인 + 사용자 카탈로그 공용).
// channels/tags 는 JSON 컬럼(raw).
type Image struct {
	ID          int64           `gorm:"primaryKey" json:"id"`
	Name        string          `gorm:"column:name" json:"name"`
	Tag         string          `gorm:"column:tag" json:"tag"`
	BaseImage   string          `gorm:"column:base_image" json:"base"`
	Dockerfile  string          `gorm:"column:dockerfile" json:"dockerfile"`
	Description string          `gorm:"column:description" json:"desc"`
	Channels    json.RawMessage `gorm:"column:channels;type:json" json:"channels"`
	Tags        json.RawMessage `gorm:"column:tags;type:json" json:"tags"`
	GPU         bool            `gorm:"column:gpu" json:"gpu"`
	ScanStatus  string          `gorm:"column:scan_status" json:"scan"`
	Signed      bool            `gorm:"column:signed" json:"sign"`
	Status      string          `gorm:"column:status" json:"status"`
	CreatedAt   time.Time       `gorm:"column:created_at" json:"-"`
	CachedNodes []string        `gorm:"-" json:"cachedNodes,omitempty"` // 캐시 완료(cached) 노드 — 빠른 생성 배지용(핸들러가 채움)
}

func (Image) TableName() string { return "images" }

const (
	StatusDraft    = "draft"
	StatusBuilding = "building"
	StatusActive   = "active"
	StatusFailed   = "failed"
)

// StatusReq — 상태 전이 요청(publish 등).
type StatusReq struct {
	Status string `json:"status" binding:"required"`
}

// BuildReq — 가이드형 이미지 빌드 요청(베이스 + 패키지 → 서버가 Dockerfile 생성).
type BuildReq struct {
	Name     string          `json:"name" binding:"required"`
	Desc     string          `json:"desc"` // 이미지 설명(목록·세션 마법사 표시)
	Base     string          `json:"base" binding:"required"`
	Apt      string          `json:"apt"`      // 공백/콤마 구분 시스템 패키지
	Pip      string          `json:"pip"`      // 공백/콤마 구분 파이썬 패키지
	Channels json.RawMessage `json:"channels"` // {vscode,jupyter,ssh}
	GPU      bool            `json:"gpu"`
}

// ExternalReq — 외부(Docker Hub/외부 레지스트리) 이미지 등록. 빌드 없이 그 ref 를 그대로 pull.
// Dockerfile 이 없으므로 재빌드 불가(외부 원본에 push 권한 없음). channels 로 VSCode/Jupyter/커스텀 웹포트를
// 선언하며, 웹 채널이 하나도 없으면 세션은 SSH 전용이 된다.
type ExternalReq struct {
	Ref      string          `json:"ref" binding:"required"` // 예: codercom/code-server:latest
	Desc     string          `json:"desc"`
	Channels json.RawMessage `json:"channels"` // {vscode,jupyter,ssh, web?:{name,port}}
	GPU      bool            `json:"gpu"`
}
