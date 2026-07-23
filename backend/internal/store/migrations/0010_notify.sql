-- 알림 규칙/채널: 사용자(scope=user, owner=user_id) 및 관리자(scope=admin, owner=0).
CREATE TABLE IF NOT EXISTS notify_rules (
  id         BIGINT      NOT NULL AUTO_INCREMENT,
  scope      VARCHAR(16) NOT NULL DEFAULT 'user',  -- user | admin
  owner_id   BIGINT      NOT NULL DEFAULT 0,
  metric     VARCHAR(32) NOT NULL,
  op         VARCHAR(8)  NOT NULL DEFAULT 'gte',    -- lte | gte
  value      INT         NOT NULL DEFAULT 0,
  channel    VARCHAR(16) NOT NULL DEFAULT 'email',  -- email | webhook
  enabled    BOOLEAN     NOT NULL DEFAULT TRUE,
  created_at DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_nrule_owner (scope, owner_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS notify_channels (
  id         BIGINT       NOT NULL AUTO_INCREMENT,
  scope      VARCHAR(16)  NOT NULL DEFAULT 'user',
  owner_id   BIGINT       NOT NULL DEFAULT 0,
  kind       VARCHAR(16)  NOT NULL DEFAULT 'email',  -- email | webhook
  target     VARCHAR(255) NOT NULL,
  created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_nchan_owner (scope, owner_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
