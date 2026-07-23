package wallet

import (
	"os"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// TestRefillScenario는 계층 이월/소멸 규칙을 로컬 MySQL 로 검증한다(GIOSK_TEST_DSN 없으면 스킵).
// 시나리오: 플랫폼 60일·이월불가, 조직 30일·이월. 30일차엔 이월(누적), 60일차(플랫폼 경계)엔 전액 소멸.
func TestRefillScenario(t *testing.T) {
	dsn := os.Getenv("GIOSK_TEST_DSN")
	if dsn == "" {
		t.Skip("GIOSK_TEST_DSN 미설정 — 스킵")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	r := &gormRepo{db: db}

	// 테스트 조직(id 1) 사용. 다른 조직은 recurring 0 이면 무시됨.
	setup := func(platCycleDaysAgo, orgCycleDaysAgo, pool int) {
		db.Exec(`UPDATE refill_state SET platform_cycle_started_at = NOW() - INTERVAL ? DAY WHERE id=1`, platCycleDaysAgo)
		db.Exec(`UPDATE organizations SET recurring_credit=1000, refill_interval_days=30, carryover=1, credit_pool=?, recurring_cycle_started_at = NOW() - INTERVAL ? DAY WHERE id=1`, pool, orgCycleDaysAgo)
	}
	poolNow := func() int {
		var p int
		db.Raw(`SELECT credit_pool FROM organizations WHERE id=1`).Scan(&p)
		return p
	}

	// A) 30일차 — 조직 주기 도래, 플랫폼 미도래 → 이월(누적): 500 + 1000 = 1500.
	setup(30, 30, 500)
	if _, err := r.RefillDue(PlatformRefill{IntervalDays: 60, Carryover: false}); err != nil {
		t.Fatalf("refill A: %v", err)
	}
	if got := poolNow(); got != 1500 {
		t.Errorf("이월 시나리오: pool=%d, want 1500", got)
	}

	// B) 60일차 — 플랫폼 경계(이월불가) 도래 → 하위 이월분 전액 소멸: 리셋 1000.
	setup(60, 30, 500)
	if _, err := r.RefillDue(PlatformRefill{IntervalDays: 60, Carryover: false}); err != nil {
		t.Fatalf("refill B: %v", err)
	}
	if got := poolNow(); got != 1000 {
		t.Errorf("소멸 시나리오: pool=%d, want 1000", got)
	}

	// 플랫폼 앵커가 now 로 전진했는지(경계 발화).
	var platAge float64
	db.Raw(`SELECT TIMESTAMPDIFF(SECOND, platform_cycle_started_at, NOW()) FROM refill_state WHERE id=1`).Scan(&platAge)
	if platAge > 60 {
		t.Errorf("플랫폼 앵커 미전진: %.0fs old", platAge)
	}
	_ = time.Now
}
