package session

import (
	"testing"
	"time"
)

// 중단 스토리지 과금은 "누적 총액 − 이미 청구액" 델타 회계다. 매 틱 내림하면 소수가 영구 손실되는데
// 이 방식은 다음 틱으로 이월된다. 소액 단가(GiB·월 몇 크레딧)에서 특히 중요하다.
func TestStorageDueOf_델타누적에서_내림손실이_이월된다(t *testing.T) {
	const price = 3 // 크레딧 / GiB·월. 홈 10GiB 이면 월 30 크레딧

	if got := storageDueOf(secondsPerMonth, price); got != homeSizeGiB*price {
		t.Fatalf("한 달 방치 = %d 크레딧이어야 하는데 %d", homeSizeGiB*price, got)
	}
	// 단가가 0(무료 스토리지)이거나 방치 시간이 없으면 청구 없음.
	if got := storageDueOf(secondsPerMonth, 0); got != 0 {
		t.Fatalf("단가 0 이면 청구 0 이어야 하는데 %d", got)
	}
	if got := storageDueOf(0, price); got != 0 {
		t.Fatalf("방치 0초면 청구 0 이어야 하는데 %d", got)
	}

	// 1분 틱을 한 달치 돌려도, 매 틱 내림했을 때와 달리 총액이 정확히 맞아야 한다.
	billed := 0
	for elapsed := 60; elapsed <= secondsPerMonth; elapsed += 60 {
		if due := storageDueOf(elapsed, price) - billed; due > 0 {
			billed += due
		}
	}
	if billed != homeSizeGiB*price {
		t.Fatalf("1분 틱 누적 결과 %d, 기대 %d (내림 손실 발생)", billed, homeSizeGiB*price)
	}
}

// 장기 방치 × 단가가 32비트를 넘어도 음수로 뒤집히지 않아야 한다(int64 계산).
func TestStorageDueOf_장기방치_오버플로없음(t *testing.T) {
	const tenYears = 10 * 365 * 24 * 3600
	if got := storageDueOf(tenYears, 1000); got <= 0 {
		t.Fatalf("10년치 청구액이 %d 다. 오버플로", got)
	}
}

// 회수 우선순위: 오래 방치할수록, 그리고 같은 사용자가 많이 물고 있을수록 먼저.
// 방치기간만 보면 과독점이 남고 점유량만 보면 정상 사용자가 매번 걸린다. 곱으로 둘 다 반영한다.
func TestReapScore_방치기간과_과독점을_함께_반영한다(t *testing.T) {
	now := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	at := func(days int) *Session {
		s := now.Add(-time.Duration(days) * 24 * time.Hour)
		return &Session{StoppedSince: &s}
	}

	// 같은 점유량이면 오래 방치된 쪽이 먼저.
	occ := map[int64]int{0: 1}
	if reapScore(at(30), occ, now) <= reapScore(at(15), occ, now) {
		t.Fatal("같은 점유량이면 오래 방치된 세션이 먼저 회수돼야 한다")
	}

	// 방치기간이 짧아도 과독점(중단 세션 5개)이면 앞선다.
	hog, light := at(15), at(30)
	hog.UserID, light.UserID = 1, 2
	occ = map[int64]int{1: 5, 2: 1}
	if reapScore(hog, occ, now) <= reapScore(light, occ, now) {
		t.Fatal("과독점 사용자의 세션이 먼저 회수돼야 한다")
	}

	// occupancy 에 없는 사용자도 0 으로 죽지 않고 방치기간만으로 순위가 매겨져야 한다.
	if reapScore(at(10), map[int64]int{}, now) <= 0 {
		t.Fatal("occupancy 미기록 사용자의 점수가 0 이하다. 영원히 회수되지 않는다")
	}
}
