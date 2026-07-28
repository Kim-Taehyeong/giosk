package node

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"giosk/internal/k8s"
	"giosk/internal/metrics"
)

// NodeOps는 K8s 노드 조작 계약(*k8s.Client 구현).
type NodeOps interface {
	ListNodes(ctx context.Context) ([]k8s.LiveNode, error)
	SetCordon(ctx context.Context, name string, on bool) error
	PVCBackingNFS(ctx context.Context, ns, name string) (server, path string, ok bool)
	SetNodeLabel(ctx context.Context, node, key, value string) error                         // value="" → 라벨 제거
	EnsureTimeSlicingKey(ctx context.Context, ns, name string, replicas int) (string, error) // device plugin 타임슬라이싱 프로파일 upsert
}

// Service는 node 비즈니스 로직(라이브 + 오버레이 + 지표 + 임대).
type Service struct {
	repo            Repository
	ops             NodeOps
	met             *metrics.Client
	nfsServer       string
	nfsPath         string
	scratchHostPath string                            // 노드로컬 스크래치 루트(/scratch). 노드별 활성은 DB scratch_enabled
	localHomeHost   string                            // 물리 로컬 home 루트(/home/giosk). 물리 SSH home = <root>/<user> (노드 로컬 디스크)
	uidBase         int                               // 물리 계정 UID = uidBase + userID(전역 안정, 재사용 안 함)
	dpConfigNS      string                            // device plugin 설정 ConfigMap 네임스페이스(빈값=자동적용 비활성 → 수동 운영)
	dpConfigName    string                            // device plugin 설정 ConfigMap 이름
	freeMode        bool                              // 자유 모드: 임대 영속(cordon 없음·만료 없음·해제 안 함) → 계정 재사용·동시접속
	cachedProvider  func() map[string][]CachedDataset // 노드별 캐시 데이터셋(세션 생성 노드 선호 표시). nil=빈 목록.
	physicalLabel   string                            // 물리노드 식별 라벨(기본 giosk.io/physical). 토글 시 이 라벨을 노드에 적용.
}

// WithPhysicalLabel은 물리노드 식별 라벨을 설정한다(물리 임대 토글이 이 라벨을 k8s 노드에 붙인다).
func (s *Service) WithPhysicalLabel(label string) *Service {
	if label != "" {
		s.physicalLabel = label
	}
	return s
}

// WithCachedDatasets는 노드별 캐시 완료 데이터셋 제공자를 주입한다(세션 생성 UI 에서 "이 노드에 있는 데이터셋" 표시).
func (s *Service) WithCachedDatasets(fn func() map[string][]CachedDataset) *Service {
	s.cachedProvider = fn
	return s
}

func NewService(repo Repository, ops NodeOps, met *metrics.Client, nfsServer, nfsPath string) *Service {
	return &Service{repo: repo, ops: ops, met: met, nfsServer: nfsServer, nfsPath: nfsPath, uidBase: 100000}
}

// WithScratch는 물리노드 스크래치 루트(/scratch)를 설정한다(노드별 활성은 DB scratch_enabled).
func (s *Service) WithScratch(hostPath string) *Service { s.scratchHostPath = hostPath; return s }

// WithLocalHomeHost는 물리 SSH 로컬 home 루트(/home/giosk)를 설정한다(home=<root>/<user>, 노드 로컬 디스크).
func (s *Service) WithLocalHomeHost(p string) *Service {
	if p != "" {
		s.localHomeHost = p
	}
	return s
}

// WithUIDBase는 물리 계정 UID 베이스를 설정한다(전역 안정 UID = base + userID).
func (s *Service) WithUIDBase(base int) *Service {
	if base > 0 {
		s.uidBase = base
	}
	return s
}

// WithFreeMode는 자유 모드를 켠다(임대 영속·cordon 없음·해제 없음 → 계정 재사용/동시접속).
func (s *Service) WithFreeMode(on bool) *Service { s.freeMode = on; return s }

