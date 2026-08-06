// Package systemconfig는 시스템 설정을 프론트엔드에 노출/갱신한다.
//   - 설치 시점 고정(deployment/billing 모드·과금모델·데이터셋·쿼터 등): env(Helm), 읽기 전용.
//   - 운영 중 조정(유휴 타임아웃·단순 기능 토글): runtime_config 저장소에 영속(PUT /admin/config).
//
// GET /api/config 는 env 위에 런타임 오버라이드를 얹어 "유효 설정"을 반환한다.
package systemconfig

import (
	"giosk/internal/config"

	"github.com/gin-gonic/gin"
)

// Handler는 config 핸들러(설치 설정 cfg + 런타임 저장소 store).
type Handler struct {
	cfg   *config.Config
	store *Store
}

func NewHandler(cfg *config.Config, store *Store) *Handler { return &Handler{cfg: cfg, store: store} }

// Config는 프론트 SystemConfigContext 와 동일한 shape 의 "유효 설정"을 반환한다(env + 런타임 오버라이드).
func (h *Handler) Config(c *gin.Context) {
	cfg := h.cfg
	rt := h.store.All()

	idleTimeout := atoiOr(rt[KeyIdleTimeoutMin], cfg.IdleTimeoutMin)
	storagePrice := atoiOr(rt[KeyStoragePriceGiBMonth], cfg.Storage.PricePerGiBMonth) // 런타임 오버라이드 우선
	// 브랜딩은 런타임 오버라이드 우선(관리자 Settings 저장), 미설정이면 설치시 env(Helm).
	brandName := cfg.Branding.Name
	if v, ok := rt[KeyBrandName]; ok && v != "" {
		brandName = v
	}
	brandAccent := cfg.Branding.Accent
	if v, ok := rt[KeyBrandAccent]; ok {
		brandAccent = v // 빈 문자열 = 기본 테마색으로 리셋 허용
	}
	brandSubtitle := cfg.Branding.Subtitle
	if v, ok := rt[KeyBrandSubtitle]; ok {
		brandSubtitle = v // 빈 문자열 = 부제 숨김 허용
	}
	brandIcon := cfg.Branding.Icon
	if v, ok := rt[KeyBrandIcon]; ok {
		brandIcon = v
	}
	brandIconUrl := cfg.Branding.IconUrl
	if v, ok := rt[KeyBrandIconUrl]; ok {
		brandIconUrl = v
	}
	feat := func(key string, def bool) bool {
		if v, ok := rt[key]; ok {
			return v == "true"
		}
		return def
	}

	c.JSON(200, gin.H{
		"setupComplete":  true, // 모드는 설치 시 결정되므로 첫 실행 위저드가 필요 없다
		"deploymentMode": cfg.Deployment.Mode,
		"branding":       gin.H{"name": brandName, "accent": brandAccent, "subtitle": brandSubtitle, "icon": brandIcon, "iconUrl": brandIconUrl},
		"billing": gin.H{
			"mode": cfg.Billing.Mode,
			"credit": gin.H{
				"pricing":        cfg.Billing.Credit.Pricing,
				"surgeIncrement": cfg.Billing.Credit.SurgeIncrement,
				"recharge": gin.H{
					"enabled":      rt[KeyRechargeEnabled] == "true",
					"intervalDays": atoiOr(rt[KeyRechargeIntervalDays], 30),
					"carryover":    rt[KeyRechargeCarryover] == "true",
				},
			},
			"dynamic": gin.H{
				"maxLeaseHours": cfg.Billing.Dynamic.MaxLeaseHours,
				"cooldownHours": cfg.Billing.Dynamic.CooldownHours,
			},
		},
		"idle": gin.H{"timeoutMin": idleTimeout},
		// 중단 세션 홈 회수 정책이다. 유휴 정지와 같은 회수 축이라 함께 운영 중 조정한다.
		// stoppedTtlDays=0 이면 자동 회수 없음(개수 상한·스토리지 과금만으로 억제).
		"reclaim": gin.H{
			"stoppedTtlDays": atoiOr(rt[KeyStoppedTTLDays], cfg.Quota.StoppedTTLDays),
			"homeReapPct":    atoiOr(rt[KeyHomeReapPct], cfg.Quota.HomeReapPct),
		},
		"lease": gin.H{"extensionHours": cfg.Billing.Dynamic.ExtensionHours, "maxExtensions": cfg.Billing.Dynamic.MaxExtensions},
		"features": gin.H{
			"signupRequest":    feat(KeySignupRequest, cfg.Features.SignupRequest),
			"datasets":         cfg.Storage.Datasets.Enabled, // 설치시 고정(인프라)
			"imageBuild":       cfg.K8s.Registry != "",       // 설치시 고정(레지스트리 유무). 없으면 외부 이미지 등록만.
			"datasetRegister":  feat(KeyDatasetRegister, cfg.Features.DatasetRegister),
			"workloadAlerts":   feat(KeyWorkloadAlerts, cfg.Features.WorkloadAlerts),
			"groupJoinRequest": feat(KeyGroupJoinRequest, cfg.Features.GroupJoinRequest),
			"creditRequest":    feat(KeyCreditRequest, cfg.Features.CreditRequest),
		},
		"quota": gin.H{
			"maxGpuCount":           cfg.Quota.MaxGpuCount,
			"maxVramGb":             cfg.Quota.MaxVramGB,
			"maxConcurrentSessions": cfg.Quota.MaxConcurrentSessions,
			"volumeQuotaGb":         cfg.Quota.VolumeQuotaGB,
		},
		"storage": gin.H{
			"pricePerGiBMonth": storagePrice, // 스토리지 크레딧 단가(GiB·월). 런타임 조정 가능. 0=무료
		},
	})
}

