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

// 면책은 사용자별로 "가장 최근 중단 세션 1개"다. 이게 없으면 방치 기간만으로 전부
// 털려서 잠깐 멈춰 둔 것까지 사라진다.
func TestStoppedProfile_사용자별_최신_중단세션은_면책된다(t *testing.T) {
	now := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	at := func(user int64, id string, days int) Session {
		ts := now.Add(-time.Duration(days) * 24 * time.Hour)
		return Session{InstanceID: id, UserID: user, StoppedSince: &ts}
	}
	rows := []Session{
		at(1, "old-1", 30),
		at(1, "new-1", 3), // 사용자 1 의 최신
		at(2, "only-2", 90),
	}
	keep := (&Service{}).stoppedProfile(rows, now)
	if keep[1] != "new-1" {
		t.Errorf("사용자 1 의 면책이 최신 세션이 아니다: %q", keep[1])
	}
	if keep[2] != "only-2" {
		t.Errorf("중단 세션이 하나뿐인 사용자도 면책돼야 한다: %q", keep[2])
	}
}
