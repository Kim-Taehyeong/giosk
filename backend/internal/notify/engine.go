package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"giosk/internal/alertlog"
	"giosk/internal/metrics"
)

// Mailer는 이메일 발송 추상화(SMTP 구현 주입; nil 이면 이메일 채널 생략).
type Mailer interface {
	Send(to []string, subject, body string) error
}

// Engine은 관리자(전역) 알림 규칙을 주기적으로 평가해, 위반 시 채널(웹훅/이메일)로 발송한다.
// 규칙/채널은 notify_rules·notify_channels(관리자 설정)에서 읽는다. 메트릭은 Prometheus(DCGM),
// 노드 비가용 수는 주입된 함수로 평가한다. 같은 규칙은 cooldown 동안 재발송하지 않는다(스팸 방지).
// Inbox는 사용자 인앱 알림 수신함(usernotify.Store)의 최소 계약이다. 결합을 줄이려 인터페이스로 받는다.
type Inbox interface {
	Record(userID int64, severity, metric string, value float64, threshold int, target string) error
}

// UserMetricFn은 사용자 규칙 지표 1개를 userID 기준 값 맵으로 평가한다(미지원이면 ok=false).
// credit_balance(잔액)·budget_pct(소모%)는 DB 로, volume_usage 등은 Prometheus 로 채운다.
type UserMetricFn func(ctx context.Context, metric string) (map[int64]float64, bool)

// SessionMetricFn은 세션 단위 지표(session_gpu/cpu/vram, %)를 특정 세션에서 평가한다(미가용이면 ok=false).
type SessionMetricFn func(ctx context.Context, metric, instanceID string) (float64, bool)

// TeamBalance는 한 사용자의 팀별 크레딧 잔액이다. Active 는 그 팀을 실제로 쓰는지(세션이나 소비 이력)를 뜻하며, 안 쓰는 빈 팀의 알림을 억제한다.
type TeamBalance struct {
	GroupID int64
	Name    string
	Balance int
	Active  bool
}

// CreditTeamsFn은 userID 별 팀 잔액을 반환한다(크레딧이 팀에 귀속되므로 팀별로 평가하고 발화한다).
type CreditTeamsFn func(ctx context.Context) map[int64][]TeamBalance

// Engine은 관리자(전역) 알림 규칙을 주기적으로 평가해, 위반 시 채널(웹훅/이메일)로 발송한다.
// 규칙/채널은 notify_rules·notify_channels(관리자 설정)에서 읽는다. 메트릭은 Prometheus(DCGM),
// 노드 비가용 수는 주입된 함수로 평가한다. 같은 규칙은 cooldown 동안 재발송하지 않는다(스팸 방지).
//
// 사용자 규칙(scope=user)은 inbox·userMetric 이 주입되면 매 tick 마다 함께 평가해, 위반 시 인앱 수신함에 적재한다.
type Engine struct {
	repo       Repository
	met        *metrics.Client
	nodesDown  func(ctx context.Context) (int, bool) // 비가용(NotReady) 노드 수; 미가용이면 ok=false
	mailer     Mailer
	http       *http.Client
	cooldown   time.Duration
	lastFired  map[int64]time.Time
	recorder   *alertlog.Store // 발화 이벤트 이력 적재(감시월 통합 피드). nil 허용.
	inbox      Inbox           // 사용자 인앱 수신함. nil 이면 사용자 규칙 평가를 건너뛴다.
	userMetric UserMetricFn    // 사용자 지표 평가기. nil 이면 사용자 규칙 평가를 건너뛴다.
	sessMetric SessionMetricFn // 세션 단위 지표 평가기(session_* 규칙용). nil 이면 세션 규칙 건너뜀.
	creditTeams CreditTeamsFn  // credit_balance 를 팀별로 평가(팀 귀속). nil 이면 팀별 크레딧 알림 건너뜀.
}

// WithSessionMetric은 세션 단위 지표 평가기를 주입한다(session_gpu/cpu/vram 규칙 활성화).
func (e *Engine) WithSessionMetric(fn SessionMetricFn) *Engine { e.sessMetric = fn; return e }

// WithCreditTeams는 팀별 크레딧 잔액 평가기를 주입한다(credit_balance 를 팀별로 발화).
func (e *Engine) WithCreditTeams(fn CreditTeamsFn) *Engine { e.creditTeams = fn; return e }

