package dashboard

import (
	"time"

	"gorm.io/gorm"
)

// Repository는 대시보드 집계(읽기 전용 프로젝션) 계약.
type Repository interface {
	UserWallet(userID int64) (balance, cap int)
	UserActiveSessions(userID int64) int
	ActiveSessions() int
	NameCredits(scope string, limit int) []NameCredit  // scope: group | org
	OfferingCapacity() []RegionRow                     // 오퍼링별 정의(가용량은 서비스에서 보정)
	UserVramTotalMB(userID int64) int                  // running 세션 VRAM 합(MB)
	UserRunningGpuPods(userID int64) []string          // running GPU 세션 instance_id(VRAM 사용량 DCGM 쿼리용)
	RunningByGpuType() map[string]int                  // gpu_type → running 세션 수
	UserSessions(userID int64, limit int) []SessionRow // 최근 세션
	MonthlyGpuHours(userID int64) int                  // 이번달 GPU 사용 시간(소비크레딧÷단가)
	MonthConsumed() int                                // 이번달 전체 소비 크레딧(관리자)
	BudgetRiskGroups() int                             // 예산 경보 임계 초과 그룹 수
	CreditTrend(days int) []TrendPoint                 // 일자별 전체 소비 추이(관리자)
	UserConsume7d(userID int64) int                    // 최근 7일 소비(번레이트 산출용)
	// 운영 대시보드 스코프 변형(orgID/groupID 둘 다 0=platform 전체).
	ActiveSessionsScoped(orgID, groupID int64) int
	MonthConsumedScoped(orgID, groupID int64) int
	CreditTrendScoped(days int, orgID, groupID int64) []TrendPoint
	BudgetRiskScoped(orgID, groupID int64) int
	NameCreditsScoped(kind string, orgID, groupID int64, limit int) []NameCredit
	// 인프라 메트릭 스냅샷(샘플러 적재 + 대시보드 추이).
	InsertSnapshot(s Snapshot) error
	SnapshotsSince(since time.Time) []Snapshot
	LatestSnapshot() (Snapshot, bool)
	RunningGpuCount() int               // 점유 GPU 수(running 세션, cpu 제외)
	SessionPhaseCounts() map[string]int // phase별 세션 수(클러스터 전체)
	RunningByMode() map[string]int      // gpu_mode별 running(클러스터 전체)
	// 운영 대시보드 세션 통계(스코프). orgID/groupID 둘 다 0=전체.
	PhaseCountsScoped(orgID, groupID int64) map[string]int
	RunningByModeScoped(orgID, groupID int64) map[string]int
	RunningByGpuTypeScoped(orgID, groupID int64) map[string]int
	GpuHoursScoped(orgID, groupID int64) int // 이번달 GPU 사용시간(스코프)
	ActiveUsers(limit int) []ActiveUser
	ActiveUsersScoped(orgID, groupID int64, limit int) []ActiveUser
}

// SessionRow — 세션 카드 원천(대시보드 전용 프로젝션).
type SessionRow struct {
	InstanceID   string
	Name         string
	Env          string
	Node         string
	GpuMode      string
	GpuType      string
	GpuCount     int
	VramMB       int
	CorePercent  int
	CPUCores     int
	MemGB        int
	Phase        string
	PricePerHour int
	Channels     string // images.channels JSON(LEFT JOIN) — 접속 채널 도출
}

// NameCredit — top 그룹/사용자 잔액.
type NameCredit struct {
	Name   string `json:"name"`
	Credit int    `json:"credit"`
}

// RegionRow — 오퍼링(자원군) 정의.
type RegionRow struct {
	Name    string
	GpuType string
}

type gormRepo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

// UserWallet은 (잔액, 전체) 를 반환한다. 전체 = 현재 잔액 + 누적 소모(= 지금까지 부여받은 총 크레딧).
// monthly_cap 은 별도 한도 개념이라 대시보드의 "전체/남은" 표시엔 쓰지 않는다.
func (r *gormRepo) UserWallet(userID int64) (balance, total int) {
	r.db.Raw(`SELECT COALESCE(SUM(balance),0) FROM user_wallets WHERE user_id = ?`, userID).Scan(&balance)
	var consumed int
	r.db.Raw(`SELECT COALESCE(SUM(-amount),0) FROM credit_transactions WHERE user_id = ? AND type = 'consume'`, userID).Scan(&consumed)
	return balance, balance + consumed
}

