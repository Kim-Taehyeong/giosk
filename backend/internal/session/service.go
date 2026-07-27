package session

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"giosk/internal/audit"
	"giosk/internal/gateway"
	"giosk/internal/k8s"
	"giosk/internal/metrics"
	"giosk/internal/policy"
	"giosk/pkg/idgen"
)

// idleCPUThreshold는 CPU 세션의 유휴 판정 CPU rate(코어). 이 미만이면 유휴로 본다.
const idleCPUThreshold = 0.05

// idleGPUThreshold는 GPU 세션의 유휴 판정 GPU 사용률(%). 이 미만이면 유휴로 본다.
// GPU 대여 세션은 GPU 점유가 목적이므로 CPU 가 아니라 GPU util(DCGM)로만 유휴를 판정한다.
const idleGPUThreshold = 5.0

// AuditReader는 세션 대상 감사 로그 조회/기록(audit.Repository 가 구현).
type AuditReader interface {
	ListByTarget(target string, limit int) ([]audit.Log, error)
	Insert(l *audit.Log) error
}

// Charger는 세션 사용량을 (user,team) 멤버십 지갑에서 차감한다(wallet.Service 가 구현).
// groupID=세션이 뜬 팀(네임스페이스). 잔액 부족이면 (false, nil).
type Charger interface {
	Consume(userID, groupID int64, credits int, ref string) (bool, error)
	Balance(userID, groupID int64) int // 세션 생성 전 잔액 검사용
}

// NodeLeaser는 물리노드 임대 생성/해제(node.Service 가 구현).
// 물리(SSH) 세션은 Pod 대신 노드 임대를 만들고 node-agent 가 물리 Home 에 프로비저닝한다.
type NodeLeaser interface {
	CreateLeaseFor(ctx context.Context, node string, userID int64, instanceID string) error
	ReleaseLeaseFor(ctx context.Context, instanceID string) error
}

// Provisioner는 K8s 프로비저닝 계약(*k8s.Client 가 구현). 테스트 시 fake 주입 가능.
type Provisioner interface {
	EnsureNamespace(ctx context.Context, ns string) error
	// 사용자 등록 SSH 공개키를 authorized_keys Secret 으로 반영(생성/갱신). sshd 사이드카가 마운트해
	// 접속마다 다시 읽으므로, 실행 중 세션에도 키 등록/교체가 즉시 반영된다.
	UpsertUserKeys(ctx context.Context, ns string, userID int64, keys string) error
	CreateSessionPod(ctx context.Context, s k8s.SessionSpec) error
	DeleteSessionPod(ctx context.Context, ns, name string) error
	PodStatus(ctx context.Context, ns, name string) (*k8s.PodStatus, error)
	PodLogs(ctx context.Context, ns, name string, tail int64) (string, error)
	PodDescribe(ctx context.Context, ns, name string) (*k8s.PodDescribe, error)
	CreatePVC(ctx context.Context, spec k8s.PVCSpec) error
	PVCPhase(ctx context.Context, ns, name string) (string, error)
	PVCBackingNFS(ctx context.Context, ns, name string) (server, path string, ok bool)
	EnsureSharedNFSPVC(ctx context.Context, spec k8s.SharedNFSSpec) error
	DeleteSharedNFSPVC(ctx context.Context, ns, name string) error
	EnsureSessionService(ctx context.Context, s k8s.SvcSpec) error
	DeleteSessionService(ctx context.Context, ns, name string) error
	SessionServiceAccess(ctx context.Context, ns, name, mode string) (k8s.SvcAccess, error)
	FirstNodeIP(ctx context.Context) string
	ListNodes(ctx context.Context) ([]k8s.LiveNode, error) // 데이터셋 캐시 노드↔GPU타입 매칭용
	ExecTerminal(ctx context.Context, ns, pod, container string, cmd []string, tio k8s.ExecIO) error // 웹터미널(컨테이너 exec)
}

// DatasetCacheReader는 데이터셋 노드 로컬 캐시 배치를 조회한다(dataset.Service 구현).
// datasetID → (캐시 완료 노드 목록, 노드 로컬 경로). 빈 맵이면 캐시 비활성 → 전부 NFS 마운트.
type DatasetCacheReader interface {
	DatasetCachePlacement(ids []int64) (cachedNodes map[int64][]string, hostPaths map[int64]string)
}

// Service는 session 비즈니스 로직.
type Service struct {
	repo         Repository
	prov         Provisioner
	nsPrefix     string
	gateway      string
	storageClass string // 홈 PVC 스토리지클래스(스토리지 단일화: NFS RWX)
	audit        AuditReader
	met          *metrics.Client
	leaser       NodeLeaser
	charger      Charger          // 크레딧 소비 회계(nil=과금 비활성)
	limits       *policy.Resolver // 하드 리소스 상한(계층 해석; nil=미강제)
	expose       string           // 세션 웹 노출 모드(nodeport|loadbalancer)

	surgeDynamic   bool                                                        // 동적(서지) 가격 활성
	surgeIncrement int                                                         // 최대 가산 크레딧/시간(가용성 0일 때)
	availFn        func(ctx context.Context, gpuType string) (free, total int) // GPU 타입별 가용 조회

	scratchEnabled  bool   // 노드로컬 스크래치 마운트 활성
	scratchHostPath string // 스크래치 루트(/scratch). 계정폴더 = <root>/<username>

	localHomeOn   bool   // 물리노드 로컬 Home 특수 볼륨 선택 허용(물리 활성 시)
	localHomeHost string // 물리노드 로컬 home 루트(hostPath). 계정폴더 = <root>/<username>
	uidBase       int    // 컨테이너 안정 UID = uidBase + userID(물리 SSH 와 동일 공식). 기본 100000.
	sharedHome    bool   // 영속 home(~/nfs) 사용. false 면 세션은 emptyDir 로컬 임시만(RWX 불필요).

	datasetCache DatasetCacheReader // 데이터셋 노드 로컬 캐시 배치 조회(nil=항상 NFS)

	dynamicLease  bool // 선착순(dynamic) 모드 — 임대 연장 허용
	maxExtensions int  // 임대 연장 최대 횟수
	leaseMaxHours int  // 1회 기본 임대 시간(시간)
	leaseExtHours int  // 연장 1회당 추가 시간(시간)

	// 접속 게이트웨이(단기 토큰 발급) — 설정 시 웹/SSH 접속을 게이트웨이 단일 접점으로 라우팅.
	gatewaySecret  []byte // API↔게이트웨이 공유 토큰키(GIOSK_GATEWAY_SECRET). 빈값=게이트웨이 비활성(직접 접속 폴백).
	gatewayScheme  string // 게이트웨이 웹 URL 스킴(https|http)
	gatewayHostArg string // SSH 접속 호스트(빈값=gateway 도메인)
	gatewaySSHPort int    // 게이트웨이 SSH 프록시 포트(기본 2222)
	sshdImage      string // 컨테이너 세션 sshd 사이드카 이미지(빈값=컨테이너 SSH 비활성)
	sshdPubKey     string // sshd 사이드카가 신뢰할 게이트웨이 공개키(authorized_keys)
	gatewayJump    string // 외부(VPN 밖) 접속용 SSH 점프 호스트(user@host). 빈값=내부 명령만.
	gatewaySSHKey  []byte // 게이트웨이 SSH 관리 개인키(PEM). 물리 세션 웹터미널이 노드로 SSH 할 때 사용(빈값=물리 웹터미널 비활성).
}

// WithGatewaySSHKey는 물리 세션 웹터미널용 게이트웨이 SSH 관리 개인키를 주입한다(빈값=물리 웹터미널 비활성).
// API 가 이 키로 물리 노드의 사용자 계정에 SSH 하며, 노드 authorized_keys 는 이미 게이트웨이 공개키를 신뢰한다.
func (s *Service) WithGatewaySSHKey(pemKey []byte) *Service {
	s.gatewaySSHKey = pemKey
	return s
}

// WithGatewayProxyJump는 외부 접속용 SSH 점프 호스트를 설정한다(빈값=미표시).
// 게이트웨이 SSH 는 MetalLB LB IP(사내망)로 열리므로, VPN 밖 사용자에겐 -J 점프 명령을 함께 준다.
func (s *Service) WithGatewayProxyJump(jump string) *Service { s.gatewayJump = jump; return s }

// WithSurge는 동적/서지 가격(가용성↓→단가↑)을 설정한다. dynamic=false면 정적 단가.
func (s *Service) WithSurge(dynamic bool, increment int, avail func(ctx context.Context, gpuType string) (int, int)) *Service {
	s.surgeDynamic, s.surgeIncrement, s.availFn = dynamic, increment, avail
	return s
}

// WithScratch는 노드로컬 스크래치(hostPath) 세션 마운트를 설정한다.
func (s *Service) WithScratch(enabled bool, hostPath string) *Service {
	s.scratchEnabled, s.scratchHostPath = enabled, hostPath
	return s
}

// WithExpose는 세션 웹 노출 모드를 설정한다.
func (s *Service) WithExpose(mode string) *Service { s.expose = mode; return s }

// WithGateway는 접속 게이트웨이(단기 토큰 발급)를 설정한다. secret 빈값이면 게이트웨이 비활성.
//   - scheme: 웹 URL 스킴(https 권장; 테스트베드 HTTP 는 "http").
//   - sshHost: SSH 접속 호스트(빈값=gateway 도메인). sshPort: 게이트웨이 SSH 프록시 포트.
//   - sshdImage: 컨테이너 sshd 사이드카 이미지(빈값=컨테이너 SSH 비활성). sshdPubKey: 사이드카가 신뢰할 게이트웨이 공개키.
func (s *Service) WithGateway(secret, scheme, sshHost string, sshPort int, sshdImage, sshdPubKey string) *Service {
	s.gatewaySecret = []byte(secret)
	s.gatewayScheme = scheme
	s.gatewayHostArg = sshHost
	s.gatewaySSHPort = sshPort
	s.sshdImage = sshdImage
	s.sshdPubKey = sshdPubKey
	return s
}

// containerSSH는 컨테이너 세션 SSH(사이드카)가 활성인지 여부.
func (s *Service) containerSSH() bool { return s.sshdImage != "" }

