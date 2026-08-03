// Package users는 플랫폼 관리자용 사용자 관리(목록/상태/생성)다. users 테이블 공용.
package users

import (
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Summary는 관리자 사용자 목록 행(소속 조직/그룹 조인 포함).
type Summary struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Role     string `json:"role"` // 플랫폼 역할: member|admin
	Status   string `json:"status"`
	Advisor  string `json:"advisor"`
	Group    string `json:"group"`
	GroupID  int64  `json:"groupId"`
	Org      string `json:"org"`
	// MemberRole 은 소속 그룹에서의 역할(org_admin|project_admin|billing_admin|member|guest).
	// 플랫폼 역할(Role)과 별개 — 조직 관리자/팀장은 여기에만 나타난다.
	MemberRole         string      `json:"memberRole"`
	Roles              []RoleBadge `json:"roles" gorm:"-"` // 이 사용자가 가진 전체 관리 역할(멀티롤 배지). 서비스에서 채움.
	Balance            int         `json:"balance"`            // user_wallets.balance(현재 잔여)
	RecurringCredit    int         `json:"recurringCredit"`    // 주기당 리필 크레딧
	RefillIntervalDays int         `json:"refillIntervalDays"` // 리필 주기(일)
	CreatedAt          time.Time   `json:"createdAt"`
}

// RoleBadge — 사용자가 가진 관리 역할 하나(멀티롤 표시용).
type RoleBadge struct {
	Role      string `json:"role"` // org_admin | project_admin
	GroupID   int64  `json:"groupId"`
	GroupName string `json:"groupName"`
	OrgName   string `json:"orgName"`
}

// CreateReq — 관리자 사용자 생성.
type CreateReq struct {
	Username  string `json:"username" binding:"required"`
	Password  string `json:"password"`
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Role      string `json:"role"`
	GroupID   *int64 `json:"groupId"`
}

type StatusReq struct {
	Status string `json:"status" binding:"required"` // approved|rejected|suspended|pending
}

type BulkReq struct {
	Users []CreateReq `json:"users"`
}

const defaultPassword = "giosk123" // 관리자 생성 시 비번 미지정 기본값(최초 로그인 후 변경 권장)

// ListFilter — 사용자 목록 조회 조건(서버측 검색/페이징).
// 목록이 수천 명이 되면 전부 내려주고 브라우저가 거르는 방식은 못 버틴다(300명에 3.3초).
type ListFilter struct {
	Q       string // 이름/아이디/이메일 부분일치
	Status  string // approved|pending|suspended|rejected ("" = 전체)
	Group   string // 그룹 표시명 ("" = 전체)
	Org     string // 조직 표시명 ("" = 전체)
	OrgID   int64  // 소속 조직 id 로 필터(스코프 인가용, 표시명보다 견고). 0 = 미적용.
	GroupID int64  // 소속 그룹 id 로 필터. 0 = 미적용.
	UserID  int64  // 단일 사용자 id 로 필터(상세 조회용). 0 = 미적용.
	Limit   int    // 0 = 전체(내부용). 핸들러는 항상 값을 준다.
	Offset  int
}

// Repository — 영속성.
type Repository interface {
	List(f ListFilter) (items []Summary, total int, err error)
	ByID(id int64) (*Summary, error)
	SetStatus(id int64, status string) error
	Create(req CreateReq) (int64, error)
}

type repo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &repo{db: db} }

