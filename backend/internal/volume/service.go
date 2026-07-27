package volume

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"time"

	"giosk/internal/k8s"
	"giosk/internal/metrics"
	"giosk/internal/policy"
)

var ErrQuotaExceeded = errors.New("volume quota exceeded")

// ErrInsufficientCredit는 크레딧 모드에서 볼륨 최소 비용(≈1일)을 감당할 잔액이 없어 생성 거부.
var ErrInsufficientCredit = errors.New("insufficient credit")

// Charger는 스토리지 사용료를 (user,team) 멤버십 지갑에서 차감한다(wallet.Service 가 구현).
// 그룹 볼륨이면 그 팀, 개인 볼륨이면 0(소유자 대표 팀으로 resolve).
type Charger interface {
	Consume(userID, groupID int64, credits int, ref string) (bool, error)
	Balance(userID, groupID int64) int
}

// gidPtr는 *int64 그룹 id 를 값(nil=0)으로.
func gidPtr(p *int64) int64 {
	if p != nil {
		return *p
	}
	return 0
}

// hoursPerMonth는 스토리지 단가(GiB·월) 기준 시간(30일).
const hoursPerMonth = 30 * 24

// Provisioner는 PVC 프로비저닝 계약(*k8s.Client 구현).
type Provisioner interface {
	CreatePVC(ctx context.Context, s k8s.PVCSpec) error
	PVCPhase(ctx context.Context, ns, name string) (string, error)
	DeletePVC(ctx context.Context, ns, name string) error
	// EnsureSharedNFSPVC는 기존 NFS 경로를 정적 PV+PVC 로 바인딩한다(임포트 볼륨).
	EnsureSharedNFSPVC(ctx context.Context, s k8s.SharedNFSSpec) error
}

// Service는 volume 비즈니스 로직.
type Service struct {
	repo             Repository
	prov             Provisioner
	storageClass     string
	nsPrefix         string
	totalGiB         int
	met              *metrics.Client
	localHomeOn      bool             // 물리노드(hybrid) 활성 시 로컬 Home 특수 볼륨 노출
	limits           *policy.Resolver // 볼륨 GiB 하드 상한(계층 해석; nil=전역 totalGiB 사용)
	charger          Charger          // 스토리지 과금(nil=비크레딧 모드=미과금)
	pricePerGiBMonth int              // GiB·월 크레딧 단가(설치 기본; 런타임 미설정 시 폴백)
	priceFn          func() int       // 라이브 단가(런타임 조정 반영; nil=pricePerGiBMonth 사용)
}

func NewService(repo Repository, prov Provisioner, storageClass, nsPrefix string, totalGiB int) *Service {
	return &Service{repo: repo, prov: prov, storageClass: storageClass, nsPrefix: nsPrefix, totalGiB: totalGiB}
}

// WithMetrics는 실사용량 조회용 Prometheus 클라이언트를 주입한다.
func (s *Service) WithMetrics(m *metrics.Client) *Service { s.met = m; return s }

// WithLocalHome은 물리노드 활성 여부에 따라 로컬 Home 특수 볼륨 노출을 켠다.
func (s *Service) WithLocalHome(on bool) *Service { s.localHomeOn = on; return s }

// WithLimits는 볼륨 GiB 하드 상한(계층 해석)을 주입한다. 사용자별/그룹별 쿼터가 전역보다 우선.
func (s *Service) WithLimits(r *policy.Resolver) *Service { s.limits = r; return s }

// WithCharger는 스토리지 과금(크레딧 모드)을 활성화한다. priceFn=라이브 GiB·월 단가(런타임 조정 반영).
func (s *Service) WithCharger(c Charger, priceFn func() int) *Service {
	s.charger, s.priceFn = c, priceFn
	return s
}

// price는 현재 유효 GiB·월 단가(라이브). priceFn 미주입이면 설치 기본.
func (s *Service) price() int {
	if s.priceFn != nil {
		return s.priceFn()
	}
	return s.pricePerGiBMonth
}

// monthlyCost는 볼륨 1개의 월 크레딧 비용(용량×단가).
func (s *Service) monthlyCost(sizeGiB int) int { return sizeGiB * s.price() }

// quotaFor는 사용자에게 적용되는 볼륨 GiB 하드 상한을 반환한다(계층 미주입/0이면 전역 totalGiB).
func (s *Service) quotaFor(userID int64) int {
	if s.limits != nil {
		if g := s.limits.Resolve(userID).MaxVolumeGiB; g > 0 {
			return g
		}
	}
	return s.totalGiB
}