func (r *gormRepo) UserActiveSessions(userID int64) int {
	var n int64
	r.db.Raw(`SELECT COUNT(*) FROM sessions WHERE user_id = ? AND phase = 'running'`, userID).Scan(&n)
	return int(n)
}

func (r *gormRepo) ActiveSessions() int {
	var n int64
	r.db.Raw(`SELECT COUNT(*) FROM sessions WHERE phase = 'running'`).Scan(&n)
	return int(n)
}

// userScopeClause는 스코프(orgID/groupID)에 속한 사용자로 한정하는 SQL 조각과 인자를 만든다.
// col = 대상 테이블의 user_id 컬럼명(예: "user_id", "s.user_id"). platform(둘 다 0)이면 항상참("1=1").
func userScopeClause(col string, orgID, groupID int64) (string, []any) {
	switch {
	case groupID > 0:
		return col + " IN (SELECT user_id FROM memberships WHERE group_id=? AND status='active')", []any{groupID}
	case orgID > 0:
		return col + " IN (SELECT m.user_id FROM memberships m JOIN `groups` g ON g.id=m.group_id WHERE g.org_id=? AND m.status='active')", []any{orgID}
	default:
		return "1=1", nil
	}
}

// ActiveSessionsScoped는 스코프 내 사용자의 running 세션 수.
func (r *gormRepo) ActiveSessionsScoped(orgID, groupID int64) int {
	cl, args := userScopeClause("user_id", orgID, groupID)
	var n int64
	r.db.Raw(`SELECT COUNT(*) FROM sessions WHERE phase='running' AND `+cl, args...).Scan(&n)
	return int(n)
}

// MonthConsumedScoped는 이번달 스코프 내 사용자 소비 크레딧 합.
func (r *gormRepo) MonthConsumedScoped(orgID, groupID int64) int {
	cl, args := userScopeClause("user_id", orgID, groupID)
	var n int
	r.db.Raw(`SELECT COALESCE(SUM(-amount),0) FROM credit_transactions
		WHERE type='consume' AND created_at >= DATE_FORMAT(CURDATE(),'%Y-%m-01') AND `+cl, args...).Scan(&n)
	return n
}

// CreditTrendScoped는 최근 days일 스코프 내 소비 추이.
func (r *gormRepo) CreditTrendScoped(days int, orgID, groupID int64) []TrendPoint {
	cl, args := userScopeClause("user_id", orgID, groupID)
	out := []TrendPoint{}
	q := `SELECT DATE_FORMAT(created_at,'%m/%d') AS date, CAST(COALESCE(SUM(-amount),0) AS SIGNED) AS amount
		FROM credit_transactions
		WHERE type='consume' AND created_at >= DATE_SUB(CURDATE(), INTERVAL ? DAY) AND ` + cl + `
		GROUP BY DATE_FORMAT(created_at,'%m/%d') ORDER BY MIN(created_at)`
	r.db.Raw(q, append([]any{days}, args...)...).Scan(&out)
	return out
}

// BudgetRiskScoped는 스코프 내 풀 소진(잔여≤0) 활성 그룹 수.
func (r *gormRepo) BudgetRiskScoped(orgID, groupID int64) int {
	where := "w.balance <= 0 AND g.status='active' AND EXISTS (SELECT 1 FROM memberships m WHERE m.group_id=w.group_id AND m.status='active')"
	args := []any{}
	if groupID > 0 {
		where += " AND g.id=?"
		args = append(args, groupID)
	} else if orgID > 0 {
		where += " AND g.org_id=?"
		args = append(args, orgID)
	}
	var n int
	r.db.Raw(`SELECT COUNT(*) FROM group_wallets w JOIN `+"`groups`"+` g ON g.id=w.group_id WHERE `+where, args...).Scan(&n)
	return n
}

