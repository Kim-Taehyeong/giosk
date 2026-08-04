package systemconfig

import "gorm.io/gorm"

// 런타임 설정 키다. 운영 중 조정할 수 있는 단순 정책만 두고 무거운 항목은 설치 시 env 로 고정한다.
const (
	KeyIdleTimeoutMin       = "idle_timeout_min"
	KeyStoragePriceGiBMonth = "storage_price_gib_month" // 스토리지 크레딧 단가(GiB·월). 런타임 조정.
	// 중단 세션 홈 회수는 유휴 타임아웃과 같은 성격의 회수 정책이라 함께 런타임에서 조정한다.
	// 운영하며 "며칠이 적당한가"가 바뀌는 값이고, 바꾸는 데 재배포가 필요할 이유가 없다.
	KeyStoppedTTLDays       = "stopped_ttl_days"              // 중단 세션이 회수 후보가 되는 방치 일수(0=회수 비활성)
	KeyHomeReapPct          = "home_reap_pct"                 // 노드 디스크 사용률 임계(%). 이 이상인 노드에서만 집행한다
	KeyRechargeEnabled      = "credit_recharge_enabled"       // 크레딧 정기 재충전 on/off
	KeyRechargeAmount       = "credit_recharge_amount"        // 재충전 크레딧 양
	KeyRechargeIntervalDays = "credit_recharge_interval_days" // 재충전 주기(일)
	KeyRechargeReset        = "credit_recharge_reset"         // (구) true 면 잔액 리셋. 계층 이월로 대체됐다
	KeyRechargeCarryover    = "credit_recharge_carryover"     // 플랫폼 이월 허용(계층 이월 상한 경계)
	// 전역 하드 상한(정책 탭의 '전역' 행). 설치 기본은 env(config.Quota), 여기 값이 있으면 우선.
	// 세션 생성 시 숫자 비교만 하는 값이라 인프라와 무관하고, 재시작 없이 런타임에서 조정할 수 있다.
	KeyQuotaMaxGpu      = "quota_max_gpu"
	KeyQuotaMaxVramGB   = "quota_max_vram_gb"
	KeyQuotaMaxVolGiB   = "quota_max_volume_gib"
	KeyQuotaMaxSessions = "quota_max_concurrent_sessions"
	// 중단 세션 상한·임시 디스크 상한도 정책 탭에서 편집한다. 이 키가 없던 동안
	// SetGlobal 이 두 값을 저장할 곳이 없어, 화면에서 바꿔도 저장이 안 되고
	// 저장 시 Resolved 에서 누락돼 0(무제한)으로 덮여 있었다.
	KeyQuotaMaxStopped   = "quota_max_stopped_sessions"
	KeyQuotaMaxEphemeral = "quota_max_ephemeral_gib"
	KeyBrandName         = "brand_name"
	KeyBrandAccent       = "brand_accent"
	KeyBrandSubtitle     = "brand_subtitle"
	KeyBrandIcon         = "brand_icon"
	KeyBrandIconUrl      = "brand_icon_url"
	KeySignupRequest     = "feature_signup_request"
	KeyDatasetRegister   = "feature_dataset_register"
	KeyWorkloadAlerts    = "feature_workload_alerts"
	KeyGroupJoinRequest  = "feature_group_join"
	KeyCreditRequest     = "feature_credit_request"
)

// BoolKeys는 PUT 으로 갱신 가능한 불리언 런타임 키 집합.
var BoolKeys = map[string]bool{
	KeySignupRequest:    true,
	KeyDatasetRegister:  true,
	KeyWorkloadAlerts:   true,
	KeyGroupJoinRequest: true,
	KeyCreditRequest:    true,
}

// Store는 런타임 설정의 key/value 영속 저장소(runtime_config).
type Store struct{ db *gorm.DB }

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

type kv struct {
	CfgKey   string `gorm:"column:cfg_key"`
	CfgValue string `gorm:"column:cfg_value"`
}

// All은 저장된 모든 런타임 설정을 map 으로 반환한다(미설정이면 빈 맵).
func (s *Store) All() map[string]string {
	out := map[string]string{}
	if s == nil || s.db == nil {
		return out
	}
	var rows []kv
	s.db.Raw(`SELECT cfg_key, cfg_value FROM runtime_config`).Scan(&rows)
	for _, r := range rows {
		out[r.CfgKey] = r.CfgValue
	}
	return out
}

// Set은 런타임 설정 1건을 upsert 한다.
func (s *Store) Set(key, value string) error {
	return s.db.Exec(
		`INSERT INTO runtime_config (cfg_key, cfg_value) VALUES (?, ?)
		 ON DUPLICATE KEY UPDATE cfg_value = VALUES(cfg_value)`, key, value).Error
}

// IntOr는 정수 런타임 설정을 읽고 미설정/파싱오류면 def 를 반환한다(라이브 동작용).
// Get은 런타임 설정 문자열 값(없으면 "").
func (s *Store) Get(key string) string {
	if s == nil || s.db == nil {
		return ""
	}
	var v string
	s.db.Raw(`SELECT cfg_value FROM runtime_config WHERE cfg_key = ?`, key).Scan(&v)
	return v
}

func (s *Store) IntOr(key string, def int) int {
	if s == nil || s.db == nil {
		return def
	}
	var v string
	s.db.Raw(`SELECT cfg_value FROM runtime_config WHERE cfg_key = ?`, key).Scan(&v)
	n := atoiOr(v, def)
	return n
}

func atoiOr(s string, def int) int {
	n, sign, seen := 0, 1, false
	for i, ch := range s {
		if i == 0 && ch == '-' {
			sign = -1
			continue
		}
		if ch < '0' || ch > '9' {
			return def
		}
		n = n*10 + int(ch-'0')
		seen = true
	}
	if !seen {
		return def
	}
	return n * sign
}
