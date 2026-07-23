-- 감사 로그: 인증된 변경(POST/PUT/DELETE) 요청을 미들웨어가 자동 기록.
CREATE TABLE IF NOT EXISTS audit_logs (
  id             BIGINT       NOT NULL AUTO_INCREMENT,
  actor_id       BIGINT       NULL,
  actor_username VARCHAR(190) NOT NULL DEFAULT '',
  action         VARCHAR(190) NOT NULL DEFAULT '',
  target         VARCHAR(190) NOT NULL DEFAULT '',
  result         VARCHAR(32)  NOT NULL DEFAULT 'success',
  ip             VARCHAR(64)  NOT NULL DEFAULT '',
  created_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_audit_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