// NameCreditsScoped는 스코프 내 상위 그룹/사용자 잔액.
// scope="group": org=산하 그룹 / group=자기 그룹. scope="user": org=조직 소속 사용자 / group=그룹 멤버.
func (r *gormRepo) NameCreditsScoped(kind string, orgID, groupID int64, limit int) []NameCredit {
	out := []NameCredit{}
	if kind == "group" {
		where, args := "g.status='active'", []any{}
		if groupID > 0 {
			where += " AND g.id=?"
			args = append(args, groupID)
		} else if orgID > 0 {
			where += " AND g.org_id=?"
			args = append(args, orgID)
		}
		// "상위 그룹" = 소비(누적 사용 크레딧) 순. 잔액이 아니라 memberships.consumed 합계.
		r.db.Raw(`SELECT g.display_name AS name, COALESCE(SUM(m.consumed),0) AS credit
			FROM `+"`groups`"+` g LEFT JOIN memberships m ON m.group_id=g.id
			WHERE `+where+` GROUP BY g.id, g.display_name ORDER BY credit DESC LIMIT ?`, append(args, limit)...).Scan(&out)
		return out
	}
	cl, args := userScopeClause("m.user_id", orgID, groupID)
	// "상위 사용자" = 소비 순. 멤버십별 consumed 를 유저 단위로 합산.
	r.db.Raw(`SELECT u.username AS name, COALESCE(SUM(m.consumed),0) AS credit
		FROM memberships m JOIN users u ON u.id=m.user_id
		WHERE `+cl+` GROUP BY u.id, u.username ORDER BY credit DESC LIMIT ?`, append(args, limit)...).Scan(&out)
	return out
}

func (r *gormRepo) NameCredits(scope string, limit int) []NameCredit {
	var out []NameCredit
	switch scope {
	case "group":
		// 소비 순(잔액 아님). memberships.consumed 합계.
		r.db.Raw(`SELECT g.display_name AS name, COALESCE(SUM(m.consumed),0) AS credit
			FROM `+"`groups`"+` g LEFT JOIN memberships m ON m.group_id = g.id
			WHERE g.status='active' GROUP BY g.id, g.display_name ORDER BY credit DESC LIMIT ?`, limit).Scan(&out)
	case "org":
		r.db.Raw(`SELECT u.username AS name, COALESCE(SUM(m.consumed),0) AS credit
			FROM memberships m JOIN users u ON u.id = m.user_id
			GROUP BY u.id, u.username ORDER BY credit DESC LIMIT ?`, limit).Scan(&out)
	}
	return out
}

func (r *gormRepo) OfferingCapacity() []RegionRow {
	var out []RegionRow
	r.db.Raw(`SELECT name, gpu_type FROM offerings WHERE is_active = 1 ORDER BY id`).Scan(&out)
	return out
}

func (r *gormRepo) UserVramTotalMB(userID int64) int {
	var mb int64
	r.db.Raw(`SELECT COALESCE(SUM(vram_mb),0) FROM sessions WHERE user_id = ? AND phase = 'running'`, userID).Scan(&mb)
	return int(mb)
}

// UserRunningGpuPods는 사용자의 running GPU 세션 instance_id 목록(=DCGM 메트릭의 pod 라벨).
// CPU·물리(SSH) 세션은 GPU VRAM 이 없으므로 제외.
func (r *gormRepo) UserRunningGpuPods(userID int64) []string {
	var out []string
	r.db.Raw(`SELECT instance_id FROM sessions
		WHERE user_id = ? AND phase = 'running' AND env <> 'ssh' AND gpu_mode <> 'cpu' AND instance_id <> ''`,
		userID).Scan(&out)
	return out
}

func (r *gormRepo) UserSessions(userID int64, limit int) []SessionRow {
	var out []SessionRow
	r.db.Raw(`SELECT s.instance_id, s.name, s.env, s.node, s.gpu_mode, s.gpu_type, s.gpu_count,
		s.vram_mb, s.core_percent, s.cpu_cores, s.mem_gb, s.phase, s.price_per_hour,
		COALESCE(i.channels, '') AS channels
		FROM sessions s LEFT JOIN images i ON i.id = s.image_id
		WHERE s.user_id = ? ORDER BY s.id DESC LIMIT ?`, userID, limit).Scan(&out)
	return out
}

// MonthlyGpuHours는 이번달(1일~) GPU 사용 시간(시간)을 gpu_usage 원장에서 합산해 반환한다.
// 원장은 정산 시 적립되므로 세션이 삭제돼도 누적이 유지된다(반올림).
func (r *gormRepo) MonthlyGpuHours(userID int64) int {
	var secs float64
	r.db.Raw(`
		SELECT COALESCE(SUM(seconds), 0)
		FROM gpu_usage
		WHERE user_id = ? AND created_at >= DATE_FORMAT(CURDATE(), '%Y-%m-01')`, userID).Scan(&secs)
	return int(secs/3600.0 + 0.5)
}

