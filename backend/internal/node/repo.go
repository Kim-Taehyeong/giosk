package node

import (
	"errors"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

var ErrNotFound = errors.New("not found")

// ErrNodeBusy는 선착순(비-자유) 모드에서 이미 다른 사용자가 노드를 점유 중일 때 반환된다.
var ErrNodeBusy = errors.New("node already leased")

// Repository는 node 영속성 계약(설정 오버레이 + 임대).
type Repository interface {
	Configs() (map[string]Config, error)
	UpsertConfig(name string, fields map[string]any) error

	CreateLease(l *Lease) error
	AcquireLease(l *Lease, exclusive bool) (reused bool, err error) // 선착순 원자 획득(갭락 직렬화)
	GetLease(id int64) (*Lease, error)
	ExtendLease(id int64, addHours int) error
	ReleaseLease(id int64) error
	ActiveLeaseUser(node string) (string, bool)            // leasedTo username
	ActiveLeaseFor(node string, userID int64) (int64, bool) // (node,user) 활성 임대 id(자유모드 멱등 재사용)
	AgentLeases(node string) ([]AgentLease, error)
	LeaseVolumes(instanceID string) []LeaseVolRow        // 물리세션에 붙은 볼륨(PVC 좌표 + 권한)
	LeaseDatasets(instanceID string) []LeaseDatasetRow   // 물리세션에 붙은 데이터셋(PVC 좌표, RO)
	RunningByNode() map[string]int                       // 노드별 running 세션 수
	ReleaseByInstance(instanceID string) (string, error) // 임대 해제 후 노드명 반환
}

type gormRepo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

func (r *gormRepo) Configs() (map[string]Config, error) {
	var rows []Config
	if err := r.db.Find(&rows).Error; err != nil {
		return nil, err
	}
	m := make(map[string]Config, len(rows))
	for _, c := range rows {
		m[c.Name] = c
	}
	return m, nil
}

// UpsertConfig는 노드 설정 오버레이를 멱등 생성/수정한다.
func (r *gormRepo) UpsertConfig(name string, fields map[string]any) error {
	if err := r.db.Exec(`INSERT IGNORE INTO nodes (name) VALUES (?)`, name).Error; err != nil {
		return err
	}
	if len(fields) == 0 {
		return nil
	}
	return r.db.Model(&Config{}).Where("name = ?", name).Updates(fields).Error
}

func (r *gormRepo) RunningByNode() map[string]int {
	var rows []struct {
		Node string
		N    int
	}
	r.db.Raw(`SELECT node, COUNT(*) AS n FROM sessions WHERE phase = 'running' AND node <> '' GROUP BY node`).Scan(&rows)
	out := map[string]int{}
	for _, r := range rows {
		out[r.Node] = r.N
	}
	return out
}

func (r *gormRepo) CreateLease(l *Lease) error { return r.db.Create(l).Error }

// AcquireLease는 노드 활성 임대를 원자적으로 획득한다(선착순 정합성).
// node_leases.lease_key(생성컬럼) 유니크 제약이 경합을 DB 레벨에서 보장한다(갭락/데드락 없음):
//   - exclusive(비-자유): lease_key=node. 둘째 INSERT 는 1062 중복이라 ErrNodeBusy 가 된다.
//   - !exclusive(자유):   lease_key=node#user. 둘째 INSERT 는 1062 중복이지만 재사용한다(reused=true).
// 거의 동시에 들어온 다수 요청 중 처음 한 건만 INSERT 에 성공한다.
func (r *gormRepo) AcquireLease(l *Lease, exclusive bool) (bool, error) {
	l.Shared = !exclusive
	// 흔한 경우는 사전 조회로 차단한다(유니크 제약은 동시 경합 백스톱). 불필요한 1062 로그를 줄인다.
	if exclusive {
		if _, taken := r.ActiveLeaseUser(l.Node); taken {
			return false, ErrNodeBusy
		}
	} else if _, ok := r.ActiveLeaseFor(l.Node, l.UserID); ok {
		return true, nil
	}
	err := r.db.Create(l).Error
	if isDuplicateKey(err) {
		if exclusive {
			return false, ErrNodeBusy
		}
		return true, nil // 자유 모드에서 (node,user) 임대가 이미 있으면 멱등하게 재사용한다
	}
	return false, err
}

// isDuplicateKey는 MySQL 유니크 제약 위반(1062)인지 판정한다.
func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	var me *mysqldriver.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

func (r *gormRepo) GetLease(id int64) (*Lease, error) {
	var l Lease
	err := r.db.Where("id = ?", id).First(&l).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &l, err
}

func (r *gormRepo) ExtendLease(id int64, addHours int) error {
	return r.db.Exec(
		`UPDATE node_leases SET expires_at = expires_at + INTERVAL ? HOUR, extensions_used = extensions_used + 1
		 WHERE id = ? AND status = 'active'`, addHours, id).Error
}

// ReleaseByInstance는 instance_id 의 활성 임대를 해제하고 해당 노드명을 반환한다.
func (r *gormRepo) ReleaseByInstance(instanceID string) (string, error) {
	var node string
	r.db.Raw(`SELECT node FROM node_leases WHERE instance_id=? AND status='active' LIMIT 1`, instanceID).Scan(&node)
	err := r.db.Exec(`UPDATE node_leases SET status='released', released_at=NOW() WHERE instance_id=? AND status='active'`, instanceID).Error
	return node, err
}

func (r *gormRepo) ReleaseLease(id int64) error {
	return r.db.Exec(`UPDATE node_leases SET status='released', released_at=NOW() WHERE id=? AND status='active'`, id).Error
}

func (r *gormRepo) ActiveLeaseUser(nodeName string) (string, bool) {
	var username string
	r.db.Raw(`
		SELECT u.username FROM node_leases l JOIN users u ON u.id = l.user_id
		WHERE l.node = ? AND l.status = 'active' AND l.expires_at > ? ORDER BY l.id DESC LIMIT 1`,
		nodeName, time.Now().UTC()).Scan(&username)
	return username, username != ""
}

// ActiveLeaseFor는 (node,user) 의 활성 임대 id 를 반환한다(자유모드에서 중복 임대 방지·멱등 재사용).
func (r *gormRepo) ActiveLeaseFor(nodeName string, userID int64) (int64, bool) {
	var id int64
	r.db.Raw(`SELECT id FROM node_leases WHERE node=? AND user_id=? AND status='active' ORDER BY id DESC LIMIT 1`,
		nodeName, userID).Scan(&id)
	return id, id != 0
}

// AgentLeases는 해당 노드의 활성 임대 + 사용자 계정/키를 반환한다(NFS 는 service 에서 채움).
func (r *gormRepo) AgentLeases(nodeName string) ([]AgentLease, error) {
	var out []AgentLease
	err := r.db.Raw(`
		SELECT l.instance_id, l.id AS lease_id, u.id AS user_id, u.username, COALESCE(u.ssh_public_key,'') AS ssh_public_key
		FROM node_leases l JOIN users u ON u.id = l.user_id
		WHERE l.node = ? AND l.status = 'active' AND l.expires_at > ?`,
		nodeName, time.Now().UTC()).Scan(&out).Error
	return out, err
}

// LeaseVolRow는 물리세션에 붙은 볼륨 1건(PVC 좌표와 서버 판정 권한).
type LeaseVolRow struct {
	PVCName      string
	PVCNamespace string
	MountPath    string
	Perm         string
}

// LeaseVolumes는 instance_id 세션에 연결된 볼륨(프로비저닝된 것만)을 반환한다.
// perm 은 세션 생성 시 서버가 판정해 저장한 값(소유=rw/공유=share perm)이라 그대로 신뢰한다.
func (r *gormRepo) LeaseVolumes(instanceID string) []LeaseVolRow {
	var out []LeaseVolRow
	r.db.Raw(`
		SELECT v.pvc_name, v.pvc_namespace, sv.mount_path, sv.perm
		FROM sessions s
		JOIN session_volumes sv ON sv.session_id = s.id
		JOIN volumes v ON v.id = sv.volume_id
		WHERE s.instance_id = ? AND v.pvc_name <> ''`, instanceID).Scan(&out)
	return out
}

// LeaseDatasetRow는 물리세션에 붙은 데이터셋 1건(이름과 PVC 좌표, 항상 RO).
type LeaseDatasetRow struct {
	Name         string
	PVCName      string
	PVCNamespace string
}

// LeaseDatasets는 instance_id 세션에 연결된 데이터셋(프로비저닝된 것만)을 반환한다(RO).
func (r *gormRepo) LeaseDatasets(instanceID string) []LeaseDatasetRow {
	var out []LeaseDatasetRow
	r.db.Raw(`
		SELECT d.name, d.pvc_name, d.pvc_namespace
		FROM sessions s
		JOIN session_datasets sd ON sd.session_id = s.id
		JOIN datasets d ON d.id = sd.dataset_id
		WHERE s.instance_id = ? AND d.pvc_name <> ''`, instanceID).Scan(&out)
	return out
}
