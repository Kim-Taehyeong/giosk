package billing

import "gorm.io/gorm"

// Range는 빌링 집계 기간(ISO datetime 문자열; 빈 값=전체 기간). To 는 배타(< To).
// 기간이 지정되면 소비는 원장(credit_transactions consume)·GPU시간은 gpu_usage 를 그 기간으로 집계한다.
type Range struct{ From, To string }

func (r Range) set() bool { return r.From != "" || r.To != "" }

// clause는 지정 컬럼에 기간 조건을 만든다(placeholder + args, document 순서).
func (r Range) clause(col string) (string, []any) {
	s, a := "", []any{}
	if r.From != "" {
		s += " AND " + col + " >= ?"
		a = append(a, r.From)
	}
	if r.To != "" {
		s += " AND " + col + " < ?"
		a = append(a, r.To)
	}
	return s, a
}

// Repository는 빌링 집계(읽기 전용) 계약.
type Repository interface {
	ByGroup(r Range) []GroupRow
	ByUser(r Range) []UserRow
	ByOrg() []OrgRow
	// Scoped 변형: orgID>0=조직 범위, groupID>0=그룹 범위, 둘 다 0=전역(플랫폼).
	ByGroupScoped(orgID, groupID int64, r Range) []GroupRow
	ByUserScoped(orgID, groupID int64, r Range) []UserRow
	ByOrgScoped(orgID, groupID int64) []OrgRow
	// UserOneScoped: 단일 사용자의 소비/세션/GPU시간을 그 스코프 범위로만 집계(팀 관점 사용자 상세).
	UserOneScoped(userID, orgID, groupID int64) *UserRow
}

type gormRepo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

// ByGroup은 그룹별 소비와 예산을 준다. 기간(r)을 안 주면 memberships.consumed 전체, 주면 원장 consume 을 기간으로 본다.
func (r *gormRepo) ByGroup(rng Range) []GroupRow {
	q, args := groupRowsQuery("", nil, rng)
	var out []GroupRow
	r.db.Raw(q, args...).Scan(&out)
	for i := range out {
		out[i].UsagePct = pct(out[i].Consumed, out[i].BudgetCap)
	}
	return out
}

// groupRowsQuery는 그룹별 소비 행 SQL 을 만든다. where 는 "WHERE ..."(스코프 필터; 없으면 빈 문자열).
func groupRowsQuery(where string, whereArgs []any, rng Range) (string, []any) {
	sR, sA := rng.clause("s.created_at")
	gR, gA := rng.clause("gu.created_at")
	var consumed string
	var cA []any
	if rng.set() {
		cR, ca := rng.clause("ct.created_at")
		consumed = "COALESCE((SELECT SUM(-ct.amount) FROM credit_transactions ct WHERE ct.group_id = g.id AND ct.type = 'consume'" + cR + "),0)"
		cA = ca
	} else {
		consumed = "COALESCE((SELECT SUM(m.consumed) FROM memberships m WHERE m.group_id = g.id),0)"
	}
	q := `
		SELECT g.id, g.display_name AS name, COALESCE(o.display_name,'') AS org_name,
		       (SELECT COUNT(*) FROM sessions s WHERE s.group_id = g.id` + sR + `) AS sessions,
		       COALESCE(ROUND((SELECT SUM(gu.seconds) FROM gpu_usage gu WHERE gu.group_id = g.id` + gR + `)/3600,1),0) AS gpu_hours,
		       ` + consumed + ` AS consumed,
		       COALESCE(w.budget_cap,0) AS budget_cap
		FROM ` + "`groups`" + ` g
		LEFT JOIN organizations o ON o.id = g.org_id
		LEFT JOIN group_wallets w ON w.group_id = g.id
		` + where + `
		ORDER BY consumed DESC`
	out := []any{}
	out = append(out, sA...)
	out = append(out, gA...)
	out = append(out, cA...)
	out = append(out, whereArgs...)
	return q, out
}

// ByUser는 (사용자와 팀) 단위 소비, 세션, GPU시간을 준다. 크레딧이 팀 지갑에서 차감되므로 다중 소속 사용자는
// 팀마다 한 행씩(각 행 숫자는 그 팀에서 쓴 만큼만). 기간(r) 지정 시 그 기간 발생분만.
func (r *gormRepo) ByUser(rng Range) []UserRow {
	q, args := userRowsQuery("WHERE m.status = 'active'", nil, rng)
	var out []UserRow
	r.db.Raw(q, args...).Scan(&out)
	return out
}