// MonthConsumed는 이번달 1일부터 전체 사용자 소비 크레딧 합(관리자 KPI).
func (r *gormRepo) MonthConsumed() int {
	var n int
	r.db.Raw(`SELECT COALESCE(SUM(-amount),0) FROM credit_transactions
		WHERE type='consume' AND created_at >= DATE_FORMAT(CURDATE(),'%Y-%m-01')`).Scan(&n)
	return n
}

// BudgetRiskGroups는 풀이 소진된(잔여 ≤ 0) 활성 그룹 수 — 추가 배분이 필요한 그룹.
// (예산 캡 모델 폐기 후, 계층형 풀 잔여 기준으로 재정의)
func (r *gormRepo) BudgetRiskGroups() int {
	var n int
	r.db.Raw(`SELECT COUNT(*) FROM group_wallets w
		JOIN ` + "`groups`" + ` g ON g.id = w.group_id AND g.status='active'
		WHERE w.balance <= 0
		  AND EXISTS (SELECT 1 FROM memberships m WHERE m.group_id=w.group_id AND m.status='active')`).Scan(&n)
	return n
}

// CreditTrend는 최근 days일 일자별 전체 소비 크레딧 추이(관리자 차트).
func (r *gormRepo) CreditTrend(days int) []TrendPoint {
	var out []TrendPoint
	r.db.Raw(`SELECT DATE_FORMAT(created_at,'%m/%d') AS date, CAST(COALESCE(SUM(-amount),0) AS SIGNED) AS amount
		FROM credit_transactions
		WHERE type='consume' AND created_at >= DATE_SUB(CURDATE(), INTERVAL ? DAY)
		GROUP BY DATE_FORMAT(created_at,'%m/%d') ORDER BY MIN(created_at)`, days).Scan(&out)
	return out
}

// UserConsume7d는 최근 7일 사용자 소비 크레딧 합(일일 번레이트 산출용).
func (r *gormRepo) UserConsume7d(userID int64) int {
	var n int
	r.db.Raw(`SELECT COALESCE(SUM(-amount),0) FROM credit_transactions
		WHERE user_id=? AND type='consume' AND created_at >= DATE_SUB(CURDATE(), INTERVAL 7 DAY)`, userID).Scan(&n)
	return n
}

func (r *gormRepo) RunningByGpuType() map[string]int {
	var rows []struct {
		GpuType string
		N       int
	}
	r.db.Raw(`SELECT gpu_type, COUNT(*) AS n FROM sessions WHERE phase = 'running' GROUP BY gpu_type`).Scan(&rows)
	out := map[string]int{}
	for _, r := range rows {
		out[r.GpuType] = r.N
	}
	return out
}

// ── 인프라 메트릭 스냅샷 ─────────────────────────────────────────────
func (r *gormRepo) InsertSnapshot(s Snapshot) error { return r.db.Create(&s).Error }

// SnapshotsSince는 since 이후 스냅샷을 시간순으로 반환한다(대시보드 추이).
func (r *gormRepo) SnapshotsSince(since time.Time) []Snapshot {
	var out []Snapshot
	r.db.Where("ts >= ?", since).Order("ts").Find(&out)
	return out
}

// RunningGpuCount는 running 세션이 점유한 GPU 수(cpu 제외, 분할은 1로 근사).
func (r *gormRepo) RunningGpuCount() int {
	var n int
	r.db.Raw(`SELECT COALESCE(SUM(CASE WHEN gpu_mode='cpu' THEN 0 ELSE GREATEST(gpu_count,1) END),0)
		FROM sessions WHERE phase='running'`).Scan(&n)
	return n
}

// SessionPhaseCounts는 phase별 세션 수(running/stopped/provisioning/terminated…).
func (r *gormRepo) SessionPhaseCounts() map[string]int {
	return r.groupCount(`SELECT phase AS k, COUNT(*) AS n FROM sessions GROUP BY phase`)
}

