package session

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
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

// idleGPUPowerW는 전용/물리 GPU 세션의 "연산 중" 판정 전력(W). DCGM util 이 오보고(0)돼도 이 이상
// 전력을 쓰면 실제 연산 중으로 보고 유휴로 죽이지 않는다. 유휴 4090 이 약 25W, 실부하가 130W 이상이라 60W 로 넉넉히 잡았다.
const idleGPUPowerW = 60.0

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
	// 세션 파드가 사내망으로 나가지 못하게 하는 이그레스 정책을 적용한다(세션 파드 라벨 대상).
	EnsureSessionEgressPolicy(ctx context.Context, spec k8s.SessionEgressSpec) error
	// 사용자 등록 SSH 공개키를 authorized_keys Secret 으로 반영(생성/갱신). sshd 사이드카가 마운트해
	// 접속마다 다시 읽으므로, 실행 중 세션에도 키 등록/교체가 즉시 반영된다.
	UpsertUserKeys(ctx context.Context, ns string, userID int64, keys string) error
	CreateSessionPod(ctx context.Context, s k8s.SessionSpec) error
	WaitPVCsBound(ctx context.Context, ns string, names []string, timeout time.Duration) // Pod 생성 전 PVC 바인딩 보장(스케줄 레이스 방지)
	DeleteSessionPod(ctx context.Context, ns, name string) error
	PodStatus(ctx context.Context, ns, name string) (*k8s.PodStatus, error)
	PodLogs(ctx context.Context, ns, name string, tail int64) (string, error)
	PodDescribe(ctx context.Context, ns, name string) (*k8s.PodDescribe, error)
	CreatePVC(ctx context.Context, spec k8s.PVCSpec) error
	DeletePVC(ctx context.Context, ns, name string) error
	PVCPhase(ctx context.Context, ns, name string) (string, error)
	PVCBackingNFS(ctx context.Context, ns, name string) (server, path string, ok bool)
	ListPVCsByPrefix(ctx context.Context, prefix string) ([]k8s.PVCRef, error) // 고아 세션 홈 PVC 탐지(T0)
	EnsureSharedNFSPVC(ctx context.Context, spec k8s.SharedNFSSpec) error
	DeleteSharedNFSPVC(ctx context.Context, ns, name string) error
	EnsureSessionService(ctx context.Context, s k8s.SvcSpec) error
	DeleteSessionService(ctx context.Context, ns, name string) error
	SessionServiceAccess(ctx context.Context, ns, name, mode string) (k8s.SvcAccess, error)
	FirstNodeIP(ctx context.Context) string
	NodeIP(ctx context.Context, node string) string                                                  // 노드 이름으로 InternalIP 조회(클러스터 DNS 에 노드명이 없어 SSH·웹터미널에 IP 가 필요하다)
	ListNodes(ctx context.Context) ([]k8s.LiveNode, error)                                           // 데이터셋 캐시 노드↔GPU타입 매칭용
	ExecTerminal(ctx context.Context, ns, pod, container string, cmd []string, tio k8s.ExecIO) error // 웹터미널(컨테이너 exec)
}

// DatasetCacheReader는 데이터셋 노드 로컬 캐시 배치를 조회한다(dataset.Service 구현).
// datasetID 별 (캐시 완료 노드 목록, 노드 로컬 경로). 빈 맵이면 캐시 비활성이라 전부 NFS 로 마운트한다.
type DatasetCacheReader interface {
	DatasetCachePlacement(ids []int64) (cachedNodes map[int64][]string, hostPaths map[int64]string)
}

// Service는 session 비즈니스 로직.
type Service struct {
	repo           Repository
	prov           Provisioner
	nsPrefix       string
	gateway        string
	storageClass   string // 영속 home(~/nfs)·공유 PVC 스토리지클래스(NFS RWX, 노드독립)
	localClass     string // 세션 전용 홈(/home/work) 로컬 스토리지클래스(노드로컬·WFFC, 예: local-path). 속도 위해 NFS 아님.
	sessionHomeGiB int    // ì¸ì í PVC íì ì©ë(GiB). local-path ë ê°ì íì§ ìëë¤
	audit          AuditReader
	met            *metrics.Client
	leaser         NodeLeaser
	charger        Charger          // 크레딧 소비 회계(nil=과금 비활성)
	limits         *policy.Resolver // 하드 리소스 상한(계층 해석; nil=미강제)
	expose         string           // 세션 웹 노출 모드(nodeport|loadbalancer)

	surgeDynamic   bool                                                        // 동적(서지) 가격 활성
	surgeIncrement int                                                         // 최대 가산 크레딧/시간(가용성 0일 때)
	availFn        func(ctx context.Context, gpuType string) (free, total int) // GPU 타입별 가용 조회
	// canPlaceFn은 "지금 이 세션이 들어갈 자리가 있는가"를 답한다(대기열 없음 정책의 관문).
	// 생성·재시작 두 경로가 같은 함수를 타야 규칙이 화면마다 갈리지 않는다.
	nodeSupportsFn func(ctx context.Context, node, gpuMode string) (ok, known bool) // 노드가 그 모드를 원리상 주는가
	canPlaceFn     func(ctx context.Context, p PlaceSpec) bool

	// admitMu·admitLock은 "관문 통과 후 예약 기록"을 한 번에 하나만 지나게 한다.
	// 이 구간을 열어두면 동시에 들어온 요청들이 모두 같은 여유를 보고 통과해(TOCTOU)
	// 뒤에 온 세션이 Pending 으로 매달린다. 대기열을 두지 않는 제품에서 가장 나쁜 상태다.
	// admitMu 는 프로세스 안, admitLock 은 replica 를 넘는 잠금(주입 없으면 프로세스 안까지만).
	admitMu   sync.Mutex
	admitLock func(ctx context.Context) (release func(), err error)

	scratchEnabled  bool   // 노드로컬 스크래치 마운트 활성
	scratchHostPath string // 스크래치 루트(/scratch). 계정폴더 = <root>/<username>

	localHomeOn   bool   // 물리노드 로컬 Home 특수 볼륨 선택 허용(물리 활성 시)
	localHomeHost string // 물리노드 로컬 home 루트(hostPath). 계정폴더 = <root>/<username>
	uidBase       int    // 컨테이너 안정 UID = uidBase + userID(물리 SSH 와 동일 공식). 기본 100000.
	sharedHome    bool   // 영속 home(~/nfs) 사용. false 면 세션은 emptyDir 로컬 임시만(RWX 불필요).
	maxStopped    int    // 사용자당 중단(대기) 세션 상한(0=무제한). 중단 세션은 로컬 홈 PVC 를 물고 있어 방치 시 노드 디스크 잠식.

	// 중단 세션의 홈 PVC 회수. 개수 상한(maxStopped)이 "새로 못 만들게" 막는 벽이라면,
	// 아래 둘은 "이미 쌓인 것"을 가격과 회수로 푸는 축이다.
	storagePrice func() int // 스토리지 GiB·월 단가(런타임 라이브 read). nil/0 이면 중단 세션 과금 없음.
	// 홈 회수(T1) 조건: (방치 일수, 노드 디스크 사용률 임계%). 유휴 타임아웃과 마찬가지로
	// 운영 중 조정되는 정책이라 매 틱 라이브 read 한다(nil=회수 비활성).
	homeReap func() (ttlDays int)

	memBurst int // 메모리 limit 배수(limit = 보장 request × 배수). 1 이하 = 상한 없음.

	// volumeUsedFn은 사용자가 이미 쓰고 있는 볼륨 용량(GiB)을 돌려준다(nil 이면 0).
	// 세션 홈을 볼륨과 같은 쿼터에서 세려면 볼륨 쪽 합이 필요한데, 세션 패키지가
	// 볼륨 저장소를 직접 알면 순환 의존이 되므로 함수로 주입받는다.
	volumeUsedFn func(userID int64) int

	datasetCache DatasetCacheReader // 데이터셋 노드 로컬 캐시 배치 조회(nil=항상 NFS)

	dynamicLease  bool // 선착순(dynamic) 모드에서 임대 연장 허용
	maxExtensions int  // 임대 연장 최대 횟수
	leaseMaxHours int  // 1회 기본 임대 시간(시간)
	leaseExtHours int  // 연장 1회당 추가 시간(시간)

	// 접속 게이트웨이(단기 토큰 발급). 설정하면 웹·SSH 접속을 게이트웨이 단일 접점으로 라우팅한다.
	gatewaySecret  []byte // API↔게이트웨이 공유 토큰키(GIOSK_GATEWAY_SECRET). 빈값=게이트웨이 비활성(직접 접속 폴백).
	gatewayScheme  string // 게이트웨이 웹 URL 스킴(https|http)
	gatewayHostArg string // SSH 접속 호스트(빈값=gateway 도메인)
	gatewaySSHPort int    // 게이트웨이 SSH 프록시 포트(기본 2222)
	sshdImage      string // 컨테이너 세션 sshd 사이드카 이미지(빈값=컨테이너 SSH 비활성)
	sshdPubKey     string // sshd 사이드카가 신뢰할 게이트웨이 공개키(authorized_keys)
	gatewayJump    string // 외부(VPN 밖) 접속용 SSH 점프 호스트(user@host). 빈값=내부 명령만.
	gatewaySSHKey  []byte // 게이트웨이 SSH 관리 개인키(PEM). 물리 세션 웹터미널이 노드로 SSH 할 때 사용(빈값=물리 웹터미널 비활성).

	// 세션 파드 이그레스 제한. 세션에서 사내망(스토리지·노드·API·다른 클러스터)으로 나가는 것을 막는다.
	// 빈 목록이면 정책을 만들지 않는다.
	egressDenyCIDRs  []string
	egressAllowCIDRs []string
	dnsServiceIP     string
}

