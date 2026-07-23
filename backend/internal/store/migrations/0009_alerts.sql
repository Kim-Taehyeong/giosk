-- 워크로드 알림: 자원(GPU)/노드(SSH) 가용 시 알림 구독(사용자별).
CREATE TABLE IF NOT EXISTS workload_alerts (
  id         BIGINT       NOT NULL AUTO_INCREMENT,
  user_id    BIGINT       NOT NULL,
  type       VARCHAR(16)  NOT NULL DEFAULT 'gpu',  -- gpu | node
  target     VARCHAR(190) NOT NULL DEFAULT '',
  enabled    BOOLEAN      NOT NULL DEFAULT TRUE,
  created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_alert_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
