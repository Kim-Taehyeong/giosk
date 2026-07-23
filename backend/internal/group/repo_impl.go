package group

import (
	"errors"

	"gorm.io/gorm"
)

func (r *gormRepo) ListAll() ([]Summary, error) {
	var out []Summary
	err := r.db.Raw(`
		SELECT g.*, o.display_name AS org_name,
		       (SELECT COUNT(*) FROM memberships m WHERE m.group_id = g.id AND m.status = 'active') AS member_count,
		       COALESCE(w.balance, 0) AS balance,
		       COALESCE(w.budget_cap, 0) AS budget_cap,
		       COALESCE(w.alert_threshold_pct, 0) AS alert_pct,
		       COALESCE(w.recurring_credit, 0) AS recurring_credit,
		       COALESCE(w.refill_interval_days, 0) AS refill_interval_days,
		       COALESCE(w.carryover, 0) AS carryover,
		       (SELECT COALESCE(SUM(m2.consumed), 0) FROM memberships m2 WHERE m2.group_id = g.id) AS consumed
		FROM ` + "`groups`" + ` g JOIN organizations o ON o.id = g.org_id
		LEFT JOIN group_wallets w ON w.group_id = g.id
		WHERE g.status = 'active' ORDER BY g.id`).Scan(&out).Error
	return out, err
}

func (r *gormRepo) Create(g *Group) error { return r.db.Create(g).Error }

func (r *gormRepo) Update(id int64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.Table("`groups`").Where("id = ?", id).Updates(fields).Error
}

// Archive는 그룹을 소프트 삭제한다(status='archived' → 목록에서 제외, FK/이력 보존).
func (r *gormRepo) Archive(id int64) error {
	return r.db.Table("`groups`").Where("id = ?", id).Update("status", "archived").Error
}

// CancelJoinRequest는 본인의 대기중(pending) 가입신청을 취소(삭제)한다(타인 신청은 영향 없음).
func (r *gormRepo) CancelJoinRequest(userID, reqID int64) error {
	return r.db.Exec(`DELETE FROM group_join_requests WHERE id = ? AND user_id = ? AND status = 'pending'`, reqID, userID).Error
}

func (r *gormRepo) Find(id int64) (*Group, error) {
	var g Group
	err := r.db.Where("id = ?", id).First(&g).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &g, err
}

func (r *gormRepo) RoleInGroup(userID, groupID int64) (string, bool) {
	var role string
	err := r.db.Raw(
		`SELECT role FROM memberships WHERE group_id = ? AND user_id = ? AND status = 'active'`,
		groupID, userID).Scan(&role).Error
	if err != nil || role == "" {
		return "", false
	}
	return role, true
}

func (r *gormRepo) ListMembers(groupID int64) ([]Member, error) {
	var out []Member
	err := r.db.Raw(`
		SELECT m.user_id, u.username, TRIM(CONCAT(COALESCE(u.last_name,''), COALESCE(u.first_name,''))) AS name,
		       u.email, m.role, m.status, m.budget, m.consumed,
		       COALESCE(uw.balance, 0) AS balance
		FROM memberships m JOIN users u ON u.id = m.user_id
		LEFT JOIN user_wallets uw ON uw.user_id = m.user_id
		WHERE m.group_id = ? AND m.status <> 'removed' ORDER BY m.id`, groupID).Scan(&out).Error
	return out, err
}

// ManagerRoleBadges은 주어진 사용자들의 전체 관리 역할(org_admin/project_admin)을 조회한다.
// 멀티롤 배지용 — 한 사용자가 여러 그룹/조직에서 갖는 역할을 모두 보여주기 위함.
func (r *gormRepo) ManagerRoleBadges(userIDs []int64) (map[int64][]RoleBadge, error) {
	out := make(map[int64][]RoleBadge)
	if len(userIDs) == 0 {
		return out, nil
	}
	type row struct {
		UserID    int64
		Role      string
		GroupID   int64
		GroupName string
		OrgName   string
	}
	var rows []row
	err := r.db.Raw(`
		SELECT m.user_id, m.role, g.id AS group_id, g.display_name AS group_name, o.display_name AS org_name
		FROM memberships m JOIN `+"`groups`"+` g ON g.id = m.group_id JOIN organizations o ON o.id = g.org_id
		WHERE m.user_id IN (?) AND m.status = 'active' AND m.role IN ('org_admin','project_admin')
		ORDER BY FIELD(m.role,'org_admin','project_admin'), g.id`, userIDs).Scan(&rows).Error
	if err != nil {
		return out, err
	}
	for _, x := range rows {
		out[x.UserID] = append(out[x.UserID], RoleBadge{Role: x.Role, GroupID: x.GroupID, GroupName: x.GroupName, OrgName: x.OrgName})
	}
	return out, nil
}

func (r *gormRepo) UpsertMembership(groupID, userID int64, role, status string) error {
	return r.db.Exec(`
		INSERT INTO memberships (group_id, user_id, role, status) VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE role = VALUES(role), status = VALUES(status)`,
		groupID, userID, role, status).Error
}