// SyncUserKeys는 사용자가 SSH 공개키를 등록/교체/삭제했을 때, 그 사용자의 활성 컨테이너 세션이
// 있는 네임스페이스마다 authorized_keys Secret 을 갱신한다. sshd 는 접속마다 파일을 다시 읽으므로
// 실행 중인 세션도 재시작 없이 새 키로 붙고, 지운 키는 즉시 막힌다.
// auth.KeySyncer 구현 — 클러스터 미가용/세션 없음은 정상(무동작).
func (s *Service) SyncUserKeys(ctx context.Context, userID int64) error {
	if !s.containerSSH() {
		return nil
	}
	list, err := s.repo.ListByUser(userID)
	if err != nil {
		return err
	}
	keys := s.repo.UserSSHKey(userID)
	seen := map[string]bool{}
	var firstErr error
	for i := range list {
		sess := &list[i]
		if sess.Env == "ssh" || (sess.Phase != PhaseRunning && sess.Phase != PhaseProvisioning) {
			continue
		}
		ns := s.namespaceOf(sess)
		if seen[ns] {
			continue
		}
		seen[ns] = true
		if err := s.prov.UpsertUserKeys(ctx, ns, userID, keys); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// svcPorts는 세션 Service 가 노출할 포트 목록(웹 채널 + 컨테이너 sshd 22)을 만든다.
func (s *Service) svcPorts(channels []k8s.WebChannelSpec) []k8s.SvcPort {
	ports := make([]k8s.SvcPort, 0, len(channels)+1)
	for _, ch := range channels {
		name := ch.Name
		if name == "vscode" {
			name = "web" // 하위호환(기존 primary 포트명)
		}
		ports = append(ports, k8s.SvcPort{Name: name, Port: ch.Port})
	}
	if s.containerSSH() {
		ports = append(ports, k8s.SvcPort{Name: "ssh", Port: 22})
	}
	return ports
}

// WithLocalHome은 물리노드 로컬 Home 특수 볼륨(컨테이너 선택→hostPath+노드핀)을 설정한다.
func (s *Service) WithLocalHome(on bool, hostPath string) *Service {
	s.localHomeOn, s.localHomeHost = on, hostPath
	return s
}

// WithUIDBase는 컨테이너 안정 UID 베이스를 설정한다(UID=base+userID, 물리 SSH 와 동일). 0=기본 100000.
func (s *Service) WithUIDBase(base int) *Service {
	if base > 0 {
		s.uidBase = base
	}
	return s
}

// WithDatasetCache는 데이터셋 노드 로컬 캐시 배치 조회기를 주입한다(캐시 노드 hostPath 마운트용).
func (s *Service) WithDatasetCache(r DatasetCacheReader) *Service { s.datasetCache = r; return s }

// WithCharger는 소비 회계용 지갑 차감자를 주입한다(크레딧 모드).
func (s *Service) WithCharger(c Charger) *Service { s.charger = c; return s }

// checkAffordable는 크레딧 모드에서 잔액이 최소 1시간 단가 이상인지 검사한다.
// 비크레딧 모드(charger nil)·무료(단가 0) 세션은 통과. 부족하면 ErrInsufficientCredit.
// (생성 시 막지 않으면 1크레딧 누적 전까지 0잔액으로도 잠시 돌아가는 구멍이 생김.)
// gidOf는 세션의 팀 id(nil=0)를 반환한다(정산/잔액검사 대상 팀 지갑).
func gidOf(sess *Session) int64 {
	if sess.GroupID != nil {
		return *sess.GroupID
	}
	return 0
}

func (s *Service) checkAffordable(userID, groupID int64, pricePerHour int) error {
	if s.charger == nil || pricePerHour <= 0 {
		return nil
	}
	if s.charger.Balance(userID, groupID) < pricePerHour {
		return ErrInsufficientCredit
	}
	return nil
}

// WithLeaser는 물리(SSH) 세션용 노드 임대자를 주입한다.
func (s *Service) WithLeaser(l NodeLeaser) *Service { s.leaser = l; return s }

// WithMetrics는 유휴 판정용 Prometheus 클라이언트를 주입한다.
func (s *Service) WithMetrics(m *metrics.Client) *Service { s.met = m; return s }


// WithLimits는 하드 리소스 상한(계층 해석)을 주입한다. 크레딧과 무관하게 항상 강제되는 1차 정책.
func (s *Service) WithLimits(r *policy.Resolver) *Service { s.limits = r; return s }

// checkHardLimits는 계층 해석된 하드 상한(GPU 개수·VRAM·동시 세션)을 강제한다(모든 과금 모드 공통).
// 0 = 무제한. 크레딧 검사(checkAffordable)보다 먼저 호출되는 절대 벽.
func (s *Service) checkHardLimits(userID int64, sess *Session) error {
	if s.limits == nil {
		return nil
	}
	lim := s.limits.Resolve(userID)
	if lim.MaxConcurrentSessions > 0 && s.repo.CountActive(userID) >= lim.MaxConcurrentSessions {
		return ErrSessionLimit
	}
	if lim.MaxGpu > 0 && sess.GpuCount > lim.MaxGpu {
		return ErrHardLimit
	}
	if lim.MaxVramGB > 0 && sess.VramMB/1024 > lim.MaxVramGB {
		return ErrHardLimit
	}
	return nil
}

// WithDynamicLease는 선착순(dynamic) 임대(연장·만료)를 활성화한다.
func (s *Service) WithDynamicLease(maxLeaseHours, extensionHours, maxExtensions int) *Service {
	s.dynamicLease = true
	s.leaseMaxHours, s.leaseExtHours, s.maxExtensions = maxLeaseHours, extensionHours, maxExtensions
	return s
}

// Extend는 선착순(dynamic) 세션의 임대를 연장한다(상한 내 연장 횟수 +1).
func (s *Service) Extend(ctx context.Context, instanceID string, userID int64) error {
	if !s.dynamicLease {
		return ErrExtendUnavailable // 선착순 모드에서만 연장 개념 존재
	}
	ok, err := s.repo.IncrementExtensions(instanceID, userID, s.maxExtensions)
	if err != nil {
		return err
	}
	if !ok {
		return ErrExtendUnavailable // 한도 도달 또는 세션 없음
	}
	return nil
}

func NewService(repo Repository, prov Provisioner, nsPrefix, gateway, storageClass string) *Service {
	return &Service{repo: repo, prov: prov, nsPrefix: nsPrefix, gateway: gateway, storageClass: storageClass, uidBase: 100000, sharedHome: true}
}

// WithSharedHome은 영속 home(~/nfs) 사용 여부를 설정한다(설치시 고정). false=세션 순수 로컬 임시.
func (s *Service) WithSharedHome(on bool) *Service { s.sharedHome = on; return s }

// WithAudit는 세션 감사 로그 조회용 리더를 주입한다.
func (s *Service) WithAudit(r AuditReader) *Service { s.audit = r; return s }

// Audit는 세션 소유자에게 해당 세션 대상 감사 로그를 반환한다.
func (s *Service) Audit(instanceID string, userID int64) ([]audit.Log, error) {
	if _, err := s.repo.Get(instanceID, userID); err != nil {
		return nil, err
	}
	if s.audit == nil {
		return []audit.Log{}, nil
	}
	return s.audit.ListByTarget(instanceID, 100)
}

const homeSizeGiB = 10 // 세션 홈(/home/work) 영속 용량 기본값

// Create는 스펙을 확정하고 Pod 를 프로비저닝한 뒤 세션을 기록한다.
func (s *Service) Create(ctx context.Context, userID int64, username string, req CreateReq) (*Session, error) {
	// 동시 세션 상한은 checkHardLimits(정책 계층 해석)에서만 강제한다.
	// billing.credit.maxConcurrentSessions 는 폐기 — 동시세션은 정책(quota)으로 일원화.
	if req.Env == "ssh" {
		return s.createSSH(ctx, userID, username, req)
	}
	sess, err := s.buildSession(ctx, userID, req)
	if err != nil {
		return nil, err
	}
	if err := s.checkHardLimits(userID, sess); err != nil {
		return nil, err // 1차: 하드 정책(크레딧 무관 절대 벽)
	}
	if err := s.checkAffordable(userID, gidOf(sess), sess.PricePerHour); err != nil {
		return nil, err // 2차: 크레딧 모드 잔액 부족 → 생성 거부
	}
	imageRef, err := s.repo.ImageRef(req.ImageID)
	if err != nil {
		return nil, err
	}
	ns := s.namespaceOf(sess)
	mounts := s.resolveMounts(ctx, ns, userID, req)
	// 통합 home 모델: home(/home/work)은 항상 노드 로컬(임시). 기본 emptyDir,
	// 로컬 Home 선택 시 그 물리노드 디스크(hostPath, 노드핀). 개인 영속 저장소는 ~/nfs(NFS PVC)로 별도 마운트.
	// → 어디서나 일관: home=로컬(임시), ~/nfs=영속. (컨테이너·물리 SSH 동일 규칙)
	var requireNode string
	if req.LocalHomeNode != "" {
		lh, err := s.resolveLocalHome(ctx, userID, username, req.LocalHomeNode)
		if err != nil {
			return nil, err
		}
		mounts = append(mounts, lh) // 물리노드 로컬 디스크 home → /home/work (노드핀)
		requireNode = req.LocalHomeNode
	} else {
		mounts = append(mounts, k8s.VolMountSpec{EmptyDir: true, MountPath: homeMount}) // 노드 로컬 임시 home
	}
	// 개인 영속 NFS 저장소 → ~/nfs (세션이 사라져도 유지; 노드 무관 이식성).
	// sharedHome=false 면 영속 home 미사용 → ~/nfs 마운트 생략(세션은 emptyDir 로컬 임시만).
	if s.sharedHome {
		persistPVC, err := s.ensureHome(ctx, ns, userID)
		if err != nil {
			return nil, err
		}
		mounts = append(mounts, k8s.VolMountSpec{PVCName: persistPVC, MountPath: homeMount + "/nfs"})
	}
	// 데이터셋은 승인·적재완료된 전체를 모든 세션에 자동 마운트한다(사용자 선택 불필요).
	// 로컬 캐시된 노드가 있으면 그 노드로 핀하고 hostPath(빠름)로, 아니면 NFS(느림) 자동.
	// 단, 로컬 Home 으로 이미 노드가 고정됐으면 그 노드 기준으로 데이터셋 배치를 판정한다.
	req.Datasets = s.repo.MountableDatasetIDs()
	dsTarget, dsCached, dsHostPath := requireNode, map[int64][]string(nil), map[int64]string(nil)
	if requireNode == "" {
		dsTarget, dsCached, dsHostPath = s.pickDatasetNode(ctx, req.Datasets, sess.GpuType, sess.GpuMode)
		requireNode = dsTarget
	} else if s.datasetCache != nil {
		dsCached, dsHostPath = s.datasetCache.DatasetCachePlacement(req.Datasets)
	}
	mounts = append(mounts, s.resolveDatasets(ctx, ns, req.Datasets, dsTarget, dsCached, dsHostPath)...)
	// 이미지 캐시 노드를 소프트 선호(빠른 시작). nodeSelector(GPU 타입) 안에서만 효과 → 타입 일치 캐시노드 우선.
	var preferNodes []string
	if req.ImageID != 0 {
		preferNodes = s.repo.CachedNodes(req.ImageID)
	}
	if err := s.provision(ctx, ns, sess, imageRef, "", mounts, preferNodes, requireNode); err != nil {
		return nil, err // home 은 mounts(emptyDir/hostPath)로 들어가므로 HomePVC 는 비움
	}
	if err := s.repo.Create(sess); err != nil {
		return nil, err
	}
	s.attachMounts(sess.ID, userID, req)
	s.recordCreate(userID, username, sess.InstanceID)
	return sess, nil
}

// createSSH는 물리노드 임대 세션을 만든다(Pod/PVC 없음; node-agent 가 물리 Home 프로비저닝).
func (s *Service) createSSH(ctx context.Context, userID int64, username string, req CreateReq) (*Session, error) {
	if s.leaser == nil {
		return nil, ErrLeaseUnavailable
	}
	if req.Node == "" {
		return nil, ErrNodeRequired
	}
	_ = s.repo.SetUserSSHKey(userID, req.SSHPublicKey) // node-agent 가 users.ssh_public_key 로 주입
	now := s.now()
	// 물리노드 대여 = 노드 통째 점유. 단가 = gpu_pricing[노드 GPU타입] × 노드 GPU수(관리자 '단가' 탭에서 설정).
	gpuType, gpuCount := s.nodeSpec(ctx, req.Node)
	sess := &Session{
		InstanceID: idgen.Token("ses-", 5),
		UserID:     userID, GroupID: req.GroupID, Name: orDefault(req.Name, "ssh-session"),
		Env: "ssh", Node: req.Node, GpuMode: "exclusive",
		GpuType: gpuType, GpuCount: gpuCount,
		Phase:     PhaseRunning, // 임대 즉시 사용 가능(node-agent reconcile ~10s) → 과금 대상
		StartedAt: &now,
	}
	sess.PricePerHour = s.priceOf(ctx, sess) // 노드 대여 단가(=GPU타입 단가×노드 GPU수)
	if err := s.checkHardLimits(userID, sess); err != nil {
		return nil, err // 1차: 하드 정책(노드 GPU수가 상한 초과면 임대 거부)
	}
	if err := s.checkAffordable(userID, gidOf(sess), sess.PricePerHour); err != nil {
		return nil, err // 2차: 크레딧 모드 잔액 부족 → 물리 임대 거부
	}
	if err := s.leaser.CreateLeaseFor(ctx, req.Node, userID, sess.InstanceID); err != nil {
		return nil, err
	}
	if err := s.repo.Create(sess); err != nil {
		return nil, err
	}
	s.attachMounts(sess.ID, userID, req)
	s.recordCreate(userID, username, sess.InstanceID)
	return sess, nil
}

func (s *Service) recordCreate(userID int64, username, instanceID string) {
	if s.audit == nil {
		return
	}
	uid := userID
	_ = s.audit.Insert(&audit.Log{ActorID: &uid, ActorUsername: username, Action: "session_create", Target: instanceID, Result: "success"})
}

// ensureHome은 사용자 홈 PVC(/home/work 영속)를 멱등 생성하고 이름을 반환한다.
// 하이브리드(물리)면 NFS 기반 RWX, 컨테이너면 설정 스토리지클래스 RWO.
func (s *Service) ensureHome(ctx context.Context, ns string, userID int64) (string, error) {
	name := fmt.Sprintf("home-%d", userID)
	// 영속 home(~/nfs)은 사용자의 모든 세션이 공유하는 노드 무관 저장소 → 항상 RWX.
	// (RWO/local-path 면 PV 가 노드에 고정돼, 다른 GPU 타입 노드의 세션이 "volume node affinity conflict"로
	//  스케줄 불가하거나 동시 세션이 multi-attach 로 막힌다. NFS 등 RWX 스토리지클래스 필요.)
	err := s.prov.CreatePVC(ctx, k8s.PVCSpec{
		Namespace: ns, Name: name, SizeGiB: homeSizeGiB,
		StorageClass: s.storageClass, AccessMode: "RWX",
	})
	if err != nil {
		return "", err
	}
	return name, nil
}

// resolveLocalHome은 로컬 Home 특수 볼륨 선택을 검증하고 hostPath 홈 마운트(/home/work)로 변환한다.
//   - 물리 비활성/미설정 → ErrLocalHomeUnavailable
//   - 사용자가 그 노드를 대여한 적 없으면 → ErrLocalHomeForbidden(난립 방지·보안)
//   - 노드가 Ready 아니면 → ErrLocalHomeUnavailable(가용성 판단)
func (s *Service) resolveLocalHome(ctx context.Context, userID int64, username, node string) (k8s.VolMountSpec, error) {
	if !s.localHomeOn || s.localHomeHost == "" {
		return k8s.VolMountSpec{}, ErrLocalHomeUnavailable
	}
	if !s.repo.UserLeasedNode(userID, node) {
		return k8s.VolMountSpec{}, ErrLocalHomeForbidden
	}
	if !s.nodeReady(ctx, node) {
		return k8s.VolMountSpec{}, ErrLocalHomeUnavailable
	}
	return k8s.VolMountSpec{HostPath: s.localHomeHost + "/" + username, MountPath: homeMount}, nil
}

// nodeSpec은 노드의 GPU 타입·개수를 라이브 인벤토리에서 조회한다(물리 대여 단가 산정용).
func (s *Service) nodeSpec(ctx context.Context, node string) (gpuType string, count int) {
	live, err := s.prov.ListNodes(ctx)
	if err != nil {
		return "", 0
	}
	for _, n := range live {
		if n.Name == node {
			c, _ := strconv.Atoi(n.GpuCapacity)
			return n.GpuType, c
		}
	}
	return "", 0
}

// nodeReady는 노드가 라이브 인벤토리에서 Ready 인지 확인한다(가용성 판단).
func (s *Service) nodeReady(ctx context.Context, node string) bool {
	live, err := s.prov.ListNodes(ctx)
	if err != nil {
		return false
	}
	for _, n := range live {
		if n.Name == node {
			return n.Ready
		}
	}
	return false
}

// resolveMounts는 요청 볼륨을 세션 네임스페이스의 PVC 마운트로 해석한다(권한 서버 강제).
func (s *Service) resolveMounts(ctx context.Context, ns string, userID int64, req CreateReq) []k8s.VolMountSpec {
	var out []k8s.VolMountSpec
	for _, v := range req.Volumes {
		if m, ok := s.mountFor(ctx, ns, userID, v.ID, v.MountPath); ok {
			out = append(out, m)
		}
	}
	return out
}

// mountFor는 볼륨 1건을 세션 ns PVC 마운트로 해석한다.
//   - 권한: volume_shares 기반 서버 판정(소유=rw, 공유=ro/rw). 권한 없으면 생략.
//   - 같은 ns(내 볼륨): 존재하는 PVC 만 마운트(없으면 생략 → "무한 준비중" 방지).
//   - 다른 ns(공유 볼륨): NFS 백엔드를 같은 경로로 세션 ns 에 정적 복제 후 마운트(전용 스토리지면 불가).
func (s *Service) mountFor(ctx context.Context, ns string, userID, volID int64, mountPath string) (k8s.VolMountSpec, bool) {
	acc, ok := s.repo.VolumeAccess(volID, userID)
	if !ok || acc.PVCName == "" {
		log.Printf("[session] 볼륨 %d 접근권한 없음(user %d) — 마운트 생략", volID, userID)
		return k8s.VolMountSpec{}, false
	}
	ro := acc.Perm == "ro"
	pvc := acc.PVCName
	switch {
	case acc.PVCNamespace == ns: // 내 볼륨 — 그대로 사용
		if _, err := s.prov.PVCPhase(ctx, ns, pvc); err != nil {
			log.Printf("[session] 볼륨 %d: PVC %s/%s 없음(%v) — 마운트 생략", volID, ns, pvc, err)
			return k8s.VolMountSpec{}, false
		}
	default: // 공유 볼륨(다른 ns) — NFS 경로를 세션 ns 에 정적 복제
		server, path, isNFS := s.prov.PVCBackingNFS(ctx, acc.PVCNamespace, acc.PVCName)
		if !isNFS {
			log.Printf("[session] 공유 볼륨 %d 는 NFS 백엔드가 아니라 마운트 불가(전용 스토리지)", volID)
			return k8s.VolMountSpec{}, false
		}
		pvc = fmt.Sprintf("shared-%d", volID)
		if err := s.prov.EnsureSharedNFSPVC(ctx, k8s.SharedNFSSpec{
			Namespace: ns, Name: pvc, NFSServer: server, NFSPath: path, SizeGiB: acc.SizeGiB,
		}); err != nil {
			log.Printf("[session] 공유 볼륨 %d PVC 복제 실패: %v", volID, err)
			return k8s.VolMountSpec{}, false
		}
	}
	return k8s.VolMountSpec{PVCName: pvc, MountPath: mountPath, ReadOnly: ro}, true
}

// resolveDatasets는 데이터셋(시스템 RWX NFS PVC)을 세션 ns 에 RO 정적 복제해 /data/<name> 에 마운트한다.
// 데이터셋은 항상 읽기전용 공유이므로 NFS 백엔드 전제(전용 스토리지면 마운트 불가).
// pickDatasetNode는 요청 데이터셋들의 로컬 캐시 배치를 보고, 가장 많이 캐시된(GPU타입 일치) 노드를 고른다.
// 반환 node!="" 이면 그 노드에 핀하고, 그 노드에 캐시된 데이터셋은 hostPath 로 마운트한다(나머지는 NFS).
// 캐시 비활성/후보 없음이면 node="" (전부 NFS).
func (s *Service) pickDatasetNode(ctx context.Context, ids []int64, gpuType, gpuMode string) (string, map[int64][]string, map[int64]string) {
	if s.datasetCache == nil || len(ids) == 0 {
		return "", nil, nil
	}
	cached, hostPaths := s.datasetCache.DatasetCachePlacement(ids)
	if len(hostPaths) == 0 {
		return "", nil, nil
	}
	typeNodes := s.nodesOfType(ctx, gpuType, gpuMode) // 후보를 GPU 타입 일치 노드로 제한
	best, bestN := "", 0
	score := map[string]int{}
	for _, nodes := range cached {
		for _, n := range nodes {
			if typeNodes != nil && !typeNodes[n] {
				continue
			}
			score[n]++
			if score[n] > bestN {
				best, bestN = n, score[n]
			}
		}
	}
	if bestN == 0 {
		return "", nil, nil // 캐시된 노드 중 타입 일치 없음 → 전부 NFS
	}
	return best, cached, hostPaths
}

// nodesOfType는 GPU 타입이 일치하는(또는 CPU면 전체) Ready 노드 집합을 반환한다(미가용 시 nil=제한없음).
// gpuShareOf는 세션이 차지하는 "노드 대비 GPU 지분"(0~1)을 반환한다.
//
//	전용 N개      → N / 노드GPU수
//	분할(코어 c%) → (c/100) / 노드GPU수
//	타임셰어 슬롯 → (1/슬롯수) / 노드GPU수
//
// 이 지분만큼 CPU·메모리를 최소 보장한다("GPU 1/N 사면 CPU도 1/N").
func gpuShareOf(sess *Session, nodeGPUs, slots int) float64 {
	g := float64(max1(nodeGPUs))
	switch sess.GpuMode {
	case "exclusive":
		return float64(max1(sess.GpuCount)) / g
	case "shared":
		return (float64(sess.CorePercent) / 100.0) / g
	case "timeslice":
		return (1.0 / float64(max1(slots))) / g
	}
	return 0
}

// applyGuarantee는 GPU 지분에 비례한 CPU·메모리 최소 보장을 세션에 채운다(Pod requests 로 나감).
// 기준은 그 GPU 타입의 "가장 작은 후보 노드" — 그래야 어느 후보에 떨어져도 보장이 성립한다.
// limits 는 걸지 않으므로 여유가 있으면 이 값을 넘겨 쓸 수 있다(최소 보장이지 상한이 아님).
func (s *Service) applyGuarantee(ctx context.Context, sess *Session) {
	if sess.GpuMode == "cpu" || sess.GpuType == "" {
		return // CPU 단일 모드는 별도 정책(요청 안 걸음) — GPU 지분 개념이 없다
	}
	n, ok := s.minNodeOf(ctx, sess.GpuType)
	if !ok || n.CPUCores <= 0 {
		return // 후보 노드 정보를 못 얻으면 보장을 걸지 않는다(스케줄 실패보다 낫다)
	}
	nodeGPUs := atoiCap(n.GpuCapacity)
	share := gpuShareOf(sess, nodeGPUs, s.timesliceSplitFor(ctx, sess.GpuType))
	if share <= 0 {
		return
	}
	// 정책: "최소만 보장(request), 상한 없음(limit 미설정 → 자유 버스트/경쟁)".
	// request 는 GPU 지분에 비례하되 requestFactor(0.5)를 곱해 보수적으로 잡는다. 이유:
	//   한 노드의 세션 GPU 지분 합은 설계상 ≤ 1(공유=나눠가짐, 전용=독점) 이므로,
	//   노드 위 CPU/Mem request 총합 = 0.5 × 노드 × Σ(share) ≤ 노드의 50%.
	//   → 항상 ≥50% 헤드룸이 남아 "CPU/Mem 부족으로 스케줄 실패(영구 Pending)"가 원천 차단된다.
	//   전용(share=1)도 노드의 50%만 요청 → 반드시 배치되고, limit 이 없어 노드 전체까지 버스트한다.
	// ⚠️ "limit만" 방식은 금물: request 없이 limit 만 주면 k8s 가 request=limit 으로 자동 설정해 다시 100% 요청이 된다.
	const requestFactor = 0.5
	// share 상한 1.0 — 요청 GPU 수가 노드 GPU 수를 초과해 share>1 이 되어도(그런 세션은 어차피
	// "한 노드에 그만큼의 GPU가 없어" GPU 부족으로 스케줄 불가) CPU/Mem request 가 노드 용량을
	// 넘어 폭주하지 않게 막는다. 이로써 request 총합은 언제나 ≤ 노드의 50% 로 유지된다.
	effShare := share
	if effShare > 1.0 {
		effShare = 1.0
	}
	sess.CPUCores = maxInt(1, int(float64(n.CPUCores)*effShare*requestFactor+0.5))
	sess.MemGB = maxInt(1, int(float64(n.MemGB)*effShare*requestFactor+0.5))
}

// minNodeOf는 해당 GPU 타입 후보 중 CPU 코어가 가장 적은(=최소 보장 기준) Ready 노드를 찾는다.
func (s *Service) minNodeOf(ctx context.Context, gpuType string) (k8s.LiveNode, bool) {
	live, err := s.prov.ListNodes(ctx)
	if err != nil {
		return k8s.LiveNode{}, false
	}
	var best k8s.LiveNode
	found := false
	for _, n := range live {
		if !n.Ready || n.GpuType != gpuType {
			continue
		}
		if !found || n.CPUCores < best.CPUCores {
			best, found = n, true
		}
	}
	return best, found
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// atoiCap은 노드 GPU capacity 문자열("8")을 정수로(비숫자 만나면 거기까지).
func atoiCap(s string) int {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return n
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

// timesliceSplitFor는 해당 GPU 타입의 타임슬라이싱 노드가 광고하는 슬롯 수를 찾는다(없으면 1).
// GPU 타입은 K8s 라벨, 슬롯 수는 DB 오버레이(nodes.split_count)라 둘을 조인해 얻는다.
func (s *Service) timesliceSplitFor(ctx context.Context, gpuType string) int {
	nodes := s.nodesOfType(ctx, gpuType, "")
	if len(nodes) == 0 {
		return 1
	}
	names := make([]string, 0, len(nodes))
	for n := range nodes {
		names = append(names, n)
	}
	return s.repo.TimesliceSplit(names)
}

// max1은 0/음수를 1로 보정한다(0 나눗셈 방지).
func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

func (s *Service) nodesOfType(ctx context.Context, gpuType, gpuMode string) map[string]bool {
	live, err := s.prov.ListNodes(ctx)
	if err != nil || len(live) == 0 {
		return nil
	}
	out := map[string]bool{}
	for _, n := range live {
		if !n.Ready {
			continue
		}
		if gpuMode == "cpu" || gpuType == "" || n.GpuType == gpuType {
			out[n.Name] = true
		}
	}
	return out
}

// resolveDatasets는 데이터셋을 RO 마운트로 변환한다. targetNode 에 캐시된 데이터셋은 노드 로컬(hostPath),
// 그 외는 NFS 정적 복제(/data/<name>). targetNode="" 이면 전부 NFS.
func (s *Service) resolveDatasets(ctx context.Context, ns string, datasetIDs []int64, targetNode string, cached map[int64][]string, hostPaths map[int64]string) []k8s.VolMountSpec {
	var out []k8s.VolMountSpec
	for _, id := range datasetIDs {
		ds, ok := s.repo.DatasetMount(id)
		if !ok {
			log.Printf("[session] 데이터셋 %d 미프로비저닝(PVC 없음) — 마운트 생략", id)
			continue
		}
		safe := mountSafe(ds.Name)
		// targetNode 에 로컬 캐시돼 있으면 hostPath(빠름) = ~/datasets/fast/<name>, 아니면 NFS(느림) = ~/datasets/slow/<name>.
		if targetNode != "" && hostPaths[id] != "" && containsStr(cached[id], targetNode) {
			out = append(out, k8s.VolMountSpec{HostPath: hostPaths[id], MountPath: datasetMount("fast", safe), ReadOnly: true})
			continue
		}
		server, path, isNFS := s.prov.PVCBackingNFS(ctx, ds.PVCNamespace, ds.PVCName)
		if !isNFS {
			log.Printf("[session] 데이터셋 %d 는 NFS 백엔드가 아니라 마운트 불가", id)
			continue
		}
		name := fmt.Sprintf("ds-%d", id)
		if err := s.prov.EnsureSharedNFSPVC(ctx, k8s.SharedNFSSpec{
			Namespace: ns, Name: name, NFSServer: server, NFSPath: path, SizeGiB: 1,
		}); err != nil {
			log.Printf("[session] 데이터셋 %d PVC 복제 실패: %v", id, err)
			continue
		}
		out = append(out, k8s.VolMountSpec{PVCName: name, MountPath: datasetMount("slow", safe), ReadOnly: true})
	}
	return out
}

// homeMount는 세션 홈 마운트 경로(/home/work). 데이터셋은 Home 하위에 자동 마운트된다.
const homeMount = "/home/work"

// datasetMount는 데이터셋 마운트 경로를 Home 하위 tier(fast=로컬캐시/slow=NFS)로 구성한다.
// 불특정 다수가 속도 차이를 직관적으로 알 수 있도록 fast/slow 영단어 폴더로 분리한다.
func datasetMount(tier, name string) string { return homeMount + "/datasets/" + tier + "/" + name }

func containsStr(ss []string, v string) bool {
	for _, x := range ss {
		if x == v {
			return true
		}
	}
	return false
}

// mountSafe는 데이터셋 이름을 마운트 경로 세그먼트로 정규화한다(공백/슬래시 → '-').
func mountSafe(name string) string {
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

// buildSession은 요청 + (오퍼링) → 세션 엔티티로 스펙을 확정한다.
func (s *Service) buildSession(ctx context.Context, userID int64, req CreateReq) (*Session, error) {
	sess := &Session{
		InstanceID:  idgen.Token("ses-", 5), // K8s Pod 이름용 RFC1123(하이픈)
		UserID:      userID,
		GroupID:     req.GroupID,
		Name:        req.Name,
		Env:         "container",
		GpuMode:     req.GpuMode,
		ImageID:     &req.ImageID,
		OfferingID:  req.OfferingID,
		VramMB:      req.VramMB,
		CorePercent: req.CorePercent,
		CPUCores:    req.CpuCores,
		MemGB:       req.MemGB,
		GpuCount:    req.GpuCount,
		GpuType:     req.GpuType,
		Phase:       PhaseProvisioning,
	}
	if req.OfferingID != nil {
		if err := s.applyOffering(sess, *req.OfferingID); err != nil {
			return nil, err
		}
	}
	// 분할(VRAM/코어%)은 shared 모드 전용 값 — 전용/CPU 세션엔 남기지 않는다(스펙 표기 혼동 방지).
	if sess.GpuMode != "shared" {
		sess.VramMB, sess.CorePercent = 0, 0
	}
	if sess.GpuMode == "exclusive" && sess.GpuCount < 1 {
		sess.GpuCount = 1
	}
	// 타임슬라이싱 슬롯은 항상 1개(=nvidia.com/gpu:1). 하드정책(max_gpu)도 슬롯 1개=GPU 1로 카운팅.
	if sess.GpuMode == "timeslice" {
		sess.GpuCount = 1
	}
	// CPU·메모리는 클라이언트 값을 믿지 않고 GPU 지분에서 서버가 산출한다(최소 보장).
	s.applyGuarantee(ctx, sess)
	sess.PricePerHour = s.priceOf(ctx, sess)
	sess.WebPassword = idgen.Token("", 12) // 웹(code-server) 랜덤 비밀번호
	now := s.now()
	sess.StartedAt = &now
	return sess, nil
}

const (
	vscodePort  = 8080 // code-server 기본 포트
	jupyterPort = 8888 // jupyter 기본 포트
)

// webChannelsFor는 이미지 channels 설정 → 활성 웹 채널 목록으로 변환한다.
// 시크릿(비밀번호=PASSWORD, 토큰=JUPYTER_TOKEN)은 세션 1개당 동일 값(WebPassword)을 재사용.
// channels 미설정 이미지는 하위호환으로 code-server(vscode)로 간주한다.
func webChannelsFor(ch ImageChannels, secret string) []k8s.WebChannelSpec {
	var out []k8s.WebChannelSpec
	if ch.VSCode {
		out = append(out, k8s.WebChannelSpec{Name: "vscode", Port: vscodePort, EnvKey: "PASSWORD", Secret: secret})
	}
	if ch.Jupyter {
		out = append(out, k8s.WebChannelSpec{Name: "jupyter", Port: jupyterPort, EnvKey: "JUPYTER_TOKEN", Secret: secret})
	}
	// 커스텀 웹 포트 — 시크릿/env 없이 포트포워딩만(제네릭 앱). EnvKey="" → channelEnv 가 주입 생략.
	if ch.Web != nil && ch.Web.Port > 0 {
		name := ch.Web.Name
		if name == "" {
			name = "web"
		}
		out = append(out, k8s.WebChannelSpec{Name: name, Port: ch.Web.Port})
	}
	// 웹 채널이 하나도 없을 때:
	//   - 이미지가 채널을 명시적으로 선언(vscode/jupyter/ssh/web 중 하나라도 true/set)했다면 그 뜻대로 SSH 전용(빈 목록).
	//   - 아무것도 선언 안 한 레거시(channels=NULL, 전부 zero-value)면 하위호환으로 code-server 기본.
	if len(out) == 0 {
		explicit := ch.VSCode || ch.Jupyter || ch.SSH || ch.Web != nil
		if !explicit {
			out = append(out, k8s.WebChannelSpec{Name: "vscode", Port: vscodePort, EnvKey: "PASSWORD", Secret: secret})
		}
	}
	return out
}

// sessionChannels는 세션 이미지의 웹 채널 목록을 반환한다(ImageID 없으면 code-server 기본).
func (s *Service) sessionChannels(imageID *int64, secret string) []k8s.WebChannelSpec {
	if imageID == nil {
		return webChannelsFor(ImageChannels{VSCode: true}, secret)
	}
	return webChannelsFor(s.repo.ImageChannels(*imageID), secret)
}

// priceOf는 세션 시간당 단가를 서버에서 산출한다(클라이언트 신뢰 X).
// 공유/CPU=오퍼링 단가, 전용 GPU=gpu_pricing[타입]×개수.
func (s *Service) priceOf(ctx context.Context, sess *Session) int {
	base := 0
	// 프리셋(오퍼링) 세션은 오퍼링 단가가 우선(관리자 '단가' 탭의 오퍼링 단가).
	if sess.OfferingID != nil {
		if sp, err := s.repo.OfferingSpec(*sess.OfferingID); err == nil {
			base = sp.PricePerHour
		}
	} else {
		perHour, perGB, perCore := s.repo.GpuTypePricing(sess.GpuType)
		switch sess.GpuMode {
		case "exclusive":
			n := sess.GpuCount
			if n < 1 {
				n = 1
			}
			base = perHour * n
		case "shared":
			// 커스텀 분할(HAMi): VRAM(GB)×GB단가 + 코어(%)×코어단가.
			base = (sess.VramMB/1024)*perGB + sess.CorePercent*perCore
		case "timeslice":
			// 타임슬라이싱 슬롯 = GPU 1개를 N분할한 몫 → 전용단가 ÷ 슬롯수(실 점유량 비례).
			base = perHour / max1(s.timesliceSplitFor(ctx, sess.GpuType))
		case "cpu":
			// CPU 단일 모드(데이터 다운로드·전처리 등) — 단일 단가(시간당). CPU/메모리 세부 단가는 두지 않는다:
			// GPU 세션의 CPU·메모리는 GPU 지분에 번들되고, CPU 전용은 이 한 값으로만 과금(단가 체계 단순화).
			base, _, _ = s.repo.GpuTypePricing("cpu")
		}
	}
	return base + s.surge(ctx, sess, base)
}

// surge는 동적 가격 가산분을 산출한다. 가용성(해당 GPU 타입 사용률)이 높을수록 최대 surgeIncrement 까지 가산.
//
//	가산 = round(increment × used/total).  static 모드/비GPU/단가0 이면 0.
func (s *Service) surge(ctx context.Context, sess *Session, base int) int {
	if !s.surgeDynamic || base <= 0 || s.availFn == nil || sess.GpuType == "" || sess.GpuMode == "cpu" {
		return 0
	}
	free, total := s.availFn(ctx, sess.GpuType)
	if total <= 0 {
		return 0
	}
	used := total - free
	if used < 0 {
		used = 0
	}
	return int(float64(s.surgeIncrement)*float64(used)/float64(total) + 0.5)
}

func (s *Service) now() time.Time { return time.Now().UTC() }

// applyOffering은 오퍼링 스펙을 세션에 병합(요청이 비운 값만 채움).
func (s *Service) applyOffering(sess *Session, offeringID int64) error {
	sp, err := s.repo.OfferingSpec(offeringID)
	if err != nil {
		return err
	}
	if sess.VramMB == 0 {
		sess.VramMB = sp.VramMB
	}
	if sess.CorePercent == 0 {
		sess.CorePercent = sp.CorePercent
	}
	if sess.GpuMode == "" {
		sess.GpuMode = mapOfferingMode(sp.Mode)
	}
	if sess.GpuType == "" {
		sess.GpuType = sp.GpuType
	}
	return nil
}

// ephemeralGiB는 세션 임시 디스크(ephemeral-storage) 상한을 정책에서 해석한다(0=매퍼 기본 캡).
func (s *Service) ephemeralGiB(userID int64) int {
	if s.limits == nil {
		return 0
	}
	return s.limits.Resolve(userID).MaxEphemeralGiB
}

func (s *Service) provision(ctx context.Context, ns string, sess *Session, imageRef, homePVC string, mounts []k8s.VolMountSpec, preferNodes []string, requireNode string) error {
	if err := s.prov.EnsureNamespace(ctx, ns); err != nil {
		return err
	}
	// 컨테이너 SSH 가 켜져 있으면 사용자 공개키 Secret 을 먼저 준비한다(키 미등록이면 빈 파일 →
	// 세션은 정상 기동하고, 나중에 등록하면 Secret 갱신만으로 이 세션에도 바로 반영된다).
	if s.containerSSH() {
		if err := s.prov.UpsertUserKeys(ctx, ns, sess.UserID, s.repo.UserSSHKey(sess.UserID)); err != nil {
			return err
		}
	}
	channels := s.sessionChannels(sess.ImageID, sess.WebPassword)
	if err := s.prov.CreateSessionPod(ctx, k8s.SessionSpec{
		Namespace:   ns,
		Name:        sess.InstanceID,
		Image:       imageRef,
		GpuType:     sess.GpuType,
		GpuMode:     sess.GpuMode,
		GpuCount:    sess.GpuCount,
		VramMB:      sess.VramMB,
		CorePercent: sess.CorePercent,
		CPUCores:    sess.CPUCores,
		MemGB:       sess.MemGB,
		EphemeralGiB: s.ephemeralGiB(sess.UserID), // 정책 해석값(0=매퍼 기본 캡). 노드 디스크 DoS 방지.
		Labels:      sess.labels(),
		UID:         s.uidBase + int(sess.UserID), // 안정 UID(물리 SSH 와 동일 공식) → NFS 권한 일관
		HomePVC:     homePVC,
		Volumes:     mounts,
		WebChannels: channels,     // 이미지 channels 기반(vscode/jupyter)
		SSHDImage:   s.sshdImage,  // 컨테이너 SSH 사이드카(빈값=비활성)
		SSHDPubKey:  s.sshdPubKey, // 사이드카가 신뢰할 게이트웨이 공개키(게이트웨이 off 면 빈값)
			// 사용자 등록 공개키 Secret(위에서 upsert). Optional 마운트라 키 미등록이어도 세션은 뜬다.
			UserKeysSecret: k8s.UserKeysSecretName(sess.UserID),
		ScratchHost: s.scratchHostOf(sess.UserID),
		PreferNodes: preferNodes, // 이미지 캐시 노드 소프트 선호(빠른 시작)
		RequireNode: requireNode, // 데이터셋 로컬 캐시 노드 하드 핀(hostPath 마운트)
	}); err != nil {
		return err
	}
	// 세션 Service — 게이트웨이 활성 시 모든 채널 포트(+sshd 22)를 ClusterIP 로 노출(라우팅용).
	return s.prov.EnsureSessionService(ctx, k8s.SvcSpec{
		Namespace: ns, Name: sess.InstanceID, Ports: s.svcPorts(channels), Mode: s.exposeMode(), Internal: s.gatewayOn(),
	})
}

// scratchHostOf는 사용자의 노드로컬 스크래치 계정폴더 경로를 반환한다(비활성이면 빈값).
func (s *Service) scratchHostOf(userID int64) string {
	if !s.scratchEnabled || s.scratchHostPath == "" {
		return ""
	}
	return s.scratchHostPath + "/" + s.usernameOf(userID)
}

// exposeMode는 세션 노출 모드를 반환한다. loadbalancer(MetalLB) 만 별도로 인정하고,
// 그 외(빈값·이전의 portforward 등)는 모두 nodeport 로 수렴한다 — 게이트웨이·인그레스 없이 바로 접속.
func (s *Service) exposeMode() string {
	if s.expose == k8s.ExposeLoadBalancer {
		return k8s.ExposeLoadBalancer
	}
	return k8s.ExposeNodePort
}

// attachMounts는 세션-볼륨 연결을 기록한다. 권한(perm)은 클라 입력이 아니라
// 서버 판정(소유=rw/공유=share perm)으로 저장하고, 접근권 없는 볼륨은 연결하지 않는다.
func (s *Service) attachMounts(sessionID, userID int64, req CreateReq) {
	for _, v := range req.Volumes {
		acc, ok := s.repo.VolumeAccess(v.ID, userID)
		if !ok {
			continue
		}
		_ = s.repo.AddVolume(sessionID, v.ID, v.MountPath, acc.Perm)
	}
	for _, dsID := range req.Datasets {
		_ = s.repo.AddDataset(sessionID, dsID, "")
	}
}

// List는 DB 세션을 라이브 Pod 상태로 갱신해 반환한다(best-effort).
func (s *Service) List(ctx context.Context, userID int64) ([]Session, error) {
	rows, err := s.repo.ListByUser(userID)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		s.refreshLive(ctx, &rows[i])
		s.fillChannels(&rows[i])
	}
	return rows, nil
}

// fillChannels는 세션이 제공하는 접속 채널 이름을 채운다(프론트 연결 탭 표시용).
// 물리(SSH)=ssh, 컨테이너=이미지 channels(vscode/jupyter).
func (s *Service) fillChannels(sess *Session) {
	if sess.Env == "ssh" {
		sess.Channels = []string{"ssh"}
		return
	}
	for _, ch := range s.sessionChannels(sess.ImageID, "") {
		sess.Channels = append(sess.Channels, ch.Name)
	}
	// 웹터미널(API 호스팅 exec) — 컨테이너 세션엔 항상 노출(사이드카/게이트웨이 불요, k8s exec 만).
	// 프론트에서 "SSH" 버튼 → 브라우저 xterm 으로 바로 접속. (물리 세션은 아래 ssh 탭에서 웹연결 제공.)
	sess.Channels = append(sess.Channels, "terminal")
	// 컨테이너 sshd 사이드카가 있으면 네이티브 SSH 탭을 노출한다 — 게이트웨이는 필요 없다.
	// 직접 접속(LB IP:22 또는 노드IP:NodePort)이 기본 경로이고, 게이트웨이는 그 위에 얹는 추가 경로.
	if s.containerSSH() {
		sess.Channels = append(sess.Channels, "ssh")
	}
}

func (s *Service) refreshLive(ctx context.Context, sess *Session) {
	if sess.Phase == PhaseStopped || sess.Phase == PhaseTerminated {
		return
	}
	if sess.Env == "ssh" {
		// 물리(SSH) 세션은 Pod 가 없다 — 임대 활성 = running 으로 간주.
		if sess.Phase == PhaseProvisioning {
			_ = s.repo.SetPhase(sess.InstanceID, PhaseRunning)
			sess.Phase = PhaseRunning
		}
		return
	}
	st, err := s.prov.PodStatus(ctx, s.namespaceOf(sess), sess.InstanceID)
	if err != nil || st == nil {
		return
	}
	phase := mapPodPhase(st.Phase)
	_ = s.repo.UpdateLive(sess.InstanceID, phase, st.Node, st.IP)
	sess.Phase, sess.Node, sess.IPAddress = phase, st.Node, st.IP
}

// Connection은 접속 정보를 생성한다.
// 물리(SSH) 세션은 SSH 만 제공(노드 직접 접속). 컨테이너 세션은 게이트웨이 라우팅(VSCode/Jupyter/SSH).
func (s *Service) Connection(ctx context.Context, instanceID string, userID int64) (*Connection, error) {
	sess, err := s.repo.Get(instanceID, userID)
	if err != nil {
		return nil, err
	}
	if sess.Env == "ssh" {
		// 물리노드 임대 — 노드로 직접 SSH(계정=username, node-agent 가 생성).
		return &Connection{
			SSH: map[string]string{"cmd": fmt.Sprintf("ssh %s@%s", s.usernameOf(userID), sess.Node)},
		}, nil
	}
	channels := s.sessionChannels(sess.ImageID, sess.WebPassword)
	conn := &Connection{}
	for i, ch := range channels {
		acc := s.channelAccess(ctx, sess, ch, i == 0) // i==0 = primary(노출 Service 포트)
		switch ch.Name {
		case "vscode":
			conn.VSCode = acc
		case "jupyter":
			conn.Jupyter = acc
		default: // 커스텀 웹 포트
			conn.Web = acc
		}
	}
	// 컨테이너 SSH — 게이트웨이 없이도 항상 제공(사용자가 등록한 공개키로 인증).
	if s.containerSSH() {
		if acc := s.containerSSHAccess(ctx, sess); acc != nil {
			conn.SSH = acc
		}
	}
	return conn, nil
}

// containerSSHAccess는 컨테이너 세션의 "직접 SSH" 접속 정보를 만든다(게이트웨이 불요).
// 노출 모드를 따라 MetalLB 가 있으면 LB IP 의 22 번으로, 없으면 노드 IP 의 NodePort 로 붙는다.
// 주소가 아직 없으면(LB IP 할당 대기 등) nil — 프론트는 웹 터미널 경로를 계속 제공한다.
// 로그인 계정은 sshd 사이드카가 세션 UID 로 만드는 work 이며, 인증은 사용자 등록 공개키뿐이다.
func (s *Service) containerSSHAccess(ctx context.Context, sess *Session) map[string]string {
	acc, err := s.prov.SessionServiceAccess(ctx, s.namespaceOf(sess), sess.InstanceID, s.exposeMode())
	if err != nil {
		return nil
	}
	const user = "work"
	if acc.LBIP != "" {
		return map[string]string{"direct": "true", "cmd": fmt.Sprintf("ssh %s@%s", user, acc.LBIP)}
	}
	if acc.SSHNodePort > 0 {
		if ip := s.prov.FirstNodeIP(ctx); ip != "" {
			return map[string]string{"direct": "true", "cmd": fmt.Sprintf("ssh -p %d %s@%s", acc.SSHNodePort, user, ip)}
		}
	}
	return nil
}

// channelAccess는 웹 채널 1개의 접속 정보(URL/시크릿)를 채운다.
// primary 채널만 노출 Service(LB/NodePort) 주소를 받는다.
func (s *Service) channelAccess(ctx context.Context, sess *Session, ch k8s.WebChannelSpec, primary bool) map[string]string {
	ns := s.namespaceOf(sess)
	m := map[string]string{}
	suffix := "/"
	switch ch.Name {
	case "jupyter":
		suffix = "/lab?token=" + ch.Secret // jupyter 토큰 인증
	case "vscode":
		m["password"] = ch.Secret // code-server 비밀번호
	default:
		// 커스텀 웹 채널 — 시크릿 없음. 비번/토큰 미설정.
	}
	mode := s.exposeMode()
	if primary {
		acc, _ := s.prov.SessionServiceAccess(ctx, ns, sess.InstanceID, mode)
		switch mode {
		case k8s.ExposeLoadBalancer:
			if acc.LBIP != "" {
				m["url"] = fmt.Sprintf("http://%s:%d%s", acc.LBIP, ch.Port, suffix)
			} else {
				m["pending"] = "LoadBalancer IP 할당 대기중"
			}
		case k8s.ExposeNodePort:
			if ip := s.prov.FirstNodeIP(ctx); ip != "" && acc.NodePort > 0 {
				m["url"] = fmt.Sprintf("http://%s:%d%s", ip, acc.NodePort, suffix)
			}
		}
	}
	// URL 을 못 만든 경우(NodePort 대기·보조 채널)에만 폴백. 게이트웨이가 실제로 켜져 있을 때만
	// 서브도메인 URL 을 쓴다 — gateway.enabled=false 여도 GatewayDomain 기본값(gw.giosk.local)이
	// 남아있으므로 s.gateway!="" 로 판단하면 안 되고 gatewayOn() 으로 판단해야 죽은 링크가 안 나간다.
	if m["url"] == "" {
		if s.gatewayOn() {
			m["url"] = fmt.Sprintf("https://%s.%s%s", sess.InstanceID, s.gateway, suffix)
		} else {
			m["url"] = fmt.Sprintf("http://localhost:%d%s", ch.Port, suffix)
			m["localUrl"] = fmt.Sprintf("http://localhost:%d%s", ch.Port, suffix)
		}
	}
	return m
}

// ── 접속 게이트웨이(단기 토큰) ──────────────────────

const (
	webAccessTTL = 3 * time.Minute // 웹 access 토큰 수명(클릭→쿠키 교환까지 짧게)
	sshAccessTTL = 5 * time.Minute // SSH 토큰 수명(복붙 지연 여유)
)

// gatewayOn은 게이트웨이(토큰 발급)가 활성인지 여부.
func (s *Service) gatewayOn() bool { return len(s.gatewaySecret) > 0 && s.gateway != "" }

// gatewayHost는 SSH 접속 호스트(설정 없으면 gateway 도메인).
func (s *Service) gatewayHost() string {
	if s.gatewayHostArg != "" {
		return s.gatewayHostArg
	}
	return s.gateway
}

func (s *Service) webScheme() string {
	if s.gatewayScheme != "" {
		return s.gatewayScheme
	}
	return "https"
}

// Access는 세션 접속을 위한 단기 서명 토큰(웹 URL·SSH 명령)을 발급한다.
// 게이트웨이 활성 시 사용자는 원본 비밀 대신 토큰으로 접속하고, 게이트웨이가 실제 비밀을 주입한다.
// 게이트웨이 비활성이면 직접 접속 정보(포트포워드/직접 URL)로 폴백한다.
func (s *Service) Access(ctx context.Context, instanceID string, userID int64) (*AccessInfo, error) {
	sess, err := s.repo.Get(instanceID, userID)
	if err != nil {
		return nil, err
	}
	info := &AccessInfo{ExpiresAt: s.now().Add(webAccessTTL)}
	if sess.Env == "ssh" { // 물리노드 임대 — SSH 만.
		info.SSH = s.sshAccess(sess.InstanceID, userID, "", sess.Node, gateway.TgtPhysical, s.usernameOf(userID))
		info.ExpiresAt = s.now().Add(sshAccessTTL)
		return info, nil
	}
	for _, ch := range s.sessionChannels(sess.ImageID, sess.WebPassword) {
		m := s.webAccess(ctx, sess, ch, userID)
		switch ch.Name {
		case "vscode":
			info.VSCode = m
		case "jupyter":
			info.Jupyter = m
		default: // 커스텀 웹 포트
			info.Web = m
		}
	}
	if s.gatewayOn() && s.containerSSH() { // 컨테이너 sshd 사이드카 → SSH 탭
		info.SSH = s.sshAccess(sess.InstanceID, userID, s.namespaceOf(sess), "", gateway.TgtContainer, "work")
	}
	return info, nil
}

// webAccess는 웹 채널(vscode/jupyter) 접속 맵을 만든다. 게이트웨이 활성 시 토큰 URL,
// 아니면 기존 채널 접속 정보(직접 URL/포트포워드)로 폴백한다.
func (s *Service) webAccess(ctx context.Context, sess *Session, ch k8s.WebChannelSpec, userID int64) map[string]string {
	if !s.gatewayOn() {
		return s.channelAccess(ctx, sess, ch, true) // 폴백: 직접 접속(nodeport/LB)
	}
	claims := gateway.Claims{
		IID: sess.InstanceID, Ch: ch.Name, NS: s.namespaceOf(sess), Port: ch.Port,
		Sub: userID, Typ: gateway.TypWeb, Tgt: gateway.TgtContainer, Secret: ch.Secret,
		Exp: s.now().Add(webAccessTTL).Unix(), Jti: idgen.Token("", 8),
	}
	tok, err := gateway.Sign(claims, s.gatewaySecret)
	if err != nil {
		return map[string]string{}
	}
	url := fmt.Sprintf("%s://%s-%s.%s/?access=%s", s.webScheme(), sess.InstanceID, ch.Name, s.gateway, tok)
	return map[string]string{"url": url}
}

// sshAccess는 SSH 접속(복붙 한 줄) 정보를 만든다. 게이트웨이 활성 시 username=1회 토큰,
// 아니면 물리 노드로의 직접 SSH 로 폴백한다.
// ⚠️ 토큰은 jti 단일사용(게이트웨이 nonce) — ssh/sftp 각각에 별도 토큰을 발급한다(같은 토큰이면 두 번째가 거부됨).
func (s *Service) sshAccess(instanceID string, userID int64, ns, node, tgt, user string) map[string]string {
	if !s.gatewayOn() {
		if tgt == gateway.TgtPhysical {
			return map[string]string{"cmd": fmt.Sprintf("ssh %s@%s", user, node), "direct": "true"}
		}
		return nil // 컨테이너 직접 SSH 는 게이트웨이 필요
	}
	mint := func() string {
		claims := gateway.Claims{
			IID: instanceID, Ch: gateway.ChanSSH, NS: ns, Sub: userID,
			Typ: gateway.TypSSH, Tgt: tgt, Host: node, User: user,
			Exp: s.now().Add(sshAccessTTL).Unix(), Jti: idgen.Token("", 8),
		}
		tok, err := gateway.Sign(claims, s.gatewaySecret)
		if err != nil {
			return ""
		}
		return tok
	}
	sshTok := mint()
	if sshTok == "" {
		return nil
	}
	host, port := s.gatewayHost(), s.gatewaySSHPort
	// 프록시(게이트웨이) 접속 — 어디서든 게이트웨이 한 점으로. 토큰은 최초 인증 1회만 소비되고,
	// 접속이 성립하면 게이트웨이가 스트림을 계속 프록시한다(중간 재검증·주기 검사 없음 → 세션 유지).
	m := map[string]string{
		"cmd":  fmt.Sprintf("ssh -p %d %s@%s", port, sshTok, host),
		"user": user,
		"host": host,
	}
	// 사내망 직접 접속(192 대역) — 사무실 네트워크 안이면 게이트웨이를 거치지 않고 노드에 바로 붙는다.
	// 물리(SSH) 세션만: 대상이 곧 노드다. 컨테이너는 노드로 직접 붙으면 컨테이너가 아니라 노드에
	// 닿으므로 직접 경로를 제공하지 않는다(프록시만).
	if tgt == gateway.TgtPhysical && node != "" {
		m["directCmd"] = fmt.Sprintf("ssh %s@%s", user, node)
	}
	return m
}

func (s *Service) usernameOf(userID int64) string {
	if u := s.repo.Username(userID); u != "" {
		return u
	}
	return fmt.Sprintf("user%d", userID)
}

func (s *Service) Logs(ctx context.Context, instanceID string, userID int64) (string, error) {
	sess, err := s.repo.Get(instanceID, userID)
	if err != nil {
		return "", err
	}
	return s.prov.PodLogs(ctx, s.namespaceOf(sess), instanceID, 200)
}

// AdminDescribe는 세션 파드의 describe 요약(상태·조건·이벤트/오류)을 반환한다(관제 진단).
func (s *Service) AdminDescribe(ctx context.Context, instanceID string) (*k8s.PodDescribe, error) {
	sess, err := s.repo.GetByInstance(instanceID)
	if err != nil {
		return nil, err
	}
	if sess.Env == "ssh" {
		return nil, nil // 물리(SSH) 세션은 파드가 없다
	}
	return s.prov.PodDescribe(ctx, s.namespaceOf(sess), instanceID)
}

// AdminLogs는 소유자 검증 없이 세션 파드의 쿠버네티스 로그(tail)를 반환한다(관제용).
func (s *Service) AdminLogs(ctx context.Context, instanceID string, tail int64) (string, error) {
	sess, err := s.repo.GetByInstance(instanceID)
	if err != nil {
		return "", err
	}
	if sess.Env == "ssh" {
		return "", nil // 물리(SSH) 세션은 파드가 없다
	}
	if tail <= 0 {
		tail = 300
	}
	return s.prov.PodLogs(ctx, s.namespaceOf(sess), instanceID, tail)
}

// Stop은 세션을 중단한다. 컨테이너는 Pod 삭제(데이터=PVC 유지), 물리(SSH)는 임대 해제(노드 반납).
func (s *Service) Stop(ctx context.Context, instanceID string, userID int64) error {
	sess, err := s.repo.Get(instanceID, userID)
	if err != nil {
		return err
	}
	return s.stopSession(ctx, sess)
}

// AdminStop은 소유자 검증 없이 세션을 중단한다(관제용). 중단이므로 재시작 가능.
func (s *Service) AdminStop(ctx context.Context, instanceID string) error {
	sess, err := s.repo.GetByInstance(instanceID)
	if err != nil {
		return err
	}
	return s.stopSession(ctx, sess)
}

// stopSession은 세션 중단 공통 절차(정산→Pod/Service 정리 또는 노드 반납→stopped).
func (s *Service) stopSession(ctx context.Context, sess *Session) error {
	s.settle(ctx, sess, true) // 중단 전 사용분 최종 정산
	if sess.Env == "ssh" {
		if s.leaser != nil {
			_ = s.leaser.ReleaseLeaseFor(ctx, sess.InstanceID) // 노드 반납(uncordon); 홈(NFS)은 유지
		}
		return s.repo.SetPhase(sess.InstanceID, PhaseStopped)
	}
	if err := s.prov.DeleteSessionPod(ctx, s.namespaceOf(sess), sess.InstanceID); err != nil {
		return err
	}
	_ = s.prov.DeleteSessionService(ctx, s.namespaceOf(sess), sess.InstanceID)
	return s.repo.SetPhase(sess.InstanceID, PhaseStopped)
}

// Start는 중단된 세션을 재개한다. 컨테이너는 Pod 재생성(같은 홈 PVC 재마운트), 물리는 임대 재취득.
func (s *Service) Start(ctx context.Context, instanceID string, userID int64) error {
	sess, err := s.repo.Get(instanceID, userID)
	if err != nil {
		return err
	}
	if sess.Env == "ssh" {
		if s.leaser == nil {
			return ErrLeaseUnavailable
		}
		if err := s.leaser.CreateLeaseFor(ctx, sess.Node, sess.UserID, instanceID); err != nil {
			return err
		}
		_ = s.repo.ResetBilling(instanceID, s.now()) // 재개 = 새 가동분
		return s.repo.SetPhase(instanceID, PhaseRunning)
	}
	if sess.ImageID == nil {
		return ErrNotFound
	}
	imageRef, err := s.repo.ImageRef(*sess.ImageID)
	if err != nil {
		return err
	}
	ns := s.namespaceOf(sess)
	// 재시작도 통합 home 모델: 로컬(임시) home + (sharedHome 시) ~/nfs 영속.
	mounts := s.restartMounts(ctx, ns, sess.UserID, sess.ID)
	mounts = append(mounts, k8s.VolMountSpec{EmptyDir: true, MountPath: homeMount})
	if s.sharedHome {
		persistPVC, err := s.ensureHome(ctx, ns, sess.UserID)
		if err != nil {
			return err
		}
		mounts = append(mounts, k8s.VolMountSpec{PVCName: persistPVC, MountPath: homeMount + "/nfs"})
	}
	dsIDs := s.repo.SessionDatasetIDs(sess.ID)
	dsNode, dsCached, dsHostPath := s.pickDatasetNode(ctx, dsIDs, sess.GpuType, sess.GpuMode)
	mounts = append(mounts, s.resolveDatasets(ctx, ns, dsIDs, dsNode, dsCached, dsHostPath)...) // 데이터셋 RO 복원
	var preferNodes []string
	if sess.ImageID != nil {
		preferNodes = s.repo.CachedNodes(*sess.ImageID)
	}
	if err := s.provision(ctx, ns, sess, imageRef, "", mounts, preferNodes, dsNode); err != nil {
		return err
	}
	_ = s.repo.ResetBilling(instanceID, s.now()) // 재개 = 새 가동분
	return s.repo.SetPhase(instanceID, PhaseProvisioning)
}

// restartMounts는 재시작 시 세션에 이미 붙어있던 볼륨을 다시 해석해 복원한다(권한 재판정 포함).
func (s *Service) restartMounts(ctx context.Context, ns string, userID, sessionID int64) []k8s.VolMountSpec {
	var out []k8s.VolMountSpec
	for _, m := range s.repo.SessionVolumeMounts(sessionID) {
		if vm, ok := s.mountFor(ctx, ns, userID, m.VolID, m.MountPath); ok {
			out = append(out, vm)
		}
	}
	return out
}

func (s *Service) Delete(ctx context.Context, instanceID string, userID int64) error {
	sess, err := s.repo.Get(instanceID, userID)
	if err != nil {
		return err
	}
	if sess.Phase == PhaseRunning {
		s.settle(ctx, sess, true) // 삭제 전 사용분 최종 정산
	}
	if sess.Env == "ssh" {
		if s.leaser != nil {
			_ = s.leaser.ReleaseLeaseFor(ctx, instanceID) // 임대 해제 + uncordon
		}
		return s.repo.Delete(instanceID)
	}
	ns := s.namespaceOf(sess)
	_ = s.prov.DeleteSessionPod(ctx, ns, instanceID)
	_ = s.prov.DeleteSessionService(ctx, ns, instanceID)
	s.cleanupSharedMounts(ctx, ns, sess.UserID, sess.ID, instanceID)
	return s.repo.Delete(instanceID)
}

// cleanupSharedMounts는 세션이 만든 교차 ns 공유 PVC/PV(정적 복제본)를 정리한다.
// 같은 공유 볼륨을 쓰는 다른 활성 세션이 있으면 보존한다(정지 세션은 시작 시 멱등 재생성되므로 무시).
// 내 볼륨/홈 PVC 는 영속이라 건드리지 않는다(복제본 이름 prefix 로 구분).
func (s *Service) cleanupSharedMounts(ctx context.Context, ns string, userID, sessionID int64, instanceID string) {
	for _, m := range s.repo.SessionVolumeMounts(sessionID) {
		acc, ok := s.repo.VolumeAccess(m.VolID, userID)
		if !ok || acc.PVCNamespace == ns {
			continue // 내 볼륨이거나 접근권 없음 — 정적 복제본 없음
		}
		if s.repo.ActiveSessionsWithVolume(userID, m.VolID, instanceID) > 0 {
			continue // 다른 활성 세션이 같은 공유 볼륨 사용 중 — 유지
		}
		if err := s.prov.DeleteSharedNFSPVC(ctx, ns, fmt.Sprintf("shared-%d", m.VolID)); err != nil {
			log.Printf("[session] 공유 마운트 정리 실패 vol %d: %v", m.VolID, err)
		}
	}
	// 데이터셋 RO 복제본(ds-<id>)도 동일 기준으로 정리.
	for _, dsID := range s.repo.SessionDatasetIDs(sessionID) {
		if s.repo.ActiveSessionsWithDataset(userID, dsID, instanceID) > 0 {
			continue
		}
		if err := s.prov.DeleteSharedNFSPVC(ctx, ns, fmt.Sprintf("ds-%d", dsID)); err != nil {
			log.Printf("[session] 데이터셋 마운트 정리 실패 ds %d: %v", dsID, err)
		}
	}
}

// AdminList는 전체 세션(관제용)을 반환한다.
func (s *Service) AdminList() ([]AdminRow, error) { return s.repo.ListAll() }

// AdminTerminate는 소유자 무관하게 세션 Pod·레코드를 제거한다.
// AdminTerminate(강제종료)는 실행 중 세션을 강제로 죽이되 이력용으로 기록은 남긴다(phase=terminated).
// 레코드 완전 삭제는 AdminDelete(삭제).
func (s *Service) AdminTerminate(ctx context.Context, instanceID string) error {
	sess, err := s.repo.GetByInstance(instanceID)
	if err != nil {
		return err
	}
	s.settle(ctx, sess, true) // 종료 전 사용분 최종 정산
	ns := s.namespaceOf(sess)
	if sess.Env == "ssh" {
		if s.leaser != nil {
			_ = s.leaser.ReleaseLeaseFor(ctx, instanceID)
		}
	} else {
		_ = s.prov.DeleteSessionPod(ctx, ns, instanceID)
		_ = s.prov.DeleteSessionService(ctx, ns, instanceID)
		s.cleanupSharedMounts(ctx, ns, sess.UserID, sess.ID, instanceID)
	}
	return s.repo.SetPhase(instanceID, PhaseTerminated)
}

// AdminDelete(삭제)는 세션 레코드를 완전히 제거한다(파드 잔여물도 정리).
func (s *Service) AdminDelete(ctx context.Context, instanceID string) error {
	sess, err := s.repo.GetByInstance(instanceID)
	if err != nil {
		return err
	}
	ns := s.namespaceOf(sess)
	if sess.Env == "ssh" {
		if s.leaser != nil {
			_ = s.leaser.ReleaseLeaseFor(ctx, instanceID)
		}
	} else {
		_ = s.prov.DeleteSessionPod(ctx, ns, instanceID)
		_ = s.prov.DeleteSessionService(ctx, ns, instanceID)
		s.cleanupSharedMounts(ctx, ns, sess.UserID, sess.ID, instanceID)
	}
	return s.repo.Delete(instanceID)
}

// AdminGet은 단일 세션의 관제용 상세 행(소유자 id 포함).
func (s *Service) AdminGet(instanceID string) (*AdminRow, error) { return s.repo.AdminOne(instanceID) }

// AdminAudit은 세션 감사 로그(소유자 검증 없이 — 관제용).
func (s *Service) AdminAudit(instanceID string) ([]audit.Log, error) {
	if s.audit == nil {
		return []audit.Log{}, nil
	}
	return s.audit.ListByTarget(instanceID, 100)
}

// RunBiller는 running 세션의 사용 시간을 주기적으로 정산(지갑 차감)한다.
// 잔액 부족 시 세션을 자동 중단한다. charger 미주입(비크레딧)이면 비활성.
func (s *Service) RunBiller(ctx context.Context, interval time.Duration) {
	if s.charger == nil {
		log.Printf("[biller] disabled (비크레딧 모드)")
		return
	}
	log.Printf("[biller] started (interval=%s)", interval)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.billOnce(ctx)
		}
	}
}

func (s *Service) billOnce(ctx context.Context) {
	rows, err := s.repo.ListRunning()
	if err != nil {
		return
	}
	for i := range rows {
		s.settle(ctx, &rows[i], false)
	}
}

// settle은 세션의 미정산 사용분을 차감한다. 잔액 부족이면 세션 중단(final=true 면 중단 생략).
func (s *Service) settle(ctx context.Context, sess *Session, final bool) {
	if s.charger == nil || sess.PricePerHour <= 0 || sess.StartedAt == nil {
		return
	}
	elapsed := s.now().Sub(*sess.StartedAt).Seconds()
	totalDue := int(float64(sess.PricePerHour) * elapsed / 3600.0) // 정수 크레딧(내림)
	due := totalDue - sess.BilledCredits
	if due <= 0 {
		return
	}
	gid := int64(0) // 세션이 뜬 팀 지갑에서 차감(팀 없는 세션은 금지 → 항상 팀). 0이면 wallet 이 대표 팀으로.
	if sess.GroupID != nil {
		gid = *sess.GroupID
	}
	ok, err := s.charger.Consume(sess.UserID, gid, due, sess.InstanceID)
	if err != nil {
		return
	}
	if !ok {
		// 잔액 부족 — 가능한 만큼은 못 받았으니 세션 중단(과금 보호).
		if !final {
			s.stopForBilling(ctx, sess)
		}
		return
	}
	sess.BilledCredits = totalDue
	_ = s.repo.SetBilled(sess.InstanceID, totalDue)
	// 사용시간 원장 적립 — 이번 틱에 과금된 크레딧이 나타내는 시간(초). 세션 삭제와 무관하게 누적 보존.
	if secs := int(float64(due) / float64(sess.PricePerHour) * 3600.0); secs > 0 {
		_ = s.repo.RecordGpuUsage(sess.UserID, gid, sess.InstanceID, secs, sess.GpuCount)
	}
}

// stopForBilling은 잔액 소진 세션을 중단하고 감사 기록한다.
func (s *Service) stopForBilling(ctx context.Context, sess *Session) {
	if sess.Env == "ssh" {
		if s.leaser != nil {
			_ = s.leaser.ReleaseLeaseFor(ctx, sess.InstanceID)
		}
	} else {
		_ = s.prov.DeleteSessionPod(ctx, s.namespaceOf(sess), sess.InstanceID)
		_ = s.prov.DeleteSessionService(ctx, s.namespaceOf(sess), sess.InstanceID)
	}
	_ = s.repo.SetPhase(sess.InstanceID, PhaseStopped)
	log.Printf("[biller] stopped %s (크레딧 소진)", sess.InstanceID)
	if s.audit != nil {
		_ = s.audit.Insert(&audit.Log{ActorUsername: "system", Action: "session_out_of_credit", Target: sess.InstanceID, Result: "applied"})
	}
}

// RunIdleReaper는 windowMin 동안 CPU 활동이 임계치 미만인 running 세션을 자동 정지한다.
// cAdvisor(container_cpu) 파드별 메트릭으로 유휴를 판정한다(블로킹; goroutine 으로 실행).
// Prometheus 미연동/window<=0 이면 비활성.
// RunIdleReaper는 유휴 세션을 자동 정지한다. window 는 매 틱 호출되는 getter 로,
// 유휴 타임아웃을 운영 중 변경(런타임 설정)하면 다음 틱부터 반영된다. window<=0 이면 해당 틱 비활성.
func (s *Service) RunIdleReaper(ctx context.Context, window func() int) {
	if s.met == nil || !s.met.Enabled() {
		log.Printf("[idle-reaper] disabled (prometheus 미연동)")
		return
	}
	log.Printf("[idle-reaper] started (GPU<%.0f%% util / CPU<%.2f cores, timeout=live)", idleGPUThreshold, idleCPUThreshold)
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if w := window(); w > 0 {
				s.reapIdleOnce(ctx, w)
			}
		}
	}
}

