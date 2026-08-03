-- 세션 단위 알림: 규칙에 대상 세션(instance_id)을 붙인다. 빈값=사용자 전역 규칙(기존 동작).
-- session_gpu/cpu/vram 지표는 이 대상 세션에서 평가한다. 인앱 수신함도 어느 세션인지 표시하도록 target 추가.
ALTER TABLE notify_rules ADD COLUMN target VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE user_notifications ADD COLUMN target VARCHAR(128) NOT NULL DEFAULT '';
