-- 세션 소비 회계: 실행 시간 기반 과금.
-- price_per_hour: 시간당 크레딧 단가(생성 시 고정). started_at: 현재 가동 시작(재개 시 갱신).
-- billed_credits: 현재 가동분에서 이미 차감된 크레딧(증분 정산용).
ALTER TABLE sessions ADD COLUMN price_per_hour INT NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN started_at DATETIME NULL;
ALTER TABLE sessions ADD COLUMN billed_credits INT NOT NULL DEFAULT 0;