// RunningByMode는 running 세션의 gpu_mode별 수(shared/exclusive/cpu).
func (r *gormRepo) RunningByMode() map[string]int {
	return r.groupCount(`SELECT COALESCE(gpu_mode,'') AS k, COUNT(*) AS n FROM sessions WHERE phase='running' GROUP BY gpu_mode`)
}

func (r *gormRepo) groupCount(q string) map[string]int {
	var rows []struct {
		K string
		N int
	}
	r.db.Raw(q).Scan(&rows)
	out := map[string]int{}
	for _, x := range rows {
		out[x.K] = x.N
	}
	return out
}

// LatestSnapshot은 가장 최근 스냅샷을 반환한다(idle 등 순간값을 요청마다 라이브 계산하지 않기 위함).
func (r *gormRepo) LatestSnapshot() (Snapshot, bool) {
	var s Snapshot
	if err := r.db.Order("ts DESC").Limit(1).Find(&s).Error; err != nil || s.TS.IsZero() {
		return Snapshot{}, false
	}
	return s, true
}

// PhaseCountsScoped는 스코프 내 사용자의 phase별 세션 수.
func (r *gormRepo) PhaseCountsScoped(orgID, groupID int64) map[string]int {
	cl, args := userScopeClause("user_id", orgID, groupID)
	return r.groupCount2(`SELECT phase AS k, COUNT(*) AS n FROM sessions WHERE `+cl+` GROUP BY phase`, args)
}

// RunningByModeScoped는 스코프 내 running 세션의 gpu_mode별 수.
func (r *gormRepo) RunningByModeScoped(orgID, groupID int64) map[string]int {
	cl, args := userScopeClause("user_id", orgID, groupID)
	return r.groupCount2(`SELECT COALESCE(gpu_mode,'') AS k, COUNT(*) AS n FROM sessions WHERE phase='running' AND `+cl+` GROUP BY gpu_mode`, args)
}

// RunningByGpuTypeScoped는 스코프 내 running 세션의 gpu_type별 수(cpu 제외).
func (r *gormRepo) RunningByGpuTypeScoped(orgID, groupID int64) map[string]int {
	cl, args := userScopeClause("user_id", orgID, groupID)
	return r.groupCount2(`SELECT COALESCE(gpu_type,'') AS k, COUNT(*) AS n FROM sessions WHERE phase='running' AND gpu_mode<>'cpu' AND gpu_type<>'' AND `+cl+` GROUP BY gpu_type`, args)
}

// GpuHoursScoped는 이번달 스코프 내 GPU 사용시간(gpu_usage 원장 seconds→시간).
func (r *gormRepo) GpuHoursScoped(orgID, groupID int64) int {
	cl, args := userScopeClause("user_id", orgID, groupID)
	var sec int
	r.db.Raw(`SELECT COALESCE(SUM(seconds),0) FROM gpu_usage
		WHERE created_at >= DATE_FORMAT(CURDATE(),'%Y-%m-01') AND `+cl, args...).Scan(&sec)
	return sec / 3600
}

func (r *gormRepo) groupCount2(q string, args []any) map[string]int {
	var rows []struct {
		K string
		N int
	}
	r.db.Raw(q, args...).Scan(&rows)
	out := map[string]int{}
	for _, x := range rows {
		out[x.K] = x.N
	}
	return out
}

// ActiveUsers는 현재 실행 세션을 가진 사용자 목록(운영 대시보드 '사용 중 유저').
func (r *gormRepo) ActiveUsers(limit int) []ActiveUser {
	return r.activeUsers("1=1", nil, limit)
}

// ActiveUsersScoped는 스코프 내 실행 세션 보유 사용자.
func (r *gormRepo) ActiveUsersScoped(orgID, groupID int64, limit int) []ActiveUser {
	cl, args := userScopeClause("s.user_id", orgID, groupID)
	return r.activeUsers(cl, args, limit)
}

func (r *gormRepo) activeUsers(where string, args []any, limit int) []ActiveUser {
	out := []ActiveUser{}
	q := `SELECT u.username AS name, COUNT(*) AS sessions, COALESCE(MAX(NULLIF(s.gpu_type,'')),'') AS gpu_type
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.phase='running' AND ` + where + `
		GROUP BY u.id ORDER BY sessions DESC, name LIMIT ?`
	r.db.Raw(q, append(args, limit)...).Scan(&out)
	return out
}
