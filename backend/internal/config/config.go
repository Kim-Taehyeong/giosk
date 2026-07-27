// Package config는 설치 시점 설정(Helm values → 환경변수)을 타입 안전한 Config로 로드한다.
//
// 단방향 주입: Helm values → ConfigMap/Secret → 환경변수 → 이 패키지.
// "하드코딩 금지" 원칙의 단일 출처 — 기본값도 여기 상수 한 곳에만 둔다.
// 런타임 토글(features 등)은 system_config 테이블에서 별도로 읽는다(이 패키지는 설치 고정값).
package config

import "fmt"

// 모드 enum — 설계 문서 §1.3 의 2축.
const (
	DeploymentContainer = "container"
	DeploymentHybrid    = "hybrid"

	BillingCredit  = "credit"
	BillingDynamic = "dynamic"
	BillingFree    = "free" // 자유: 크레딧·임대·동시세션 제한 모두 없음

	NFSModeInCluster = "in-cluster"
	NFSModeExternal  = "external"
)

// Config는 설치 고정 설정 전체. 환경변수 prefix 는 GIOSK_.
type Config struct {
	Env            string // dev | prod
	HTTPAddr       string // 리슨 주소(:8080)
	Deployment     Deployment
	Billing        Billing
	Storage        Storage
	PhysicalNodes  PhysicalNodes
	Database       Database
	Branding       Branding
	Admin          Admin
	K8s            K8s
	Features       Features
	IdleTimeoutMin int
	Quota          Quota
	PrometheusURL  string // 메트릭 조회(대시보드). 빈 값이면 메트릭 0/미가용.
	SMTP           SMTP   // 알림 이메일 발송(빈 Host면 이메일 채널 비활성)
}

// SMTP — 알림 이메일 발송 설정(Helm smtp.*). Host 비면 이메일 발송 생략(웹훅만).
type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// SMTPEnabled는 이메일 발송 가능 여부(Host 설정 시).
func (c *Config) SMTPEnabled() bool { return c.SMTP.Host != "" }

// Features — 런타임 기능 토글 초기값(설치 시 주입; 이후 운영 변경은 별도).
type Features struct {
	SignupRequest    bool
	DatasetRegister  bool
	WorkloadAlerts   bool
	GroupJoinRequest bool
	CreditRequest    bool
}

// Quota — 플랫폼 전역 상한.
type Quota struct {
	MaxGpuCount           int
	MaxVramGB             int
	MaxConcurrentSessions int
	VolumeQuotaGB         int
	MaxEphemeralGiB       int // 세션 임시 디스크(ephemeral-storage) 전역 상한(GiB)
}

// K8s — 클러스터 연동 설정. GPU 타입은 노드 라벨에서 수집(운영=GFD 라벨, 개발=fake 라벨).
type K8s struct {
	Kubeconfig        string // 빈 값이면 in-cluster→기본 kubeconfig 자동 로딩
	GpuTypeLabel      string // 예: giosk.io/gpu-type (개발) / nvidia.com/gpu.product (운영)
	CudaLabel         string // 노드 CUDA 툴킷(런타임) 버전 라벨. 기본은 GFD runtime.major/.minor 합성; 없으면 표기 생략.
	NamespacePrefix   string // 세션 네임스페이스 prefix (그룹별)
	GatewayDomain     string // 접속 게이트웨이 도메인(서브도메인 = <iid>-<ch>.<domain>)
	GatewaySecret     string // API↔게이트웨이 공유 토큰키(GIOSK_GATEWAY_SECRET). 빈값=게이트웨이 비활성.
	GatewaySSHKey     string // 게이트웨이 SSH 관리 개인키(PEM). 물리 세션 웹터미널이 노드 SSH 에 사용(GIOSK_GATEWAY_SSH_KEY). 빈값=물리 웹터미널 비활성.
	GatewayScheme     string // 게이트웨이 웹 URL 스킴(https|http; 테스트베드 HTTP)
	GatewayHost       string // SSH 접속 호스트(빈값=도메인). 예: LB IP.
	GatewaySSHPort    int    // 게이트웨이 SSH 프록시 포트(기본 2222)
	GatewayJump       string // 외부(VPN 밖) 접속용 SSH 점프 호스트(user@host). 빈값=내부 명령만 표시.
	SessionSSHDImage  string // 컨테이너 세션 sshd 사이드카 이미지. 빈값=컨테이너 SSH 비활성.
	SessionSSHDPubKey string // sshd 사이드카가 신뢰할 게이트웨이 공개키(authorized_keys)
	SessionExpose     string // 세션 웹 노출: nodeport(기본) | loadbalancer(MetalLB)
	// NVIDIA device plugin 설정 ConfigMap(GPU Operator). 지정 시 관리자가 웹에서 타임셰어링을 켜면
	// 프로파일 upsert + 노드 라벨 부여로 즉시 반영된다. 빈값이면 자동 적용 생략(수동 운영).
	DevicePluginConfigNS   string
	DevicePluginConfigName string
	Registry               string // 이미지 빌드 푸시/세션 풀 레지스트리(예: giosk-registry:5000). 빈값=빌드 비활성
	CosignKeySecret        string // cosign 서명키 Secret 이름(cosign.key + password). 빈값=서명 단계 생략
}

