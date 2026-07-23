package image

import "gorm.io/gorm"

// Repository는 image 영속성 계약.
type Repository interface {
	List(activeOnly bool) ([]Image, error)
	Save(img *Image) error
	Create(img *Image) error // 신규 삽입(img.ID 채움)
	Get(id int64) (*Image, error)
	ListBuilding() ([]Image, error)                     // status=building (빌드 리컨실 대상)
	SetBuildResult(id int64, status, scan string) error // 빌드 종료 상태 반영
	SetScanStatus(id int64, scan string) error          // 파이프라인 단계(scanning/signing/pass/high) 표시
	SetSigned(id int64, signed bool) error
	SetStatus(id int64, status string) error
	Delete(id int64) error

	CacheSet(imageID int64, node, status string) error // 노드 캐시 상태 upsert
	CacheDelete(imageID int64, node string) error
	CacheByImage(imageID int64) []NodeCache // 이미지의 노드별 캐시 상태
	CachePulling() []NodeCache              // status=pulling (리컨실 대상, ImageID 포함)
	CachedNodesMap() map[int64][]string     // image_id → 캐시 완료(cached) 노드 목록(빠른 생성 배지)
}

// NodeCache — 이미지 노드 캐시 1건.
type NodeCache struct {
	ImageID int64  `gorm:"column:image_id" json:"-"`
	Node    string `gorm:"column:node" json:"node"`
	Status  string `gorm:"column:status" json:"status"`
}

type gormRepo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

func (r *gormRepo) List(activeOnly bool) ([]Image, error) {
	var out []Image
	q := r.db.Order("id")
	if activeOnly {
		q = q.Where("status = ?", StatusActive)
	}
	return out, q.Find(&out).Error
}

// Omit("created_at"): Save 가 0값 created_at 으로 덮어쓰면 MySQL strict 모드가 거부(1292). 생성=DB default, 수정=유지.
func (r *gormRepo) Save(img *Image) error { return r.db.Omit("created_at").Save(img).Error }

func (r *gormRepo) Create(img *Image) error { return r.db.Create(img).Error }

func (r *gormRepo) Get(id int64) (*Image, error) {
	var img Image
	return &img, r.db.Where("id = ?", id).First(&img).Error
}

func (r *gormRepo) ListBuilding() ([]Image, error) {
	var out []Image
	return out, r.db.Where("status = ?", StatusBuilding).Find(&out).Error
}

func (r *gormRepo) SetBuildResult(id int64, status, scan string) error {
	return r.db.Model(&Image{}).Where("id = ?", id).
		Updates(map[string]any{"status": status, "scan_status": scan}).Error
}

func (r *gormRepo) SetScanStatus(id int64, scan string) error {
	return r.db.Model(&Image{}).Where("id = ?", id).Update("scan_status", scan).Error
}

func (r *gormRepo) SetSigned(id int64, signed bool) error {
	return r.db.Model(&Image{}).Where("id = ?", id).Update("signed", signed).Error
}

func (r *gormRepo) SetStatus(id int64, status string) error {
	return r.db.Model(&Image{}).Where("id = ?", id).Update("status", status).Error
}

func (r *gormRepo) Delete(id int64) error { return r.db.Delete(&Image{}, id).Error }

func (r *gormRepo) CacheSet(imageID int64, node, status string) error {
	return r.db.Exec(`INSERT INTO image_node_cache (image_id, node, status) VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE status = VALUES(status)`, imageID, node, status).Error
}

func (r *gormRepo) CacheDelete(imageID int64, node string) error {
	return r.db.Exec(`DELETE FROM image_node_cache WHERE image_id = ? AND node = ?`, imageID, node).Error
}

func (r *gormRepo) CacheByImage(imageID int64) []NodeCache {
	var out []NodeCache
	r.db.Raw(`SELECT image_id, node, status FROM image_node_cache WHERE image_id = ? ORDER BY node`, imageID).Scan(&out)
	return out
}

func (r *gormRepo) CachePulling() []NodeCache {
	var out []NodeCache
	r.db.Raw(`SELECT image_id, node, status FROM image_node_cache WHERE status = 'pulling'`).Scan(&out)
	return out
}

// CachedNodesMap은 캐시 완료(cached) 상태의 image_id → 노드 목록을 반환한다(빠른 생성 배지).
func (r *gormRepo) CachedNodesMap() map[int64][]string {
	var rows []NodeCache
	r.db.Raw(`SELECT image_id, node, status FROM image_node_cache WHERE status = 'cached' ORDER BY node`).Scan(&rows)
	out := map[int64][]string{}
	for _, c := range rows {
		out[c.ImageID] = append(out[c.ImageID], c.Node)
	}
	return out
}