// WithRecorder는 발화 경고를 alert_events 에 이력으로 남기게 한다.
func (e *Engine) WithRecorder(r *alertlog.Store) *Engine { e.recorder = r; return e }

// WithUserAlerts는 사용자 규칙 평가를 켠다. 위반하면 inbox 에 인앱 알림을 적재한다.
func (e *Engine) WithUserAlerts(inbox Inbox, fn UserMetricFn) *Engine {
	e.inbox, e.userMetric = inbox, fn
	return e
}

func NewEngine(repo Repository, met *metrics.Client, nodesDown func(context.Context) (int, bool), mailer Mailer) *Engine {
	return &Engine{
		repo: repo, met: met, nodesDown: nodesDown, mailer: mailer,
		http:      &http.Client{Timeout: 8 * time.Second},
		cooldown:  10 * time.Minute,
		lastFired: map[int64]time.Time{},
	}
}

// Run은 interval 마다 규칙을 평가한다(블로킹; goroutine 으로 실행).
func (e *Engine) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	log.Printf("[alert-engine] started (interval=%s, cooldown=%s, email=%v)", interval, e.cooldown, e.mailer != nil)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			e.tick(ctx)
		}
	}
}

func (e *Engine) tick(ctx context.Context) {
	cfg, err := e.repo.Get(ScopeAdmin, 0)
	if err != nil {
		return
	}
	for _, r := range cfg.Rules {
		if !r.Enabled {
			continue
		}
		val, ok := e.evalMetric(ctx, r.Metric)
		if !ok {
			continue // 미지원 메트릭이거나 메트릭이 없으면 건너뛴다
		}
		if !breached(r.Op, val, float64(r.Value)) {
			continue
		}
		if last, seen := e.lastFired[r.ID]; seen && time.Since(last) < e.cooldown {
			continue // 쿨다운 중이라 반복 발송을 억제한다
		}
		e.lastFired[r.ID] = time.Now()
		e.deliver(ctx, r, cfg, val)
	}
	e.userTick(ctx)
}

// userTick은 활성 사용자 규칙을 평가해, 위반 시 그 사용자의 인앱 수신함에 알림을 적재한다.
// 지표는 metric 별로 한 번만 조회해서 규칙이 많아도 쿼리가 폭증하지 않게 한다.
func (e *Engine) userTick(ctx context.Context) {
	if e.inbox == nil || e.userMetric == nil {
		return
	}
	rules, err := e.repo.EnabledUserRules()
	if err != nil || len(rules) == 0 {
		return
	}
	e.creditTick(ctx, rules) // credit_balance 는 팀별로 따로 발화(팀 귀속)
	// metric 별 (userID 기준 값) 캐시. 같은 지표를 여러 사용자가 걸어도 한 번만 평가한다.
	cache := map[string]map[int64]float64{}
	ok := map[string]bool{}
	for _, r := range rules {
		if r.Metric == "credit_balance" && r.Target == "" {
			continue // 위 creditTick 에서 팀별로 처리
		}
		var val float64
		if r.Target != "" {
			// 세션 단위 규칙(session_gpu/cpu/vram)은 대상 세션에서 직접 평가한다.
			if e.sessMetric == nil {
				continue
			}
			v, good := e.sessMetric(ctx, r.Metric, r.Target)
			if !good {
				continue // 세션이 없거나 정지했거나 측정 불가면 발화하지 않는다
			}
			val = v
		} else {
			// 사용자 전역 규칙(credit_balance 등)은 지표별로 한 번 평가해 캐시한다.
			vals, seen := cache[r.Metric]
			if !seen {
				vals, ok[r.Metric] = e.userMetric(ctx, r.Metric)
				cache[r.Metric] = vals
			}
			if !ok[r.Metric] {
				continue
			}
			v, has := vals[r.OwnerID]
			if !has {
				continue
			}
			val = v
		}
		if !breached(r.Op, val, float64(r.Value)) {
			continue
		}
		if last, seen := e.lastFired[r.ID]; seen && time.Since(last) < e.cooldown {
			continue
		}
		e.lastFired[r.ID] = time.Now()
		// 표시 문자열 대신 metric 과 파라미터만 적재해 프론트가 언어별로 렌더한다(i18n). target 은 대상 세션이다.
		_ = e.inbox.Record(r.OwnerID, userSeverity(r.Metric), r.Metric, val, r.Value, r.Target)
		log.Printf("[alert-engine] USER FIRE uid=%d %s%s %s %d (현재 %.0f)", r.OwnerID, r.Metric, targetLog(r.Target), opSymbol(r.Op), r.Value, val)
	}
}