// userRowsQuery는 (사용자×팀) 소비 행 SQL 을 만든다. extraWhere 는 "WHERE ..." 시작 문자열(스코프 필터 포함),
// whereArgs 는 그 스코프 인자, rng 는 기간. 소비=원장 consume(기간 지정 시)·미지정 시 sessions.billed_credits.
func userRowsQuery(where string, whereArgs []any, rng Range) (string, []any) {
	sR, sA := rng.clause("s.created_at")  // sessions 수 기간(세션 시작 기준)
	gR, gA := rng.clause("gu.created_at") // gpu_hours 기간
	// 소비: 기간 지정이면 원장(정확한 기간별), 아니면 기존 billed_credits(전체).
	var consumed string
	var cA []any
	if rng.set() {
		cR, ca := rng.clause("ct.created_at")
		consumed = "COALESCE((SELECT SUM(-ct.amount) FROM credit_transactions ct WHERE ct.user_id = u.id AND ct.group_id = m.group_id AND ct.type = 'consume'" + cR + "),0)"
		cA = ca
	} else {
		consumed = "COALESCE((SELECT SUM(s2.billed_credits) FROM sessions s2 WHERE s2.user_id = u.id AND s2.group_id = m.group_id),0)"
	}
	q := `
		SELECT u.id, m.group_id AS group_id, u.username AS name,
		       COALESCE(o.display_name,'') AS org, COALESCE(g.display_name,'') AS ` + "`group`" + `,
		       (SELECT COUNT(*) FROM sessions s WHERE s.user_id = u.id AND s.group_id = m.group_id` + sR + `) AS sessions,
		       COALESCE(ROUND((SELECT SUM(gu.seconds) FROM gpu_usage gu WHERE gu.user_id = u.id AND gu.group_id = m.group_id` + gR + `)/3600,1),0) AS gpu_hours,
		       ` + consumed + ` AS consumed
		FROM memberships m
		JOIN users u ON u.id = m.user_id
		JOIN ` + "`groups`" + ` g ON g.id = m.group_id
		LEFT JOIN organizations o ON o.id = g.org_id
		` + where + `
		ORDER BY consumed DESC, u.username`
	// 인자 순서는 SQL 등장 순서와 같다: sessions range, gpu range, consumed range, WHERE 스코프.
	out := []any{}
	out = append(out, sA...)
	out = append(out, gA...)
	out = append(out, cA...)
	out = append(out, whereArgs...)
	return q, out
}

// ByOrg는 조직별 그룹·사용자 수, 소비(산하 그룹 consume 합), 풀을 준다.
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

// ByGroupScoped는 그룹 범위 필터다. group 은 자기 그룹, org 는 산하 그룹을 본다. 기간(r)을 지원한다.
func (r *gormRepo) ByGroupScoped(orgID, groupID int64, rng Range) []GroupRow {
	if orgID <= 0 && groupID <= 0 {
		return r.ByGroup(rng)
	}
	where, warg := "WHERE g.org_id = ?", orgID
	if groupID > 0 {
		where, warg = "WHERE g.id = ?", groupID
	}
	q, args := groupRowsQuery(where, []any{warg}, rng)
	var out []GroupRow
	r.db.Raw(q, args...).Scan(&out)
	for i := range out {
		out[i].UsagePct = pct(out[i].Consumed, out[i].BudgetCap)
	}
	return out
}

// ByUserScoped는 스코프(팀이나 조직) 범위의 (사용자와 팀) 소비다. 각 행은 그 팀에서 쓴 만큼만 집계한다.
// group 스코프면 그 팀의 멤버 1행씩, org 스코프면 산하 팀별로 한 행씩. 기간(r) 지원.
func (r *gormRepo) ByUserScoped(orgID, groupID int64, rng Range) []UserRow {
	if orgID <= 0 && groupID <= 0 {
		return r.ByUser(rng)
	}
	where := "WHERE m.status = 'active' AND g.id = ?"
	arg := groupID
	if groupID <= 0 {
		where = "WHERE m.status = 'active' AND g.org_id = ?"
		arg = orgID
	}
	q, args := userRowsQuery(where, []any{arg}, rng)
	var out []UserRow
	r.db.Raw(q, args...).Scan(&out)
	return out
}

// UserOneScoped는 단일 사용자의 세션 수, GPU시간, 소비를 스코프 범위로만 집계한다.
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

// ByOrgScoped는 조직 범위 필터다. org 는 자기 조직, group 은 부모 조직을 읽는다.
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
