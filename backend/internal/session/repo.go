package session

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("not found")

// ErrSessionLimit는 사용자 동시 활성 세션 상한 초과.
var ErrSessionLimit = errors.New("session limit reached")

// 물리(SSH) 세션 관련 오류.
var (
	ErrLeaseUnavailable     = errors.New("physical lease unavailable")
	ErrNodeRequired         = errors.New("node required")
	ErrLocalHomeForbidden   = errors.New("local home not leased by user")
	ErrLocalHomeUnavailable = errors.New("local home node unavailable")
)

// ErrExtendUnavailable는 연장 불가(선착순 모드 아님 / 연장 한도 초과 / 세션 없음).
var ErrExtendUnavailable = errors.New("extend unavailable")

// ErrInsufficientCredit는 크레딧 모드에서 잔액이 1시간 단가에 못 미쳐 세션 생성 거부.
var ErrInsufficientCredit = errors.New("insufficient credit")

// ErrMustJoinTeam은 크레딧 모드에서 팀 소속이 없어 세션을 만들 수 없을 때(차감할 팀 지갑 없음).
var ErrMustJoinTeam = errors.New("must belong to a team")

// ErrHardLimit는 하드 리소스 상한(GPU 개수·VRAM 등, 크레딧 무관) 초과로 세션 생성 거부.
var ErrHardLimit = errors.New("hard resource limit exceeded")

// Repository는 session 영속성 계약. 이미지/오퍼링 스펙은 read 프로젝션으로 조회.
type Repository interface {
	Create(s *Session) error
	ListByUser(userID int64) ([]Session, error)
	Get(instanceID string, userID int64) (*Session, error)
	UpdateLive(instanceID, phase, node, ip string) error
	SetPhase(instanceID, phase string) error
	SetBilled(instanceID string, billed int) error                                 // 정산 누적 갱신
	RecordGpuUsage(userID, groupID int64, ref string, seconds, gpuCount int) error // 사용시간 원장 적립(세션 삭제와 무관)
	ResetBilling(instanceID string, startedAt time.Time) error                     // 재개 시 가동 시작/정산 초기화
	IncrementExtensions(instanceID string, userID int64, maxExt int) (bool, error) // 임대 연장(상한 내 원자적 +1; false=상한도달/없음)
	Delete(instanceID string) error
	ListAll() ([]AdminRow, error)
	AdminOne(instanceID string) (*AdminRow, error)
	ListRunning() ([]Session, error)
	ListActiveContainer() ([]Session, error)       // 컨테이너 활성(provisioning|running) 세션 — phase 리컨실용
	CountActive(userID int64) int                  // 활성(provisioning|running) 세션 수
	UserLeasedNode(userID int64, node string) bool // 사용자가 그 물리노드를 대여한 적 있는지(로컬 Home 접근 검증)
	SetUserSSHKey(userID int64, key string) error
	UserSSHKey(userID int64) string // 등록된 SSH 공개키(미등록=""). 컨테이너 SSH authorized_keys 주입용
	Username(userID int64) string
	GetByInstance(instanceID string) (*Session, error)

	ImageRef(imageID int64) (string, error)    // name(:tag)
	ImageChannels(imageID int64) ImageChannels // 이미지 제공 채널(vscode/jupyter/ssh)
	CachedNodes(imageID int64) []string        // 이미지가 캐시 완료(cached)된 노드 — 스케줄 선호용
	OfferingSpec(offeringID int64) (*OfferingSpec, error)
	GpuTypePrice(gpuType string) int                                  // 전용 GPU 시간당 단가
	GpuTypePricing(gpuType string) (perHour, perGB, perCore int)      // 전용/분할 단위 단가
	TimesliceSplit(nodes []string) int                                // 타임슬라이싱 노드의 슬롯 수(없으면 0)
	VolumePVC(volID int64) (name, namespace string, ok bool)          // 볼륨의 PVC 좌표
	VolumeAccess(volID, userID int64) (VolAccess, bool)               // 접근 권한(소유=rw/공유=share perm) + PVC 좌표
	SessionVolumeMounts(sessionID int64) []VolMountRow                // 재시작 복원용 볼륨 마운트
	ActiveSessionsWithVolume(userID, volID int64, exclude string) int // 같은 볼륨 쓰는 다른 활성 세션 수(공유 복제본 정리 판단)

	AddVolume(sessionID, volID int64, path, perm string) error
	AddDataset(sessionID, dsID int64, node string) error
	DatasetMount(dsID int64) (DatasetRow, bool)                       // 데이터셋 PVC 좌표 + 이름(세션 RO 마운트용)
	MountableDatasetIDs() []int64                                     // 승인·적재완료된 전체 글로벌 데이터셋(자동 마운트 대상)
	SessionDatasetIDs(sessionID int64) []int64                        // 세션에 붙은 데이터셋 ID
	ActiveSessionsWithDataset(userID, dsID int64, exclude string) int // 같은 데이터셋 쓰는 다른 활성 세션 수
}

