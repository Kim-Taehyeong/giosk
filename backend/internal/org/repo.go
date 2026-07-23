package org

import "gorm.io/gorm"

// Repository는 org 영속성 계약.
type Repository interface {
	List() ([]Summary, error)
	Create(o *Organization) error
	Update(id int64, fields map[string]any) error
	Archive(id int64) error
	AddCredit(id, amount int64) error
	SetRecurring(id int64, amount int) error
	SetRefill(id int64, intervalDays int, carryover bool) error
	AddPool(orgID int64, amount int) error          // 조직 풀 가산(발행/자동 조직 부여)
	DrawPool(orgID int64, amount int) (bool, error) // 조직 풀에서 차감(잔여 부족이면 false)
	OrgOfGroup(groupID int64) (int64, bool)         // 그룹의 조직 id
	OrgOfUser(userID int64) (int64, bool)           // 사용자의 (대표 멤버십) 조직 id
	GroupOfUser(userID int64) (int64, bool)         // 사용자의 (대표 멤버십) 그룹 id
	PublicCatalog() ([]PublicOrg, error)
	Overview(orgID int64) (*Overview, error)
	GroupsOf(orgID int64) ([]GroupBudget, error)
	IsOrgAdmin(userID, orgID int64) bool
}

type gormRepo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

// List는 그룹/유저 카운트를 포함한 조직 목록을 반환한다.
func (r *gormRepo) List() ([]Summary, error) {
	var out []Summary
	err := r.db.Raw(`
		SELECT o.*,
		       (SELECT COUNT(*) FROM ` + "`groups`" + ` g WHERE g.org_id = o.id) AS group_count,
		       (SELECT COUNT(DISTINCT m.user_id) FROM ` + "`groups`" + ` g
		          JOIN memberships m ON m.group_id = g.id WHERE g.org_id = o.id) AS user_count
		FROM organizations o
		WHERE o.status = 'active'
		ORDER BY o.id`).Scan(&out).Error
	return out, err
}

func (r *gormRepo) Create(o *Organization) error { return r.db.Create(o).Error }

func (r *gormRepo) Update(id int64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.Model(&Organization{}).Where("id = ?", id).Updates(fields).Error
}

func (r *gormRepo) Archive(id int64) error {
	return r.db.Model(&Organization{}).Where("id = ?", id).Update("status", "archived").Error
}

func (r *gormRepo) AddCredit(id, amount int64) error {
	return r.db.Model(&Organization{}).Where("id = ?", id).
		Update("credit_pool", gorm.Expr("credit_pool + ?", amount)).Error
}

// SetRecurring은 조직의 정기 크레딧(매 주기 재생성 양)을 설정한다.
func (r *gormRepo) SetRecurring(id int64, amount int) error {
	return r.db.Model(&Organization{}).Where("id = ?", id).
		Update("recurring_credit", amount).Error
}

// SetRefill은 조직의 리필 주기(일)·이월 여부를 설정한다.
func (r *gormRepo) SetRefill(id int64, intervalDays int, carryover bool) error {
	return r.db.Model(&Organization{}).Where("id = ?", id).
		Updates(map[string]any{"refill_interval_days": intervalDays, "carryover": carryover}).Error
}

// AddPool은 조직 풀에 amount 를 가산한다(최고관리자 발행/자동 조직 부여).
func (r *gormRepo) AddPool(orgID int64, amount int) error {
	return r.db.Model(&Organization{}).Where("id = ?", orgID).
		Update("credit_pool", gorm.Expr("credit_pool + ?", amount)).Error
}

// GroupOfUser는 사용자의 대표(활성) 멤버십 그룹 id 를 반환한다(없으면 false).
func (r *gormRepo) GroupOfUser(userID int64) (int64, bool) {
	var id int64
	r.db.Raw("SELECT group_id FROM memberships WHERE user_id=? AND status='active' ORDER BY id LIMIT 1", userID).Scan(&id)
	return id, id > 0
}

// DrawPool은 조직 풀에서 amount 만큼 원자적으로 차감한다(잔여 부족이면 차감 없이 false).
func (r *gormRepo) DrawPool(orgID int64, amount int) (bool, error) {
	res := r.db.Model(&Organization{}).
		Where("id = ? AND credit_pool >= ?", orgID, amount).
		Update("credit_pool", gorm.Expr("credit_pool - ?", amount))
	return res.RowsAffected > 0, res.Error
}

// OrgOfGroup은 그룹의 조직 id 를 반환한다.
func (r *gormRepo) OrgOfGroup(groupID int64) (int64, bool) {
	var id int64
	r.db.Raw("SELECT org_id FROM `groups` WHERE id = ?", groupID).Scan(&id)
	return id, id > 0
}

