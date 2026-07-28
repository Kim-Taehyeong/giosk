-- 중단 세션 스토리지 과금 + 회수(T1) 근거 컬럼.
--
-- 중단(stopped) 세션은 Pod 가 없어 GPU 과금이 멈추지만 세션 홈 PVC(노드 로컬 디스크)는 계속
-- 점유한다. 방치된 중단 세션이 노드 디스크를 잠식하는 것을 "정책"이 아니라 "가격"으로 풀기 위해,
-- 중단 상태로 머문 시간에 대해 홈 용량 × 스토리지 단가(GiB·월)를 소액 정산한다.
--
--   stopped_seconds         누적 중단 시간(초). 중단/재개를 반복해도 누적된다 = 과금의 단조 기준.
--   storage_billed_credits  스토리지로 이미 청구한 크레딧(내림). 세션 과금(billed_credits)과 분리 —
--                           재개 시 billed_credits 는 0으로 리셋되지만(ResetBilling) 스토리지 누적은
--                           살아남아야 하므로 같은 컬럼을 쓸 수 없다.
--   stopped_since           현재 중단 구간의 시작 시각(UTC). 실행 중이면 NULL.
--                           과금 델타 계산 기준이자 T1 회수의 "방치 기간" 근거.
--
-- 기존 stopped 세션은 stopped_since 가 NULL 이라 과금이 시작되지 않는다 —
-- 첫 정산 틱이 NULL 인 stopped 세션에 now() 를 찍어 소급 과금 없이 그 시점부터 시작한다.
ALTER TABLE sessions ADD COLUMN stopped_seconds INT NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN storage_billed_credits INT NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN stopped_since DATETIME NULL;
