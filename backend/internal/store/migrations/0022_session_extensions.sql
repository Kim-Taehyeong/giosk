-- 0022 세션 임대 연장 횟수 — 선착순(dynamic) 모드에서 사용자가 임대를 연장한 횟수.
-- 남은 임대 시간 = maxLeaseHours + extensions_used×extensionHours − 경과시간 (프론트 계산).
ALTER TABLE sessions ADD COLUMN extensions_used INT NOT NULL DEFAULT 0;
