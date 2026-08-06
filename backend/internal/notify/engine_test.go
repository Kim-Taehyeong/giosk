package notify

import (
	"context"
	"testing"
)

type stubRepo struct{ rules []Rule }

func (s *stubRepo) Get(string, int64) (Config, error)   { return Config{}, nil }
func (s *stubRepo) Replace(string, int64, PutReq) error { return nil }
func (s *stubRepo) EnabledUserRules() ([]Rule, error)   { return s.rules, nil }

type stubInbox struct{ n int }

func (s *stubInbox) Record(int64, string, string, float64, int, string) error { s.n++; return nil }

// 세션 규칙 평가기는 (대상 세션, 지표) 순서로 호출돼야 한다.
// 두 인자가 모두 string 이라 뒤바뀌어도 컴파일이 통과한다. 실제로 한동안 뒤집혀 있었고,
// 그동안 엔진은 지표 이름("session_gpu")으로 세션을 찾다가 매번 못 찾아 세션 알림이 한 번도 울리지 않았다.
func TestSessionRuleArgOrder(t *testing.T) {
	const (
		target = "ses-abc123"
		metric = "session_gpu"
	)
	repo := &stubRepo{rules: []Rule{{
		ID: 1, Scope: "user", OwnerID: 7,
		Metric: metric, Op: "gte", Value: 80, Channel: "email", Enabled: true, Target: target,
	}}}

	var gotID, gotMetric string
	e := NewEngine(repo, nil, nil, nil).
		WithUserAlerts(&stubInbox{}, func(context.Context, string) (map[int64]float64, bool) {
			return nil, false
		}).
		WithSessionMetric(func(_ context.Context, instanceID, m string) (float64, bool) {
			gotID, gotMetric = instanceID, m
			return 0, false
		})

	e.userTick(context.Background())

	if gotID != target {
		t.Errorf("첫 인자가 대상 세션이어야 한다: got %q, want %q", gotID, target)
	}
	if gotMetric != metric {
		t.Errorf("둘째 인자가 지표여야 한다: got %q, want %q", gotMetric, metric)
	}
}

// 임계 위반이면 수신함에 적재한다(세션 규칙 경로가 끝까지 도는지 확인).
func TestSessionRuleFires(t *testing.T) {
	repo := &stubRepo{rules: []Rule{{
		ID: 1, Scope: "user", OwnerID: 7,
		Metric: "session_gpu", Op: "lte", Value: 10, Channel: "email", Enabled: true, Target: "ses-abc123",
	}}}
	inbox := &stubInbox{}
	e := NewEngine(repo, nil, nil, nil).
		WithUserAlerts(inbox, func(context.Context, string) (map[int64]float64, bool) { return nil, false }).
		WithSessionMetric(func(context.Context, string, string) (float64, bool) { return 3, true })

	e.userTick(context.Background())

	if inbox.n != 1 {
		t.Errorf("GPU 3%% 는 lte 10 위반이라 1건 적재돼야 한다: got %d", inbox.n)
	}
}

// 쿨다운은 규칙 id 가 아니라 규칙 내용으로 기억해야 한다.
//
// 알림 설정 저장은 규칙을 통째로 지웠다가 다시 넣어서 같은 규칙이라도 id 가 매번 바뀐다.
// 화면을 열면 저장이 한 번 나가던 때가 있었는데, id 로 기억하면 그때마다 쿨다운이 사라져
// 같은 알림이 1분 간격으로 계속 왔다.
func TestCooldownSurvivesRuleIDChange(t *testing.T) {
	e := NewEngine(nil, nil, nil, nil)
	before := Rule{ID: 41, Scope: ScopeUser, OwnerID: 1, Metric: "session_gpu", Op: "lte", Value: 10, Target: "ses-a"}
	after := before
	after.ID = 907 // 저장하면서 새로 채번된 같은 규칙

	if e.cooling(ruleKey(before, "")) {
		t.Fatal("첫 발화는 억제되면 안 된다")
	}
	if !e.cooling(ruleKey(after, "")) {
		t.Error("id 만 바뀐 같은 규칙은 쿨다운에 걸려야 한다")
	}

	// 대상 세션이 다르면 별개 규칙이라 억제하지 않는다.
	other := before
	other.Target = "ses-b"
	if e.cooling(ruleKey(other, "")) {
		t.Error("대상이 다른 규칙까지 억제하면 안 된다")
	}
}