func (r *repo) List(f ListFilter) ([]Summary, int, error) {
	var out []Summary
	// 대표 멤버십을 한 번만 고르고(역할 우선순위) 그룹·조직·역할을 모두 거기서 가져온다.
	// (예전엔 그룹/조직을 각각 독립 LIMIT 1 서브쿼리로 뽑아, 다중 소속 시 서로 다른 멤버십을
	//  가리킬 수 있었다. 이제 한 행에서 일관되게 나온다.)
	const base = `
		SELECT u.id, u.username,
		       TRIM(CONCAT(COALESCE(u.last_name,''), COALESCE(u.first_name,''))) AS name,
		       COALESCE(u.email,'') AS email, u.role, u.status,
		       COALESCE(u.sponsor_name,'') AS advisor, u.created_at,
		       COALESCE(g.display_name,'') AS ` + "`group`" + `,
		       COALESCE(g.id,0) AS group_id,
		       COALESCE(o.display_name,'') AS org,
		       COALESCE(m.role,'') AS member_role,
		       COALESCE(uw.balance,0) AS balance,
		       COALESCE(uw.recurring_credit,0) AS recurring_credit,
		       COALESCE(uw.refill_interval_days,0) AS refill_interval_days
		FROM users u
		LEFT JOIN memberships m ON m.id = (
		    SELECT m2.id FROM memberships m2
		    WHERE m2.user_id=u.id AND m2.status='active'
		    ORDER BY FIELD(m2.role,'org_admin','project_admin','billing_admin','member','guest') LIMIT 1)
		LEFT JOIN ` + "`groups`" + ` g ON g.id=m.group_id
		LEFT JOIN organizations o ON o.id=g.org_id
		LEFT JOIN user_wallets uw ON uw.user_id=u.id`

	// 조건은 조인 이후에 걸어야 그룹/조직 표시명으로 거를 수 있다(대표 멤버십 기준).
	where := []string{}
	args := []any{}
	if q := strings.TrimSpace(f.Q); q != "" {
		like := "%" + q + "%"
		where = append(where, "(u.username LIKE ? OR u.email LIKE ? OR TRIM(CONCAT(COALESCE(u.last_name,''), COALESCE(u.first_name,''))) LIKE ?)")
		args = append(args, like, like, like)
	}
	if f.Status != "" {
		where = append(where, "u.status = ?")
		args = append(args, f.Status)
	}
	if f.Group != "" {
		where = append(where, "g.display_name = ?")
		args = append(args, f.Group)
	}
	if f.Org != "" {
		where = append(where, "o.display_name = ?")
		args = append(args, f.Org)
	}
	if f.OrgID > 0 {
		where = append(where, "g.org_id = ?")
		args = append(args, f.OrgID)
	}
	if f.GroupID > 0 {
		where = append(where, "g.id = ?")
		args = append(args, f.GroupID)
	}
	if f.UserID > 0 {
		where = append(where, "u.id = ?")
		args = append(args, f.UserID)
	}
	cond := ""
	if len(where) > 0 {
		cond = " WHERE " + strings.Join(where, " AND ")
	}

	var total int64
	if err := r.db.Raw("SELECT COUNT(*) FROM ("+base+cond+") t", args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	q := base + cond + " ORDER BY u.id"
	if f.Limit > 0 {
		q += " LIMIT ? OFFSET ?"
		args = append(args, f.Limit, f.Offset)
	}
	if err := r.db.Raw(q, args...).Scan(&out).Error; err != nil {
		return nil, 0, err
	}
	r.attachRoleBadges(out) // 멀티롤 배지 — 각 사용자의 전체 관리 역할
	return out, int(total), nil
}

// attachRoleBadges은 목록의 각 사용자에게 전체 관리 역할(org_admin/project_admin) 배지를 채운다.
func (r *repo) attachRoleBadges(out []Summary) {
	if len(out) == 0 {
		return
	}
	ids := make([]int64, 0, len(out))
	for _, s := range out {
		ids = append(ids, s.ID)
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
		WHERE m.user_id IN (?) AND m.status='active' AND m.role IN ('org_admin','project_admin')
		ORDER BY FIELD(m.role,'org_admin','project_admin'), g.id`, ids).Scan(&rows).Error
	if err != nil {
		return
	}
	byUser := map[int64][]RoleBadge{}
	for _, x := range rows {
		byUser[x.UserID] = append(byUser[x.UserID], RoleBadge{Role: x.Role, GroupID: x.GroupID, GroupName: x.GroupName, OrgName: x.OrgName})
	}
	for i := range out {
		out[i].Roles = byUser[out[i].ID]
	}
}

// ByID는 단일 사용자의 Summary(그룹/조직/멀티롤 배지 포함)를 반환한다. 없으면 (nil, nil).
func (r *repo) ByID(id int64) (*Summary, error) {
	items, _, err := r.List(ListFilter{UserID: id, Limit: 1})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	return &items[0], nil
}

func (r *repo) SetStatus(id int64, status string) error {
	if status != "approved" {
		return r.db.Exec(`UPDATE users SET status=? WHERE id=?`, status, id).Error
	}
	// 승인 시: 가입신청에서 고른 그룹(signup_group_id)이 있으면 active 멤버십을 만들고 비운다.
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`UPDATE users SET status='approved' WHERE id=?`, id).Error; err != nil {
			return err
		}
		var gid *int64
		tx.Raw(`SELECT signup_group_id FROM users WHERE id=?`, id).Scan(&gid)
		if gid != nil && *gid > 0 {
			if err := tx.Exec(`INSERT INTO memberships (group_id, user_id, role, status) VALUES (?, ?, 'member', 'active')
				ON DUPLICATE KEY UPDATE status='active'`, *gid, id).Error; err != nil {
				return err
			}
			// 자체가입은 group_join_requests(pending)도 함께 남긴다. 여기서 active 멤버십을 만들면서
			// 그 신청도 approved 로 해소하지 않으면 "내 가입 신청 현황"이 영구 "대기 중"으로 남는다.
			tx.Exec(`UPDATE group_join_requests SET status='approved' WHERE user_id=? AND group_id=? AND status='pending'`, id, *gid)
			tx.Exec(`UPDATE users SET signup_group_id=NULL WHERE id=?`, id)
		}
		return nil
	})
}

// Create는 승인된 사용자를 만들고(비번 해시), groupId 가 있으면 active 멤버십도 생성한다.
func (r *repo) Create(req CreateReq) (int64, error) {
	pw := req.Password
	if pw == "" {
		pw = defaultPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}
	role := req.Role
	if role == "" {
		role = "member"
	}
	var id int64
	err = r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Exec(`INSERT INTO users (username, password_hash, email, first_name, last_name, role, status)
			VALUES (?, ?, ?, ?, ?, ?, 'approved')`, req.Username, string(hash), req.Email, req.FirstName, req.LastName, role)
		if res.Error != nil {
			return res.Error
		}
		if e := tx.Raw(`SELECT id FROM users WHERE username=?`, req.Username).Scan(&id).Error; e != nil {
			return e
		}
		if req.GroupID != nil {
			return tx.Exec(`INSERT INTO memberships (group_id, user_id, role, status) VALUES (?, ?, 'member', 'active')`,
				*req.GroupID, id).Error
		}
		return nil
	})
	return id, err
}