func (r *gormRepo) RemoveMember(groupID, userID int64) error {
	return r.db.Exec(`UPDATE memberships SET status='removed' WHERE group_id=? AND user_id=?`,
		groupID, userID).Error
}

// MoveMember는 사용자를 from 그룹에서 to 그룹으로 옮긴다(한 트랜잭션).
// 원본 멤버십은 status='removed' 로 남겨 이력(consumed)을 보존하고, 대상 그룹에 활성 멤버십을 만든다.
// budget 은 그룹 풀에 종속이라 따라가지 않는다(대상 그룹에서 다시 배정).
// 다른 그룹의 멤버십은 건드리지 않는다 — 지정한 from 에서만 옮긴다.
func (r *gormRepo) MoveMember(fromGroupID, toGroupID, userID int64, role string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Exec(`UPDATE memberships SET status='removed' WHERE group_id=? AND user_id=? AND status='active'`,
			fromGroupID, userID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound // from 그룹의 활성 멤버가 아님 — 이동할 대상이 없다.
		}
		return tx.Exec(`
			INSERT INTO memberships (group_id, user_id, role, status) VALUES (?, ?, ?, 'active')
			ON DUPLICATE KEY UPDATE role = VALUES(role), status = 'active'`,
			toGroupID, userID, role).Error
	})
}

func (r *gormRepo) SetMemberRole(groupID, userID int64, role string) error {
	return r.db.Exec(`UPDATE memberships SET role=? WHERE group_id=? AND user_id=?`,
		role, groupID, userID).Error
}

func (r *gormRepo) SetMemberBudget(groupID, userID int64, budget int) error {
	return r.db.Exec(`UPDATE memberships SET budget=? WHERE group_id=? AND user_id=?`,
		budget, groupID, userID).Error
}

// Usage는 멤버별 소모(consumed) 집계를 반환한다(byUser).
func (r *gormRepo) Usage(groupID int64) ([]UsageRow, error) {
	var out []UsageRow
	err := r.db.Raw(`
		SELECT u.username, TRIM(CONCAT(COALESCE(u.last_name,''),COALESCE(u.first_name,''))) AS name,
		       m.consumed AS credit
		FROM memberships m JOIN users u ON u.id = m.user_id
		WHERE m.group_id = ? AND m.status = 'active' ORDER BY m.consumed DESC`, groupID).Scan(&out).Error
	return out, err
}

// UsageTrend는 그룹의 최근 days일 일자별 GPU 사용시간(시간) 추이(gpu_usage 원장).
func (r *gormRepo) UsageTrend(groupID int64, days int) []UsageTrendPoint {
	var out []UsageTrendPoint
	r.db.Raw(`SELECT DATE_FORMAT(created_at,'%m/%d') AS date, CAST(ROUND(SUM(seconds)/3600) AS SIGNED) AS hours
		FROM gpu_usage WHERE group_id = ? AND created_at >= DATE_SUB(CURDATE(), INTERVAL ? DAY)
		GROUP BY DATE_FORMAT(created_at,'%m/%d') ORDER BY MIN(created_at)`, groupID, days).Scan(&out)
	return out
}

// UsageBySession은 그룹의 세션별 GPU 사용시간(시간)을 반환한다(세션 삭제돼도 원장 유지, 상위 50).
func (r *gormRepo) UsageBySession(groupID int64) []SessionUsage {
	var out []SessionUsage
	r.db.Raw(`SELECT COALESCE(s.name, g.session_ref) AS session,
		       TRIM(CONCAT(COALESCE(u.last_name,''),COALESCE(u.first_name,''))) AS user,
		       CAST(ROUND(SUM(g.seconds)/3600) AS SIGNED) AS gpu_hours
		FROM gpu_usage g
		LEFT JOIN sessions s ON s.instance_id = g.session_ref
		LEFT JOIN users u ON u.id = g.user_id
		WHERE g.group_id = ?
		GROUP BY g.session_ref, s.name, u.last_name, u.first_name
		ORDER BY gpu_hours DESC LIMIT 50`, groupID).Scan(&out)
	return out
}

func (r *gormRepo) GetAcceptsJoin(groupID int64) (bool, error) {
	var v bool
	err := r.db.Raw(`SELECT accepts_join FROM `+"`groups`"+` WHERE id = ?`, groupID).Scan(&v).Error
	return v, err
}

func (r *gormRepo) SetAcceptsJoin(groupID int64, v bool) error {
	return r.db.Exec(`UPDATE `+"`groups`"+` SET accepts_join=? WHERE id=?`, v, groupID).Error
}

func (r *gormRepo) ListJoinRequests(groupID int64, status string) ([]JoinRequest, error) {
	var out []JoinRequest
	err := r.db.Raw(`
		SELECT j.id, j.user_id, TRIM(CONCAT(COALESCE(u.last_name,''), COALESCE(u.first_name,''))) AS name,
		       u.username, u.email, j.status, j.requested_at
		FROM group_join_requests j JOIN users u ON u.id = j.user_id
		WHERE j.group_id = ? AND j.status = ? ORDER BY j.id`, groupID, status).Scan(&out).Error
	return out, err
}