func (s *Service) reapIdleOnce(ctx context.Context, windowMin int) {
	rows, err := s.repo.ListRunning()
	if err != nil {
		return
	}
	window := time.Duration(windowMin) * time.Minute
	for i := range rows {
		sess := rows[i]
		if time.Since(sess.CreatedAt) < window {
			continue // 갓 시작한 세션은 제외
		}
		if idle, ok := s.isIdle(ctx, &sess, windowMin); ok && idle {
			s.idleStop(ctx, &sess, windowMin)
		}
	}
}

// CountIdleRunning은 running 세션 중 유휴(저사용)인 개수를 센다 — 대시보드 스냅샷/통계용.
// isIdle 과 동일 판정(GPU util<5% / CPU rate<0.05). 메트릭 미가용이면 0(판정 불가).
func (s *Service) CountIdleRunning(ctx context.Context) int {
	if s.met == nil || !s.met.Enabled() {
		return 0
	}
	rows, err := s.repo.ListRunning()
	if err != nil {
		return 0
	}
	const window = 5 // 분 — 유휴 판정 이동평균 창(리퍼 창과 별개, 스냅샷 순간 판정용)
	idle := 0
	for i := range rows {
		sess := rows[i]
		if is, ok := s.isIdle(ctx, &sess, window); ok && is {
			idle++
		}
	}
	return idle
}