// localHomes는 사용자가 대여한 적 있는 물리노드별 "로컬 Home" 특수 볼륨을 만든다(물리 비활성 시 빈 슬라이스).
func (s *Service) localHomes(userID int64) []LocalHome {
	if !s.localHomeOn {
		return []LocalHome{}
	}
	nodes := s.repo.LeasedNodes(userID)
	out := make([]LocalHome, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, LocalHome{Node: n, Name: "Local Home @ " + n})
	}
	return out
}

// List는 소유/공유 볼륨 + 쿼터를 반환한다.
func (s *Service) List(userID int64) (*ListRes, error) {
	owned, err := s.repo.ListOwned(userID)
	if err != nil {
		return nil, err
	}
	shared, err := s.repo.ListShared(userID)
	if err != nil {
		return nil, err
	}
	// 볼륨별 사용량은 채우지 않는다 — 볼륨이 NFS 익스포트의 하위 디렉터리라
	// kubelet 통계(kubelet_volume_stats_used_bytes)가 그 볼륨이 아니라 익스포트 전체 사용량을
	// 돌려준다(실측: 50Gi PVC 가 capacity 97.87GiB). 예전엔 그 값을 볼륨 사용량으로 표시해
	// "50/10 GB" 같은 거짓 수치가 나왔다. 실제 사용량은 NFS 하위 디렉터리 du 가 필요(미구현).
	alloc, _ := s.repo.AllocatedGiB(userID)
	return &ListRes{Owned: owned, Shared: shared, LocalHomes: s.localHomes(userID), Quota: Quota{AllocatedGB: alloc, TotalGB: s.quotaFor(userID)}}, nil
}

// StorageOverview — 관리자 스토리지 현황(노드 디스크 + NFS 실용량 + 사용자별 할당).
type StorageOverview struct {
	Nodes []NodeDisk `json:"nodes"`
	// NFS 백엔드의 실제 파일시스템 용량/사용량(GB). 메트릭 미연동 시 0.
	NfsTotalGB int         `json:"nfsTotalGb"`
	NfsUsedGB  int         `json:"nfsUsedGb"`
	Users      []UserUsage `json:"users"`
}

type NodeDisk struct {
	Node    string `json:"node"`
	UsedGB  int    `json:"usedGb"`
	TotalGB int    `json:"totalGb"`
}

type UserUsage struct {
	UserID      int64  `json:"userId"`
	Name        string `json:"name"`
	AllocatedGB int    `json:"allocatedGb"`
	UsedGB      int    `json:"usedGb"`
}

// AdminStorage는 관리자용 스토리지 현황을 집계한다.
//
//	nodes: 노드별 물리 디스크(node-exporter, kube_pod_info 조인). 미연동 시 빈 슬라이스.
//	nfsTotalGb/nfsUsedGb: NFS 익스포트 실제 총량/사용량. users: 사용자별 볼륨 할당(DB).
func (s *Service) AdminStorage(ctx context.Context) StorageOverview {
	out := StorageOverview{Nodes: []NodeDisk{}, Users: []UserUsage{}}
	out.NfsTotalGB, out.NfsUsedGB = s.nfsCapacity(ctx)
	out.Nodes = s.nodeDisk(ctx)
	for _, u := range s.repo.UsersAllocated() {
		out.Users = append(out.Users, UserUsage{UserID: u.UserID, Name: u.Name, AllocatedGB: u.AllocatedGB})
	}
	return out
}

// nfsCapacity는 NFS 백엔드의 실제 파일시스템 총량/사용량(GB)을 반환한다.
//
// NFS PVC(nfs-subdir provisioner)는 익스포트의 하위 디렉터리라, kubelet 이 마운트에 statfs 를 걸면
// PVC 요청 용량이 아니라 **익스포트 파일시스템 전체** 수치가 나온다(실측: 50Gi 요청 PVC 가
// capacity 97.87GiB / available 91.86GiB = df /srv/nfs 와 동일). 따라서
//   - 모든 NFS PVC 가 같은 값을 보고하므로 max() 로 하나만 취한다.
//     (예전엔 PVC 마다 더해서 NFS 사용량이 PVC 개수배로 부풀었다.)
//   - storageclass 로 걸러 로컬 디스크 PVC(local-path 등)와 섞이지 않게 한다.
func (s *Service) nfsCapacity(ctx context.Context) (totalGB, usedGB int) {
	if s.met == nil || !s.met.Enabled() || s.storageClass == "" {
		return 0, 0
	}
	join := `* on(persistentvolumeclaim,namespace) group_left() kube_persistentvolumeclaim_info{storageclass="` + s.storageClass + `"}`
	gb := func(b float64) int { return int(b / (1024 * 1024 * 1024)) }
	total, ok1 := s.met.Scalar(ctx, `max(kubelet_volume_stats_capacity_bytes `+join+`)`)
	used, ok2 := s.met.Scalar(ctx, `max(kubelet_volume_stats_used_bytes `+join+`)`)
	if !ok1 || !ok2 {
		return 0, 0
	}
	return gb(total), gb(used)
}

