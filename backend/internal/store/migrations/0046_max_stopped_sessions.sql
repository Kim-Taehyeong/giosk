-- 중단(대기) 세션 상한 — 다른 하드리밋과 동일한 계층 규칙(개인→그룹→조직→전역).
-- NULL = 그 레벨 미지정(상위 폴백). 중단 세션은 세션별 로컬 홈 PVC(노드 디스크)를 물고 있어
-- 무한정 쌓이면 노드 디스크를 잠식하므로, 새 세션 생성 시 이 상한을 강제해 정리를 유도한다.
ALTER TABLE users ADD COLUMN max_stopped_sessions INT NULL;
ALTER TABLE `groups` ADD COLUMN max_stopped_sessions INT NULL;
ALTER TABLE organizations ADD COLUMN max_stopped_sessions INT NULL;