// AdminRow — 관리자 세션 관제 행(소유자/조직/그룹 조인).
type AdminRow struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	UserID       int64  `json:"userId"` // 소유자 id(상세 페이지에서 사용자 상세로 링크)
	Image        string `json:"image"`  // 사용 이미지(name:tag)
	Owner        string `json:"owner"`
	Org          string `json:"org"`
	Group        string `json:"group"`
	GpuType      string `json:"gpuType"`
	GpuMode      string `json:"gpuMode"`
	Env          string `json:"env"`
	Node         string `json:"node"`
	Status       string `json:"status"`
	CreatedAt    string `json:"createdAt"`
	StartedAt    string `json:"startedAt"`    // 실행시간 산출(프론트)
	PricePerHour int    `json:"pricePerHour"` // 시간당 크레딧
	Consumed     int    `json:"consumed"`     // 실제 소비 크레딧(billed_credits)
}

// OfferingSpec — 오퍼링에서 도출한 세션 스펙.
type OfferingSpec struct {
	VramMB       int    `gorm:"column:vram_mb"`
	CorePercent  int    `gorm:"column:core_percent"`
	Mode         string `gorm:"column:mode"`
	GpuType      string `gorm:"column:gpu_type"`
	PricePerHour int    `gorm:"column:price_per_hour"`
}

type gormRepo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

func (r *gormRepo) Create(s *Session) error { return r.db.Create(s).Error }

func (r *gormRepo) ListByUser(userID int64) ([]Session, error) {
	var out []Session
	return out, r.db.Where("user_id = ?", userID).Order("id DESC").Find(&out).Error
}

func (r *gormRepo) Get(instanceID string, userID int64) (*Session, error) {
	var s Session
	err := r.db.Where("instance_id = ? AND user_id = ?", instanceID, userID).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &s, err
}

func (r *gormRepo) UpdateLive(instanceID, phase, node, ip string) error {
	return r.db.Model(&Session{}).Where("instance_id = ?", instanceID).
		Updates(map[string]any{"phase": phase, "node": node, "ip_address": ip}).Error
}

func (r *gormRepo) SetPhase(instanceID, phase string) error {
	return r.db.Model(&Session{}).Where("instance_id = ?", instanceID).Update("phase", phase).Error
}

func (r *gormRepo) SetBilled(instanceID string, billed int) error {
	return r.db.Model(&Session{}).Where("instance_id = ?", instanceID).Update("billed_credits", billed).Error
}

// RecordGpuUsage는 정산 틱마다 적립된 사용 시간(초)을 gpu_usage 원장에 기록한다(그룹 귀속 포함).
func (r *gormRepo) RecordGpuUsage(userID, groupID int64, ref string, seconds, gpuCount int) error {
	if seconds <= 0 {
		return nil
	}
	if gpuCount < 1 {
		gpuCount = 1
	}
	var gid interface{}
	if groupID > 0 {
		gid = groupID
	}
	return r.db.Exec(`INSERT INTO gpu_usage (user_id, group_id, session_ref, gpu_count, seconds) VALUES (?, ?, ?, ?, ?)`,
		userID, gid, ref, gpuCount, seconds).Error
}

