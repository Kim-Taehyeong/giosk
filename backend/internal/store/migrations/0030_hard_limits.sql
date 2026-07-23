-- 하드 리소스 제한(크레딧과 무관한 절대 상한). 바텀업 오버라이드로 해석한다:
--   개인(users) → 그룹(groups) → 조직(organizations) → 전역(config).
-- 각 컬럼은 nullable: NULL 이면 "이 레벨은 제한을 지정 안 함" → 상위 레벨로 폴백.
-- 전역은 config.Quota(설치 고정)가 최종 폴백을 제공하므로 해석 결과는 항상 구체값이 된다.

ALTER TABLE users ADD COLUMN max_gpu INT NULL;
ALTER TABLE users ADD COLUMN max_vram_gb INT NULL;
ALTER TABLE users ADD COLUMN max_volume_gib INT NULL;
ALTER TABLE users ADD COLUMN max_concurrent_sessions INT NULL;

ALTER TABLE `groups` ADD COLUMN max_gpu INT NULL;
ALTER TABLE `groups` ADD COLUMN max_vram_gb INT NULL;
ALTER TABLE `groups` ADD COLUMN max_volume_gib INT NULL;
ALTER TABLE `groups` ADD COLUMN max_concurrent_sessions INT NULL;

ALTER TABLE organizations ADD COLUMN max_gpu INT NULL;
ALTER TABLE organizations ADD COLUMN max_vram_gb INT NULL;
ALTER TABLE organizations ADD COLUMN max_volume_gib INT NULL;
ALTER TABLE organizations ADD COLUMN max_concurrent_sessions INT NULL;
