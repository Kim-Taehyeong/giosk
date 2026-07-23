-- 0006 노드 + 임대 (설계 §3.2 C·D)
-- nodes 는 라이브 K8s 노드에 대한 DB 오버레이(설정/임대/접근). 라이브 상태는 K8s 에서 머지.
CREATE TABLE IF NOT EXISTS nodes (
  name             VARCHAR(64) PRIMARY KEY,
  cluster          VARCHAR(64),
  gpu_desc         VARCHAR(64),
  cpu_desc         VARCHAR(32),
  mem_desc         VARCHAR(32),
  disk_desc        VARCHAR(32),
  share_mode       VARCHAR(16) NOT NULL DEFAULT 'exclusive', -- exclusive | hami
  split_count      INT NOT NULL DEFAULT 10,
  home_quota_gb    INT NOT NULL DEFAULT 0,
  dataset_quota_gb INT NOT NULL DEFAULT 0,
  ssh_enabled      BOOLEAN NOT NULL DEFAULT FALSE,
  physical_lease   BOOLEAN NOT NULL DEFAULT FALSE,
  local_accounts   JSON,
  temp_invites     JSON,
  updated_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS node_leases (
  id              BIGINT PRIMARY KEY AUTO_INCREMENT,
  node            VARCHAR(64) NOT NULL,
  user_id         BIGINT NOT NULL,
  instance_id     VARCHAR(50),
  status          VARCHAR(16) NOT NULL DEFAULT 'active', -- active | expired | released
  lease_hours     INT NOT NULL,
  extensions_used INT NOT NULL DEFAULT 0,
  started_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  expires_at      TIMESTAMP NOT NULL,
  released_at     TIMESTAMP NULL,
  INDEX idx_lease_node_status (node, status),
  INDEX idx_lease_user (user_id)
);