func (r *gormRepo) ResetBilling(instanceID string, startedAt time.Time) error {
	return r.db.Model(&Session{}).Where("instance_id = ?", instanceID).
		Updates(map[string]any{"started_at": startedAt, "billed_credits": 0}).Error
}

// IncrementExtensions는 상한(maxExt) 내에서 연장 횟수를 원자적으로 +1 한다(false=상한도달/없음).
func (r *gormRepo) IncrementExtensions(instanceID string, userID int64, maxExt int) (bool, error) {
	res := r.db.Exec(`UPDATE sessions SET extensions_used = extensions_used + 1
		WHERE instance_id = ? AND user_id = ? AND extensions_used < ?`, instanceID, userID, maxExt)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *gormRepo) Delete(instanceID string) error {
	return r.db.Where("instance_id = ?", instanceID).Delete(&Session{}).Error
}

func (r *gormRepo) ListAll() ([]AdminRow, error) {
	var out []AdminRow
	err := r.db.Raw(adminRowSelect + ` ORDER BY s.id DESC`).Scan(&out).Error
	return out, err
}

// adminRowSelect은 AdminRow 조회 공통 SELECT(목록/단건 공유).
const adminRowSelect = `
	SELECT s.instance_id AS id, s.name, s.user_id,
	       COALESCE(NULLIF(CONCAT(img.name, IF(img.tag IS NULL OR img.tag='','',CONCAT(':',img.tag))), ''), '') AS image,
	       TRIM(CONCAT(COALESCE(u.last_name,''),COALESCE(u.first_name,''))) AS owner,
	       COALESCE(o.display_name,'') AS org, COALESCE(g.display_name,'') AS ` + "`group`" + `,
	       s.gpu_type, s.gpu_mode, s.env, s.node, s.phase AS status,
	       DATE_FORMAT(s.created_at, '%Y-%m-%dT%H:%i:%sZ') AS created_at,
	       DATE_FORMAT(s.started_at, '%Y-%m-%dT%H:%i:%sZ') AS started_at,
	       s.price_per_hour, s.billed_credits AS consumed
	FROM sessions s JOIN users u ON u.id = s.user_id
	LEFT JOIN ` + "`groups`" + ` g ON g.id = COALESCE(s.group_id,
	    (SELECT m.group_id FROM memberships m WHERE m.user_id = s.user_id AND m.status = 'active' ORDER BY m.id LIMIT 1))
	LEFT JOIN organizations o ON o.id = g.org_id
	LEFT JOIN images img ON img.id = s.image_id`