func targetLog(t string) string {
	if t == "" {
		return ""
	}
	return "@" + t
}

// creditTick은 credit_balance 규칙을 팀별로 평가한다. 크레딧이 팀 지갑에 귀속되므로 "어느 팀"의
// 잔액이 낮은지 팀 이름과 함께 발화한다. 안 쓰는 빈 팀(세션/소비 이력 없음)은 억제해 스팸을 막는다.
func (e *Engine) creditTick(ctx context.Context, rules []Rule) {
	if e.inbox == nil || e.creditTeams == nil {
		return
	}
	var creditRules []Rule
	for _, r := range rules {
		if r.Metric == "credit_balance" && r.Target == "" {
			creditRules = append(creditRules, r)
		}
	}
	if len(creditRules) == 0 {
		return
	}
	perUser := e.creditTeams(ctx)
	for _, r := range creditRules {
		for _, tb := range perUser[r.OwnerID] {
			if !tb.Active {
				continue // 안 쓰는 팀은 알림하지 않음
			}
			if !breached(r.Op, float64(tb.Balance), float64(r.Value)) {
				continue
			}
			key := r.ID*1_000_000 + tb.GroupID // 규칙×팀 쿨다운(팀마다 개별 억제)
			if last, seen := e.lastFired[key]; seen && time.Since(last) < e.cooldown {
				continue
			}
			e.lastFired[key] = time.Now()
			_ = e.inbox.Record(r.OwnerID, userSeverity(r.Metric), r.Metric, float64(tb.Balance), r.Value, tb.Name)
			log.Printf("[alert-engine] USER FIRE uid=%d credit_balance@%s %s %d (현재 %d)", r.OwnerID, tb.Name, opSymbol(r.Op), r.Value, tb.Balance)
		}
	}
}

// userMsg는 사용자 알림의 제목·본문을 만든다(한국어).
func userMsg(metric, op string, threshold int, val float64) (string, string) {
	switch metric {
	case "credit_balance":
		return "크레딧 잔액 부족", fmt.Sprintf("크레딧 잔액이 %.0f 로 임계(%d) 이하입니다. 세션이 곧 중단될 수 있어요.", val, threshold)
	case "budget_pct":
		return "예산 소진 임박", fmt.Sprintf("이번 주기 크레딧을 %.0f%% 사용했습니다(임계 %d%%).", val, threshold)
	case "volume_usage":
		return "볼륨 사용률 높음", fmt.Sprintf("볼륨 사용률이 %.0f%% 로 임계(%d%%)를 넘었습니다. 정리하지 않으면 쓰기 실패가 발생할 수 있어요.", val, threshold)
	case "session_idle":
		return "세션 유휴", fmt.Sprintf("세션이 %.0f분 이상 유휴 상태입니다(임계 %d분). 크레딧 낭비를 막으려면 중단하세요.", val, threshold)
	default:
		return "알림", fmt.Sprintf("%s %s %d (현재 %.0f)", metric, opSymbol(op), threshold, val)
	}
}

func userSeverity(metric string) string {
	switch metric {
	case "credit_balance", "volume_usage":
		return "err"
	default:
		return "warn"
	}
}

