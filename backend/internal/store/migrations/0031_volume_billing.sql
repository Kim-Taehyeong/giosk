-- 스토리지 크레딧 과금(모든 리소스 크레딧화). 볼륨은 생성 시점(created_at)부터
-- 용량(size_gib)×스토리지 단가(GiB·월)로 주기 정산된다. billed_credits 는 지금까지
-- 누적 차감한 크레딧(내림)으로, 세션 과금(sessions.billed_credits)과 동일한 델타 회계 방식.
ALTER TABLE volumes ADD COLUMN billed_credits INT NOT NULL DEFAULT 0;
