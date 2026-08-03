package policy

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

var errUnknownLevel = errors.New("unknown policy level")

type gormRepo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

// limitRow는 nullable 제한 컬럼을 그대로 받는다(미지정=NULL→nil).
type limitRow struct {
	MaxGpu                *int `gorm:"column:max_gpu"`
	MaxVramGB             *int `gorm:"column:max_vram_gb"`
	MaxVolumeGiB          *int `gorm:"column:max_volume_gib"`
	MaxConcurrentSessions *int `gorm:"column:max_concurrent_sessions"`
	MaxEphemeralGiB       *int `gorm:"column:max_ephemeral_gib"`
}

func (l limitRow) toLimits() Limits {
	return Limits{MaxGpu: l.MaxGpu, MaxVramGB: l.MaxVramGB, MaxVolumeGiB: l.MaxVolumeGiB, MaxConcurrentSessions: l.MaxConcurrentSessions, MaxEphemeralGiB: l.MaxEphemeralGiB}
}

func (r *gormRepo) UserLimits(userID int64) (Limits, error) {
	return r.fetch("users", "id", userID)
}

func (r *gormRepo) GroupLimits(groupID int64) (Limits, error) {
	return r.fetch("`groups`", "id", groupID)
}

func (r *gormRepo) OrgLimits(orgID int64) (Limits, error) {
	return r.fetch("organizations", "id", orgID)
}

func (r *gormRepo) fetch(table, idCol string, id int64) (Limits, error) {
	var row limitRow
	err := r.db.Raw(
		"SELECT max_gpu, max_vram_gb, max_volume_gib, max_concurrent_sessions, max_ephemeral_gib FROM "+table+" WHERE "+idCol+"=?",
		id,
	).Scan(&row).Error
	return row.toLimits(), err
}

// SetLimits는 한 엔티티의 4개 하드 제한 컬럼을 통째로 설정한다(nil=NULL로 명시 초기화).
func (r *gormRepo) SetLimits(level Level, id int64, l Limits) error {
	table, ok := tableOf(level)
	if !ok {
		return errUnknownLevel
	}
	return r.db.Exec(
		"UPDATE "+table+" SET max_gpu=?, max_vram_gb=?, max_volume_gib=?, max_concurrent_sessions=?, max_ephemeral_gib=? WHERE id=?",
		l.MaxGpu, l.MaxVramGB, l.MaxVolumeGiB, l.MaxConcurrentSessions, l.MaxEphemeralGiB, id,
	).Error
}

// ListPolicies는 상한이 하나라도 설정된 모든 범위(개인/그룹/조직)를 한 목록으로 반환한다.
// "정책" 화면이 범위별로 흩어보지 않고 한눈에 보게 하는 것이 목적.
func (r *gormRepo) ListPolicies() ([]PolicyRow, error) {
	const cols = "max_gpu, max_vram_gb, max_volume_gib, max_concurrent_sessions, max_ephemeral_gib"
	const anySet = "(max_gpu IS NOT NULL OR max_vram_gb IS NOT NULL OR max_volume_gib IS NOT NULL OR max_concurrent_sessions IS NOT NULL OR max_ephemeral_gib IS NOT NULL)"
	q := `
		SELECT 'org' AS scope, id, display_name AS name, ` + cols + ` FROM organizations WHERE ` + anySet + `
		UNION ALL
		SELECT 'group' AS scope, id, display_name AS name, ` + cols + " FROM `groups` WHERE " + anySet + `
		UNION ALL
		SELECT 'user' AS scope, id,
		       COALESCE(NULLIF(TRIM(CONCAT(COALESCE(last_name,''),COALESCE(first_name,''))),''), username) AS name,
		       ` + cols + ` FROM users WHERE ` + anySet + `
		ORDER BY FIELD(scope,'org','group','user'), name`
	var rows []PolicyRow
	err := r.db.Raw(q).Scan(&rows).Error
	return rows, err
}

// ListPoliciesScoped는 호출자 스코프의 상한 목록만 반환한다.
// org: 자기 조직 + 산하 그룹 + 조직 소속 사용자. group: 자기 그룹 + 멤버.
func (r *gormRepo) ListPoliciesScoped(orgID, groupID int64) ([]PolicyRow, error) {
	const cols = "max_gpu, max_vram_gb, max_volume_gib, max_concurrent_sessions, max_ephemeral_gib"
	const anySet = "(max_gpu IS NOT NULL OR max_vram_gb IS NOT NULL OR max_volume_gib IS NOT NULL OR max_concurrent_sessions IS NOT NULL OR max_ephemeral_gib IS NOT NULL)"
	const uname = "COALESCE(NULLIF(TRIM(CONCAT(COALESCE(last_name,''),COALESCE(first_name,''))),''), username)"
	var parts []string
	var args []any
	if groupID > 0 {
		parts = append(parts, "SELECT 'group' AS scope, id, display_name AS name, "+cols+" FROM `groups` WHERE "+anySet+" AND id=?")
		args = append(args, groupID)
		parts = append(parts, "SELECT 'user' AS scope, id, "+uname+" AS name, "+cols+
			" FROM users WHERE "+anySet+" AND id IN (SELECT user_id FROM memberships WHERE group_id=? AND status='active')")
		args = append(args, groupID)
	} else if orgID > 0 {
		parts = append(parts, "SELECT 'org' AS scope, id, display_name AS name, "+cols+" FROM organizations WHERE "+anySet+" AND id=?")
		args = append(args, orgID)
		parts = append(parts, "SELECT 'group' AS scope, id, display_name AS name, "+cols+" FROM `groups` WHERE "+anySet+" AND org_id=?")
		args = append(args, orgID)
		parts = append(parts, "SELECT 'user' AS scope, id, "+uname+" AS name, "+cols+
			" FROM users WHERE "+anySet+" AND id IN (SELECT m.user_id FROM memberships m JOIN `groups` g ON g.id=m.group_id WHERE g.org_id=? AND m.status='active')")
		args = append(args, orgID)
	} else {
		return r.ListPolicies() // platform
	}
	q := strings.Join(parts, " UNION ALL ") + " ORDER BY FIELD(scope,'org','group','user'), name"
	var rows []PolicyRow
	err := r.db.Raw(q, args...).Scan(&rows).Error
	return rows, err
}

func tableOf(level Level) (string, bool) {
	switch level {
	case LevelUser:
		return "users", true
	case LevelGroup:
		return "`groups`", true
	case LevelOrg:
		return "organizations", true
	}
	return "", false
}
