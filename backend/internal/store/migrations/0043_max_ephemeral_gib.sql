-- 세션 임시 디스크(ephemeral-storage) 하드 상한 — 다른 하드리밋과 동일한 계층 규칙
-- (개인→그룹→조직→전역). NULL = 그 레벨 미지정(상위 폴백). 매퍼가 세션 컨테이너의
-- ephemeral-storage limit 으로 사용해 노드 루트디스크 무단점유(DoS)를 막는다.
ALTER TABLE users ADD COLUMN max_ephemeral_gib INT NULL;
ALTER TABLE `groups` ADD COLUMN max_ephemeral_gib INT NULL;
ALTER TABLE organizations ADD COLUMN max_ephemeral_gib INT NULL;
