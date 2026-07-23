-- 0038 runtime_config.cfg_value 를 TEXT 로 확장 — 브랜드 아이콘 업로드(data URL, 수 KB)를 저장하기 위함.
-- 기존 VARCHAR(255) 는 base64 data URL 을 담기엔 작다.
ALTER TABLE runtime_config MODIFY cfg_value TEXT NOT NULL;
