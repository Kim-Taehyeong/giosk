-- 0040 조직 정기 크레딧 — 조직에 부여한 "정기 크레딧"을 주기(월 등)마다 재생성한다.
-- recurring_credit: 매 주기 재생성될 크레딧 양(0이면 정기 재생성 안 함).
-- recurring_cycle_started_at: 마지막 재생성 시각(주기 도래 판정). NULL이면 즉시 도래.
ALTER TABLE organizations ADD COLUMN recurring_credit INT NOT NULL DEFAULT 0;
ALTER TABLE organizations ADD COLUMN recurring_cycle_started_at TIMESTAMP NULL;