// isIdle은 세션 종류별로 유휴 여부를 판정한다.
//   - GPU 세션(exclusive/shared): GPU 사용률(DCGM)이 idleGPUThreshold% 미만이면 유휴.
//   - CPU 세션(cpu): CPU rate 가 idleCPUThreshold 코어 미만이면 유휴.
//
// ok=false 면 판정 불가(메트릭 없음/물리 세션) → 보수적으로 정지하지 않는다.
func (s *Service) isIdle(ctx context.Context, sess *Session, windowMin int) (idle, ok bool) {
	if sess.Env == "ssh" {
		return false, false // 물리(SSH) 세션은 Pod 메트릭이 없어 유휴 리퍼 대상 아님
	}
	if sess.GpuMode == "cpu" {
		q := fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{pod=%q,container!=""}[%dm]))`, sess.InstanceID, windowMin)
		v, got := s.met.Scalar(ctx, q)
		if !got {
			return false, false
		}
		return v < idleCPUThreshold, true
	}
	// GPU 대여 세션 — 윈도 평균 GPU 사용률로만 판정(CPU 무시).
	// 분할(HAMi)은 DCGM 이 Pod 단위로 보고하지 않는다(vGPUmonitor 의 hami_* / exported_pod 라벨).
	// 예전엔 분할 세션도 DCGM 을 봐서 항상 빈 결과 → ok=false → 유휴로 판정된 적이 없어 리퍼가 무력했다.
	if sess.GpuMode == "shared" {
		q := fmt.Sprintf(`avg(avg_over_time(hami_container_device_utilization_ratio{exported_pod=%q}[%dm]))`, sess.InstanceID, windowMin)
		if v, got := s.met.Scalar(ctx, q); got {
			return v < idleGPUThreshold, true
		}
		// 컨테이너 시리즈가 아예 없음 = CUDA 미기동(GPU 미사용). 모니터가 살아있으면(host 시리즈 존재)
		// GPU 를 안 쓰는 유휴로 확정한다. 모니터 자체가 죽었으면 보수적으로 판정 불가.
		if v, got := s.met.Scalar(ctx, `count(hami_host_gpu_utilization_ratio)`); got && v > 0 {
			return true, true
		}
		return false, false
	}
	if sess.GpuMode == "timeslice" {
		return false, false // 타임슬라이싱은 컨테이너별 GPU 계측 지점이 없어 유휴 판정 불가
	}
	// 전용(exclusive/mig) — DCGM 이 곧 그 세션 사용량. 워크로드 Pod 라벨은 pod/exported_pod 로 갈린다.
	inner := fmt.Sprintf(`avg_over_time(DCGM_FI_DEV_GPU_UTIL{%%s}[%dm])`, windowMin)
	q := fmt.Sprintf(`avg(%s)`, metrics.DCGMPodScalar(inner, fmt.Sprintf("%q", sess.InstanceID)))
	v, got := s.met.Scalar(ctx, q)
	if !got {
		return false, false // GPU 메트릭 미가용 → 정지하지 않음(보수적)
	}
	return v < idleGPUThreshold, true
}

// RunPhaseReconciler는 컨테이너 세션의 라이브 Pod 상태를 주기적으로 DB 에 반영한다.
// 세션 목록을 열지 않아도(대시보드만 봐도) 관제/대시보드가 최신 phase·접속정보를 읽게 한다.
func (s *Service) RunPhaseReconciler(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			rows, err := s.repo.ListActiveContainer()
			if err != nil {
				continue
			}
			for i := range rows {
				s.refreshLive(ctx, &rows[i])
			}
		}
	}
}

// idleStop은 유휴 세션을 정지한다(자동 정지 공통 경로 사용).
func (s *Service) idleStop(ctx context.Context, sess *Session, windowMin int) {
	s.autoStop(ctx, sess, "session_idle_stop", fmt.Sprintf("[idle-reaper] stopped %s (idle > %dm)", sess.InstanceID, windowMin))
}

// autoStop은 세션 Pod/Service 를 정리하고 최종 정산 후 stopped 로 전이한다(소유자 무관, 시스템 동작).
func (s *Service) autoStop(ctx context.Context, sess *Session, action, logMsg string) {
	s.settle(ctx, sess, true) // 정지 전 사용분 최종 정산
	_ = s.prov.DeleteSessionPod(ctx, s.namespaceOf(sess), sess.InstanceID)
	_ = s.prov.DeleteSessionService(ctx, s.namespaceOf(sess), sess.InstanceID)
	if err := s.repo.SetPhase(sess.InstanceID, PhaseStopped); err != nil {
		return
	}
	log.Printf("%s", logMsg)
	if s.audit != nil {
		_ = s.audit.Insert(&audit.Log{ActorUsername: "system", Action: action, Target: sess.InstanceID, Result: "applied"})
	}
}

// RunLeaseReaper는 선착순(dynamic) 컨테이너 세션의 임대 시간이 만료되면 자동 정지한다.
// 임대 만료 = started_at + (maxLeaseHours + 연장횟수×extensionHours) 경과. dynamic 모드에서만 동작.
func (s *Service) RunLeaseReaper(ctx context.Context, interval time.Duration) {
	if !s.dynamicLease {
		return
	}
	if interval <= 0 {
		interval = time.Minute
	}
	log.Printf("[lease-reaper] started (maxLease=%dh, ext=%dh×%d)", s.leaseMaxHours, s.leaseExtHours, s.maxExtensions)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.reapLeaseOnce(ctx)
		}
	}
}

func (s *Service) reapLeaseOnce(ctx context.Context) {
	rows, err := s.repo.ListRunning()
	if err != nil {
		return
	}
	now := s.now()
	for i := range rows {
		sess := rows[i]
		if sess.Env == "ssh" || sess.StartedAt == nil { // 물리(SSH)는 노드 임대 만료로 별도 관리
			continue
		}
		totalH := s.leaseMaxHours + sess.ExtensionsUsed*s.leaseExtHours
		if totalH <= 0 {
			continue
		}
		if now.Sub(*sess.StartedAt) >= time.Duration(totalH)*time.Hour {
			s.autoStop(ctx, &sess, "session_lease_expired", fmt.Sprintf("[lease-reaper] stopped %s (lease > %dh)", sess.InstanceID, totalH))
		}
	}
}

// namespaceOf는 그룹(없으면 사용자) 기준 세션 네임스페이스를 만든다.
func (s *Service) namespaceOf(sess *Session) string {
	if sess.GroupID != nil {
		return fmt.Sprintf("%sgrp-%d", s.nsPrefix, *sess.GroupID)
	}
	return fmt.Sprintf("%suser-%d", s.nsPrefix, sess.UserID)
}
