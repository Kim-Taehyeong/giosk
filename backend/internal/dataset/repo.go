package dataset

import (
	"errors"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("not found")

// ErrNameTaken은 같은 이름의 데이터셋이 이미 있을 때(정규경로 충돌 방지).
var ErrNameTaken = errors.New("dataset name already taken")

// Repository는 dataset 영속성 계약.
type Repository interface {
	GlobalActive() ([]GlobalItem, error)
	Mine(userID int64) ([]Dataset, error)
	Delete(id int64) error

	CreateRequest(r *Request) error
	SetRequestSize(id int64, bytes int64) error // 등록 후 백그라운드 측정 용량 반영
	ListPendingRequests() ([]Request, error)
	GetRequest(id int64) (*Request, error)
	MarkRequest(id, reviewer int64, status string) error
	CreateDataset(d *Dataset) error
	Get(id int64) (*Dataset, error)
	SetPVC(id int64, name, ns string) error      // RWX NFS 실체 PVC 좌표 기록
	SetLoadStatus(id int64, status string) error // loading|ready|failed
	SetHash(id int64, hash string) error         // 해제 잡이 계산한 아카이브 해시 저장
	SetDescription(id int64, desc string) error
	ListLoading() []Dataset                      // 적재중 데이터셋(리컨실러)

	CacheStatus(datasetID int64, node string) (status string, exists bool)
	CacheUpsert(datasetID int64, node, status string) error
	CacheDelete(datasetID int64, node string) error
	ListCaching() []DatasetCacheRow      // 복사중 캐시 행(리컨실러)
	CachedNodesOf(datasetID int64) []string // 캐시 완료(cached) 노드(세션 마운트 판정)
	CacheDeleteAll(datasetID int64) error   // 데이터셋 삭제 시 그 데이터셋의 모든 캐시 행 정리
	NameTaken(name string) bool             // 동일 이름의 데이터셋/대기 신청 존재 여부(정규경로 충돌 방지)
}

type gormRepo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

func (r *gormRepo) GlobalActive() ([]GlobalItem, error) {
	var out []GlobalItem
	if err := r.db.Raw(`SELECT d.* FROM datasets d
		WHERE d.scope = 'global' AND d.status = 'active' ORDER BY d.id`).Scan(&out).Error; err != nil {
		return nil, err
	}
	// dataset_id → 노드 로컬 캐시(상태 포함). Nodes 는 cached 완료분, Caches 는 전체(caching/cached/failed).
	var rows []struct {
		DatasetID int64
		Node      string
		Status    string
	}
	r.db.Raw(`SELECT dataset_id, node, status FROM dataset_node_cache ORDER BY node`).Scan(&rows)
	cached := map[int64][]string{}
	all := map[int64][]DatasetCache{}
	for _, x := range rows {
		all[x.DatasetID] = append(all[x.DatasetID], DatasetCache{Node: x.Node, Status: x.Status})
		if x.Status == "cached" {
			cached[x.DatasetID] = append(cached[x.DatasetID], x.Node)
		}
	}
	for i := range out {
		out[i].Nodes = cached[out[i].ID]
		out[i].Caches = all[out[i].ID]
	}
	return out, nil
}

// Mine — 내 개인 데이터셋 + 내 대기 신청(pending 으로 표시).
func (r *gormRepo) Mine(userID int64) ([]Dataset, error) {
	var out []Dataset
	err := r.db.Raw(`
		SELECT id, name, scope, owner, size_class, size_gb, hash, status, created_at
		FROM datasets WHERE owner_user_id = ?
		UNION ALL
		SELECT id, name, target_scope AS scope, '' AS owner, size_class, size_gb, hash, 'pending' AS status, created_at
		FROM dataset_requests WHERE requester_user_id = ? AND status = 'pending'
		ORDER BY created_at DESC`, userID, userID).Scan(&out).Error
	return out, err
}

func (r *gormRepo) Delete(id int64) error { return r.db.Delete(&Dataset{}, id).Error }

func (r *gormRepo) CreateRequest(req *Request) error { return r.db.Create(req).Error }

func (r *gormRepo) SetRequestSize(id int64, bytes int64) error {
	return r.db.Model(&Request{}).Where("id = ?", id).Update("size_bytes", bytes).Error
}

func (r *gormRepo) ListPendingRequests() ([]Request, error) {
	var out []Request
	return out, r.db.Where("status = ?", StatusPending).Order("id").Find(&out).Error
}

func (r *gormRepo) GetRequest(id int64) (*Request, error) {
	var req Request
	err := r.db.Where("id = ?", id).First(&req).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &req, err
}

func (r *gormRepo) MarkRequest(id, reviewer int64, status string) error {
	return r.db.Exec(`UPDATE dataset_requests SET status=?, reviewer_user_id=?, reviewed_at=NOW() WHERE id=? AND status='pending'`,
		status, reviewer, id).Error
}

func (r *gormRepo) CreateDataset(d *Dataset) error { return r.db.Create(d).Error }

func (r *gormRepo) Get(id int64) (*Dataset, error) {
	var d Dataset
	err := r.db.Where("id = ?", id).First(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &d, err
}

func (r *gormRepo) SetPVC(id int64, name, ns string) error {
	return r.db.Model(&Dataset{}).Where("id = ?", id).
		Updates(map[string]any{"pvc_name": name, "pvc_namespace": ns}).Error
}

func (r *gormRepo) SetHash(id int64, hash string) error {
	if hash == "" {
		return nil
	}
	return r.db.Exec(`UPDATE datasets SET hash = ? WHERE id = ?`, hash, id).Error
}

func (r *gormRepo) SetLoadStatus(id int64, status string) error {
	return r.db.Model(&Dataset{}).Where("id = ?", id).Update("load_status", status).Error
}

func (r *gormRepo) SetDescription(id int64, desc string) error {
	return r.db.Model(&Dataset{}).Where("id = ?", id).Update("description", desc).Error
}

// ListLoading은 적재중(loading) 데이터셋을 반환한다(리컨실러 폴링 대상).
func (r *gormRepo) ListLoading() []Dataset {
	var out []Dataset
	r.db.Where("load_status = ?", "loading").Find(&out)
	return out
}

// CacheStatus는 (dataset,node) 캐시 행의 상태를 반환한다(없으면 exists=false).
func (r *gormRepo) CacheStatus(datasetID int64, node string) (status string, exists bool) {
	var s string
	err := r.db.Raw(`SELECT status FROM dataset_node_cache WHERE dataset_id=? AND node=?`, datasetID, node).Scan(&s).Error
	if err != nil || s == "" {
		return "", false
	}
	return s, true
}

// CacheUpsert는 (dataset,node) 캐시 상태를 upsert 한다.
func (r *gormRepo) CacheUpsert(datasetID int64, node, status string) error {
	return r.db.Exec(`INSERT INTO dataset_node_cache (dataset_id, node, status) VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE status=VALUES(status)`, datasetID, node, status).Error
}

// CacheDelete는 (dataset,node) 캐시 행을 제거한다.
func (r *gormRepo) CacheDeleteAll(datasetID int64) error {
	return r.db.Exec(`DELETE FROM dataset_node_cache WHERE dataset_id=?`, datasetID).Error
}

func (r *gormRepo) CacheDelete(datasetID int64, node string) error {
	return r.db.Exec(`DELETE FROM dataset_node_cache WHERE dataset_id=? AND node=?`, datasetID, node).Error
}

// NameTaken은 같은 이름의 데이터셋(또는 대기 신청)이 있는지 본다(정규경로 /dataset/<name> 충돌 방지).
func (r *gormRepo) NameTaken(name string) bool {
	var n int64
	r.db.Raw(`SELECT
		(SELECT COUNT(*) FROM datasets WHERE name=?) +
		(SELECT COUNT(*) FROM dataset_requests WHERE name=? AND status='pending')`, name, name).Scan(&n)
	return n > 0
}

// CachedNodesOf는 (cached 완료) 노드 목록을 반환한다(세션 hostPath 마운트 판정용).
func (r *gormRepo) CachedNodesOf(datasetID int64) []string {
	var nodes []string
	r.db.Raw(`SELECT node FROM dataset_node_cache WHERE dataset_id=? AND status='cached'`, datasetID).Scan(&nodes)
	return nodes
}

// ListCaching은 복사 진행중(caching) 캐시 행을 반환한다(리컨실러 폴링 대상).
func (r *gormRepo) ListCaching() []DatasetCacheRow {
	var out []DatasetCacheRow
	r.db.Raw(`SELECT dataset_id, node FROM dataset_node_cache WHERE status='caching'`).Scan(&out)
	return out
}

// DatasetCacheRow — 캐시 리컨실 대상(dataset+node).
type DatasetCacheRow struct {
	DatasetID int64  `gorm:"column:dataset_id"`
	Node      string `gorm:"column:node"`
}