// UpdateReq는 운영 중 조정 가능한 항목만 받는다(무거운 항목은 설치 시 고정이라 무시한다).
type UpdateReq struct {
	Idle *struct {
		TimeoutMin *int `json:"timeoutMin"`
	} `json:"idle"`
	Branding *struct {
		Name     *string `json:"name"`
		Accent   *string `json:"accent"`
		Subtitle *string `json:"subtitle"`
		Icon     *string `json:"icon"`
		IconUrl  *string `json:"iconUrl"`
	} `json:"branding"`
	Recharge *struct {
		Enabled      *bool `json:"enabled"`
		IntervalDays *int  `json:"intervalDays"`
		Carryover    *bool `json:"carryover"`
	} `json:"recharge"`
	Storage *struct {
		PricePerGiBMonth *int `json:"pricePerGiBMonth"`
	} `json:"storage"`
	Reclaim *struct {
		StoppedTtlDays *int `json:"stoppedTtlDays"`
		HomeReapPct    *int `json:"homeReapPct"`
	} `json:"reclaim"`
	Features map[string]bool `json:"features"`
}

// 프론트 features 키를 런타임 저장소 키로 옮긴다(설치 시 고정 항목은 매핑이 없어 무시된다).
var featureKeyMap = map[string]string{
	"signupRequest":    KeySignupRequest,
	"datasetRegister":  KeyDatasetRegister,
	"workloadAlerts":   KeyWorkloadAlerts,
	"groupJoinRequest": KeyGroupJoinRequest,
	"creditRequest":    KeyCreditRequest,
}

// Update는 운영 중 조정 항목을 영속화한다(PUT /admin/config, admin 전용).
func (h *Handler) Update(c *gin.Context) {
	var req UpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "본문 오류"})
		return
	}
	if req.Idle != nil && req.Idle.TimeoutMin != nil && *req.Idle.TimeoutMin > 0 {
		_ = h.store.Set(KeyIdleTimeoutMin, itoa(*req.Idle.TimeoutMin))
	}
	if req.Storage != nil && req.Storage.PricePerGiBMonth != nil && *req.Storage.PricePerGiBMonth >= 0 {
		_ = h.store.Set(KeyStoragePriceGiBMonth, itoa(*req.Storage.PricePerGiBMonth))
	}
	if req.Reclaim != nil {
		// TTL 은 0 허용 = "자동 회수 끄기". 다른 값과 달리 0 이 유효한 설정이라 >=0 으로 받는다.
		if req.Reclaim.StoppedTtlDays != nil && *req.Reclaim.StoppedTtlDays >= 0 {
			_ = h.store.Set(KeyStoppedTTLDays, itoa(*req.Reclaim.StoppedTtlDays))
		}
		// 임계는 50~99%만. 너무 낮으면 상시 회수가 돌고, 100%는 이미 kubelet DiskPressure 라 늦다.
		if p := req.Reclaim.HomeReapPct; p != nil && *p >= 50 && *p <= 99 {
			_ = h.store.Set(KeyHomeReapPct, itoa(*p))
		}
	}
	if req.Recharge != nil {
		if req.Recharge.Enabled != nil {
			_ = h.store.Set(KeyRechargeEnabled, boolStr(*req.Recharge.Enabled))
		}
		if req.Recharge.IntervalDays != nil && *req.Recharge.IntervalDays > 0 {
			_ = h.store.Set(KeyRechargeIntervalDays, itoa(*req.Recharge.IntervalDays))
		}
		if req.Recharge.Carryover != nil {
			_ = h.store.Set(KeyRechargeCarryover, boolStr(*req.Recharge.Carryover))
		}
	}
	if req.Branding != nil {
		if req.Branding.Name != nil {
			_ = h.store.Set(KeyBrandName, *req.Branding.Name)
		}
		if req.Branding.Accent != nil {
			_ = h.store.Set(KeyBrandAccent, *req.Branding.Accent)
		}
		if req.Branding.Subtitle != nil {
			_ = h.store.Set(KeyBrandSubtitle, *req.Branding.Subtitle)
		}
		if req.Branding.Icon != nil {
			_ = h.store.Set(KeyBrandIcon, *req.Branding.Icon)
		}
		if req.Branding.IconUrl != nil {
			_ = h.store.Set(KeyBrandIconUrl, *req.Branding.IconUrl)
		}
	}
	for k, v := range req.Features {
		key, ok := featureKeyMap[k]
		if !ok {
			continue // 설치시 고정(datasets 등)은 무시
		}
		val := "false"
		if v {
			val = "true"
		}
		_ = h.store.Set(key, val)
	}
	c.JSON(200, gin.H{"ok": true})
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// Register는 공개 config(GET)를 등록한다. 갱신(PUT)은 admin 라우트에서 별도 등록.
func Register(api gin.IRouter, h *Handler) {
	api.GET("/config", h.Config)
}