// OrgOfUser는 사용자의 대표(활성) 멤버십 조직 id 를 반환한다(없으면 false).
func (r *gormRepo) OrgOfUser(userID int64) (int64, bool) {
	var id int64
	r.db.Raw("SELECT g.org_id FROM memberships m JOIN `groups` g ON g.id=m.group_id WHERE m.user_id=? AND m.status='active' ORDER BY m.id LIMIT 1", userID).Scan(&id)
	return id, id > 0
}

// PublicCatalog은 활성 조직+그룹을 가입 폼용으로 반환한다(읽기 전용 프로젝션).
func (r *gormRepo) PublicCatalog() ([]PublicOrg, error) {
	var orgs []Organization
	if err := r.db.Where("status = 'active'").Order("id").Find(&orgs).Error; err != nil {
		return nil, err
	}
	out := make([]PublicOrg, 0, len(orgs))
	for _, o := range orgs {
		groups, err := r.publicGroups(o.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, PublicOrg{ID: o.ID, Name: o.Name, DisplayName: o.DisplayName, Groups: groups})
	}
	return out, nil
}

// Overview는 조직 관리자 대시보드 집계(풀/소진/그룹수/멤버수/대기요청/상위그룹).
func (r *gormRepo) Overview(orgID int64) (*Overview, error) {
	var o Overview
	err := r.db.Raw(`
		SELECT org.display_name AS org_name, org.credit_pool AS pool, org.default_group_budget,
		       (SELECT COUNT(*) FROM `+"`groups`"+` g WHERE g.org_id = org.id AND g.status='active') AS group_count,
		       (SELECT COUNT(DISTINCT m.user_id) FROM `+"`groups`"+` g JOIN memberships m ON m.group_id=g.id
		          WHERE g.org_id=org.id AND m.status='active') AS member_count,
		       (SELECT COALESCE(SUM(w.balance),0) FROM `+"`groups`"+` g JOIN group_wallets w ON w.group_id=g.id
		          WHERE g.org_id=org.id) AS consumed,
		       (SELECT COUNT(*) FROM topup_requests t JOIN `+"`groups`"+` g ON g.id=t.target_id
		          WHERE t.target_type='group' AND g.org_id=org.id AND t.status='pending') AS pending_req
		FROM organizations org WHERE org.id = ?`, orgID).Scan(&o).Error
	if err != nil {
		return nil, err
	}
	o.TopGroups, _ = r.topGroups(orgID)
	return &o, nil
}

func (r *gormRepo) topGroups(orgID int64) ([]GroupCredit, error) {
	var out []GroupCredit
	err := r.db.Raw(`
		SELECT g.display_name AS name, COALESCE(w.balance,0) AS credit
		FROM `+"`groups`"+` g LEFT JOIN group_wallets w ON w.group_id=g.id
		WHERE g.org_id=? AND g.status='active' ORDER BY credit DESC LIMIT 5`, orgID).Scan(&out).Error
	return out, err
}

// GroupsOf는 조직 내 그룹 목록(멤버수/지갑 포함).
func (r *gormRepo) GroupsOf(orgID int64) ([]GroupBudget, error) {
	var out []GroupBudget
	err := r.db.Raw(`
		SELECT g.id, g.name, g.display_name, g.status,
		       (SELECT COUNT(*) FROM memberships m WHERE m.group_id=g.id AND m.status='active') AS member_count,
		       COALESCE(w.balance,0) AS balance, COALESCE(w.budget_cap,0) AS budget_cap
		FROM `+"`groups`"+` g LEFT JOIN group_wallets w ON w.group_id=g.id
		WHERE g.org_id=? AND g.status='active' ORDER BY g.id`, orgID).Scan(&out).Error
	return out, err
}

// IsOrgAdmin은 사용자가 해당 조직 산하 그룹에서 org_admin 멤버십을 가지는지.
func (r *gormRepo) IsOrgAdmin(userID, orgID int64) bool {
	var n int64
	r.db.Raw(`
		SELECT COUNT(*) FROM memberships m JOIN `+"`groups`"+` g ON g.id=m.group_id
		WHERE m.user_id=? AND g.org_id=? AND m.role='org_admin' AND m.status='active'`,
		userID, orgID).Scan(&n)
	return n > 0
}

// publicGroups는 가입 화면에서 고를 수 있는 그룹만 반환한다.
// accepts_join=false 인 그룹(조직 생성 시 만들어지는 '일반' = 조직 관리자 전용)은 제외 —
// 로그인 후 가입 신청 경로(RequestJoin)와 같은 규칙을 쓴다(예전엔 여기서만 안 걸러 '일반'이 노출됐다).
func (r *gormRepo) publicGroups(orgID int64) ([]PublicGroup, error) {
	var groups []PublicGroup
	err := r.db.Table("`groups`").
		Select("id, name, display_name").
		Where("org_id = ? AND status = 'active' AND accepts_join = ?", orgID, true).
		Order("id").Scan(&groups).Error
	return groups, err
}