// nodeDisk는 노드별 루트 파일시스템 사용/총량(GB)을 반환한다(node-exporter→node 조인).
func (s *Service) nodeDisk(ctx context.Context) []NodeDisk {
	out := []NodeDisk{}
	if s.met == nil || !s.met.Enabled() {
		return out
	}
	const join = ` * on(pod,namespace) group_left(node) kube_pod_info`
	size, ok1 := s.met.VectorByLabel(ctx, `node_filesystem_size_bytes{mountpoint="/"}`+join, "node")
	avail, ok2 := s.met.VectorByLabel(ctx, `node_filesystem_avail_bytes{mountpoint="/"}`+join, "node")
	if !ok1 || !ok2 {
		return out
	}
	for node, total := range size {
		gb := func(b float64) int { return int(b / (1024 * 1024 * 1024)) }
		out = append(out, NodeDisk{Node: node, TotalGB: gb(total), UsedGB: gb(total - avail[node])})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Node < out[j].Node })
	return out
}

// Create는 쿼터 확인 후 볼륨 행 + PVC 를 만든다.
func (s *Service) Create(ctx context.Context, userID int64, req CreateReq) (*Volume, error) {
	alloc, _ := s.repo.AllocatedGiB(userID)
	if alloc+req.SizeGiB > s.quotaFor(userID) {
		return nil, ErrQuotaExceeded
	}
	// 크레딧 모드: 최소 ~1일치 비용을 감당할 잔액이 있어야 생성 허용(0잔액 신규 생성 차단).
	// 기존 볼륨은 잔액 부족이어도 유예(빌러가 즉시 삭제하지 않음).
	if s.charger != nil && s.price() > 0 {
		dayCost := s.monthlyCost(req.SizeGiB) / 30
		if s.charger.Balance(userID, 0) < dayCost {
			return nil, ErrInsufficientCredit
		}
	}
	v := &Volume{
		// 스토리지 단일화: 모든 볼륨은 NFS(RWX). 교차-ns 공유가 어디서나 동작하고
		// 물리노드에서도 직접 NFS 마운트가 가능해진다(설계: NFS 단일 백엔드).
		Name: req.Name, Kind: "personal", OwnerUserID: &userID,
		SizeGiB: req.SizeGiB, AccessMode: "RWX", Status: StatusPending,
	}
	if err := s.repo.Create(v); err != nil {
		return nil, err
	}
	if err := s.provision(ctx, v); err != nil {
		return nil, err
	}
	return v, nil
}

// ImportReq — 관리자 NFS 기존 볼륨 임포트 입력.
type ImportReq struct {
	Name        string `json:"name" binding:"required"`
	OwnerUserID *int64 `json:"ownerUserId"` // 개인 볼륨 소유자(그룹 볼륨이면 nil)
	GroupID     *int64 `json:"groupId"`     // 그룹 볼륨(개인 볼륨이면 nil)
	NFSServer   string `json:"nfsServer" binding:"required"`
	NFSPath     string `json:"nfsPath" binding:"required"`
	SizeGiB     int    `json:"sizeGiB"` // 표시/쿼터용(정적 PV 라 실제 제한은 아님)
}

// AdminImport는 이미 데이터가 있는 기존 NFS 경로를 볼륨으로 매핑한다(정적 PV 바인딩).
// 도입 환경 어댑션용 관리자 액션 — 쿼터/크레딧 검사는 건너뛴다(기존 데이터라 신규 할당이 아님).
func (s *Service) AdminImport(ctx context.Context, req ImportReq) (*Volume, error) {
	if req.OwnerUserID == nil && req.GroupID == nil {
		return nil, errors.New("owner user or group required")
	}
	if req.SizeGiB < 1 {
		req.SizeGiB = 1
	}
	kind := "personal"
	if req.GroupID != nil {
		kind = "group"
	}
	v := &Volume{
		Name: req.Name, Kind: kind, OwnerUserID: req.OwnerUserID, GroupID: req.GroupID,
		SizeGiB: req.SizeGiB, AccessMode: "RWX", Status: StatusPending,
		NFSServer: req.NFSServer, NFSPath: req.NFSPath,
	}
	if err := s.repo.Create(v); err != nil {
		return nil, err
	}
	// 소유자 ns(개인) 또는 시스템 ns(그룹)에 정적 PV+PVC 로 기존 경로를 바인딩한다.
	ns := s.nsPrefix + "system"
	if req.OwnerUserID != nil {
		ns = fmt.Sprintf("%suser-%d", s.nsPrefix, *req.OwnerUserID)
	}
	pvcName := fmt.Sprintf("vol-%d", v.ID)
	status := StatusBound
	if err := s.prov.EnsureSharedNFSPVC(ctx, k8s.SharedNFSSpec{
		Namespace: ns, Name: pvcName, NFSServer: req.NFSServer, NFSPath: req.NFSPath, SizeGiB: req.SizeGiB,
	}); err != nil {
		status = StatusFailed
	}
	v.PVCName, v.PVCNamespace, v.Status = pvcName, ns, status
	if err := s.repo.SetBound(v.ID, pvcName, ns, status); err != nil {
		return nil, err
	}
	return v, nil
}