// WithSessionEgress는 세션 파드 이그레스 제한 대역을 주입한다(deny 가 비면 정책 비활성).
func (s *Service) WithSessionEgress(deny, allow []string, dnsIP string) *Service {
	s.egressDenyCIDRs, s.egressAllowCIDRs, s.dnsServiceIP = deny, allow, dnsIP
	return s
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

// WithSurge는 동적/서지 가격(가용성이 낮을수록 단가 상승)을 설정한다. dynamic=false면 정적 단가.
// WithCapacityGate는 가용성 관문을 주입한다. 미주입이면 게이트 미적용(기존 동작).
// WithNodeSupports는 "이 노드가 그 GPU 모드를 주는가"를 묻는 함수를 주입한다.
// 관리자가 노드 공유 모드를 바꾼 경우를 자리 부족과 갈라 말하기 위해 쓴다.
func (s *Service) WithNodeSupports(fn func(ctx context.Context, node, gpuMode string) (bool, bool)) *Service {
	s.nodeSupportsFn = fn
	return s
}

func (s *Service) WithCapacityGate(fn func(ctx context.Context, p PlaceSpec) bool) *Service {
	s.canPlaceFn = fn
	return s
}

// WithAdmissionLock은 replica 를 넘는 배치 잠금을 주입한다(예: MySQL GET_LOCK).
// 미주입이면 프로세스 안 뮤텍스만으로도 단일 replica 배포에서는 충분하다.
func (s *Service) WithAdmissionLock(fn func(ctx context.Context) (func(), error)) *Service {
	s.admitLock = fn
	return s
}

// admit은 검사와 예약을 상호배제 구간에서 실행한다. fn 안에서 관문(상한·크레딧·가용성)을 통과시키고
// 반드시 예약까지(=세션 행 기록·phase 전이) 마쳐야 다음 요청이 이 자리를 다시 보지 않는다.
// Pod 생성·PVC 대기 같은 느린 작업은 fn 밖에서 한다(잠금을 오래 쥐면 전체가 직렬화된다).
func (s *Service) admit(ctx context.Context, fn func() error) error {
	s.admitMu.Lock()
	defer s.admitMu.Unlock()
	if s.admitLock != nil {
		release, err := s.admitLock(ctx)
		if err == nil {
			defer release()
		} else {
			// 분산 잠금을 못 얻어도 진행한다. 프로세스 안 뮤텍스는 이미 쥐었고, 잠금 배관 문제로
			// 세션 생성을 통째로 막는 편이 더 나쁘다. 다만 조용히 넘어가지는 않는다.
			log.Printf("[session] admission lock unavailable, falling back to in-process lock: %v", err)
		}
	}
	return fn()
}

// PlaceSpec은 가용성 관문에 넘기는 배치 요청이다. Node 가 비면 클러스터 전체,
// 채워지면 그 노드 한 대 안에서만 자리를 묻는다(노드 고정 세션).
type PlaceSpec struct {
	Node        string
	GpuMode     string
	GpuType     string
	GpuCount    int
	VramMB      int
	CorePercent int
}

// checkCapacity는 세션 사양으로 관문을 통과시킨다. 자리가 없으면 ErrNoCapacity.
func (s *Service) checkCapacity(ctx context.Context, sess *Session) error {
	return s.checkCapacityOn(ctx, sess, "")
}

// checkCapacityOn은 node 를 지정해 그 노드 안에서만 자리를 묻는다(빈 문자열이면 전체).
func (s *Service) checkCapacityOn(ctx context.Context, sess *Session, node string) error {
	if s.canPlaceFn == nil || sess == nil || sess.Env == "ssh" {
		return nil // 물리(SSH)는 임대 경로가 따로 판정한다(ErrLeaseUnavailable)
	}
	if s.canPlaceFn(ctx, PlaceSpec{
		Node: node, GpuMode: sess.GpuMode, GpuType: sess.GpuType,
		GpuCount: sess.GpuCount, VramMB: sess.VramMB, CorePercent: sess.CorePercent,
	}) {
		return nil
	}
	return ErrNoCapacity
}

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
// auth.KeySyncer 구현. 클러스터 미가용이나 세션 없음은 정상이라 아무것도 하지 않는다.
func (s *Service) SyncUserKeys(ctx context.Context, userID int64) error {
	if !s.containerSSH() {
		return nil
	}
	list, err := s.repo.ListByUser(userID, 0)
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

// WithLocalHome은 물리노드 로컬 Home 특수 볼륨(컨테이너가 고르면 hostPath + 노드핀)을 설정한다.
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
	// 중단(대기) 세션 상한. 각 중단 세션이 로컬 홈 PVC(노드 디스크)를 점유하므로 무한정 쌓이면 안 된다.
	// 초과하면 새 세션 생성을 막아 기존 중단 세션 삭제를 유도한다. 물리 SSH 는 로컬 PVC 가 없어 제외한다.
	// 리졸버가 없으면 전역 config 값(s.maxStopped)으로 폴백.
	if s.limits == nil {
		if s.maxStopped > 0 && sess.Env != "ssh" && s.repo.CountStopped(userID) >= s.maxStopped {
			return ErrStoppedLimit
		}
		return nil
	}
	lim := s.limits.Resolve(userID)
	// 중단 상한은 계층 해석값(개인, 그룹, 조직, 전역 순). 전역 폴백은 globalQuota(=cfg.Quota.MaxStoppedSessions).
	if lim.MaxStoppedSessions > 0 && sess.Env != "ssh" && s.repo.CountStopped(userID) >= lim.MaxStoppedSessions {
		return ErrStoppedLimit
	}
	if lim.MaxConcurrentSessions > 0 && s.repo.CountActive(userID) >= lim.MaxConcurrentSessions {
		return ErrSessionLimit
	}
	if err := s.checkHomeQuota(userID, sess, lim.MaxVolumeGiB); err != nil {
		return err
	}
	return s.checkResourceLimits(userID, sess)
}

// checkHomeQuota는 이 세션의 홈이 계정 볼륨 쿼터 안에 들어가는지 본다.
//
// 홈이 이미지 기반이 되면서 PVC 에 적은 용량이 실제로 노드 디스크를 예약한다. 볼륨과
// 성격이 같아졌으므로 같은 쿼터에서 센다. 그러지 않으면 사용자가 세션만 계속 만들어
// 볼륨 쿼터를 우회할 수 있다.
//
// 물리(SSH) 임대는 노드를 통째로 빌려주는 것이라 홈 용량 개념이 없어 제외한다.
// 쿼터가 0(미설정)이면 제한하지 않는다. 다른 하드리밋과 같은 규칙이다.
func (s *Service) checkHomeQuota(userID int64, sess *Session, quotaGiB int) error {
	if sess.Env == "ssh" || quotaGiB <= 0 {
		return nil
	}
	want := s.sessionHomeGiB
	if sess.HomeGiB != nil {
		want = *sess.HomeGiB
	}
	used := s.repo.AllocatedHomeGiB(userID, s.sessionHomeGiB)
	if s.volumeUsedFn != nil {
		used += s.volumeUsedFn(userID)
	}
	if used+want > quotaGiB {
		return ErrHomeQuota
	}
	return nil
}

// checkResourceLimits는 사양 자체의 상한(GPU 개수·VRAM)만 본다. 세션 수(동시·중단) 상한은 제외.
// 중단 세션 재구성은 세션 수를 늘리지 않으므로 개수 상한을 다시 물으면 안 된다(이미 세어진 세션).
func (s *Service) checkResourceLimits(userID int64, sess *Session) error {
	if s.limits == nil {
		return nil
	}
	lim := s.limits.Resolve(userID)
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
	return &Service{repo: repo, prov: prov, nsPrefix: nsPrefix, gateway: gateway, storageClass: storageClass, localClass: "local-path", sessionHomeGiB: sessionHomeGiBDefault, uidBase: 100000, sharedHome: true}
}

// WithSharedHome은 영속 home(~/nfs) 사용 여부를 설정한다(설치시 고정). false=세션 순수 로컬 임시.
func (s *Service) WithSharedHome(on bool) *Service { s.sharedHome = on; return s }

// WithLocalClass는 세션 전용 홈(/home/work) 로컬 스토리지클래스를 설정한다(설치시 고정, 예: local-path).
func (s *Service) WithLocalClass(sc string) *Service {
	if sc != "" {
		s.localClass = sc
	}
	return s
}

// WithSessionHomeGiB는 세션 홈 PVC 표시 용량을 설정한다(0 이하면 기본값).
func (s *Service) WithSessionHomeGiB(gib int) *Service {
	if gib > 0 {
		s.sessionHomeGiB = gib
	}
	return s
}

// WithMaxStopped는 사용자당 중단(대기) 세션 상한을 설정한다(0=무제한).
func (s *Service) WithMaxStopped(n int) *Service { s.maxStopped = n; return s }

// WithStoragePrice는 중단 세션 홈 스토리지 과금 단가(GiB·월)를 라이브 getter 로 주입한다.
// 볼륨 과금과 같은 단가를 쓴다. 사용자 입장에서 "디스크는 어디에 두든 같은 값"이어야 하므로.
func (s *Service) WithStoragePrice(f func() int) *Service { s.storagePrice = f; return s }

// WithMemBurst는 메모리 limit 배수를 설정한다(1 이하=상한 없음).
// request 는 GPU 지분 비례 최소 보장이고, limit 은 그 배수까지만 버스트를 허용하는 천장이다.
func (s *Service) WithMemBurst(n int) *Service { s.memBurst = n; return s }

// WithVolumeUsage는 사용자의 볼륨 사용량 조회를 주입한다. 세션 홈을 볼륨과 같은
// 쿼터에서 세기 위한 것으로, 미주입이면 홈만 세고 볼륨은 세지 않는다.
func (s *Service) WithVolumeUsage(fn func(userID int64) int) *Service {
	s.volumeUsedFn = fn
	return s
}

// WithHomeReap는 중단 세션 홈 회수(T1)의 방치 일수를 라이브 getter 로 주입한다.
// 매 틱 다시 읽으므로 관리자가 운영 중 바꾼 값이 다음 틱부터 반영된다(재배포 불필요).
// getter 가 0 을 주면 그 틱은 회수하지 않는다.
func (s *Service) WithHomeReap(f func() (ttlDays int)) *Service {
	s.homeReap = f
	return s
}

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

const homeSizeGiB = 10 // 영속 home(~/nfs) 용량. NFS 라 여러 세션이 공유한다.

// 세션 홈(/home/work) PVC 표시 용량 기본값.
//
// local-path 는 hostPath 디렉터리라 이 값을 하드 쿼터로 강제하지 않는다. 노드 디스크를 지키는 것은
// nodeCleaner(scratch 85%, home 92%)와 홈 회수(88%), 그리고 디스크 알림(85%)이다.
// 그래서 실제 쓸 수 있는 만큼 넉넉히 잡아 사용자에게 거짓 한도를 보여주지 않는다.
const sessionHomeGiBDefault = 200

// Create는 스펙을 확정하고 Pod 를 프로비저닝한 뒤 세션을 기록한다.
func (s *Service) Create(ctx context.Context, userID int64, username string, req CreateReq) (*Session, error) {
	// 동시 세션 상한은 checkHardLimits(정책 계층 해석)에서만 강제한다.
	// billing.credit.maxConcurrentSessions 는 폐기했다. 동시세션은 정책(quota)으로 일원화.
	// 요청이 팀을 지정하면(활성 스코프) 활성 멤버십을 먼저 검증한다(SSH·컨테이너 공통).
	// 임의 팀에 붙어 남의 크레딧을 소모하는 것 차단.
	if req.GroupID != nil && !s.repo.IsGroupMember(userID, *req.GroupID) {
		return nil, ErrMustJoinTeam
	}
	if req.Env == "ssh" {
		return s.createSSH(ctx, userID, username, req)
	}
	sess, err := s.buildSession(ctx, userID, req)
	if err != nil {
		return nil, err
	}
	// 팀 없는 세션은 금지다. 세션은 항상 팀 컨텍스트에서 만든다(크레딧이 팀 지갑에 귀속). GroupID 미지정이면
	// 사용자의 대표 팀으로 채운다. 크레딧 모드에서 소속 팀이 전혀 없으면 거부(차감할 팀 지갑 없음).
	if sess.GroupID == nil && s.limits != nil {
		if _, gid := s.limits.HierOfUser(userID); gid > 0 {
			sess.GroupID = &gid
		}
	}
	if s.charger != nil && sess.GroupID == nil {
		return nil, ErrMustJoinTeam
	}
	imageRef, err := s.repo.ImageRef(req.ImageID)
	if err != nil {
		return nil, err
	}
	// 관문 통과와 예약(세션 행 기록)은 한 덩어리로 처리한다. 예전에는 Pod 를 다 만든 뒤에야 행을 남겨,
	// 그 사이에 들어온 요청이 같은 자리를 또 승인받고 뒤에 온 세션이 Pending 으로 매달렸다.
	// 이제 행이 곧 예약이다. 이 시점부터 다른 요청의 가용성 계산에 즉시 반영된다.
	if err := s.admit(ctx, func() error {
		if err := s.checkHardLimits(userID, sess); err != nil {
			return err // 1차: 하드 정책(크레딧 무관 절대 벽)
		}
		if err := s.checkAffordable(userID, gidOf(sess), sess.PricePerHour); err != nil {
			return err // 2차: 크레딧 모드 잔액 부족이면 생성 거부
		}
		if err := s.checkCapacity(ctx, sess); err != nil {
			return err // 3차: 지금 들어갈 자리가 있는지. 없으면 대기시키지 않고 거절한다
		}
		return s.repo.Create(sess) // 예약 확정
	}); err != nil {
		return nil, err
	}
	ns := s.namespaceOf(sess)
	mounts := s.resolveMounts(ctx, ns, userID, req)
	// 통합 home 모델: home(/home/work)은 항상 노드 로컬(임시). 기본 emptyDir,
	// 로컬 Home 선택 시 그 물리노드 디스크(hostPath, 노드핀). 개인 영속 저장소는 ~/nfs(NFS PVC)로 별도 마운트.
	// 어디서나 일관된 규칙이다: home 은 로컬(임시), ~/nfs 는 영속. 컨테이너와 물리 SSH 가 같다.
	var requireNode string
	if req.Node != "" {
		requireNode = req.Node // 사용자가 데이터셋 노드 picker 에서 고른 실행 노드 = 하드 핀(자동 배치 대신)
	}
	if req.LocalHomeNode != "" {
		lh, err := s.resolveLocalHome(ctx, userID, username, req.LocalHomeNode)
		if err != nil {
			return nil, s.rollback(ctx, sess, err)
		}
		mounts = append(mounts, lh) // 물리노드 로컬 디스크 home 을 /home/work 로(노드핀, 노드 디스크에 영속)
		requireNode = req.LocalHomeNode
	} else {
		// 세션 전용 영속 홈: 중단해도 데이터 유지, 삭제 시 함께 제거. (예전 emptyDir=중단 시 유실이었음)
		homePVC, err := s.ensureSessionHome(ctx, ns, sess)
		if err != nil {
			return nil, s.rollback(ctx, sess, err)
		}
		mounts = append(mounts, k8s.VolMountSpec{PVCName: homePVC, MountPath: homeMount})
	}
	// 개인 영속 NFS 저장소를 ~/nfs 로 붙인다(세션이 사라져도 유지되고 노드에 묶이지 않는다).
	// sharedHome=false 면 영속 home 을 쓰지 않으므로 ~/nfs 마운트를 생략한다(세션은 emptyDir 로컬 임시만).
	if s.sharedHome {
		persistPVC, err := s.ensureHome(ctx, ns, userID)
		if err != nil {
			return nil, s.rollback(ctx, sess, err)
		}
		mounts = append(mounts, k8s.VolMountSpec{PVCName: persistPVC, MountPath: homeMount + "/nfs"})
	}
	// 데이터셋은 승인·적재완료된 전체를 모든 세션에 자동 마운트한다(사용자 선택 불필요).
	// 로컬 캐시된 노드가 있으면 그 노드로 핀하고 hostPath(빠름)로, 아니면 NFS(느림) 자동.
	// 단, 로컬 Home 으로 이미 노드가 고정됐으면 그 노드 기준으로 데이터셋 배치를 판정한다.
	req.Datasets = s.repo.MountableDatasetIDs()
	dsTarget, dsCached, dsHostPath := requireNode, map[int64][]string(nil), map[int64]string(nil)
	if requireNode == "" {
		dsTarget, dsCached, dsHostPath = s.pickDatasetNode(ctx, req.Datasets, sess.GpuType, sess.GpuMode, sess.GpuCount)
		requireNode = dsTarget
	} else if s.datasetCache != nil {
		dsCached, dsHostPath = s.datasetCache.DatasetCachePlacement(req.Datasets)
	}
	mounts = append(mounts, s.resolveDatasets(ctx, ns, req.Datasets, dsTarget, dsCached, dsHostPath)...)
	// 이미지 캐시 노드를 소프트 선호(빠른 시작). nodeSelector(GPU 타입) 안에서만 효과가 있어 타입이 맞는 캐시노드를 우선한다.
	var preferNodes []string
	if req.ImageID != 0 {
		preferNodes = s.repo.CachedNodes(req.ImageID)
	}
	if err := s.provision(ctx, ns, sess, imageRef, "", mounts, preferNodes, requireNode); err != nil {
		return nil, s.rollback(ctx, sess, err) // home 은 mounts(emptyDir/hostPath)로 들어가므로 HomePVC 는 비움
	}
	s.attachMounts(sess.ID, userID, req)
	s.recordCreate(userID, username, sess.InstanceID)
	return sess, nil
}

// rollback은 예약(세션 행)을 만든 뒤 프로비저닝이 실패했을 때 그 예약을 되돌린다.
// 되돌리지 않으면 아무것도 뜨지 않은 세션이 남아 남의 자리를 계속 막는다(유령 예약).
// 원래 실패 사유를 그대로 돌려주고, 정리 실패는 로그로만 남긴다(사용자에게 알릴 것은 원인 하나).
func (s *Service) rollback(ctx context.Context, sess *Session, cause error) error {
	if err := s.deleteSession(ctx, sess); err != nil {
		log.Printf("[session] rollback failed for %s: %v (cause: %v)", sess.InstanceID, err, cause)
	}
	return cause
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
		Phase:     PhaseRunning, // 임대는 즉시 사용 가능하므로(node-agent reconcile 약 10초) 바로 과금 대상
		StartedAt: &now,
	}
	sess.PricePerHour = s.priceOf(ctx, sess) // 노드 대여 단가(=GPU타입 단가×노드 GPU수)
	// 노드 임대 자체는 이미 원자적이지만(AcquireLease), 동시 세션 상한 검사와 세션 행 기록까지
	// 한 구간에 묶어야 "동시에 두 개가 상한을 통과"하는 창이 사라진다.
	if err := s.admit(ctx, func() error {
		if err := s.checkHardLimits(userID, sess); err != nil {
			return err // 1차: 하드 정책(노드 GPU수가 상한 초과면 임대 거부)
		}
		if err := s.checkAffordable(userID, gidOf(sess), sess.PricePerHour); err != nil {
			return err // 2차: 크레딧 모드 잔액 부족이면 물리 임대 거부
		}
		if err := s.leaser.CreateLeaseFor(ctx, req.Node, userID, sess.InstanceID); err != nil {
			return err
		}
		if err := s.repo.Create(sess); err != nil {
			_ = s.leaser.ReleaseLeaseFor(ctx, sess.InstanceID) // 행이 없으면 반납할 길이 없어진다(유령 임대)
			return err
		}
		return nil
	}); err != nil {
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

// recordAct는 세션 대상 감사 로그 1건을 남긴다(사용자명은 저장소에서 조회).
func (s *Service) recordAct(userID int64, action, instanceID string) {
	if s.audit == nil {
		return
	}
	uid := userID
	_ = s.audit.Insert(&audit.Log{ActorID: &uid, ActorUsername: s.usernameOf(userID), Action: action, Target: instanceID, Result: "success"})
}

// homePVCPrefix는 세션 전용 홈 PVC 이름 접두사. 고아 탐지(T0)가 이 접두사로 훑는다.
const homePVCPrefix = "sh-"

// sessionHomePVC는 세션 전용 영속 홈(/home/work) PVC 이름(세션 인스턴스에 귀속).
func sessionHomePVC(instanceID string) string { return homePVCPrefix + instanceID }

// ensureSessionHome은 세션 전용 영속 홈(/home/work) PVC 를 멱등 생성한다.
// 중단(Stop)해도 유지되고 재개하면 그대로 복원된다. 삭제(Delete)할 때만 함께 제거한다.
// 로컬 스토리지클래스(local-path, RWO·WFFC): 노드 로컬 디스크라 빠르다(홈 I/O 를 NFS 로 보내면 느림).
// WFFC 라 Pod 스케줄 시점에 그 노드에 바인딩되고, 이후 PV 노드 어피니티가 재시작을 같은 노드로 되돌린다.
func (s *Service) ensureSessionHome(ctx context.Context, ns string, sess *Session) (string, error) {
	name := sessionHomePVC(sess.InstanceID)
	if err := s.prov.CreatePVC(ctx, k8s.PVCSpec{
		Namespace: ns, Name: name, SizeGiB: s.homeGiBOf(sess),
		StorageClass: s.localClass, AccessMode: "RWO",
	}); err != nil {
		return "", err
	}
	return name, nil
}

// homeGiBOf는 이 세션의 홈 용량이다. 세션에 기록된 값이 없으면(이 기능 이전에 만들어진
// 세션) 설치 기본값으로 본다.
func (s *Service) homeGiBOf(sess *Session) int {
	if sess != nil && sess.HomeGiB != nil && *sess.HomeGiB > 0 {
		return *sess.HomeGiB
	}
	return s.sessionHomeGiB
}

// ensureHome은 사용자 홈 PVC(/home/work 영속)를 멱등 생성하고 이름을 반환한다.
// 하이브리드(물리)면 NFS 기반 RWX, 컨테이너면 설정 스토리지클래스 RWO.
func (s *Service) ensureHome(ctx context.Context, ns string, userID int64) (string, error) {
	name := fmt.Sprintf("home-%d", userID)
	// 영속 home(~/nfs)은 사용자의 모든 세션이 공유하는 노드 무관 저장소라 항상 RWX 다.
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
//   - 물리 비활성이거나 미설정이면 ErrLocalHomeUnavailable
//   - 사용자가 그 노드를 대여한 적 없으면 ErrLocalHomeForbidden(난립 방지·보안)
//   - 노드가 Ready 아니면 ErrLocalHomeUnavailable(가용성 판단)
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
//   - 같은 ns(내 볼륨): 존재하는 PVC 만 마운트한다. 없으면 생략해서 "무한 준비중"을 막는다.
//   - 다른 ns(공유 볼륨): NFS 백엔드를 같은 경로로 세션 ns 에 정적 복제 후 마운트(전용 스토리지면 불가).
func (s *Service) mountFor(ctx context.Context, ns string, userID, volID int64, mountPath string) (k8s.VolMountSpec, bool) {
	acc, ok := s.repo.VolumeAccess(volID, userID)
	if !ok || acc.PVCName == "" {
		log.Printf("[session] 볼륨 %d 접근권한 없음(user %d): 마운트 생략", volID, userID)
		return k8s.VolMountSpec{}, false
	}
	ro := acc.Perm == "ro"
	pvc := acc.PVCName
	switch {
	case acc.PVCNamespace == ns: // 내 볼륨은 그대로 사용
		if _, err := s.prov.PVCPhase(ctx, ns, pvc); err != nil {
			log.Printf("[session] 볼륨 %d: PVC %s/%s 없음(%v), 마운트 생략", volID, ns, pvc, err)
			return k8s.VolMountSpec{}, false
		}
	default: // 공유 볼륨(다른 ns)은 NFS 경로를 세션 ns 에 정적 복제
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
func (s *Service) pickDatasetNode(ctx context.Context, ids []int64, gpuType, gpuMode string, gpuCount int) (string, map[int64][]string, map[int64]string) {
	if s.datasetCache == nil || len(ids) == 0 {
		return "", nil, nil
	}
	cached, hostPaths := s.datasetCache.DatasetCachePlacement(ids)
	if len(hostPaths) == 0 {
		return "", nil, nil
	}
	// 후보는 "핀을 걸어도 실제로 그 노드에 뜰 수 있는" 노드로 제한한다. GPU 타입뿐 아니라
	// 공유 전략(전용/HAMi/타임슬라이싱)과 cordon 까지 봐야 한다. 파드에는 공유 전략 affinity 가 함께
	// 구워지므로, 그걸 무시하고 핀하면 두 조건이 서로를 배제해 영구 Pending 이 된다.
	typeNodes := s.pinnableNodes(ctx, gpuType, gpuMode)
	// 사용자가 노드를 직접 고르지 않은 경우, 데이터셋 캐시 노드로 "하드핀"하면 그 노드가 만석일 때
	// 다른 빈 노드가 있어도 영구 Pending 이 된다. 그러니 GPU 여유가 있는 캐시 노드만 후보로 삼고,
	// 여유 있는 캐시 노드가 없으면 핀하지 않는다(빈 값이면 데이터셋은 NFS 로 읽고 스케줄러가 알아서 배치).
	freeGpu := s.freeGpuByNode(ctx)
	need := 1
	if gpuMode == "exclusive" {
		need = max1(gpuCount)
	}
	best, bestN := "", 0
	score := map[string]int{}
	for _, nodes := range cached {
		for _, n := range nodes {
			if typeNodes != nil && !typeNodes[n] {
				continue
			}
			if freeGpu != nil && freeGpu[n] < need {
				continue // 캐시돼 있어도 GPU 여유가 없으면 핀 후보에서 뺀다(만석 노드에 핀 금지)
			}
			score[n]++
			if score[n] > bestN {
				best, bestN = n, score[n]
			}
		}
	}
	if bestN == 0 {
		// 캐시는 있는데 그 노드에 이 세션을 못 넣는 상황이다(만석이거나 공유 전략이 안 맞거나 cordon).
		// 속도를 포기하고 NFS 로 읽는 편이, 뜨지 못하는 세션보다 낫다.
		log.Printf("[session] 데이터셋 캐시 노드 핀 생략(gpuType=%s mode=%s 로 배치 가능한 캐시 노드 없음): 데이터셋을 NFS 로 마운트", gpuType, gpuMode)
		return "", nil, nil
	}
	return best, cached, hostPaths
}

// pinnableNodes는 하드 핀을 걸어도 실제로 스케줄될 수 있는 노드 집합이다(nil 이면 판단 불가라 제한 없음).
// 파드에 구워지는 required 제약과 같은 조건을 미리 적용한다.
//   - GPU 타입 일치(nodeSelector 와 동일 기준)
//   - 공유 전략 호환(k8s.ShareModeAllows, 파드 affinity 와 동일 판정)
//   - Ready 이고 cordon 아님. 물리 임대가 노드를 cordon 하므로 이걸 빼면 임대 중 노드에 핀할 수 있다.
func (s *Service) pinnableNodes(ctx context.Context, gpuType, gpuMode string) map[string]bool {
	live, err := s.prov.ListNodes(ctx)
	if err != nil || len(live) == 0 {
		return nil
	}
	out := map[string]bool{}
	for _, n := range live {
		if !n.Ready || n.Cordoned {
			continue
		}
		if !(gpuMode == "cpu" || gpuType == "" || n.GpuType == gpuType) {
			continue
		}
		if !k8s.ShareModeAllows(gpuMode, n.ShareMode) {
			continue
		}
		out[n.Name] = true
	}
	return out
}

// freeGpuByNode는 노드별 남은 GPU 수를 근사한다(데이터셋 캐시 핀의 여유 판정용).
// 전용 세션은 GPU 개수만큼 점유로 계산, 분할/타임셰어는 한 장을 나눠 쓰므로 여유를 깎지 않는다(패킹 가능).
func (s *Service) freeGpuByNode(ctx context.Context) map[string]int {
	live, err := s.prov.ListNodes(ctx)
	if err != nil || len(live) == 0 {
		return nil
	}
	free := map[string]int{}
	for _, n := range live {
		free[n.Name] = atoiCap(n.GpuCapacity)
	}
	running, err := s.repo.ListRunning()
	if err != nil {
		return free
	}
	for i := range running {
		r := running[i]
		if r.Node == "" || r.Env == "ssh" || r.GpuMode != "exclusive" {
			continue
		}
		free[r.Node] -= max1(r.GpuCount)
	}
	return free
}

// nodesOfType는 GPU 타입이 일치하는(또는 CPU면 전체) Ready 노드 집합을 반환한다(미가용 시 nil=제한없음).
// gpuShareOf는 세션이 차지하는 "노드 대비 GPU 지분"(0~1)을 반환한다.
//
//	전용 N개      : N / 노드GPU수
//	분할(코어 c%) : (c/100) / 노드GPU수
//	타임셰어 슬롯 : (1/슬롯수) / 노드GPU수
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
// 기준은 그 GPU 타입의 "가장 작은 후보 노드"다. 그래야 어느 후보에 떨어져도 보장이 성립한다.
// limits 는 걸지 않으므로 여유가 있으면 이 값을 넘겨 쓸 수 있다(최소 보장이지 상한이 아님).
func (s *Service) applyGuarantee(ctx context.Context, sess *Session) {
	if sess.GpuMode == "cpu" || sess.GpuType == "" {
		return // CPU 단일 모드는 별도 정책이라 요청을 걸지 않는다(GPU 지분 개념이 없다)
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
	// 정책: 최소만 보장(request)하고 상한은 두지 않는다(limit 미설정이면 자유 버스트·경쟁).
	// request 는 GPU 지분에 비례하되 requestFactor(0.5)를 곱해 보수적으로 잡는다. 이유:
	//   한 노드의 세션 GPU 지분 합은 설계상 ≤ 1(공유=나눠가짐, 전용=독점) 이므로,
	//   노드 위 CPU/Mem request 총합 = 0.5 × 노드 × Σ(share) ≤ 노드의 50%.
	//   항상 50% 이상 헤드룸이 남아 "CPU/Mem 부족으로 스케줄 실패(영구 Pending)"가 원천 차단된다.
	//   전용(share=1)도 노드의 50%만 요청하므로 반드시 배치되고, limit 이 없어 노드 전체까지 버스트한다.
	// "limit만" 방식은 금물: request 없이 limit 만 주면 k8s 가 request=limit 으로 자동 설정해 다시 100% 요청이 된다.
	const requestFactor = 0.5
	// share 상한은 1.0 이다. 요청 GPU 수가 노드 GPU 수를 초과해 share>1 이 되어도(그런 세션은 어차피
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
			log.Printf("[session] 데이터셋 %d 미프로비저닝(PVC 없음): 마운트 생략", id)
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

// mountSafe는 데이터셋 이름을 마운트 경로 세그먼트로 정규화한다(공백과 슬래시를 '-' 로).
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

// buildSession은 요청과 오퍼링을 받아 세션 엔티티로 스펙을 확정한다.
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
	// 홈 용량은 세션마다 기록한다. 재시작 때 같은 크기로 다시 올려야 하고, 나중에
	// 기본값을 바꿔도 이미 만든 세션의 홈이 달라지면 안 되기 때문이다.
	if req.HomeGiB > 0 {
		sess.HomeGiB = &req.HomeGiB
	} else {
		def := s.sessionHomeGiB
		sess.HomeGiB = &def
	}
	if req.OfferingID != nil {
		if err := s.applyOffering(sess, *req.OfferingID); err != nil {
			return nil, err
		}
	}
	// 분할(VRAM/코어%)은 shared 모드 전용 값이라 전용·CPU 세션엔 남기지 않는다(스펙 표기 혼동 방지).
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

// webChannelsFor는 이미지 channels 설정을 활성 웹 채널 목록으로 변환한다.
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
	// 커스텀 웹 포트는 시크릿이나 env 없이 포트포워딩만 한다(제네릭 앱). EnvKey 가 비면 channelEnv 가 주입을 생략한다.
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
			// 타임슬라이싱 슬롯은 GPU 1개를 N분할한 몫이라 전용단가를 슬롯수로 나눈다(실 점유량 비례).
			base = perHour / max1(s.timesliceSplitFor(ctx, sess.GpuType))
		case "cpu":
			// CPU 단일 모드(데이터 다운로드·전처리 등)는 단일 단가(시간당)를 쓴다. CPU/메모리 세부 단가는 두지 않는다:
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
	// 세션 파드 이그레스 제한. 파드가 뜨기 전에 걸어야 정책 없는 틈이 생기지 않는다.
	// 정책 적용 실패로 세션 생성을 막지는 않되(가용성 우선), 조용히 넘어가면 안 되므로 로그를 남긴다.
	if len(s.egressDenyCIDRs) > 0 {
		if err := s.prov.EnsureSessionEgressPolicy(ctx, k8s.SessionEgressSpec{
			Namespace:    ns,
			DenyCIDRs:    s.egressDenyCIDRs,
			AllowCIDRs:   s.egressAllowCIDRs,
			DNSServiceIP: s.dnsServiceIP,
		}); err != nil {
			log.Printf("[session] %s 이그레스 정책 적용 실패(세션은 계속 생성): %v", ns, err)
		}
	}
	// 컨테이너 SSH 가 켜져 있으면 사용자 공개키 Secret 을 먼저 준비한다(키를 등록하지 않았으면 빈 파일이라
	// 세션은 정상 기동하고, 나중에 등록하면 Secret 갱신만으로 이 세션에도 바로 반영된다).
	if s.containerSSH() {
		if err := s.prov.UpsertUserKeys(ctx, ns, sess.UserID, s.repo.UserSSHKey(sess.UserID)); err != nil {
			return err
		}
	}
	// PVC(홈/볼륨/데이터셋)는 생성 즉시 바인딩되지 않는다. hami-scheduler 는 unbound PVC 로 스케줄
	// 실패 후 재큐잉을 안 해 Pod 가 영구 Pending 이 되므로, Pod 생성 전에 바인딩을 기다린다.
	var pvcNames []string
	if homePVC != "" {
		pvcNames = append(pvcNames, homePVC)
	}
	for _, m := range mounts {
		if m.PVCName != "" {
			pvcNames = append(pvcNames, m.PVCName)
		}
	}
	s.prov.WaitPVCsBound(ctx, ns, pvcNames, 90*time.Second)

	channels := s.sessionChannels(sess.ImageID, sess.WebPassword)
	if err := s.prov.CreateSessionPod(ctx, k8s.SessionSpec{
		Namespace:    ns,
		Name:         sess.InstanceID,
		Image:        imageRef,
		GpuType:      sess.GpuType,
		GpuMode:      sess.GpuMode,
		GpuCount:     sess.GpuCount,
		VramMB:       sess.VramMB,
		CorePercent:  sess.CorePercent,
		CPUCores:     sess.CPUCores,
		MemGB:        sess.MemGB,
		EphemeralGiB: s.ephemeralGiB(sess.UserID), // 정책 해석값(0=매퍼 기본 캡). 노드 디스크 DoS 방지.
		MemBurst:     s.memBurst,                  // 메모리 limit 배수. 노드 RAM 고갈로 남의 세션이 축출되는 것을 막는다
		Labels:       sess.labels(),
		UID:          s.uidBase + int(sess.UserID), // 안정 UID(물리 SSH 와 같은 공식)라 NFS 권한이 일관된다
		HomePVC:      homePVC,
		Volumes:      mounts,
		WebChannels:  channels,     // 이미지 channels 기반(vscode/jupyter)
		SSHDImage:    s.sshdImage,  // 컨테이너 SSH 사이드카(빈값=비활성)
		SSHDPubKey:   s.sshdPubKey, // 사이드카가 신뢰할 게이트웨이 공개키(게이트웨이 off 면 빈값)
		// 사용자 등록 공개키 Secret(위에서 upsert). Optional 마운트라 키 미등록이어도 세션은 뜬다.
		UserKeysSecret: k8s.UserKeysSecretName(sess.UserID),
		ScratchHost:    s.scratchHostOf(sess.UserID),
		PreferNodes:    preferNodes, // 이미지 캐시 노드 소프트 선호(빠른 시작)
		RequireNode:    requireNode, // 데이터셋 로컬 캐시 노드 하드 핀(hostPath 마운트)
	}); err != nil {
		return err
	}
	// 세션 Service. 게이트웨이가 켜져 있으면 모든 채널 포트(+sshd 22)를 ClusterIP 로 노출한다(라우팅용).
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
// 그 외(빈값이나 예전 portforward 등)는 모두 nodeport 로 수렴한다. 게이트웨이·인그레스 없이 바로 접속한다.
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
func (s *Service) List(ctx context.Context, userID, groupID int64) ([]Session, error) {
	rows, err := s.repo.ListByUser(userID, groupID)
	if err != nil {
		return nil, err
	}
	// 회수 면책 대상은 사용자 전체 기준으로 한 번만 조회한다(목록은 팀 스코프로 걸러져 있을 수 있다).
	exempt := s.repo.NewestStoppedInstance(userID)
	for i := range rows {
		s.refreshLive(ctx, &rows[i])
		s.fillChannels(&rows[i])
		rows[i].ReclaimExempt = exempt != "" && rows[i].InstanceID == exempt
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
	// 웹터미널(API 호스팅 exec)은 컨테이너 세션에 항상 노출한다(사이드카·게이트웨이 없이 k8s exec 만 쓴다).
	// 프론트에서 "SSH" 버튼을 누르면 브라우저 xterm 으로 바로 접속한다. (물리 세션은 아래 ssh 탭에서 웹연결을 제공한다.)
	sess.Channels = append(sess.Channels, "terminal")
	// 컨테이너 sshd 사이드카가 있으면 네이티브 SSH 탭을 노출한다. 게이트웨이는 필요 없다.
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
		// 물리(SSH) 세션은 Pod 가 없으므로 임대가 활성이면 running 으로 본다.
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
		// 물리노드 임대는 노드 IP 로 직접 SSH 한다(노드 이름은 클러스터·외부 DNS 에 없어 lookup 이 실패한다).
		return &Connection{
			SSH: map[string]string{"cmd": fmt.Sprintf("ssh %s@%s", s.usernameOf(userID), s.prov.NodeIP(ctx, sess.Node))},
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
	// 컨테이너 SSH 는 게이트웨이 없이도 항상 제공한다(사용자가 등록한 공개키로 인증).
	if s.containerSSH() {
		if acc := s.containerSSHAccess(ctx, sess); acc != nil {
			conn.SSH = acc
		}
	}
	return conn, nil
}

// containerSSHAccess는 컨테이너 세션의 "직접 SSH" 접속 정보를 만든다(게이트웨이 불요).
// 노출 모드를 따라 MetalLB 가 있으면 LB IP 의 22 번으로, 없으면 노드 IP 의 NodePort 로 붙는다.
// 주소가 아직 없으면(LB IP 할당 대기 등) nil 을 준다. 프론트는 웹 터미널 경로를 계속 제공한다.
// 로그인 계정은 sshd 사이드카가 세션 UID 로 만드는 work 이며, 인증은 사용자 등록 공개키뿐이다.
func (s *Service) containerSSHAccess(ctx context.Context, sess *Session) map[string]string {
	acc, err := s.prov.SessionServiceAccess(ctx, s.namespaceOf(sess), sess.InstanceID, s.exposeMode())
	if err != nil {
		return nil
	}
	const user = "work"
	// 게이트웨이 모드에서는 SSH 전용 보조 Service 가 MetalLB IP 를 받는다. 이게 제일 붙기 쉽다.
	if acc.SSHLBIP != "" {
		return map[string]string{"direct": "true", "cmd": fmt.Sprintf("ssh %s@%s", user, acc.SSHLBIP)}
	}
	if acc.LBIP != "" {
		return map[string]string{"direct": "true", "cmd": fmt.Sprintf("ssh %s@%s", user, acc.LBIP)}
	}
	if acc.SSHNodePort > 0 {
		// NodePort 는 어느 노드로 붙어도 되지만, 세션이 뜬 노드를 알면 그쪽 IP 를 준다(한 홉 절약).
		ip := ""
		if sess.Node != "" {
			ip = s.prov.NodeIP(ctx, sess.Node)
		}
		if ip == "" || ip == sess.Node {
			ip = s.prov.FirstNodeIP(ctx)
		}
		if ip != "" {
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
		// 커스텀 웹 채널은 시크릿이 없다. 비번·토큰도 설정하지 않는다.
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
	// 서브도메인 URL 을 쓴다. gateway.enabled=false 여도 GatewayDomain 기본값(gw.giosk.local)이
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
	webAccessTTL = 3 * time.Minute // 웹 access 토큰 수명(클릭에서 쿠키 교환까지만 짧게)
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
	if sess.Env == "ssh" { // 물리노드 임대는 SSH 만 준다. 노드 이름은 DNS 로 안 풀려서 IP 로 접속한다.
		info.SSH = s.sshAccess(sess.InstanceID, userID, "", s.prov.NodeIP(ctx, sess.Node), gateway.TgtPhysical, s.usernameOf(userID))
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
	if s.gatewayOn() && s.containerSSH() { // 컨테이너 sshd 사이드카가 있으면 SSH 탭
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
// 토큰은 jti 단일사용(게이트웨이 nonce)이라 ssh 와 sftp 에 각각 별도 토큰을 발급한다(같은 토큰이면 두 번째가 거부된다).
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
	// 프록시(게이트웨이) 접속은 어디서든 한 점으로 모은다. 토큰은 최초 인증 1회만 소비되고,
	// 접속이 성립하면 게이트웨이가 스트림을 계속 프록시한다(중간 재검증이나 주기 검사가 없어 세션이 유지된다).
	m := map[string]string{
		"cmd":  fmt.Sprintf("ssh -p %d %s@%s", port, sshTok, host),
		"user": user,
		"host": host,
	}
	// 사내망 직접 접속(192 대역). 사무실 네트워크 안이면 게이트웨이를 거치지 않고 노드에 바로 붙는다.
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

// stopSession은 세션 중단 공통 절차다. 정산한 뒤 Pod/Service 를 정리하거나 노드를 반납하고 stopped 로 넘긴다.
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
	// 중단 구간 시작. 여기서부터 홈 PVC 는 GPU 과금 없이 노드 디스크만 점유한다.
	// 스토리지 과금과 회수(T1) 방치기간이 모두 이 시각을 기준으로 센다.
	_ = s.repo.MarkStopped(sess.InstanceID, s.now())
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
		// 물리도 같은 CAS 로 중복 시작을 막는다(임대 자체는 원자적이지만 과금 시작점이 두 번 리셋되는 것 방지).
		ok, err := s.repo.ClaimForStart(instanceID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrAlreadyStarting
		}
		if err := s.leaser.CreateLeaseFor(ctx, sess.Node, sess.UserID, instanceID); err != nil {
			_ = s.repo.SetPhase(instanceID, PhaseStopped)
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
	// 재시작도 생성과 같은 관문을 탄다. 예전에는 여기에 검사가 없어, 자리가 없어도 Pod 가 Pending 으로
	// 매달려 "보이지 않는 대기열"이 됐다. 신규 생성은 막히는데 중단 세션을 가진 사람만 줄을 설 수 있어
	// 중단 세션이 사실상 대기표가 되는 역효과가 있었다.
	//
	// 판정은 이 세션이 묶인 노드 기준이다. 홈(/home/work)이 노드 로컬이라 재개는 그 노드에서만 되므로,
	// 클러스터 전체 여유로 통과시키면 "승인은 됐는데 뜨지는 않는" Pending 이 된다.
	// phase 전이(=예약)를 관문과 같은 구간에서 끝내, 다음 요청이 이 자리를 다시 보지 않게 한다.
	if err := s.admit(ctx, func() error {
		if err := s.checkCapacityOn(ctx, sess, sess.Node); err != nil {
			return s.pinnedFailure(ctx, sess, err)
		}
		// 예약 확정. stopped 일 때만 성공하는 조건부 전이라 연타나 동시 요청이 같은 세션을 두 번 띄우지 못한다.
		ok, err := s.repo.ClaimForStart(instanceID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrAlreadyStarting
		}
		return nil
	}); err != nil {
		return err
	}
	// 여기서부터 실패하면 예약(phase)을 중단 상태로 되돌린다. 안 그러면 뜨지도 않은 세션이 자리를 문다.
	resume := func(err error) error {
		_ = s.repo.SetPhase(instanceID, PhaseStopped)
		return err
	}
	ns := s.namespaceOf(sess)
	// 재시작도 동일: 세션 전용 영속 홈 재마운트(중단 전 데이터 그대로 복원) + (sharedHome 시) ~/nfs 영속.
	mounts := s.restartMounts(ctx, ns, sess.UserID, sess.ID)
	homePVC, err := s.ensureSessionHome(ctx, ns, sess)
	if err != nil {
		return resume(err)
	}
	mounts = append(mounts, k8s.VolMountSpec{PVCName: homePVC, MountPath: homeMount})
	if s.sharedHome {
		persistPVC, err := s.ensureHome(ctx, ns, sess.UserID)
		if err != nil {
			return resume(err)
		}
		mounts = append(mounts, k8s.VolMountSpec{PVCName: persistPVC, MountPath: homeMount + "/nfs"})
	}
	dsIDs := s.repo.SessionDatasetIDs(sess.ID)
	dsNode, dsCached, dsHostPath := s.pickDatasetNode(ctx, dsIDs, sess.GpuType, sess.GpuMode, sess.GpuCount)
	mounts = append(mounts, s.resolveDatasets(ctx, ns, dsIDs, dsNode, dsCached, dsHostPath)...) // 데이터셋 RO 복원
	var preferNodes []string
	if sess.ImageID != nil {
		preferNodes = s.repo.CachedNodes(*sess.ImageID)
	}
	if err := s.provision(ctx, ns, sess, imageRef, "", mounts, preferNodes, dsNode); err != nil {
		return resume(err)
	}
	// 중단 구간 종료. 마지막 델타까지 스토리지를 정산한 뒤 시작점을 비운다.
	// ResetBilling 은 billed_credits 만 0으로 되돌리므로 스토리지 누적(storage_billed_credits)은 보존된다.
	s.settleStorage(ctx, sess)
	_ = s.repo.ClearStopped(instanceID, 0)
	_ = s.repo.ResetBilling(instanceID, s.now()) // 재개 = 새 가동분
	return nil                                   // phase 는 관문에서 이미 provisioning 으로 예약됐다
}

// Reconfigure는 중단된 컨테이너 세션의 계산자원을 바꾼다. CPU로 데이터를 준비하고 GPU를 붙여 학습하는 흐름(반대도)이다.
// 홈(/home/work)·볼륨·데이터셋은 그대로 두고 "다음에 어떤 자원으로 뜰지"만 갱신한다.
// 세션을 새로 만들면 준비해 둔 데이터를 다시 옮겨야 하므로, 자원만 갈아끼우는 길을 준다.
//
// 실행 중에는 불가하다(ErrNotStopped): GPU 자원은 Pod 스펙에 박히고 k8s 는 이를 in-place 로 못 바꾼다.
// 생성과 같은 관문(하드 상한, 크레딧, 가용성)을 다시 통과해야 하며, 가용성은 클러스터 전체가 아니라
// 이 세션이 묶인 노드 기준으로 묻는다. 홈 PVC(local-path)가 그 노드에 바인딩돼 있어 다른 노드로는
// 뜰 수 없기 때문이다(전체 기준으로 통과시키면 재개가 영구 Pending 이 된다).
func (s *Service) Reconfigure(ctx context.Context, instanceID string, userID int64, req ReconfigureReq) (*Session, error) {
	sess, err := s.repo.Get(instanceID, userID)
	if err != nil {
		return nil, err
	}
	if sess.Env == "ssh" {
		return nil, ErrReconfigureUnavailable // 물리 임대는 노드를 통째로 빌려주는 것이라 바꿀 사양이 없다
	}
	if sess.Phase != PhaseStopped {
		return nil, ErrNotStopped
	}
	next, err := s.nextSpec(ctx, sess, req)
	if err != nil {
		return nil, err
	}
	if err := s.checkResourceLimits(userID, next); err != nil {
		return nil, err
	}
	if err := s.checkAffordable(userID, gidOf(next), next.PricePerHour); err != nil {
		return nil, err
	}
	if err := s.checkCapacityOn(ctx, next, sess.Node); err != nil {
		// 원인을 갈라 알려준다. 자리 부족·홈 위치·노드 설정 변경은 사용자가 할 일이 각각 다르다.
		next.Node = sess.Node
		return nil, s.pinnedFailure(ctx, next, err)
	}
	if err := s.repo.UpdateSpec(instanceID, next); err != nil {
		return nil, err
	}
	s.recordAct(userID, "session_reconfigure", instanceID)
	if req.Start {
		if err := s.Start(ctx, instanceID, userID); err != nil {
			return next, err // 사양은 이미 저장됐다. 사용자는 재시작만 다시 누르면 된다
		}
	}
	return next, nil
}

// pinnedFailure는 "이 노드에서 못 뜬다"의 원인을 갈라 준다.
// 노드가 그 모드를 아예 안 주면 설정 변경(ErrNodeModeChanged), 클러스터엔 자리가 있으면
// 홈 위치 문제(ErrNodePinned), 둘 다 아니면 그냥 자리 부족이다.
func (s *Service) pinnedFailure(ctx context.Context, sess *Session, fallback error) error {
	if sess.Node == "" {
		return fallback
	}
	if s.nodeSupportsFn != nil {
		if ok, known := s.nodeSupportsFn(ctx, sess.Node, sess.GpuMode); known && !ok {
			return ErrNodeModeChanged
		}
	}
	if s.checkCapacity(ctx, sess) == nil {
		return ErrNodePinned
	}
	return fallback
}

// Reallocate는 세션을 다른 노드에서 다시 시작한다. 홈(/home/work)을 버리고 노드 핀을 푼다.
//
// 홈이 노드 로컬 디스크라 세션은 원래 노드에서만 재개된다. 그 노드가 막혔거나(만석) 원하는 GPU 방식을
// 주지 못하면 사용자는 갇힌다. 홈을 다른 노드로 복사하는 길도 있지만, 홈은 "빠른 작업 공간"이지
// 영속 저장소가 아니다(영속은 ~/nfs 와 볼륨). 그래서 옮기지 않고 버린 뒤 다시 배치한다.
//
// 지우는 것은 홈 PVC 뿐이다. ~/nfs·볼륨·데이터셋은 노드와 무관하므로 그대로 다시 붙는다.
// 되돌릴 수 없으므로 호출부(콘솔)가 반드시 경고하고 확인을 받아야 한다.
func (s *Service) Reallocate(ctx context.Context, instanceID string, userID int64, start bool) error {
	sess, err := s.repo.Get(instanceID, userID)
	if err != nil {
		return err
	}
	if sess.Env == "ssh" {
		return ErrReconfigureUnavailable // 물리 임대는 노드를 통째로 빌려주는 것이라 재배치 개념이 없다
	}
	if sess.Phase != PhaseStopped {
		return ErrNotStopped
	}
	ns := s.namespaceOf(sess)
	// 홈 PVC 를 지우면 다음 시작에서 새 노드에 새로 만들어진다(WFFC).
	if err := s.prov.DeletePVC(ctx, ns, sessionHomePVC(instanceID)); err != nil {
		return err
	}
	if err := s.repo.ClearNode(instanceID); err != nil {
		return err
	}
	s.recordAct(userID, "session_reallocate", instanceID)
	if !start {
		return nil
	}
	return s.Start(ctx, instanceID, userID)
}

// nextSpec은 재구성 요청을 검증·정규화해 "다음 사양"을 만든다(생성 경로 buildSession 과 같은 규칙).
func (s *Service) nextSpec(ctx context.Context, sess *Session, req ReconfigureReq) (*Session, error) {
	next := *sess
	switch req.GpuMode {
	case "cpu", "shared", "exclusive":
	default:
		return nil, ErrBadSpec
	}
	next.GpuMode, next.GpuType, next.GpuCount = req.GpuMode, req.GpuType, req.GpuCount
	next.OfferingID, next.VramMB, next.CorePercent = req.OfferingID, 0, 0
	if req.ImageID != nil {
		if _, err := s.repo.ImageRef(*req.ImageID); err != nil {
			return nil, ErrBadSpec // 없는 이미지
		}
		next.ImageID = req.ImageID
	}
	if next.ImageID == nil {
		return nil, ErrBadSpec // 컨테이너 세션은 이미지가 있어야 뜬다
	}
	if next.OfferingID != nil {
		if err := s.applyOffering(&next, *next.OfferingID); err != nil {
			return nil, err
		}
	}
	// 이하 정규화는 buildSession 과 같다. 화면에 표기되는 사양과 실제 Pod 스펙이 갈라지지 않게 한다.
	if next.GpuMode != "shared" {
		next.VramMB, next.CorePercent = 0, 0
		next.OfferingID = nil // 오퍼링은 분할 전용 개념(전용/CPU 로 가면 남겨두지 않는다)
	}
	if next.GpuMode == "cpu" {
		next.GpuType, next.GpuCount = "", 0
		next.CPUCores, next.MemGB = 0, 0 // CPU 모드는 GPU 지분 개념이 없어 보장을 걸지 않는다
	} else if next.GpuType == "" {
		return nil, ErrBadSpec // GPU 모드인데 모델이 없으면 스케줄 불가
	}
	if next.GpuMode == "exclusive" && next.GpuCount < 1 {
		next.GpuCount = 1
	}
	if next.GpuMode == "shared" {
		next.GpuCount = 1
		if next.VramMB <= 0 || next.CorePercent <= 0 {
			return nil, ErrBadSpec // 분할은 오퍼링(VRAM·코어%)이 있어야 한다
		}
	}
	s.applyGuarantee(ctx, &next) // CPU·메모리 보장은 서버가 GPU 지분에서 산출(클라이언트 값 불신)
	next.PricePerHour = s.priceOf(ctx, &next)
	return &next, nil
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
	return s.deleteSession(ctx, sess)
}

// deleteSession은 소유자 검증을 마친 세션의 실제 삭제 절차다(회수 리퍼도 이 경로를 쓴다).
func (s *Service) deleteSession(ctx context.Context, sess *Session) error {
	instanceID := sess.InstanceID
	if sess.Phase == PhaseStopped {
		s.settleStorage(ctx, sess) // 삭제 전 중단 스토리지 최종 정산(남은 델타 청구)
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
	_ = s.prov.DeletePVC(ctx, ns, sessionHomePVC(instanceID)) // 삭제=세션 홈 데이터 제거(중단 때는 보존)
	return s.repo.Delete(instanceID)
}

// cleanupSharedMounts는 세션이 만든 교차 ns 공유 PVC/PV(정적 복제본)를 정리한다.
// 같은 공유 볼륨을 쓰는 다른 활성 세션이 있으면 보존한다(정지 세션은 시작 시 멱등 재생성되므로 무시).
// 내 볼륨/홈 PVC 는 영속이라 건드리지 않는다(복제본 이름 prefix 로 구분).
func (s *Service) cleanupSharedMounts(ctx context.Context, ns string, userID, sessionID int64, instanceID string) {
	for _, m := range s.repo.SessionVolumeMounts(sessionID) {
		acc, ok := s.repo.VolumeAccess(m.VolID, userID)
		if !ok || acc.PVCNamespace == ns {
			continue // 내 볼륨이거나 접근권이 없다. 정적 복제본도 없다
		}
		if s.repo.ActiveSessionsWithVolume(userID, m.VolID, instanceID) > 0 {
			continue // 다른 활성 세션이 같은 공유 볼륨을 쓰는 중이라 유지한다
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
		_ = s.prov.DeletePVC(ctx, ns, sessionHomePVC(instanceID)) // 삭제=세션 홈 데이터 제거
	}
	return s.repo.Delete(instanceID)
}

// AdminGet은 단일 세션의 관제용 상세 행(소유자 id 포함).
func (s *Service) AdminGet(instanceID string) (*AdminRow, error) { return s.repo.AdminOne(instanceID) }

// AdminAudit은 세션 감사 로그를 반환한다(관제용이라 소유자 검증을 하지 않는다).
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
	s.billStoppedStorageOnce(ctx)
}

// secondsPerMonth는 스토리지 GiB·월 단가를 초 단위로 환산하는 분모(730시간 = 볼륨 과금과 동일 기준).
const secondsPerMonth = 730 * 3600

// billStoppedStorageOnce는 중단 세션이 점유 중인 홈 PVC 를 정산한다.
// 중단 세션은 GPU 과금이 멈춰 "공짜 보관소"가 되기 쉬운데, 실제로는 노드 로컬 디스크를 계속 문다.
// 정책(TTL 삭제)보다 가격이 먼저 작동하게 해서 대부분의 방치가 자발적으로 정리되도록 한다.
func (s *Service) billStoppedStorageOnce(ctx context.Context) {
	rows, err := s.repo.ListStopped()
	if err != nil {
		return
	}
	now := s.now()
	for i := range rows {
		sess := &rows[i]
		if sess.StoppedSince == nil {
			// 이 기능 도입 전에 중단됐던 세션이다. 소급 과금 없이 지금부터 구간을 연다.
			_ = s.repo.MarkStopped(sess.InstanceID, now)
			continue
		}
		s.settleStorage(ctx, sess)
	}
}

// settleStorage는 세션의 미정산 중단 스토리지 사용분을 차감한다.
// 세션 과금(settle)과 동일한 델타 회계: 누적 총액(내림) − 이미 청구액.
// 잔액이 부족하면 유예한다. 누적 시간(stopped_seconds)만 전진하고 청구액은 그대로라,
// 밀린 만큼이 다음 틱에 자동으로 다시 청구된다. 잔액 부족으로 홈을 지우는 일은 없다(회수는 T1 의 몫).
func (s *Service) settleStorage(ctx context.Context, sess *Session) {
	if sess.StoppedSince == nil {
		return // 실행 중 = 열린 중단 구간 없음
	}
	now := s.now()
	elapsed := int(now.Sub(*sess.StoppedSince).Seconds())
	if elapsed < 0 {
		elapsed = 0 // 시계 역행 방어(음수 누적 금지)
	}
	total := sess.StoppedSeconds + elapsed
	billed := sess.StorageBilledCredits
	if price := s.storagePriceOf(); price > 0 && s.charger != nil {
		totalDue := storageDueOf(total, price)
		if due := totalDue - billed; due > 0 {
			gid := int64(0)
			if sess.GroupID != nil {
				gid = *sess.GroupID
			}
			if ok, err := s.charger.Consume(sess.UserID, gid, due, "sh-"+sess.InstanceID); err == nil && ok {
				billed = totalDue
			}
		}
	}
	sess.StoppedSeconds, sess.StorageBilledCredits = total, billed
	sess.StoppedSince = &now
	_ = s.repo.SetStorageBilled(sess.InstanceID, total, billed, now)
}

// storageDueOf는 누적 중단 시간에 대한 총 청구액(크레딧, 내림)을 구한다.
// 이 값에서 이미 청구한 액수를 뺀 것이 이번 틱의 차감분이라, 내림에서 잘린 소수가
// 다음 틱으로 이월된다(매 틱 내림하면 영구 손실이지만 이 방식은 손실이 없다).
// int64 로 계산한다. 장기 방치(수천만 초)에 단가를 곱하면 32비트를 넘길 수 있다.
func storageDueOf(stoppedSeconds, priceGiBMonth int) int {
	if stoppedSeconds <= 0 || priceGiBMonth <= 0 {
		return 0
	}
	return int(int64(homeSizeGiB) * int64(priceGiBMonth) * int64(stoppedSeconds) / int64(secondsPerMonth))
}

// storagePriceOf는 스토리지 GiB·월 단가를 읽는다(미주입=0=과금 없음).
func (s *Service) storagePriceOf() int {
	if s.storagePrice == nil {
		return 0
	}
	return s.storagePrice()
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
	gid := int64(0) // 세션이 뜬 팀 지갑에서 차감한다(팀 없는 세션은 금지라 항상 팀이 있다). 0이면 wallet 이 대표 팀으로 잡는다.
	if sess.GroupID != nil {
		gid = *sess.GroupID
	}
	ok, err := s.charger.Consume(sess.UserID, gid, due, sess.InstanceID)
	if err != nil {
		return
	}
	if !ok {
		// 잔액 부족이다. 가능한 만큼은 못 받았으니 세션을 중단한다(과금 보호).
		if !final {
			s.stopForBilling(ctx, sess)
		}
		return
	}
	sess.BilledCredits = totalDue
	_ = s.repo.SetBilled(sess.InstanceID, totalDue)
	// 사용시간 원장 적립. 이번 틱에 과금된 크레딧이 나타내는 시간(초)이다. 세션 삭제와 무관하게 누적 보존한다.
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
		_ = s.repo.MarkStopped(sess.InstanceID, s.now()) // 중단 구간 시작(스토리지 과금·회수 기준)
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

// runStart는 이번 가동이 시작된 시각이다. 재개하면 started_at 이 갱신되므로 그것을 쓰고,
// 아직 한 번도 안 뜬 세션은 생성 시각으로 본다.
func runStart(s *Session) time.Time {
	if s.StartedAt != nil && s.StartedAt.After(s.CreatedAt) {
		return *s.StartedAt
	}
	return s.CreatedAt
}

func (s *Service) reapIdleOnce(ctx context.Context, windowMin int) {
	rows, err := s.repo.ListRunning()
	if err != nil {
		return
	}
	window := time.Duration(windowMin) * time.Minute
	for i := range rows {
		sess := rows[i]
		// 유예는 "이번 가동"이 시작된 시각부터 잰다. 생성 시각으로 재면 예전에 만들어 둔 세션을
		// 재개했을 때 유예가 처음부터 0 이라, 지표가 쌓이기도 전에 유휴로 보고 바로 멈춘다.
		// (실제로 재개 39초 만에 정지된 사례가 있었다. 재개 시 started_at 은 갱신된다.)
		if time.Since(runStart(&sess)) < window {
			continue // 갓 시작한 세션은 제외
		}
		if idle, ok := s.isIdle(ctx, &sess, windowMin); ok && idle {
			s.idleStop(ctx, &sess, windowMin)
		}
	}
}

// CountIdleRunning은 running 세션 중 유휴(저사용)인 개수를 센다. 대시보드 스냅샷·통계용이다.
// isIdle 과 동일 판정(GPU util<5% / CPU rate<0.05). 메트릭 미가용이면 0(판정 불가).
func (s *Service) CountIdleRunning(ctx context.Context) int {
	if s.met == nil || !s.met.Enabled() {
		return 0
	}
	rows, err := s.repo.ListRunning()
	if err != nil {
		return 0
	}
	const window = 5 // 분. 유휴 판정 이동평균 창(리퍼 창과 별개로 스냅샷 순간 판정용)
	idle := 0
	for i := range rows {
		sess := rows[i]
		// 방금 뜬 세션은 아직 지표가 없어 0% 로 읽힌다. 리퍼와 같은 기준으로 제외한다.
		if time.Since(runStart(&sess)) < window*time.Minute {
			continue
		}
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
// ok=false 면 판정 불가(메트릭 없음)라 보수적으로 정지하지 않는다.
// 물리(SSH) 세션은 GpuMode="exclusive" 라 아래 전용 분기(노드 GPU 사용률)로 흐른다. 노드를 통째
// 점유하므로 노드 GPU 사용률이 곧 임대 사용량이고, 유휴면 회수해 다른 세션이 들어오게 한다.
func (s *Service) isIdle(ctx context.Context, sess *Session, windowMin int) (idle, ok bool) {
	if sess.GpuMode == "cpu" {
		// CPU 세션은 유휴로 종료하지 않는다(사용자 정책: "CPU 유휴로 끄면 안 됨").
		// 유휴 리퍼는 희소 자원인 GPU 세션만 회수한다.
		return false, false
	}
	// GPU 대여 세션은 윈도 평균 GPU 사용률로만 판정한다(CPU 는 무시).
	// 분할(HAMi)은 DCGM 이 Pod 단위로 보고하지 않는다(vGPUmonitor 의 hami_* / exported_pod 라벨).
	// 예전엔 분할 세션도 DCGM 을 봐서 항상 빈 결과가 나왔고, ok=false 라 유휴로 판정된 적이 없어 리퍼가 무력했다.
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
	// 전용(exclusive/mig)은 노드 GPU 가 곧 세션 사용량이다. HAMi 가 클러스터 전체 device-plugin 이라 DCGM 의
	// pod 매핑이 불가능해(워크로드 pod 라벨이 없다) 예전엔 by-pod 쿼리가 항상 빈 결과였고 전용 세션이 유휴로
	// 판정된 적이 없었다(미종료 버그). 세션이 놓인 노드로 귀속한다(dcgm-exporter 의 pod 을 node 로 옮기는 건 kube_pod_info).
	q := fmt.Sprintf(`avg(avg_over_time(DCGM_FI_DEV_GPU_UTIL[%dm]) * on(pod,namespace) group_left(node) kube_pod_info{node=%q})`, windowMin, sess.Node)
	v, got := s.met.Scalar(ctx, q)
	if !got {
		return false, false // GPU 메트릭이 없으면 정지하지 않는다(보수적)
	}
	if v >= idleGPUThreshold {
		return false, true // 사용 중
	}
	// util 이 낮게 나와도 유휴로 단정하지 않는다. 컨슈머 GeForce+최신 드라이버에선 DCGM 이 util(%)을
	// 종종 0 으로 오보고하지만(전력·클럭은 정상), 이를 그대로 믿으면 100% 학습 중인 세션이 유휴로 죽는다.
	// 전력을 보조 신호로 쓴다. GPU 가 유의미한 전력을 쓰면 실제 연산 중이라 유휴가 아니다. (유휴 4090 약 25W, 부하 130W 이상)
	pq := fmt.Sprintf(`avg(avg_over_time(DCGM_FI_DEV_POWER_USAGE[%dm]) * on(pod,namespace) group_left(node) kube_pod_info{node=%q})`, windowMin, sess.Node)
	if p, ok := s.met.Scalar(ctx, pq); ok && p >= idleGPUPowerW {
		return false, true // 전력을 쓰는 중이라 util 오보고를 방어한다(유휴 아님)
	}
	return true, true
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
				s.reapUnschedulable(ctx, &rows[i])
			}
		}
	}
}

// unschedulableGrace는 "스케줄 안 됨"을 실패로 볼 때까지 기다리는 시간.
// 짧은 미스케줄은 정상이다(PVC 바인딩, 이미지 풀, 노드 재기동 중). 그 창은 봐주고,
// 그보다 오래 매달리면 대기열이 되므로 정리한다.
const unschedulableGrace = 3 * time.Minute

// reapUnschedulable은 노드를 못 잡고 매달린 세션을 중단시킨다.
//
// 관문(CanPlace)이 있어도 완벽하지는 않다. CPU/메모리, 볼륨 노드 어피니티, taint 처럼 관문이 보지 않는
// 이유로도 스케줄은 실패할 수 있다. 그때 Pod 를 그대로 두면 사용자에겐 "준비 중"으로만 보이는
// 무기한 대기가 된다(이 제품이 두지 않기로 한 바로 그 상태). 차라리 중단시켜 사유를 남기고,
// 사용자가 다시 시도하거나 다른 자원을 고르게 한다. 자리는 즉시 남에게 돌아간다.
func (s *Service) reapUnschedulable(ctx context.Context, sess *Session) {
	if sess.Env == "ssh" || sess.Phase != PhaseProvisioning || sess.Node != "" {
		return // 물리 세션·이미 배치된 세션은 대상 아님
	}
	since := sess.StartedAt
	if since == nil || s.now().Sub(*since) < unschedulableGrace {
		return
	}
	st, err := s.prov.PodStatus(ctx, s.namespaceOf(sess), sess.InstanceID)
	if errors.Is(err, k8s.ErrNotFound) {
		// Pod 가 없는데 준비 중으로 남은 세션이다. 외부에서 지워졌거나 생성이 중간에 끊긴 경우다.
		// 이 상태로 두면 아무것도 뜨지 않은 세션이 영원히 남의 자리를 문다(유령 예약).
		s.autoStop(ctx, sess, "session_orphaned",
			fmt.Sprintf("[scheduler] stopped %s: pod missing for %s", sess.InstanceID, unschedulableGrace))
		return
	}
	if err != nil || st == nil || !st.Unschedulable {
		return
	}
	s.autoStop(ctx, sess, "session_unschedulable",
		fmt.Sprintf("[scheduler] stopped %s: unschedulable for %s: %s", sess.InstanceID, unschedulableGrace, st.Reason))
}

// idleStop은 유휴 세션을 정지한다(자동 정지 공통 경로 사용).
func (s *Service) idleStop(ctx context.Context, sess *Session, windowMin int) {
	s.autoStop(ctx, sess, "session_idle_stop", fmt.Sprintf("[idle-reaper] stopped %s (idle > %dm)", sess.InstanceID, windowMin))
}

// autoStop은 세션 Pod/Service 를 정리하고 최종 정산 후 stopped 로 전이한다(소유자 무관, 시스템 동작).
func (s *Service) autoStop(ctx context.Context, sess *Session, action, logMsg string) {
	s.settle(ctx, sess, true) // 정지 전 사용분 최종 정산
	if sess.Env == "ssh" {
		if s.leaser != nil {
			_ = s.leaser.ReleaseLeaseFor(ctx, sess.InstanceID) // 물리 임대 반납(uncordon); 홈(NFS)은 유지
		}
	} else {
		_ = s.prov.DeleteSessionPod(ctx, s.namespaceOf(sess), sess.InstanceID)
		_ = s.prov.DeleteSessionService(ctx, s.namespaceOf(sess), sess.InstanceID)
		_ = s.repo.MarkStopped(sess.InstanceID, s.now()) // 중단 구간 시작(스토리지 과금·회수 기준)
	}
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

// HomeUsage는 세션 하나가 붙들고 있는 홈 용량이다. 볼륨 화면에서 저장공간 할당량이
// 어디에 쓰였는지 보여 주는 데 쓴다. 홈은 볼륨과 같은 쿼터에서 세면서도 볼륨 목록에는
// 없어서, 사용자 눈에는 쓴 적 없는 용량이 사라진 것처럼 보였다.
type HomeUsage struct {
	InstanceID string `json:"instanceId"`
	Name       string `json:"name"`
	Node       string `json:"node"`
	Phase      string `json:"phase"`
	SizeGiB    int    `json:"sizeGib"`
}

// HomeUsages는 사용자의 살아있는 세션 홈 내역이다. 합계는 쿼터 검사(checkHomeQuota)와
// 같은 기준이어야 하므로 물리(SSH) 임대는 빼고 phase 로도 거르지 않는다. 홈은 세션을
// 지워야 사라지기 때문이다.
func (s *Service) HomeUsages(userID int64) []HomeUsage {
	rows, err := s.repo.ListByUser(userID, 0)
	if err != nil {
		return nil
	}
	out := make([]HomeUsage, 0, len(rows))
	for _, r := range rows {
		if r.Env == "ssh" {
			continue
		}
		out = append(out, HomeUsage{
			InstanceID: r.InstanceID, Name: r.Name, Node: r.Node,
			Phase: r.Phase, SizeGiB: s.homeGiBOf(&r),
		})
	}
	return out
}
