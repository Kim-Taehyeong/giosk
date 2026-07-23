-- 0041 계층형 정기 크레딧 리필 (정책 하드캡과 동일한 결)
--
-- 캐스케이드: 플랫폼→조직, 조직→팀(group), 팀→개인(user). 각 상위가 하위의 리필을 설정한다.
-- refill_interval_days: 이 풀의 리필 주기(일). 0=상위 기본 사용. 상위보다 길 수 없다(캡).
-- carryover: 이월(1=미사용분 이월, 0=주기마다 리셋). 상위가 이월불가면 그 경계에서 하위 이월분도 전액 소멸.
-- cycle_started_at: 마지막 리필 시각(주기 도래 판정). NULL=즉시 도래.

-- 조직: recurring_credit·recurring_cycle_started_at 은 0040 에 있음. 주기·이월 추가.
ALTER TABLE organizations ADD COLUMN refill_interval_days INT NOT NULL DEFAULT 0;
ALTER TABLE organizations ADD COLUMN carryover TINYINT(1) NOT NULL DEFAULT 0;

-- 팀(그룹) 지갑: 정기 리필 설정. cycle_started_at 은 이미 있음.
ALTER TABLE group_wallets ADD COLUMN recurring_credit INT NOT NULL DEFAULT 0;
ALTER TABLE group_wallets ADD COLUMN refill_interval_days INT NOT NULL DEFAULT 0;
ALTER TABLE group_wallets ADD COLUMN carryover TINYINT(1) NOT NULL DEFAULT 0;

-- 개인 지갑: 정기 리필 설정. cycle_started_at 은 이미 있음.
ALTER TABLE user_wallets ADD COLUMN recurring_credit INT NOT NULL DEFAULT 0;
ALTER TABLE user_wallets ADD COLUMN refill_interval_days INT NOT NULL DEFAULT 0;
ALTER TABLE user_wallets ADD COLUMN carryover TINYINT(1) NOT NULL DEFAULT 0;

-- 플랫폼 리필 주기 앵커(전역 1행) — 최고관리자 이월불가 경계 계산 기준.
CREATE TABLE IF NOT EXISTS refill_state (
  id                        TINYINT PRIMARY KEY,
  platform_cycle_started_at TIMESTAMP NULL
);
INSERT IGNORE INTO refill_state (id, platform_cycle_started_at) VALUES (1, NOW());