func (s *Service) provision(ctx context.Context, v *Volume) error {
	ns := fmt.Sprintf("%suser-%d", s.nsPrefix, *v.OwnerUserID)
	pvcName := fmt.Sprintf("vol-%d", v.ID)
	err := s.prov.CreatePVC(ctx, k8s.PVCSpec{
		Namespace: ns, Name: pvcName, SizeGiB: v.SizeGiB,
		StorageClass: s.storageClass, AccessMode: v.AccessMode,
	})
	status := StatusBound
	if err != nil {
		status = StatusFailed
	}
	v.PVCName, v.PVCNamespace, v.Status = pvcName, ns, status
	return s.repo.SetBound(v.ID, pvcName, ns, status)
}

// Share는 대상(user/group)에게 볼륨을 공유한다.
func (s *Service) Share(volumeID int64, req ShareReq) error {
	perm := req.Permission
	if perm == "" {
		perm = "ro"
	}
	if req.Target == "group" {
		gid, err := strconv.ParseInt(req.Value, 10, 64)
		if err != nil {
			return errors.New("invalid groupId")
		}
		return s.repo.AddShare(volumeID, nil, &gid, perm)
	}
	uid, err := s.repo.ResolveUserID(req.Value)
	if err != nil {
		return err
	}
	return s.repo.AddShare(volumeID, uid, nil, perm)
}

// Delete는 소유자 확인 후 PVC + 행을 삭제한다.
func (s *Service) Delete(ctx context.Context, volumeID, userID int64) error {
	v, err := s.repo.Get(volumeID)
	if err != nil {
		return err
	}
	if v.OwnerUserID == nil || *v.OwnerUserID != userID {
		return ErrNotFound
	}
	if v.PVCName != "" {
		_ = s.prov.DeletePVC(ctx, v.PVCNamespace, v.PVCName)
	}
	return s.repo.Delete(volumeID)
}

// RunStorageBiller는 과금 대상 볼륨의 사용료(용량×GiB·월 단가)를 주기적으로 정산한다.
// charger 미주입(비크레딧)·단가 0(무료)이면 비활성. 잔액 부족 시 유예(볼륨 삭제하지 않음).
func (s *Service) RunStorageBiller(ctx context.Context, interval time.Duration) {
	if s.charger == nil {
		log.Printf("[storage-biller] disabled (비크레딧 모드)")
		return
	}
	log.Printf("[storage-biller] started (interval=%s, live 단가)", interval)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.billStorageOnce(ctx)
		}
	}
}

func (s *Service) billStorageOnce(ctx context.Context) {
	if s.price() <= 0 {
		return // 단가 0(무료) — 이번 틱 과금 없음(런타임에 >0 로 바뀌면 다음 틱부터 과금)
	}
	vols, err := s.repo.ListBillable()
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for i := range vols {
		v := &vols[i]
		if v.OwnerUserID == nil {
			continue
		}
		elapsedHours := now.Sub(v.CreatedAt).Hours()
		if elapsedHours <= 0 {
			continue
		}
		// 세션 과금과 동일한 델타 회계: 누적 총액(내림) − 이미 청구액.
		totalDue := int(float64(s.monthlyCost(v.SizeGiB)) * elapsedHours / float64(hoursPerMonth))
		due := totalDue - v.BilledCredits
		if due <= 0 {
			continue
		}
		ref := fmt.Sprintf("vol-%d", v.ID)
		ok, err := s.charger.Consume(*v.OwnerUserID, gidPtr(v.GroupID), due, ref)
		if err != nil || !ok {
			continue // 잔액 부족 → 유예(다음 틱 재시도; 삭제하지 않음)
		}
		_ = s.repo.SetBilled(v.ID, totalDue)
	}
}