// evalMetric은 메트릭명을 실제 값으로 평가한다(지원: gpu_util/gpu_temp/node_down/disk_usage).
// 그 외 메트릭(budget_pct, credit_balance, session_idle 등)은 지원하지 않으므로 ok=false 다.
func (e *Engine) evalMetric(ctx context.Context, metric string) (float64, bool) {
	switch metric {
	case "disk_usage":
		// 노드 로컬 디스크 최대 사용률(%). 로컬 Home·Scratch 가 노드 디스크를 채우면
		// 정리 DaemonSet 이 오래된 파일을 지우기 시작하므로, 그 전에 관리자가 알도록 한다.
		// 100% 에 닿으면 kubelet DiskPressure 로 파드가 축출돼 노드가 통째로 빠진다.
		if e.met == nil {
			return 0, false
		}
		return e.met.Scalar(ctx,
			`max((node_filesystem_size_bytes{mountpoint="/"} - node_filesystem_avail_bytes{mountpoint="/"}) / node_filesystem_size_bytes{mountpoint="/"} * 100)`)
	case "gpu_util":
		if e.met == nil {
			return 0, false
		}
		return e.met.Scalar(ctx, "avg(DCGM_FI_DEV_GPU_UTIL)")
	case "gpu_temp":
		if e.met == nil {
			return 0, false
		}
		return e.met.Scalar(ctx, "max(DCGM_FI_DEV_GPU_TEMP)")
	case "node_idle":
		// 가장 한가한 노드의 평균 GPU 사용률(%). 임계 이하(lte)면 유휴 노드가 있다는 뜻이다.
		// GPU 를 점유만 하고 놀리는 세션을 관리자가 회수하도록 유도.
		if e.met == nil {
			return 0, false
		}
		return e.met.Scalar(ctx, "min(avg by(Hostname)(DCGM_FI_DEV_GPU_UTIL))")
	case "capacity":
		// 사용 중(util>0)인 GPU 비율(%). 임계 이상(gte)이면 클러스터 여유가 부족하다는 증설·대기열 신호다.
		if e.met == nil {
			return 0, false
		}
		return e.met.Scalar(ctx, "count(DCGM_FI_DEV_GPU_UTIL > 0) / count(DCGM_FI_DEV_GPU_UTIL) * 100")
	case "volume_usage":
		// 가장 가득 찬 PVC(볼륨) 사용률(%). 임계 이상(gte)이면 볼륨이 꽉 차 쓰기 실패가 임박.
		if e.met == nil {
			return 0, false
		}
		return e.met.Scalar(ctx, "max(kubelet_volume_stats_used_bytes / kubelet_volume_stats_capacity_bytes) * 100")
	case "node_down":
		if e.nodesDown == nil {
			return 0, false
		}
		n, ok := e.nodesDown(ctx)
		return float64(n), ok
	default:
		return 0, false
	}
}

func breached(op string, v, threshold float64) bool {
	switch op {
	case "gte":
		return v >= threshold
	case "gt":
		return v > threshold
	case "lte":
		return v <= threshold
	case "lt":
		return v < threshold
	case "eq":
		return v == threshold
	}
	return false
}

// deliver는 규칙 채널(email|webhook)로 알림을 보낸다.
func (e *Engine) deliver(ctx context.Context, r Rule, cfg Config, val float64) {
	msg := fmt.Sprintf("[Giosk] %s %s %d (현재 %.0f)", r.Metric, opSymbol(r.Op), r.Value, val)
	log.Printf("[alert-engine] FIRE %s via %s", msg, r.Channel)
	// 감시월 통합 피드에 이력으로 남긴다(웹훅/메일과 별개, 채널 무관).
	e.recorder.Record(alertlog.Event{
		Source: "infra", Severity: severityOf(r.Metric), Type: r.Metric,
		Target: r.Metric, Message: fmt.Sprintf("%s %s %d (현재 %.0f)", r.Metric, opSymbol(r.Op), r.Value, val),
	})
	switch r.Channel {
	case "webhook":
		e.sendWebhooks(ctx, cfg.Webhooks, r, val, msg)
	case "email":
		e.sendEmails(cfg.Emails, msg)
	}
}

// sendWebhooks는 각 웹훅 URL 에 JSON 페이로드를 POST 한다.
func (e *Engine) sendWebhooks(ctx context.Context, urls []string, r Rule, val float64, msg string) {
	if len(urls) == 0 {
		return
	}
	body, _ := json.Marshal(map[string]any{
		"metric": r.Metric, "op": r.Op, "threshold": r.Value, "value": val,
		"message": msg, "time": time.Now().UTC().Format(time.RFC3339),
	})
	for _, u := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := e.http.Do(req)
		if err != nil {
			log.Printf("[alert-engine] webhook %s 실패: %v", u, err)
			continue
		}
		_ = resp.Body.Close()
	}
}

func (e *Engine) sendEmails(to []string, msg string) {
	if e.mailer == nil || len(to) == 0 {
		return
	}
	if err := e.mailer.Send(to, "Giosk 알림", msg); err != nil {
		log.Printf("[alert-engine] email 실패: %v", err)
	}
}

func opSymbol(op string) string {
	switch op {
	case "gte":
		return "≥"
	case "gt":
		return ">"
	case "lte":
		return "≤"
	case "lt":
		return "<"
	}
	return op
}

// severityOf는 경고 지표를 심각도로 매핑한다(감시월 피드 색상용).
func severityOf(metric string) string {
	switch metric {
	case "node_down", "gpu_temp", "volume_usage":
		return "err"
	default:
		return "warn"
	}
}
