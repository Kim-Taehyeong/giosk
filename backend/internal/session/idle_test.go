package session

import (
	"testing"
	"time"
)

// 유휴 유예는 "이번 가동" 기준이어야 한다.
//
// 예전에는 생성 시각(CreatedAt)으로 쟀다. 그래서 며칠 전에 만들어 둔 세션을 재개하면
// 유예가 처음부터 0 이라, Prometheus 에 지표가 쌓이기도 전에 0% 로 읽혀 곧바로 정지됐다.
// 실제로 재개 39초 만에 session_idle_stop 이 걸린 사례가 있었다.
func TestRunStartUsesLatestStart(t *testing.T) {
	created := time.Now().Add(-72 * time.Hour)
	justStarted := time.Now().Add(-30 * time.Second)

	resumed := &Session{CreatedAt: created, StartedAt: &justStarted}
	if got := runStart(resumed); !got.Equal(justStarted) {
		t.Errorf("재개한 세션은 마지막 시작 시각을 써야 한다: got %v, want %v", got, justStarted)
	}
	// 30분 유예 안에 있으므로 회수 대상이 아니다.
	if time.Since(runStart(resumed)) >= 30*time.Minute {
		t.Error("방금 재개한 세션이 유예 밖으로 판정됐다")
	}

	// 한 번도 뜬 적 없으면 생성 시각으로 본다.
	fresh := &Session{CreatedAt: created}
	if got := runStart(fresh); !got.Equal(created) {
		t.Errorf("started_at 이 없으면 생성 시각: got %v, want %v", got, created)
	}

	// started_at 이 생성보다 이르면(데이터 이상) 생성 시각을 쓴다.
	stale := created.Add(-time.Hour)
	odd := &Session{CreatedAt: created, StartedAt: &stale}
	if got := runStart(odd); !got.Equal(created) {
		t.Errorf("started_at 이 생성보다 이르면 생성 시각: got %v, want %v", got, created)
	}
}
