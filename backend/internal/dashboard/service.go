// Package dashboard는 사용자/관리자 대시보드 KPI·추이를 집계한다.
//
// 데이터 출처는 둘로 나뉜다:
//   - DB 집계(잔액/세션 수/그룹·사용자 랭킹): repo
//   - 클러스터 라이브 지표(GPU util/VRAM, 추이): Prometheus(DCGM), 미가용 시 0
//
// 모드(credit|dynamic)에 따라 노출 KPI 가 달라진다(크레딧 관련은 credit 모드에서만 의미).
package dashboard

import (
	"context"
	"fmt"
	"time"

	"giosk/internal/alertlog"
	"giosk/internal/config"
	"giosk/internal/k8s"
	"giosk/internal/metrics"
	"giosk/internal/policy"
)

// NodeLister는 클러스터 노드 라이브 상태(k8s.Client 구현). 미가용이면 빈 슬라이스.
type NodeLister interface {
	ListNodes(ctx context.Context) ([]k8s.LiveNode, error)
}

// LimitResolver는 사용자별 계층 해석된 정책 상한을 준다(정책 = 동시세션 상한의 단일 출처).
type LimitResolver interface {
	Resolve(userID int64) policy.Resolved
}

// Service는 대시보드 집계 로직.
type Service struct {
	repo   Repository
	met    *metrics.Client
	nodes  NodeLister
	cfg    *config.Config
	alerts *alertlog.Store // 발화 경고 이력(감시월 통합 피드). nil 허용.
	limits LimitResolver   // 정책 기반 동시세션 상한. nil 이면 무제한 표기.
}

// WithLimits는 정책 리졸버를 주입한다(대시보드 동시세션 KPI 를 정책값으로 표기).
func (s *Service) WithLimits(r LimitResolver) *Service { s.limits = r; return s }

func NewService(repo Repository, met *metrics.Client, nodes NodeLister, cfg *config.Config) *Service {
	return &Service{repo: repo, met: met, nodes: nodes, cfg: cfg}
}

// WithAlertStore는 통합 경고 피드 소스를 주입한다.
func (s *Service) WithAlertStore(a *alertlog.Store) *Service { s.alerts = a; return s }

// alertFeed는 최근 경고 이벤트를 반환한다(감시월 통합 피드). 스토어 없으면 빈 슬라이스.
func (s *Service) alertFeed(limit int) []alertlog.Event {
	if s.alerts == nil {
		return []alertlog.Event{}
	}
	return s.alerts.Recent(limit)
}

// clusterSessionStats는 클러스터 전체 세션 상태 분해(인프라 대시보드). 유휴는 최신 스냅샷 기준.
func (s *Service) clusterSessionStats() SessionStats {
	pc := s.repo.SessionPhaseCounts()
	idle := 0
	if snap, ok := s.repo.LatestSnapshot(); ok {
		idle = snap.IdleSessions
	}
	return SessionStats{
		Running:      pc["running"],
		Idle:         idle,
		Provisioning: pc["provisioning"],
		Stopped:      pc["stopped"],
		ByMode:       s.repo.RunningByMode(),
		ByGpuType:    s.repo.RunningByGpuType(),
	}
}

// scopedSessionStats는 스코프 내 세션 상태 분해(운영 대시보드). 스코프 유휴는 미측정(0).
func (s *Service) scopedSessionStats(orgID, groupID int64) SessionStats {
	pc := s.repo.PhaseCountsScoped(orgID, groupID)
	idle := 0
	if orgID == 0 && groupID == 0 { // platform 운영뷰는 클러스터 유휴를 그대로 쓴다
		if snap, ok := s.repo.LatestSnapshot(); ok {
			idle = snap.IdleSessions
		}
	}
	return SessionStats{
		Running:      pc["running"],
		Idle:         idle,
		Provisioning: pc["provisioning"],
		Stopped:      pc["stopped"],
		ByMode:       s.repo.RunningByModeScoped(orgID, groupID),
		ByGpuType:    s.repo.RunningByGpuTypeScoped(orgID, groupID),
	}
}