// List는 라이브 노드에 DB 오버레이/임대/라이브 지표를 머지한다.
func (s *Service) List(ctx context.Context) ([]View, error) {
	live, err := s.ops.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	cfgs, err := s.repo.Configs()
	if err != nil {
		return nil, err
	}
	running := s.repo.RunningByNode()
	nm := s.nodeMetrics(ctx)
	out := make([]View, 0, len(live))
	for _, n := range live {
		out = append(out, s.mergeView(n, cfgs[n.Name], running[n.Name], nm[n.Name]))
	}
	return out, nil
}

func (s *Service) mergeView(n k8s.LiveNode, cfg Config, sessions int, m nodeMetric) View {
	mode := orStr(cfg.ShareMode, "exclusive")
	v := View{
		Name: n.Name, GpuType: n.GpuType, GpuCapacity: n.GpuCapacity, CudaVersion: n.CudaVersion,
		Ready: n.Ready, Cordoned: n.Cordoned, Physical: n.Physical,
		ShareMode: mode, SplitCount: orInt(cfg.SplitCount, 10),
		SSHEnabled:     cfg.SSHEnabled,
		CPUCores:       n.CPUCores,
		MemGB:          n.MemGB,
		Sessions:       sessions,
		DatasetQuotaGB: cfg.DatasetQuotaGB,
		PhysicalLease:  cfg.PhysicalLease,
		ScratchEnabled: cfg.ScratchEnabled,
		Hami:           mode == "hami" || mode == "fractional",
		Util:           m.Util,
		Temp:           m.Temp,
		VramUsed:       m.VramUsed,
		VramTotal:      m.VramTotal,
		CpuUtil:        m.CpuUtil,
		MemUtil:        m.MemUtil,
	}
	if user, ok := s.repo.ActiveLeaseUser(n.Name); ok {
		v.LeasedTo = user
	}
	return v
}

// nodeMetric은 노드 1대의 라이브 지표.
type nodeMetric struct {
	Util, Temp, VramUsed, VramTotal, CpuUtil, MemUtil int
}

// GPUStat은 노드 내 개별 GPU 1장의 라이브 지표(DCGM).
type GPUStat struct {
	Index      string `json:"index"`
	Util       int    `json:"util"`
	VramUsedMB int    `json:"vramUsedMb"`
	VramTotMB  int    `json:"vramTotalMb"`
	Temp       int    `json:"temp"`
}

// NodeGPUs는 노드의 GPU별 실측 지표(DCGM)를 반환한다. 미연동이면 빈 슬라이스.
func (s *Service) NodeGPUs(ctx context.Context, node string) []GPUStat {
	if s.met == nil || !s.met.Enabled() {
		return []GPUStat{}
	}
	join := fmt.Sprintf(` * on(pod,namespace) group_left(node) kube_pod_info{node=%q}`, node)
	util := s.byGPU(ctx, "DCGM_FI_DEV_GPU_UTIL"+join)
	used := s.byGPU(ctx, "DCGM_FI_DEV_FB_USED"+join)
	free := s.byGPU(ctx, "DCGM_FI_DEV_FB_FREE"+join)
	temp := s.byGPU(ctx, "DCGM_FI_DEV_GPU_TEMP"+join)
	out := make([]GPUStat, 0)
	for _, idx := range sortedGPUKeys(used, util) {
		out = append(out, GPUStat{
			Index: idx, Util: round(util[idx]),
			VramUsedMB: round(used[idx]), VramTotMB: round(used[idx] + free[idx]),
			Temp: round(temp[idx]),
		})
	}
	return out
}

func (s *Service) byGPU(ctx context.Context, q string) map[string]float64 {
	m, ok := s.met.VectorByLabel(ctx, q, "gpu")
	if !ok {
		return map[string]float64{}
	}
	return m
}

