package dayspine

import (
	"testing"
	"time"
)

func TestKeys_앵커포함_오래된순_days개(t *testing.T) {
	anchor := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	got := Keys(anchor, 14)

	if len(got) != 14 {
		t.Fatalf("14일 스파인인데 %d개", len(got))
	}
	if got[13] != "2026-07-28" {
		t.Fatalf("마지막은 앵커(오늘)여야 하는데 %q", got[13])
	}
	if got[0] != "2026-07-15" {
		t.Fatalf("첫 날은 앵커-13일이어야 하는데 %q", got[0])
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Fatalf("오래된→최근 정렬이 깨짐: %q >= %q", got[i-1], got[i])
		}
	}
}

// 키를 "%m/%d" 같은 짧은 포맷으로 두면 연말에 문자열 정렬이 뒤집힌다(12/31 > 01/01).
// ISO 키는 연을 넘겨도 사전순 = 시간순이어야 한다.
func TestKeys_연말경계에서_정렬유지(t *testing.T) {
	got := Keys(time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC), 5)
	want := []string{"2026-12-29", "2026-12-30", "2026-12-31", "2027-01-01", "2027-01-02"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%d] = %q, 기대 %q", i, got[i], want[i])
		}
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Fatalf("연말 경계에서 사전순 정렬이 깨짐: %q >= %q", got[i-1], got[i])
		}
	}
}

// DST 가 있는 지역 시간대에서도 하루씩 정확히 전진해야 한다
// (24시간 더하기로 구현하면 DST 전환일에 같은 날짜가 두 번 나오거나 하루가 통째로 빠진다).
func TestKeys_DST전환일에도_하루씩(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("tzdata 없음")
	}
	// 2026-03-08 이 미국 DST 시작일.
	got := Keys(time.Date(2026, 3, 10, 0, 0, 0, 0, loc), 5)
	want := []string{"2026-03-06", "2026-03-07", "2026-03-08", "2026-03-09", "2026-03-10"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%d] = %q, 기대 %q", i, got[i], want[i])
		}
	}
}

func TestKeys_비정상입력(t *testing.T) {
	if got := Keys(time.Now(), 0); got != nil {
		t.Fatalf("days=0 이면 nil 이어야 하는데 %v", got)
	}
	if got := Keys(time.Now(), -3); got != nil {
		t.Fatalf("days<0 이면 nil 이어야 하는데 %v", got)
	}
}