// User는 사용자 대시보드를 만든다. groupID=활성 팀(스코프); 잔액/총액을 그 팀 지갑으로 산정.
func (s *Service) User(ctx context.Context, userID, groupID int64) UserDashboard {
	balance, cap := s.repo.UserWallet(userID, groupID)
	used, total := s.userVRAM(ctx, userID)
	burn := s.repo.UserConsume7d(userID) / 7 // 일일 평균 소비(최근 7일)
	eta := 0
	if burn > 0 {
		eta = balance / burn
	}
	return UserDashboard{
		KPIs: UserKPIs{
			Balance:        balance,
			Cap:            cap,
			ActiveSessions: s.repo.UserActiveSessions(userID),
			MaxSessions:    s.userMaxSessions(userID),
			VramUsed:       used,
			VramTotal:      total,
			GpuHoursMonth:  s.repo.MonthlyGpuHours(userID),
			EtaDays:        eta,
			Burn:           burn,
		},
		Regions:  s.regions(ctx),
		Sessions: s.userSessions(userID),
	}
}

// Admin은 인프라 대시보드(감시월)를 만든다. 스냅샷(우리 DB) 우선, 없으면 라이브 폴백.
func (s *Service) Admin(ctx context.Context) AdminDashboard {
	up, total := s.nodeCounts(ctx)
	gpusTotal := 0
	for _, c := range s.gpuCapacityByType(ctx) {
		gpusTotal += c
	}
	return AdminDashboard{
		KPIs: AdminKPIs{
			GpuUtil:        s.scalar(ctx, "avg(DCGM_FI_DEV_GPU_UTIL)"),
			VramAlloc:      s.vramAllocPct(ctx),
			GpuTempAvg:     s.scalar(ctx, "avg(DCGM_FI_DEV_GPU_TEMP)"),
			GpuTempMax:     s.scalar(ctx, "max(DCGM_FI_DEV_GPU_TEMP)"),
			GpusUsed:       s.repo.RunningGpuCount(),
			GpusTotal:      gpusTotal,
			ActiveSessions: s.repo.ActiveSessions(),
			MaxSessions:    s.cfg.Quota.MaxConcurrentSessions,
			Queue:          0,
			MonthCredit:    s.repo.MonthConsumed(),
			NodesUp:        up,
			NodesTotal:     total,
			HealthAlerts:   total - up,
			BudgetRisk:     s.repo.BudgetRiskGroups(),
		},
		TopGroups:    s.repo.NameCredits("group", 5),
		TopUsers:     s.repo.NameCredits("org", 5),
		GpuTrend7d:   s.gpuTrend(ctx),
		CreditTrend:  s.creditTrend(),
		Alerts:       s.adminAlerts(ctx),
		AlertFeed:    s.alertFeed(30),
		Snapshots:    s.repo.SnapshotsSince(time.Now().Add(-24 * time.Hour)),
		SessionStats: s.clusterSessionStats(),
		ActiveUsers:  s.repo.ActiveUsers(8),
		Metrics:      s.metricsStatus(ctx),
	}
}

// metricsStatus는 지표 스택 설치 여부를 확인한다. DCGM 은 시리즈가 실제로 존재해야 true 다.
// Prometheus 만 있고 dcgm-exporter 가 없으면 GPU 지표는 영원히 0 이므로 구분해야 한다.
func (s *Service) metricsStatus(ctx context.Context) MetricsStatus {
	st := MetricsStatus{Prometheus: s.met.Enabled()}
	if st.Prometheus {
		if v, ok := s.met.Scalar(ctx, "count(DCGM_FI_DEV_GPU_UTIL)"); ok && v > 0 {
			st.DCGM = true
		}
	}
	return st
}

