package billing

import "gorm.io/gorm"

// Repository는 빌링 집계(읽기 전용) 계약.
type Repository interface {
	ByGroup() []GroupRow
	ByUser() []UserRow
	ByOrg() []OrgRow
	// Scoped 변형: orgID>0=조직 범위, groupID>0=그룹 범위, 둘 다 0=전역(플랫폼).
	ByGroupScoped(orgID, groupID int64) []GroupRow
	ByUserScoped(orgID, groupID int64) []UserRow
	ByOrgScoped(orgID, groupID int64) []OrgRow
}

type gormRepo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

// ByGroup — 그룹별 소비/예산. consumed=소속 멤버 사용량 합(memberships.consumed).
func (r *gormRepo) ByGroup() []GroupRow {
	var out []GroupRow
	r.db.Raw(`
		SELECT g.id, g.display_name AS name, COALESCE(o.display_name,'') AS org_name,
		       (SELECT COUNT(*) FROM sessions s WHERE s.group_id = g.id) AS sessions,
		       COALESCE(ROUND((SELECT SUM(gu.seconds) FROM gpu_usage gu WHERE gu.group_id = g.id)/3600,1),0) AS gpu_hours,
		       COALESCE((SELECT SUM(m.consumed) FROM memberships m WHERE m.group_id = g.id),0) AS consumed,
		       COALESCE(w.budget_cap,0) AS budget_cap
		FROM ` + "`groups`" + ` g
		LEFT JOIN organizations o ON o.id = g.org_id
		LEFT JOIN group_wallets w ON w.group_id = g.id
		ORDER BY consumed DESC`).Scan(&out)
	for i := range out {
		out[i].UsagePct = pct(out[i].Consumed, out[i].BudgetCap)
	}
	return out
}

// ByUser — 멤버별 소비(memberships.consumed) + 세션 수.
func (r *gormRepo) ByUser() []UserRow {
	var out []UserRow
	r.db.Raw(`
		SELECT u.id, u.username AS name, COALESCE(g.display_name,'') AS ` + "`group`" + `,
		       (SELECT COUNT(*) FROM sessions s WHERE s.user_id = u.id) AS sessions,
		       COALESCE(ROUND((SELECT SUM(gu.seconds) FROM gpu_usage gu WHERE gu.user_id = u.id)/3600,1),0) AS gpu_hours,
		       COALESCE((SELECT SUM(s2.billed_credits) FROM sessions s2 WHERE s2.user_id = u.id),0) AS consumed
		FROM memberships m
		JOIN users u ON u.id = m.user_id
		LEFT JOIN ` + "`groups`" + ` g ON g.id = m.group_id
		WHERE m.status = 'active'
		ORDER BY consumed DESC`).Scan(&out)
	return out
}

// ByOrg — 조직별 그룹/사용자 수 + 소비(산하 그룹 consume 합) + 풀.
func (r *gormRepo) ByOrg() []OrgRow {
	var out []OrgRow
	r.db.Raw(`
		SELECT o.id, o.display_name AS name, o.credit_pool,
		       (SELECT COUNT(*) FROM ` + "`groups`" + ` g WHERE g.org_id = o.id) AS ` + "`groups`" + `,
		       (SELECT COUNT(DISTINCT m.user_id) FROM memberships m
		         JOIN ` + "`groups`" + ` g2 ON g2.id = m.group_id WHERE g2.org_id = o.id) AS users,
		       COALESCE(ROUND((SELECT SUM(gu.seconds) FROM gpu_usage gu
		         JOIN ` + "`groups`" + ` g4 ON g4.id = gu.group_id WHERE g4.org_id = o.id)/3600,1),0) AS gpu_hours,
		       COALESCE((SELECT SUM(m.consumed) FROM memberships m
		         JOIN ` + "`groups`" + ` g3 ON g3.id = m.group_id WHERE g3.org_id = o.id),0) AS consumed
		FROM organizations o
		ORDER BY consumed DESC`).Scan(&out)
	return out
}

