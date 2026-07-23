-- 0034 공지 타겟 — 전역(둘 다 NULL) 또는 특정 조직/그룹 대상.
-- org/group 관리자가 자기 범위에만 공지를 띄울 수 있게 한다(사용자 노출은 멤버십으로 필터).
ALTER TABLE announcements
  ADD COLUMN target_org_id   BIGINT NULL AFTER pinned,
  ADD COLUMN target_group_id BIGINT NULL AFTER target_org_id,
  ADD INDEX idx_ann_target_org (target_org_id),
  ADD INDEX idx_ann_target_group (target_group_id);