// Ops는 운영 대시보드(사용·거버넌스)를 스코프+과금모드로 반환한다.
// 공통 코어(세션통계·GPU시간·경고피드)는 전 모드, 크레딧 위젯은 credit 모드만 채운다.
func (s *Service) Ops(orgID, groupID int64) OpsDashboard {
	mode := s.cfg.Billing.Mode
	out := OpsDashboard{
		BillingMode: mode,
		KPIs: OpsKPIs{
			ActiveSessions: s.repo.ActiveSessionsScoped(orgID, groupID),
		},
		SessionStats: s.scopedSessionStats(orgID, groupID),
		GpuHours:     s.repo.GpuHoursScoped(orgID, groupID),
		ActiveUsers:  s.repo.ActiveUsersScoped(orgID, groupID, 8),
		AlertFeed:    s.alertFeed(30),
	}
	if mode == "credit" { // 크레딧 전용 위젯(소비/추이/예산/상위)
		out.KPIs.MonthCredit = s.repo.MonthConsumedScoped(orgID, groupID)
		out.KPIs.BudgetRisk = s.repo.BudgetRiskScoped(orgID, groupID)
		out.CreditTrend = s.repo.CreditTrendScoped(14, orgID, groupID)
		out.TopGroups = s.repo.NameCreditsScoped("group", orgID, groupID, 5)
		out.TopUsers = s.repo.NameCreditsScoped("user", orgID, groupID, 5)
	}
	return out
}

// creditTrend는 최근 14일 일자별 전체 소비 추이(빈 결과면 빈 슬라이스 보장).
func (s *Service) creditTrend() []TrendPoint {
	pts := s.repo.CreditTrend(14)
	if pts == nil {
		return []TrendPoint{}
	}
	return pts
}

// adminAlerts는 라이브 신호(노드 비가용/cordon + 예산 임계 그룹)에서 운영 알림을 합성한다.
// 별도 알림 테이블 없이 현재 상태에서 도출하므로 항상 최신이다.
func (s *Service) adminAlerts(ctx context.Context) []AdminAlert {
	out := []AdminAlert{}
	if nodes, err := s.nodes.ListNodes(ctx); err == nil {
		for _, n := range nodes {
			switch {
			case !n.Ready:
				out = append(out, AdminAlert{Type: "err", Title: n.Name + " NotReady", Detail: "노드가 비가용 상태입니다", Age: "now"})
			case n.Cordoned:
				out = append(out, AdminAlert{Type: "warn", Title: n.Name + " cordon", Detail: "스케줄링이 차단되었습니다", Age: "now"})
			}
		}
	}
	if br := s.repo.BudgetRiskGroups(); br > 0 {
		out = append(out, AdminAlert{Type: "warn", Title: fmt.Sprintf("크레딧 풀 소진 그룹 %d개", br), Detail: "그룹 풀 잔여가 없어 추가 배분이 필요합니다", Age: "now"})
	}
	return out
}

// userMaxSessions는 사용자 동시 세션 상한을 정책(계층 해석)에서 가져온다(0=무제한).
// billing.credit.maxConcurrentSessions 는 폐기했다. 정책(quota)으로 일원화한다.
func (s *Service) userMaxSessions(userID int64) int {
	if s.limits == nil {
		return 0
	}
	return s.limits.Resolve(userID).MaxConcurrentSessions
}

// scalar는 Prometheus 스칼라를 int 로(미가용 0).
func (s *Service) scalar(ctx context.Context, q string) int {
	v, ok := s.met.Scalar(ctx, q)
	if !ok {
		return 0
	}
	return int(v + 0.5)
}

// vramAllocPct는 전체 VRAM 사용 비율(%).
func (s *Service) vramAllocPct(ctx context.Context) int {
	return s.scalar(ctx, "avg(DCGM_FI_DEV_FB_USED/(DCGM_FI_DEV_FB_USED+DCGM_FI_DEV_FB_FREE))*100")
}

// gpuTrend는 7일 GPU 평균 사용률 추이(미가용 시 빈 슬라이스).
func (s *Service) gpuTrend(ctx context.Context) []TrendPoint {
	if !s.met.Enabled() {
		return []TrendPoint{}
	}
	end := time.Now()
	pts, ok := s.met.Range(ctx, "avg(DCGM_FI_DEV_GPU_UTIL)", end.Add(-7*24*time.Hour), end, 24*time.Hour)
	if !ok {
		return []TrendPoint{}
	}
	out := make([]TrendPoint, 0, len(pts))
	for _, p := range pts {
		out = append(out, TrendPoint{Date: time.Unix(int64(p.T), 0).Format("1/2"), Util: int(p.V + 0.5)})
	}
	return out
}