// ApproveJoin은 트랜잭션으로 신청을 승인하고 active 멤버십을 만든다.
func (r *gormRepo) ApproveJoin(reqID int64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var req struct {
			UserID  int64
			GroupID int64
		}
		if err := tx.Raw(`SELECT user_id, group_id FROM group_join_requests WHERE id = ? AND status = 'pending'`,
			reqID).Scan(&req).Error; err != nil {
			return err
		}
		if req.UserID == 0 {
			return ErrNotFound
		}
		if err := tx.Exec(`
			INSERT INTO memberships (group_id, user_id, role, status) VALUES (?, ?, 'member', 'active')
			ON DUPLICATE KEY UPDATE status='active'`, req.GroupID, req.UserID).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE group_join_requests SET status='approved', reviewed_at=NOW() WHERE id=?`, reqID).Error
	})
}

func (r *gormRepo) RejectJoin(reqID int64) error {
	return r.db.Exec(`UPDATE group_join_requests SET status='rejected', reviewed_at=NOW() WHERE id=? AND status='pending'`,
		reqID).Error
}

// PrimaryMembership은 사용자의 최고 권한 멤버십(+그룹/조직)을 반환한다.
func (r *gormRepo) PrimaryMembership(userID int64) (*PrimaryMembership, error) {
	var p PrimaryMembership
	err := r.db.Raw(`
		SELECT m.role, g.id AS group_id, g.display_name AS group_name, o.id AS org_id, o.display_name AS org_name
		FROM memberships m JOIN `+"`groups`"+` g ON g.id = m.group_id JOIN organizations o ON o.id = g.org_id
		WHERE m.user_id = ? AND m.status = 'active'
		ORDER BY FIELD(m.role,'org_admin','project_admin','billing_admin','member','guest') LIMIT 1`,
		userID).Scan(&p).Error
	if err != nil {
		return nil, err
	}
	if p.Role == "" {
		return nil, ErrNotFound
	}
	return &p, nil
}

// ManagerMemberships은 사용자의 모든 관리(org_admin/project_admin) 멤버십을 우선순위 순으로 반환한다.
// PrimaryMembership 과 달리 LIMIT 1 이 없다 — 멀티롤(조직+그룹 동시 관리) 전환기용.
func (r *gormRepo) ManagerMemberships(userID int64) ([]PrimaryMembership, error) {
	var out []PrimaryMembership
	err := r.db.Raw(`
		SELECT m.role, g.id AS group_id, g.display_name AS group_name, o.id AS org_id, o.display_name AS org_name
		FROM memberships m JOIN `+"`groups`"+` g ON g.id = m.group_id JOIN organizations o ON o.id = g.org_id
		WHERE m.user_id = ? AND m.status = 'active' AND m.role IN ('org_admin','project_admin')
		ORDER BY FIELD(m.role,'org_admin','project_admin'), o.id, g.id`,
		userID).Scan(&out).Error
	return out, err
}

func (r *gormRepo) MyGroups(userID int64) ([]GroupRef, error) {
	var out []GroupRef
	err := r.db.Raw(`
		SELECT g.id, g.name, g.display_name, g.org_id, o.display_name AS org_name
		FROM memberships m JOIN `+"`groups`"+` g ON g.id = m.group_id JOIN organizations o ON o.id = g.org_id
		WHERE m.user_id = ? AND m.status = 'active' ORDER BY g.id`, userID).Scan(&out).Error
	return out, err
}

func (r *gormRepo) Directory(userID int64) ([]DirItem, error) {
	var out []DirItem
	err := r.db.Raw(`
		SELECT g.id, g.name, g.display_name, g.org_id, o.display_name AS org_name, g.accepts_join,
		       EXISTS(SELECT 1 FROM memberships m WHERE m.group_id = g.id AND m.user_id = ? AND m.status='active') AS joined
		FROM `+"`groups`"+` g JOIN organizations o ON o.id = g.org_id
		WHERE g.status = 'active' ORDER BY o.id, g.id`, userID).Scan(&out).Error
	return out, err
}

func (r *gormRepo) MyJoinRequests(userID int64) ([]MyJoinRequest, error) {
	var out []MyJoinRequest
	err := r.db.Raw(`
		SELECT j.id, j.group_id, g.display_name AS group_name, o.display_name AS org_name, j.status, j.requested_at
		FROM group_join_requests j JOIN `+"`groups`"+` g ON g.id = j.group_id JOIN organizations o ON o.id = g.org_id
		WHERE j.user_id = ? ORDER BY j.id DESC`, userID).Scan(&out).Error
	return out, err
}

func (r *gormRepo) CreateJoinRequest(userID, groupID int64) error {
	return r.db.Exec(
		`INSERT INTO group_join_requests (user_id, group_id, status) VALUES (?, ?, 'pending')`,
		userID, groupID).Error
}

func (r *gormRepo) ResolveUserID(account string) (int64, error) {
	var id int64
	err := r.db.Raw(`SELECT id FROM users WHERE username = ? OR email = ? LIMIT 1`, account, account).Scan(&id).Error
	if err != nil {
		return 0, err
	}
	if id == 0 {
		return 0, ErrNotFound
	}
	return id, nil
}
