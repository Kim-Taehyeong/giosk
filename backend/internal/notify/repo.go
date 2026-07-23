package notify

import "gorm.io/gorm"

// Repository는 알림 설정 영속성 계약(scope/owner 단위 전체 교체).
type Repository interface {
	Get(scope string, owner int64) (Config, error)
	Replace(scope string, owner int64, cfg PutReq) error
	// EnabledUserRules는 활성화된 사용자 규칙 전체(OwnerID 포함)를 반환한다(엔진 사용자 평가용).
	EnabledUserRules() ([]Rule, error)
}

type gormRepo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

// Get은 규칙/채널을 조회한다. 규칙이 없으면 scope 기본 규칙을 반환(미저장).
func (r *gormRepo) Get(scope string, owner int64) (Config, error) {
	var rules []Rule
	if err := r.db.Where("scope = ? AND owner_id = ?", scope, owner).Order("id").Find(&rules).Error; err != nil {
		return Config{}, err
	}
	if len(rules) == 0 {
		rules = defaultRules(scope)
	}
	var chans []Channel
	if err := r.db.Where("scope = ? AND owner_id = ?", scope, owner).Order("id").Find(&chans).Error; err != nil {
		return Config{}, err
	}
	cfg := Config{Rules: rules, Emails: []string{}, Webhooks: []string{}}
	for _, c := range chans {
		if c.Kind == "webhook" {
			cfg.Webhooks = append(cfg.Webhooks, c.Target)
		} else {
			cfg.Emails = append(cfg.Emails, c.Target)
		}
	}
	return cfg, nil
}

// EnabledUserRules는 활성 사용자 규칙 전체를 OwnerID 포함해 반환한다.
// 기본 규칙(미저장)은 대상이 아니다 — 사용자가 명시적으로 켠 규칙만 발화한다.
func (r *gormRepo) EnabledUserRules() ([]Rule, error) {
	var rules []Rule
	err := r.db.Where("scope = ? AND enabled = ?", ScopeUser, true).Order("owner_id, id").Find(&rules).Error
	return rules, err
}

// Replace는 scope/owner 의 규칙·채널을 통째로 교체한다(트랜잭션).
func (r *gormRepo) Replace(scope string, owner int64, in PutReq) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("scope = ? AND owner_id = ?", scope, owner).Delete(&Rule{}).Error; err != nil {
			return err
		}
		if err := tx.Where("scope = ? AND owner_id = ?", scope, owner).Delete(&Channel{}).Error; err != nil {
			return err
		}
		for _, rule := range in.Rules {
			rule.ID, rule.Scope, rule.OwnerID = 0, scope, owner
			if err := tx.Create(&rule).Error; err != nil {
				return err
			}
		}
		if err := insertChannels(tx, scope, owner, "email", in.Emails); err != nil {
			return err
		}
		return insertChannels(tx, scope, owner, "webhook", in.Webhooks)
	})
}

func insertChannels(tx *gorm.DB, scope string, owner int64, kind string, targets []string) error {
	for _, target := range targets {
		if target == "" {
			continue
		}
		if err := tx.Create(&Channel{Scope: scope, OwnerID: owner, Kind: kind, Target: target}).Error; err != nil {
			return err
		}
	}
	return nil
}
