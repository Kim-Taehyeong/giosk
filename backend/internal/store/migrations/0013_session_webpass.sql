-- 세션 웹 접속(code-server/Jupyter) 랜덤 비밀번호. Giosk 가 생성·소유자에게만 노출.
ALTER TABLE sessions ADD COLUMN web_password VARCHAR(64) NOT NULL DEFAULT '';