// AdminOne은 단일 세션의 관제용 상세 행(없으면 sql.ErrNoRows).
func (r *gormRepo) AdminOne(instanceID string) (*AdminRow, error) {
	var row AdminRow
	err := r.db.Raw(adminRowSelect+` WHERE s.instance_id = ? LIMIT 1`, instanceID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == "" {
		return nil, ErrNotFound
	}
	return &row, nil
}

func (r *gormRepo) ListRunning() ([]Session, error) {
	var out []Session
	return out, r.db.Where("phase = ?", PhaseRunning).Find(&out).Error
}

// ListActiveContainer는 Pod 가 있는(컨테이너) 활성 세션을 반환한다(라이브 phase 리컨실 대상).
func (r *gormRepo) ListActiveContainer() ([]Session, error) {
	var out []Session
	return out, r.db.Where("env <> ? AND phase IN ?", "ssh", []string{PhaseProvisioning, PhaseRunning}).Find(&out).Error
}

func (r *gormRepo) SetUserSSHKey(userID int64, key string) error {
	if key == "" {
		return nil
	}
	return r.db.Exec(`UPDATE users SET ssh_public_key = ? WHERE id = ?`, key, userID).Error
}

// UserSSHKey는 사용자가 등록한 SSH 공개키(OpenSSH 1줄)를 반환한다. 미등록이면 빈 문자열.
// 컨테이너 세션 sshd 사이드카가 신뢰할 authorized_keys Secret 을 채우는 데 쓴다.
func (r *gormRepo) UserSSHKey(userID int64) string {
	var key string
	r.db.Raw(`SELECT ssh_public_key FROM users WHERE id = ?`, userID).Scan(&key)
	return key
}

func (r *gormRepo) Username(userID int64) string {
	var name string
	r.db.Raw(`SELECT username FROM users WHERE id = ?`, userID).Scan(&name)
	return name
}

func (r *gormRepo) CountActive(userID int64) int {
	var n int64
	r.db.Model(&Session{}).Where("user_id = ? AND phase IN ?", userID, []string{PhaseProvisioning, PhaseRunning}).Count(&n)
	return int(n)
}

// UserLeasedNode는 사용자가 그 물리노드를 대여한 적 있는지(이력 포함) 확인한다 — 로컬 Home 접근 검증.
func (r *gormRepo) UserLeasedNode(userID int64, node string) bool {
	var n int64
	r.db.Raw(`SELECT COUNT(*) FROM node_leases WHERE user_id = ? AND node = ?`, userID, node).Scan(&n)
	return n > 0
}

func (r *gormRepo) GetByInstance(instanceID string) (*Session, error) {
	var s Session
	err := r.db.Where("instance_id = ?", instanceID).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &s, err
}

func (r *gormRepo) ImageRef(imageID int64) (string, error) {
	var row struct {
		Name string
		Tag  string
	}
	if err := r.db.Raw(`SELECT name, tag FROM images WHERE id = ?`, imageID).Scan(&row).Error; err != nil {
		return "", err
	}
	if row.Name == "" {
		return "", ErrNotFound
	}
	if row.Tag != "" {
		return row.Name + ":" + row.Tag, nil
	}
	return row.Name, nil
}

// CachedNodes는 이미지가 캐시 완료(cached)된 노드 목록을 반환한다(스케줄 소프트 선호용).
func (r *gormRepo) CachedNodes(imageID int64) []string {
	var nodes []string
	r.db.Raw(`SELECT node FROM image_node_cache WHERE image_id = ? AND status = 'cached'`, imageID).Scan(&nodes)
	return nodes
}

// ImageChannels — 이미지가 제공하는 접속 채널(images.channels JSON).
//
//	Web: 표준(VSCode 8080/Jupyter 8888) 외 커스텀 포트의 웹 앱(예: {name:"app", port:8501}).
//	     시크릿 주입 없이 포트포워딩만 하는 제네릭 채널. 외부 이미지 등록에서 선택적으로 지정.
type ImageChannels struct {
	VSCode  bool         `json:"vscode"`
	Jupyter bool         `json:"jupyter"`
	SSH     bool         `json:"ssh"`
	Web     *WebPortSpec `json:"web,omitempty"`
}

// WebPortSpec — 커스텀 웹 채널(포트포워딩 대상). Name 은 k8s 포트명 규칙(소문자·≤15).
type WebPortSpec struct {
	Name string `json:"name"`
	Port int    `json:"port"`
}

// ImageChannels는 images.channels JSON 을 파싱해 반환한다(미설정/오류=전부 false).
func (r *gormRepo) ImageChannels(imageID int64) ImageChannels {
	var raw string
	r.db.Raw(`SELECT channels FROM images WHERE id = ?`, imageID).Scan(&raw)
	var c ImageChannels
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &c)
	}
	return c
}

func (r *gormRepo) OfferingSpec(offeringID int64) (*OfferingSpec, error) {
	var sp OfferingSpec
	err := r.db.Raw(`SELECT vram_mb, core_percent, mode, gpu_type, price_per_hour FROM offerings WHERE id = ?`, offeringID).
		Scan(&sp).Error
	return &sp, err
}

// GpuTypePrice는 전용 GPU 시간당 단가(gpu_pricing). 미설정=0.
func (r *gormRepo) GpuTypePrice(gpuType string) int {
	var p int
	r.db.Raw(`SELECT price_per_hour FROM gpu_pricing WHERE gpu_type = ?`, gpuType).Scan(&p)
	return p
}