// Admin — 최초 플랫폼 관리자 부트스트랩 자격(없으면 생성). Helm Secret 주입.
type Admin struct {
	Username string
	Password string
}

// Deployment — 실행 환경 축(container | hybrid).
type Deployment struct {
	Mode string
}

// Billing — 과금 축(credit | dynamic). 세부 정책은 설치 고정.
type Billing struct {
	Mode    string
	Credit  CreditPolicy
	Dynamic DynamicPolicy
}

type CreditPolicy struct {
	Pricing               string // static | dynamic
	SurgeIncrement        int
	MaxConcurrentSessions int
}

type DynamicPolicy struct {
	MaxLeaseHours         int
	CooldownHours         int
	MaxConcurrentSessions int
	ExtensionHours        int
	MaxExtensions         int
}

// Storage — 영속성/데이터셋 스토리지 전략(설계 문서 §3).
type Storage struct {
	PersistenceClass string  // 영속 home(~/nfs) 스토리지클래스 (RWX·노드독립 필수, 예: nfs/longhorn)
	SharedHome       bool    // 사용자 영속 home(~/nfs) 사용 여부. false 면 세션은 순수 로컬 임시(emptyDir)만 — RWX 불필요
	NFSClass         string  // RWX NFS 스토리지클래스(데이터셋·공유 볼륨용 — 항상 NFS 고정)
	VolumeQuotaGB    int     // 플랫폼 전역 볼륨 쿼터
	PricePerGiBMonth int     // 스토리지 크레딧 단가(GiB·월). 0=무료. 크레딧 모드에서만 과금.
	Datasets         Dataset // 데이터셋 사용 시 NFS
	ScratchEnabled   bool    // 노드로컬 스크래치(hostPath) 세션 마운트 활성
	ScratchHostPath  string  // 노드의 스크래치 루트(계정별 하위폴더). 기본 /scratch
	DatasetCacheHost string  // 데이터셋 노드 로컬 캐시 루트(hostPath). 기본 /dataset-cache
	PhysicalHomeHost string  // 물리노드 로컬 home 루트(계정별 하위폴더, hostPath). 기본 /home/giosk. "로컬 Home" 특수 볼륨 원천
}

type Dataset struct {
	Enabled bool
	NFS     NFS
}

// NFS — in-cluster(클러스터 내 provisioner) 또는 external(외부 서버 마운트).
type NFS struct {
	Mode   string // in-cluster | external
	Server string // external 시 필수
	Path   string // external 시 필수
}

// PhysicalNodes — hybrid 물리노드. 활성 시 외부 NFS 필수.
type PhysicalNodes struct {
	Enabled    bool
	Label      string // 물리노드 식별 K8s 라벨(멀티 인스턴스 격리; 기본 giosk.io/physical)
	NFS        NFS    // 외부 NFS(server+path 필수)
	AgentImage string
	AgentToken string // node-agent ↔ API 공유 토큰
	UIDBase    int    // 물리 계정 UID = UIDBase + userID(전역 안정, 재사용 안 함). 시스템/LDAP UID 충돌 회피용 높은 베이스
}

type Database struct {
	Host string
	Port int
	Name string
	User string
	Pass string
}

type Branding struct {
	Name     string
	Accent   string
	Subtitle string
	Icon     string
	IconUrl  string // 업로드 아이콘(data URL). 있으면 Icon(Lucide)보다 우선.
}

