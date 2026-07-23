-- 0018 GPU 사용량 원장에 그룹 귀속 추가 — 빌링 showback(그룹/조직별 GPU 시간) 집계용.
ALTER TABLE gpu_usage ADD COLUMN group_id BIGINT, ADD INDEX idx_gpu_usage_group (group_id, created_at);