// TimesliceSplit는 주어진 노드들 중 타임슬라이싱 노드의 슬롯 수(split_count)를 반환한다(없으면 0).
// 슬롯 단가 = GPU 전용단가 ÷ 슬롯 수 산출에 쓴다.
func (r *gormRepo) TimesliceSplit(nodes []string) int {
	if len(nodes) == 0 {
		return 0
	}
	var n int
	r.db.Raw(`SELECT split_count FROM nodes WHERE name IN (?) AND share_mode = 'timeslicing' AND split_count > 0 LIMIT 1`, nodes).Scan(&n)
	return n
}

// GpuTypePricing은 전용/분할 단위 단가(시간당, GB당, 코어%당). 미설정=0.
func (r *gormRepo) GpuTypePricing(gpuType string) (int, int, int) {
	var row struct {
		PricePerHour int
		PricePerGB   int
		PricePerCore int
	}
	r.db.Raw(`SELECT price_per_hour, price_per_gb, price_per_core FROM gpu_pricing WHERE gpu_type = ?`, gpuType).Scan(&row)
	return row.PricePerHour, row.PricePerGB, row.PricePerCore
}

// VolMountRow — 세션에 붙은 볼륨 마운트(재시작 복원용).
type VolMountRow struct {
	VolID     int64  `gorm:"column:volume_id"`
	MountPath string `gorm:"column:mount_path"`
	Perm      string `gorm:"column:perm"`
}

func (r *gormRepo) SessionVolumeMounts(sessionID int64) []VolMountRow {
	var out []VolMountRow
	r.db.Raw(`SELECT volume_id, mount_path, perm FROM session_volumes WHERE session_id = ?`, sessionID).Scan(&out)
	return out
}

// VolAccess — 볼륨 접근 권한 + PVC 좌표. Perm 은 소유자=rw, 그 외엔 volume_shares.permission.
type VolAccess struct {
	PVCName      string
	PVCNamespace string
	SizeGiB      int
	Perm         string // rw | ro
}

// VolumeAccess는 사용자의 볼륨 접근권을 서버에서 판정한다(클라 입력 perm 불신).
// 소유자면 rw, 공유받았으면 share.permission, 권한 없으면 ok=false.
func (r *gormRepo) VolumeAccess(volID, userID int64) (VolAccess, bool) {
	// gorm:"column:" 필수 — 기본 네이밍은 SizeGiB 를 size_gi_b 로 바꿔 size_gib 와 어긋난다
	// (값이 0 으로 떨어져 공유 PV 가 항상 1Gi 로 만들어졌다). 같은 이유로 PVC* 도 명시한다.
	var v struct {
		PVCName      string `gorm:"column:pvc_name"`
		PVCNamespace string `gorm:"column:pvc_namespace"`
		OwnerUserID  *int64 `gorm:"column:owner_user_id"`
		SizeGiB      int    `gorm:"column:size_gib"`
	}
	r.db.Raw(`SELECT pvc_name, pvc_namespace, owner_user_id, size_gib FROM volumes WHERE id = ?`, volID).Scan(&v)
	if v.PVCName == "" {
		return VolAccess{}, false
	}
	acc := VolAccess{PVCName: v.PVCName, PVCNamespace: v.PVCNamespace, SizeGiB: v.SizeGiB}
	if v.OwnerUserID != nil && *v.OwnerUserID == userID {
		acc.Perm = "rw"
		return acc, true
	}
	// 직접 공유 + 그룹 공유(활성 멤버십) 중 가장 강한 권한을 채택(ro 다수보다 rw 1건 우선).
	var perms []string
	r.db.Raw(`
		SELECT permission FROM volume_shares
		WHERE volume_id = ? AND (
			shared_with_user_id = ?
			OR shared_with_group_id IN (
				SELECT group_id FROM memberships WHERE user_id = ? AND status = 'active'
			)
		)`, volID, userID, userID).Scan(&perms)
	if len(perms) == 0 {
		return VolAccess{}, false // 접근 권한 없음
	}
	acc.Perm = "ro"
	for _, p := range perms {
		if p == "rw" {
			acc.Perm = "rw"
			break
		}
	}
	return acc, true
}

