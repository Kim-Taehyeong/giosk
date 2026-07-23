-- 0042 사용자 알림 i18n — 렌더된 한국어 문자열 대신 metric+파라미터를 저장한다.
-- 프론트가 metric+value+threshold 로 현지화 렌더한다. 구 알림은 title/body(한국어)로 폴백 표시.
ALTER TABLE user_notifications ADD COLUMN value DOUBLE NOT NULL DEFAULT 0;
ALTER TABLE user_notifications ADD COLUMN threshold INT NOT NULL DEFAULT 0;