// Load는 환경변수에서 Config를 읽고 검증한다.
// getter는 lookup 함수를 주입받아 테스트 가능(os.LookupEnv 또는 fake).
func Load(lookup func(string) (string, bool)) (*Config, error) {
	g := newGetter(lookup)
	c := &Config{
		Env:      g.str("GIOSK_ENV", "dev"),
		HTTPAddr: g.str("GIOSK_HTTP_ADDR", ":8080"),
		Deployment: Deployment{
			Mode: g.str("GIOSK_DEPLOYMENT_MODE", DeploymentContainer),
		},
		Billing: Billing{
			Mode: g.str("GIOSK_BILLING_MODE", BillingCredit),
			Credit: CreditPolicy{
				Pricing:               g.str("GIOSK_CREDIT_PRICING", "static"),
				SurgeIncrement:        g.intv("GIOSK_CREDIT_SURGE_INCREMENT", 10),
				MaxConcurrentSessions: g.intv("GIOSK_CREDIT_MAX_SESSIONS", 3),
			},
			Dynamic: DynamicPolicy{
				MaxLeaseHours:         g.intv("GIOSK_DYNAMIC_MAX_LEASE_HOURS", 8),
				CooldownHours:         g.intv("GIOSK_DYNAMIC_COOLDOWN_HOURS", 2),
				MaxConcurrentSessions: g.intv("GIOSK_DYNAMIC_MAX_SESSIONS", 1),
				ExtensionHours:        g.intv("GIOSK_LEASE_EXTENSION_HOURS", 4),
				MaxExtensions:         g.intv("GIOSK_LEASE_MAX_EXTENSIONS", 2),
			},
		},
		Storage: Storage{
			PersistenceClass: g.str("GIOSK_PERSISTENCE_CLASS", "longhorn"),
			SharedHome:       g.boolv("GIOSK_SHARED_HOME", true),
			NFSClass:         g.str("GIOSK_NFS_CLASS", "nfs"),
			VolumeQuotaGB:    g.intv("GIOSK_VOLUME_QUOTA_GB", 2000),
			PricePerGiBMonth: g.intv("GIOSK_STORAGE_PRICE_PER_GIB_MONTH", 0), // 0=스토리지 무료(크레딧 모드에서만 과금)
			ScratchEnabled:   g.boolv("GIOSK_SCRATCH_ENABLED", false),
			ScratchHostPath:  g.str("GIOSK_SCRATCH_HOSTPATH", "/scratch"),
			DatasetCacheHost: g.str("GIOSK_DATASET_CACHE_HOSTPATH", "/dataset-cache"),
			PhysicalHomeHost: g.str("GIOSK_PHYSICAL_HOME_HOSTPATH", "/home/giosk"),
			Datasets: Dataset{
				Enabled: g.boolv("GIOSK_DATASETS_ENABLED", false),
				NFS: NFS{
					Mode:   g.str("GIOSK_DATASETS_NFS_MODE", NFSModeInCluster),
					Server: g.str("GIOSK_DATASETS_NFS_SERVER", ""),
					Path:   g.str("GIOSK_DATASETS_NFS_PATH", ""),
				},
			},
		},
		PhysicalNodes: PhysicalNodes{
			Enabled: g.boolv("GIOSK_PHYSICAL_NODES_ENABLED", false),
			Label:   g.str("GIOSK_PHYSICAL_LABEL", "giosk.io/physical"),
			NFS: NFS{
				Mode:   NFSModeExternal, // 물리노드는 항상 외부 NFS
				Server: g.str("GIOSK_PHYSICAL_NFS_SERVER", ""),
				Path:   g.str("GIOSK_PHYSICAL_NFS_PATH", ""),
			},
			AgentImage: g.str("GIOSK_NODE_AGENT_IMAGE", ""),
			AgentToken: g.str("GIOSK_AGENT_TOKEN", ""),
			UIDBase:    g.intv("GIOSK_PHYSICAL_UID_BASE", 100000),
		},
		Database: Database{
			Host: g.str("GIOSK_DB_HOST", "127.0.0.1"),
			Port: g.intv("GIOSK_DB_PORT", 3306),
			Name: g.str("GIOSK_DB_NAME", "giosk"),
			User: g.str("GIOSK_DB_USER", "giosk"),
			Pass: g.str("GIOSK_DB_PASS", ""),
		},
		Branding: Branding{
			Name:   g.str("GIOSK_BRAND_NAME", "Giosk"),
			Accent: g.str("GIOSK_BRAND_ACCENT", ""),
		},
		Admin: Admin{
			Username: g.str("GIOSK_ADMIN_USERNAME", "admin"),
			Password: g.str("GIOSK_ADMIN_PASSWORD", ""),
		},
		K8s: K8s{
			Kubeconfig:             g.str("GIOSK_KUBECONFIG", ""),
			GpuTypeLabel:           g.str("GIOSK_GPU_TYPE_LABEL", "giosk.io/gpu-type"),
			CudaLabel:              g.str("GIOSK_CUDA_LABEL", "nvidia.com/cuda.runtime-version"),
			NamespacePrefix:        g.str("GIOSK_NS_PREFIX", "giosk-grp-"),
			GatewayDomain:          g.str("GIOSK_GATEWAY_DOMAIN", "gw.giosk.local"),
			GatewaySecret:          g.str("GIOSK_GATEWAY_SECRET", ""),
			GatewaySSHKey:          g.str("GIOSK_GATEWAY_SSH_KEY", ""),
			GatewayScheme:          g.str("GIOSK_GATEWAY_SCHEME", "https"),
			GatewayHost:            g.str("GIOSK_GATEWAY_HOST", ""),
			GatewaySSHPort:         g.intv("GIOSK_GATEWAY_SSH_PORT", 2222),
			GatewayJump:            g.str("GIOSK_GATEWAY_SSH_PROXY_JUMP", ""),
			SessionSSHDImage:       g.str("GIOSK_SESSION_SSHD_IMAGE", ""),
			SessionSSHDPubKey:      g.str("GIOSK_GATEWAY_SSH_PUBKEY", ""),
			SessionExpose:          g.str("GIOSK_SESSION_EXPOSE", "nodeport"),
			DevicePluginConfigNS:   g.str("GIOSK_DEVICE_PLUGIN_CONFIG_NS", ""),
			DevicePluginConfigName: g.str("GIOSK_DEVICE_PLUGIN_CONFIG_NAME", ""),
			Registry:               g.str("GIOSK_REGISTRY", ""),
			CosignKeySecret:        g.str("GIOSK_COSIGN_KEY_SECRET", ""),
		},
		IdleTimeoutMin: g.intv("GIOSK_IDLE_TIMEOUT_MIN", 30),
		Features: Features{
			SignupRequest:    g.boolv("GIOSK_FEATURE_SIGNUP_REQUEST", true),
			DatasetRegister:  g.boolv("GIOSK_FEATURE_DATASET_REGISTER", true),
			WorkloadAlerts:   g.boolv("GIOSK_FEATURE_WORKLOAD_ALERTS", true),
			GroupJoinRequest: g.boolv("GIOSK_FEATURE_GROUP_JOIN", true),
			CreditRequest:    g.boolv("GIOSK_FEATURE_CREDIT_REQUEST", true),
		},
		Quota: Quota{
			MaxGpuCount:           g.intv("GIOSK_QUOTA_MAX_GPU", 64),
			MaxVramGB:             g.intv("GIOSK_QUOTA_MAX_VRAM_GB", 512),
			MaxConcurrentSessions: g.intv("GIOSK_QUOTA_MAX_SESSIONS", 50),
			VolumeQuotaGB:         g.intv("GIOSK_VOLUME_QUOTA_GB", 2000),
			MaxEphemeralGiB:       g.intv("GIOSK_QUOTA_MAX_EPHEMERAL_GIB", 100), // 넉넉한 기본 캡: 정상사용 무영향·노드 독점(DoS) 방지. 정책으로 티어별 조정.
		},
		PrometheusURL: g.str("GIOSK_PROMETHEUS_URL", ""),
		SMTP: SMTP{
			Host:     g.str("GIOSK_SMTP_HOST", ""),
			Port:     g.intv("GIOSK_SMTP_PORT", 587),
			Username: g.str("GIOSK_SMTP_USERNAME", ""),
			Password: g.str("GIOSK_SMTP_PASSWORD", ""),
			From:     g.str("GIOSK_SMTP_FROM", ""),
		},
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// DSN은 GORM(MySQL)용 접속 문자열을 만든다.
func (d Database) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=UTC&multiStatements=true",
		d.User, d.Pass, d.Host, d.Port, d.Name)
}

// IsHybrid는 SSH 물리노드 환경 여부.
func (c *Config) IsHybrid() bool { return c.Deployment.Mode == DeploymentHybrid }

// IsCredit는 크레딧 과금 모드 여부.
func (c *Config) IsCredit() bool { return c.Billing.Mode == BillingCredit }

// IsFree는 자유 모드 여부(크레딧·임대·동시세션 제한 없음).
func (c *Config) IsFree() bool { return c.Billing.Mode == BillingFree }

// MaxSessionsPerUser는 과금 모드별 사용자당 동시 활성 세션 상한(0=무제한).
func (c *Config) MaxSessionsPerUser() int {
	switch c.Billing.Mode {
	case BillingFree:
		return 0 // 무제한
	case BillingCredit:
		return c.Billing.Credit.MaxConcurrentSessions
	default:
		return c.Billing.Dynamic.MaxConcurrentSessions
	}
}
