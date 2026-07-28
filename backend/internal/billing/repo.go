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
	// UserOneScoped: 단일 사용자의 소비/세션/GPU시간을 그 스코프 범위로만 집계(팀 관점 사용자 상세).
	UserOneScoped(userID, orgID, groupID int64) *UserRow
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

// ByUser — 사용자별 소비/세션/GPU시간. 사용자 1행(여러 팀 소속이어도 중복 안 됨) + 대표 조직/팀 표시.
// 예전엔 memberships 를 JOIN 해 다중소속 사용자가 중복 행으로 나오고 조직도 안 보였다.
func (r *gormRepo) ByUser() []UserRow {
	var out []UserRow
	r.db.Raw(`
		SELECT u.id, u.username AS name,
		       COALESCE((SELECT o.display_name FROM memberships m2 JOIN ` + "`groups`" + ` g2 ON g2.id=m2.group_id JOIN organizations o ON o.id=g2.org_id WHERE m2.user_id=u.id AND m2.status='active' ORDER BY m2.id LIMIT 1),'') AS org,
		       COALESCE((SELECT g3.display_name FROM memberships m3 JOIN ` + "`groups`" + ` g3 ON g3.id=m3.group_id WHERE m3.user_id=u.id AND m3.status='active' ORDER BY m3.id LIMIT 1),'') AS ` + "`group`" + `,
		       (SELECT COUNT(*) FROM sessions s WHERE s.user_id = u.id) AS sessions,
		       COALESCE(ROUND((SELECT SUM(gu.seconds) FROM gpu_usage gu WHERE gu.user_id = u.id)/3600,1),0) AS gpu_hours,
		       COALESCE((SELECT SUM(s2.billed_credits) FROM sessions s2 WHERE s2.user_id = u.id),0) AS consumed
		FROM users u
		WHERE u.id IN (SELECT DISTINCT user_id FROM memberships WHERE status='active')
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

// ByUserScoped — 스코프(팀/조직) 범위의 사용자별 소비. 사용자 1행(중복 없음)이고, 소비/세션/GPU시간은
// 그 범위(팀=group_id, 조직=산하 그룹)에서 발생한 것만 집계한다(전역 아님) → 팀/조직 showback 이 정확.
func (r *gormRepo) ByUserScoped(orgID, groupID int64) []UserRow {
	if orgID <= 0 && groupID <= 0 {
		return r.ByUser()
	}
	// 대상 사용자 필터(어떤 사용자가 나오는지) + 세션/사용량 서브쿼리에 걸 스코프 절.
	var memberWhere, scS, scGU, scS2 string
	var scopeArg any
	if groupID > 0 {
		memberWhere = "group_id = ?"
		scS, scGU, scS2 = " AND s.group_id = ?", " AND gu.group_id = ?", " AND s2.group_id = ?"
		scopeArg = groupID
	} else {
		gsub := " IN (SELECT id FROM `groups` WHERE org_id = ?)"
		memberWhere = "group_id" + gsub
		scS, scGU, scS2 = " AND s.group_id"+gsub, " AND gu.group_id"+gsub, " AND s2.group_id"+gsub
		scopeArg = orgID
	}
	q := `
		SELECT u.id, u.username AS name,
		       COALESCE((SELECT o.display_name FROM memberships m2 JOIN ` + "`groups`" + ` g2 ON g2.id=m2.group_id JOIN organizations o ON o.id=g2.org_id WHERE m2.user_id=u.id AND m2.status='active' ORDER BY m2.id LIMIT 1),'') AS org,
		       COALESCE((SELECT g3.display_name FROM memberships m3 JOIN ` + "`groups`" + ` g3 ON g3.id=m3.group_id WHERE m3.user_id=u.id AND m3.status='active' ORDER BY m3.id LIMIT 1),'') AS ` + "`group`" + `,
		       (SELECT COUNT(*) FROM sessions s WHERE s.user_id = u.id` + scS + `) AS sessions,
		       COALESCE(ROUND((SELECT SUM(gu.seconds) FROM gpu_usage gu WHERE gu.user_id = u.id` + scGU + `)/3600,1),0) AS gpu_hours,
		       COALESCE((SELECT SUM(s2.billed_credits) FROM sessions s2 WHERE s2.user_id = u.id` + scS2 + `),0) AS consumed
		FROM users u
		WHERE u.id IN (SELECT DISTINCT user_id FROM memberships WHERE status='active' AND ` + memberWhere + `)
		ORDER BY consumed DESC`
	// 인자 순서: sessions scope, gpu scope, consumed scope, memberWhere scope.
	args := []any{scopeArg, scopeArg, scopeArg, scopeArg}
	var out []UserRow
	r.db.Raw(q, args...).Scan(&out)
	return out
}

// UserOneScoped — 단일 사용자의 세션 수·GPU시간·소비를 스코프 범위로만 집계한다.
// groupID>0 이면 그 팀에서의 사용만, orgID>0 이면 그 조직 산하 사용만, 둘 다 0 이면 전역.
// ByUserScoped 는 "어떤 사용자가 나오는지"만 걸렀고 per-user 집계는 전역이라, 팀 관점 사용자 상세엔
// 이 메서드로 서브쿼리 자체에 스코프를 걸어야 "내 팀에서 쓴 만큼"만 나온다.
func (r *gormRepo) UserOneScoped(userID, orgID, groupID int64) *UserRow {
	// 각 서브쿼리에 붙일 스코프 절(팀 > 조직 > 전역).
	var scS, scGU, scS2 string
	var scopeArg any
	scoped := false
	switch {
	case groupID > 0:
		scS, scGU, scS2 = " AND s.group_id = ?", " AND gu.group_id = ?", " AND s2.group_id = ?"
		scopeArg, scoped = groupID, true
	case orgID > 0:
		gsub := " IN (SELECT id FROM `groups` WHERE org_id = ?)"
		scS, scGU, scS2 = " AND s.group_id"+gsub, " AND gu.group_id"+gsub, " AND s2.group_id"+gsub
		scopeArg, scoped = orgID, true
	}
	q := `
		SELECT u.id, u.username AS name,
		       COALESCE((SELECT g.display_name FROM ` + "`groups`" + ` g WHERE g.id = ?),'') AS ` + "`group`" + `,
		       (SELECT COUNT(*) FROM sessions s WHERE s.user_id = ?` + scS + `) AS sessions,
		       COALESCE(ROUND((SELECT SUM(gu.seconds) FROM gpu_usage gu WHERE gu.user_id = ?` + scGU + `)/3600,1),0) AS gpu_hours,
		       COALESCE((SELECT SUM(s2.billed_credits) FROM sessions s2 WHERE s2.user_id = ?` + scS2 + `),0) AS consumed
		FROM users u WHERE u.id = ?`
	args := []any{groupID, userID}
	if scoped {
		args = append(args, scopeArg)
	}
	args = append(args, userID)
	if scoped {
		args = append(args, scopeArg)
	}
	args = append(args, userID)
	if scoped {
		args = append(args, scopeArg)
	}
	args = append(args, userID)
	var row UserRow
	if err := r.db.Raw(q, args...).Scan(&row).Error; err != nil || row.ID == 0 {
		return nil
	}
	return &row
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