// ByGroupScoped — 그룹 범위 필터. group=자기 그룹, org=산하 그룹.
func (r *gormRepo) ByGroupScoped(orgID, groupID int64) []GroupRow {
	if orgID <= 0 && groupID <= 0 {
		return r.ByGroup()
	}
	where, args := "", []any{}
	if groupID > 0 {
		where, args = "WHERE g.id = ?", []any{groupID}
	} else {
		where, args = "WHERE g.org_id = ?", []any{orgID}
	}
	var out []GroupRow
	r.db.Raw(`
		SELECT g.id, g.display_name AS name, COALESCE(o.display_name,'') AS org_name,
		       (SELECT COUNT(*) FROM sessions s WHERE s.group_id = g.id) AS sessions,
		       COALESCE(ROUND((SELECT SUM(gu.seconds) FROM gpu_usage gu WHERE gu.group_id = g.id)/3600,1),0) AS gpu_hours,
		       COALESCE((SELECT SUM(m.consumed) FROM memberships m WHERE m.group_id = g.id),0) AS consumed,
		       COALESCE(w.budget_cap,0) AS budget_cap
		FROM `+"`groups`"+` g
		LEFT JOIN organizations o ON o.id = g.org_id
		LEFT JOIN group_wallets w ON w.group_id = g.id
		`+where+`
		ORDER BY consumed DESC`, args...).Scan(&out)
	for i := range out {
		out[i].UsagePct = pct(out[i].Consumed, out[i].BudgetCap)
	}
	return out
}

// ByUserScoped — 사용자 범위 필터. group=그룹 멤버, org=조직 소속.
func (r *gormRepo) ByUserScoped(orgID, groupID int64) []UserRow {
	if orgID <= 0 && groupID <= 0 {
		return r.ByUser()
	}
	where, args := "", []any{}
	if groupID > 0 {
		where, args = "AND m.group_id = ?", []any{groupID}
	} else {
		where, args = "AND m.group_id IN (SELECT id FROM `groups` WHERE org_id = ?)", []any{orgID}
	}
	var out []UserRow
	r.db.Raw(`
		SELECT u.id, u.username AS name, COALESCE(g.display_name,'') AS `+"`group`"+`,
		       (SELECT COUNT(*) FROM sessions s WHERE s.user_id = u.id) AS sessions,
		       COALESCE(ROUND((SELECT SUM(gu.seconds) FROM gpu_usage gu WHERE gu.user_id = u.id)/3600,1),0) AS gpu_hours,
		       COALESCE((SELECT SUM(s2.billed_credits) FROM sessions s2 WHERE s2.user_id = u.id),0) AS consumed
		FROM memberships m
		JOIN users u ON u.id = m.user_id
		LEFT JOIN `+"`groups`"+` g ON g.id = m.group_id
		WHERE m.status = 'active' `+where+`
		ORDER BY consumed DESC`, args...).Scan(&out)
	return out
}

// ByOrgScoped — 조직 범위 필터. org=자기 조직, group=부모 조직(읽기용).
func (r *gormRepo) ByOrgScoped(orgID, groupID int64) []OrgRow {
	if orgID <= 0 && groupID <= 0 {
		return r.ByOrg()
	}
	where, args := "", []any{}
	if orgID > 0 {
		where, args = "WHERE o.id = ?", []any{orgID}
	} else {
		where, args = "WHERE o.id = (SELECT org_id FROM `groups` WHERE id = ?)", []any{groupID}
	}
	var out []OrgRow
	r.db.Raw(`
		SELECT o.id, o.display_name AS name, o.credit_pool,
		       (SELECT COUNT(*) FROM `+"`groups`"+` g WHERE g.org_id = o.id) AS `+"`groups`"+`,
		       (SELECT COUNT(DISTINCT m.user_id) FROM memberships m
		         JOIN `+"`groups`"+` g2 ON g2.id = m.group_id WHERE g2.org_id = o.id) AS users,
		       COALESCE(ROUND((SELECT SUM(gu.seconds) FROM gpu_usage gu
		         JOIN `+"`groups`"+` g4 ON g4.id = gu.group_id WHERE g4.org_id = o.id)/3600,1),0) AS gpu_hours,
		       COALESCE((SELECT SUM(m.consumed) FROM memberships m
		         JOIN `+"`groups`"+` g3 ON g3.id = m.group_id WHERE g3.org_id = o.id),0) AS consumed
		FROM organizations o
		`+where+`
		ORDER BY consumed DESC`, args...).Scan(&out)
	return out
}

func pct(consumed, cap int) int {
	if cap <= 0 {
		return 0
	}
	return consumed * 100 / cap
}