// nodeMetrics는 Prometheus(DCGM+node-exporter)에서 노드별 지표를 일괄 조회한다.
// pod→node 매핑은 kube_pod_info 조인으로 해결(DCGM/node-exporter 메트릭엔 node 라벨 없음).
// 미연동/미가용이면 빈 맵(0).
func (s *Service) nodeMetrics(ctx context.Context) map[string]nodeMetric {
	out := map[string]nodeMetric{}
	if s.met == nil || !s.met.Enabled() {
		return out
	}
	const join = " * on(pod,namespace) group_left(node) kube_pod_info"
	util := s.byNode(ctx, "avg by(node)(DCGM_FI_DEV_GPU_UTIL"+join+")")
	temp := s.byNode(ctx, "max by(node)(DCGM_FI_DEV_GPU_TEMP"+join+")")
	vUsed := s.byNode(ctx, "sum by(node)(DCGM_FI_DEV_FB_USED"+join+")/1024")
	vTotal := s.byNode(ctx, "sum by(node)((DCGM_FI_DEV_FB_USED+DCGM_FI_DEV_FB_FREE)"+join+")/1024")
	cpu := s.byNode(ctx, "100 - avg by(node)(rate(node_cpu_seconds_total{mode=\"idle\"}[2m])"+join+")*100")
	mem := s.byNode(ctx, "100 * (1 - sum by(node)(node_memory_MemAvailable_bytes"+join+") / sum by(node)(node_memory_MemTotal_bytes"+join+"))")
	for _, node := range keysOf(util, temp, vUsed, vTotal, cpu, mem) {
		out[node] = nodeMetric{
			Util: round(util[node]), Temp: round(temp[node]),
			VramUsed: round(vUsed[node]), VramTotal: round(vTotal[node]),
			CpuUtil: round(cpu[node]), MemUtil: round(mem[node]),
		}
	}
	return out
}

func (s *Service) byNode(ctx context.Context, q string) map[string]float64 {
	m, ok := s.met.VectorByLabel(ctx, q, "node")
	if !ok {
		return map[string]float64{}
	}
	return m
}

func round(f float64) int {
	if f < 0 {
		return 0
	}
	return int(f + 0.5)
}

// sortedGPUKeys는 GPU 인덱스 키("0".."N")를 숫자 순으로 정렬한 합집합.
func sortedGPUKeys(maps ...map[string]float64) []string {
	keys := keysOf(maps...)
	sort.Slice(keys, func(i, j int) bool { return atoiSafe(keys[i]) < atoiSafe(keys[j]) })
	return keys
}

// keysOf는 여러 맵의 노드 키 합집합.
func keysOf(maps ...map[string]float64) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range maps {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	return out
}

// PhysicalNodes는 사용자 화면용 물리 노드 목록(라이브 + DB 쿼터 오버레이 + 가용 GPU).
func (s *Service) PhysicalNodes(ctx context.Context) ([]UserNodeView, error) {
	live, err := s.ops.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	cfgs, _ := s.repo.Configs()
	running := s.repo.RunningByNode()
	var cached map[string][]CachedDataset
	if s.cachedProvider != nil {
		cached = s.cachedProvider()
	}
	out := make([]UserNodeView, 0)
	for _, n := range live {
		if !n.Physical {
			continue
		}
		out = append(out, s.mergeUserNode(n, cfgs[n.Name], running[n.Name], cached[n.Name]))
	}
	return out, nil
}

func (s *Service) mergeUserNode(n k8s.LiveNode, cfg Config, used int, cached []CachedDataset) UserNodeView {
	total := atoiSafe(n.GpuCapacity)
	free := total - used
	if free < 0 {
		free = 0
	}
	v := UserNodeView{
		Node: n.Name, Gpu: gpuLabel(n.GpuType, total),
		Available:      n.Ready && !n.Cordoned,
		GpuTotal:       total,
		GpuFree:        free,
		DatasetQuotaGb: cfg.DatasetQuotaGB,
		Cached:         cached,
	}
	if v.Cached == nil {
		v.Cached = []CachedDataset{}
	}
	if user, ok := s.repo.ActiveLeaseUser(n.Name); ok && user != "" {
		v.Available = false
	}
	return v
}

func gpuLabel(gpuType string, n int) string {
	if gpuType == "" {
		return fmt.Sprintf("GPU ×%d", n)
	}
	return fmt.Sprintf("%s ×%d", gpuType, n)
}