// ActiveSessionsWithVolume는 userID 의 활성(provisioning|running) 세션 중
// 해당 볼륨을 마운트한 세션 수를 반환한다(exclude instance_id 제외). 공유 복제본 정리 안전판단용.
func (r *gormRepo) ActiveSessionsWithVolume(userID, volID int64, exclude string) int {
	var n int64
	r.db.Raw(`
		SELECT COUNT(*) FROM sessions s
		JOIN session_volumes sv ON sv.session_id = s.id
		WHERE s.user_id = ? AND sv.volume_id = ? AND s.instance_id <> ?
		  AND s.phase IN (?, ?)`,
		userID, volID, exclude, PhaseProvisioning, PhaseRunning).Scan(&n)
	return int(n)
}

func (r *gormRepo) VolumePVC(volID int64) (string, string, bool) {
	var row struct {
		PVCName      string
		PVCNamespace string
	}
	r.db.Raw(`SELECT pvc_name, pvc_namespace FROM volumes WHERE id = ?`, volID).Scan(&row)
	if row.PVCName == "" {
		return "", "", false
	}
	return row.PVCName, row.PVCNamespace, true
}

func (r *gormRepo) AddVolume(sessionID, volID int64, path, perm string) error {
	return r.db.Exec(`INSERT INTO session_volumes (session_id, volume_id, mount_path, perm) VALUES (?, ?, ?, ?)`,
		sessionID, volID, path, perm).Error
}

func (r *gormRepo) AddDataset(sessionID, dsID int64, node string) error {
	return r.db.Exec(`INSERT INTO session_datasets (session_id, dataset_id, node) VALUES (?, ?, ?)`,
		sessionID, dsID, node).Error
}

// DatasetRow — 데이터셋 마운트 원천(이름 + PVC 좌표).
type DatasetRow struct {
	Name         string
	PVCName      string
	PVCNamespace string
}

// DatasetMount는 데이터셋의 이름 + RWX NFS PVC 좌표를 반환한다(미프로비저닝이면 ok=false).
func (r *gormRepo) DatasetMount(dsID int64) (DatasetRow, bool) {
	var row DatasetRow
	r.db.Raw(`SELECT name, pvc_name, pvc_namespace FROM datasets WHERE id = ?`, dsID).Scan(&row)
	if row.PVCName == "" {
		return DatasetRow{}, false
	}
	return row, true
}

// MountableDatasetIDs는 모든 세션에 자동 마운트할 데이터셋(승인=active + 적재완료=ready + PVC 보유)을 반환한다.
func (r *gormRepo) MountableDatasetIDs() []int64 {
	var out []int64
	r.db.Raw(`SELECT id FROM datasets
		WHERE scope='global' AND status='active' AND load_status='ready' AND pvc_name <> ''
		ORDER BY id`).Scan(&out)
	return out
}

func (r *gormRepo) SessionDatasetIDs(sessionID int64) []int64 {
	var out []int64
	r.db.Raw(`SELECT dataset_id FROM session_datasets WHERE session_id = ?`, sessionID).Scan(&out)
	return out
}

// ActiveSessionsWithDataset은 같은 데이터셋을 마운트한 다른 활성 세션 수(공유 복제본 정리 판단).
func (r *gormRepo) ActiveSessionsWithDataset(userID, dsID int64, exclude string) int {
	var n int64
	r.db.Raw(`
		SELECT COUNT(*) FROM sessions s
		JOIN session_datasets sd ON sd.session_id = s.id
		WHERE s.user_id = ? AND sd.dataset_id = ? AND s.instance_id <> ?
		  AND s.phase IN (?, ?)`,
		userID, dsID, exclude, PhaseProvisioning, PhaseRunning).Scan(&n)
	return int(n)
}
