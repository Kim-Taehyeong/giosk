package session

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// 관문(검사)과 예약(기록) 사이가 열려 있으면 동시에 들어온 요청이 모두 같은 여유를 보고 통과한다.
// admit 은 그 구간을 한 번에 하나만 지나게 해야 한다.
func TestAdmit_검사와_예약_사이를_직렬화한다(t *testing.T) {
	s := &Service{}
	var mu sync.Mutex
	inside, maxInside := 0, 0

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.admit(context.Background(), func() error {
				mu.Lock()
				inside++
				if inside > maxInside {
					maxInside = inside
				}
				mu.Unlock()
				mu.Lock()
				inside--
				mu.Unlock()
				return nil
			})
		}()
	}
	wg.Wait()
	if maxInside > 1 {
		t.Fatalf("임계구간에 동시에 %d 개가 들어왔다. 같은 자리를 두 번 내주게 된다", maxInside)
	}
}

// 분산 잠금(DB)을 못 얻어도 세션 생성이 통째로 막히면 안 된다. 프로세스 안 잠금으로 진행한다.
func TestAdmit_분산잠금_실패해도_진행한다(t *testing.T) {
	s := (&Service{}).WithAdmissionLock(func(context.Context) (func(), error) {
		return nil, errors.New("db down")
	})
	ran := false
	if err := s.admit(context.Background(), func() error { ran = true; return nil }); err != nil {
		t.Fatalf("잠금 실패가 요청 실패로 새어 나왔다: %v", err)
	}
	if !ran {
		t.Fatal("잠금을 못 얻었다고 관문 자체를 건너뛰었다")
	}
}

// 잠금은 성공 여부와 무관하게 반드시 해제돼야 한다(다음 요청이 영영 막히지 않게).
func TestAdmit_실패해도_잠금을_해제한다(t *testing.T) {
	released := 0
	s := (&Service{}).WithAdmissionLock(func(context.Context) (func(), error) {
		return func() { released++ }, nil
	})
	want := errors.New("no capacity")
	if err := s.admit(context.Background(), func() error { return want }); !errors.Is(err, want) {
		t.Fatalf("관문 오류가 그대로 전달되지 않았다: %v", err)
	}
	if released != 1 {
		t.Fatalf("잠금 해제 %d 회. 실패 경로에서 새어 나갔다", released)
	}
}