func atoiSafe(s string) int {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return n
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

func (s *Service) Cordon(ctx context.Context, name string, on bool) error {
	return s.ops.SetCordon(ctx, name, on)
}

// WithDevicePluginConfig는 device plugin 설정 ConfigMap 좌표를 주입한다(예: gpu-operator/time-slicing-config).
// 지정하면 관리자가 웹에서 타임셰어링을 켜는 즉시 프로파일 upsert + 노드 라벨 부여 → operator 가 슬롯 광고를 반영.
// 빈 값이면 자동 적용을 건너뛴다(수동 운영).
func (s *Service) WithDevicePluginConfig(ns, name string) *Service {
	s.dpConfigNS, s.dpConfigName = ns, name
	return s
}

// applyDevicePlugin은 공유 전략에 맞춰 노드의 device plugin 설정을 실제로 적용한다.
//   - timeslicing : 슬롯 수 프로파일을 ConfigMap 에 보장하고 노드에 라벨 부여 → GPU 가 N슬롯으로 광고됨.
//   - 그 외        : 라벨 제거 → 기본(1 GPU = 1) 설정으로 복귀.
//
// 슬롯 수는 이번 요청값이 없으면 기존 DB 설정을 쓴다(모드만 바꾸는 경우).
func (s *Service) applyDevicePlugin(node, mode string, split *int) {
	if s.dpConfigNS == "" || s.dpConfigName == "" {
		return // 자동 적용 비활성 — 운영자가 직접 device plugin 을 설정
	}
	ctx := context.Background()
	if mode != ShareTimeslicing {
		_ = s.ops.SetNodeLabel(ctx, node, k8s.DevicePluginConfigLabel, "")
		return
	}
	n := 0
	if split != nil {
		n = *split
	} else if cfgs, err := s.repo.Configs(); err == nil {
		n = cfgs[node].SplitCount
	}
	if n < 2 {
		n = 2 // 타임셰어링은 최소 2슬롯(1이면 전용과 같음)
	}
	key, err := s.ops.EnsureTimeSlicingKey(ctx, s.dpConfigNS, s.dpConfigName, n)
	if err != nil {
		log.Printf("[node] %s: device plugin 프로파일 생성 실패(%v) — 라벨 생략", node, err)
		return
	}
	_ = s.ops.SetNodeLabel(ctx, node, k8s.DevicePluginConfigLabel, key)
	log.Printf("[node] %s: 타임셰어링 %d슬롯 적용(device plugin config=%s)", node, n, key)
}

func (s *Service) SetConfig(name string, req ConfigReq) error {
	fields := map[string]any{}
	if req.SplitCount != nil {
		fields["split_count"] = *req.SplitCount
	}
	if req.ShareMode != nil {
		fields["share_mode"] = *req.ShareMode
		// 공유 전략을 노드 라벨로 노출 → 세션이 nodeSelector/affinity 로 올바른 노드를 고른다.
		// 특히 타임셰어링 노드는 GPU 를 N슬롯으로 광고하므로, 전용 요청이 잘못 떨어지면
		// 통 GPU 대신 슬롯만 받는다. 라벨이 그 오배치를 막는 유일한 수단.
		_ = s.ops.SetNodeLabel(context.Background(), name, ShareModeLabel, *req.ShareMode)
		s.applyDevicePlugin(name, *req.ShareMode, req.SplitCount)
	}
	if req.SSHEnabled != nil {
		fields["ssh_enabled"] = *req.SSHEnabled
	}
	if req.PhysicalLease != nil {
		fields["physical_lease"] = *req.PhysicalLease
		// DB 뿐 아니라 k8s 노드 라벨(giosk.io/physical)도 적용해야 물리 임대 목록(/nodes/physical)에 잡힌다.
		// 이 라벨이 곧 Physical 판정 기준이므로 토글=라벨 on/off.
		label := s.physicalLabel
		if label == "" {
			label = "giosk.io/physical"
		}
		val := ""
		if *req.PhysicalLease {
			val = "true"
		}
		_ = s.ops.SetNodeLabel(context.Background(), name, label, val)
	}
	if req.ScratchEnabled != nil {
		fields["scratch_enabled"] = *req.ScratchEnabled
		// 노드 라벨로 scratch-cleaner DaemonSet 배치 제어(켜면 true, 끄면 라벨 제거).
		val := ""
		if *req.ScratchEnabled {
			val = "true"
		}
		_ = s.ops.SetNodeLabel(context.Background(), name, "giosk.io/scratch", val)
	}
	if req.DatasetQuotaGB != nil {
		fields["dataset_quota_gb"] = *req.DatasetQuotaGB
	}
	return s.repo.UpsertConfig(name, fields)
}

// CreateLeaseFor는 물리(SSH) 세션용 임대를 만든다(instance_id 연결, 노드 cordon).
// 임대 시간 기본값은 lease 정책이 없으면 8시간.
func (s *Service) CreateLeaseFor(ctx context.Context, nodeName string, userID int64, instanceID string) error {
	now := time.Now().UTC()
	if s.freeMode {
		// 자유 모드: 노드는 공유(cordon 없음), 임대는 영속(만료 100년) → 계정 1회 생성 후 재사용·동시접속.
		// AcquireLease(exclusive=false)가 (node,user) 활성 임대를 원자적으로 재사용(멱등). 계정 삭제도 없음.
		l := &Lease{
			Node: nodeName, UserID: userID, InstanceID: instanceID, Status: LeaseActive,
			LeaseHours: 0, StartedAt: now, ExpiresAt: now.AddDate(100, 0, 0),
		}
		_, err := s.repo.AcquireLease(l, false)
		return err
	}
	// 선착순(비-자유): 노드 전용. AcquireLease(exclusive=true)가 갭락으로 직렬화하여
	// 거의 동시 요청 중 처음 한 건만 임대를 잡고, 나머지는 ErrNodeBusy.
	// 고정 만료시간은 두지 않는다 — 물리 임대 회수는 유휴 리퍼(노드 GPU 유휴)·수동 반납·크레딧
	// 잔액 소진이 담당한다. (예전의 8h 하드코딩은 이를 처리하는 리퍼가 없어 무의미했다.)
	l := &Lease{
		Node: nodeName, UserID: userID, InstanceID: instanceID, Status: LeaseActive,
		LeaseHours: 0, StartedAt: now, ExpiresAt: now.AddDate(100, 0, 0),
	}
	if _, err := s.repo.AcquireLease(l, true); err != nil {
		return err // ErrNodeBusy 포함
	}
	_ = s.ops.SetCordon(ctx, nodeName, true) // 물리 임대 = 노드 전용(컨테이너 스케줄 차단), best-effort
	return nil
}

// ReleaseLeaseFor는 instance_id 로 임대를 해제하고 노드 cordon 을 푼다.
// 자유 모드에서는 계정 영속이 목적이므로 no-op(임대 유지 → node-agent 가 계정 삭제하지 않음).
func (s *Service) ReleaseLeaseFor(ctx context.Context, instanceID string) error {
	if s.freeMode {
		return nil
	}
	node, err := s.repo.ReleaseByInstance(instanceID)
	if err != nil {
		return err
	}
	if node != "" {
		_ = s.ops.SetCordon(ctx, node, false)
	}
	return nil
}

// CreateLease는 임대를 만들고 노드를 cordon(스케줄 차단)한다.
func (s *Service) CreateLease(ctx context.Context, nodeName string, req LeaseReq) (*Lease, error) {
	now := time.Now().UTC()
	l := &Lease{
		Node: nodeName, UserID: req.UserID, Status: LeaseActive,
		LeaseHours: req.LeaseHours, StartedAt: now, ExpiresAt: now.Add(time.Duration(req.LeaseHours) * time.Hour),
	}
	if err := s.repo.CreateLease(l); err != nil {
		return nil, err
	}
	_ = s.ops.SetCordon(ctx, nodeName, true) // best-effort
	return l, nil
}

func (s *Service) Extend(leaseID, addHours int64) error {
	return s.repo.ExtendLease(leaseID, int(addHours))
}

// Release는 임대를 해제하고 노드 cordon 을 푼다.
func (s *Service) Release(ctx context.Context, leaseID int64) error {
	l, err := s.repo.GetLease(leaseID)
	if err != nil {
		return err
	}
	if err := s.repo.ReleaseLease(leaseID); err != nil {
		return err
	}
	return s.ops.SetCordon(ctx, l.Node, false)
}

// AgentLeases는 node-agent reconcile 용 임대 목록(홈 NFS + 추가/공유 볼륨 마운트 포함).
// 홈은 고정 NFS, 추가 볼륨은 각 볼륨 PVC 의 백엔드 NFS 경로를 읽어 직접 마운트(RW/RO 반영).
func (s *Service) AgentLeases(ctx context.Context, nodeName string) ([]AgentLease, error) {
	leases, err := s.repo.AgentLeases(nodeName)
	if err != nil {
		return nil, err
	}
	cfgs, _ := s.repo.Configs()
	scratchOn := s.scratchHostPath != "" && cfgs[nodeName].ScratchEnabled // 노드별 스크래치 활성
	homeBase := s.localHomeHost
	if homeBase == "" {
		homeBase = "/home/giosk"
	}
	for i := range leases {
		// home 은 노드 로컬 디스크(useradd -m -d). NFSServer/NFSPath 는 ~/nfs(개인 영속) base 로 전달.
		home := homeBase + "/" + leases[i].Username
		leases[i].UID = s.uidBase + int(leases[i].UserID) // 전역 안정 UID(재사용 안 함)
		leases[i].NFSServer = s.nfsServer                 // ~/nfs base export (예: 192.168.0.10:/srv/nfs/home)
		leases[i].NFSPath = s.nfsPath
		leases[i].MountPath = home
		mounts := s.leaseVolumes(ctx, leases[i].InstanceID, home)
		mounts = append(mounts, s.leaseDatasets(ctx, leases[i].InstanceID, home)...) // 데이터셋 RO 마운트
		leases[i].Volumes = mounts
		if scratchOn {
			leases[i].Scratch = s.scratchHostPath + "/" + leases[i].Username // 계정 폴더(RWX 본인만)
		}
	}
	return leases, nil
}

// leaseDatasets는 물리세션 데이터셋을 home/datasets/slow/<name> 에 RO NFS 마운트로 변환한다.
// 물리노드는 NFS 직접 마운트(=slow tier)이며, 컨테이너 세션의 ~/datasets/slow 와 경로 규칙을 통일한다.
func (s *Service) leaseDatasets(ctx context.Context, instanceID, home string) []AgentVolMount {
	var out []AgentVolMount
	for _, ld := range s.repo.LeaseDatasets(instanceID) {
		server, path, ok := s.ops.PVCBackingNFS(ctx, ld.PVCNamespace, ld.PVCName)
		if !ok {
			continue
		}
		out = append(out, AgentVolMount{
			NFSServer: server, NFSPath: path,
			MountPath: home + "/datasets/slow/" + dsMountName(ld.Name),
			ReadOnly:  true, // 데이터셋은 항상 읽기전용
		})
	}
	return out
}

// dsMountName은 데이터셋 이름을 마운트 경로 세그먼트로 정규화(공백/슬래시→'-').
func dsMountName(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range name {
		if r == '/' || r == ' ' || r == '\\' {
			out = append(out, '-')
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return "dataset"
	}
	return string(out)
}

// leaseVolumes는 물리세션 볼륨을 NFS 직접 마운트 스펙으로 변환한다(home 트리 하위에 마운트).
func (s *Service) leaseVolumes(ctx context.Context, instanceID, home string) []AgentVolMount {
	var out []AgentVolMount
	for _, lv := range s.repo.LeaseVolumes(instanceID) {
		server, path, ok := s.ops.PVCBackingNFS(ctx, lv.PVCNamespace, lv.PVCName)
		if !ok {
			continue // NFS 백엔드가 아니면 물리노드 직접 마운트 불가(전용 스토리지)
		}
		out = append(out, AgentVolMount{
			NFSServer: server, NFSPath: path,
			MountPath: home + ensureLeadingSlash(lv.MountPath),
			ReadOnly:  lv.Perm == "ro",
		})
	}
	return out
}

func ensureLeadingSlash(p string) string {
	if p == "" {
		return "/data"
	}
	if p[0] != '/' {
		return "/" + p
	}
	return p
}

func orStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
func orInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}
