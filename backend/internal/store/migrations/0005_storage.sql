-- 0005 스토리지/데이터셋 (설계 §3.2 F)
CREATE TABLE IF NOT EXISTS volumes (
  id            BIGINT PRIMARY KEY AUTO_INCREMENT,
  name          VARCHAR(128) NOT NULL,
  kind          VARCHAR(16) NOT NULL DEFAULT 'personal', -- personal|group|dataset
  owner_user_id BIGINT,
  group_id      BIGINT,
  size_gib      INT NOT NULL DEFAULT 0,
  used_gib      INT NOT NULL DEFAULT 0,
  access_mode   VARCHAR(8) NOT NULL DEFAULT 'RWO',       -- RWO|RWX
  pvc_name      VARCHAR(128),
  pvc_namespace VARCHAR(128),
  status        VARCHAR(16) NOT NULL DEFAULT 'pending',  -- pending|bound|failed
  created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_vol_owner (owner_user_id),
  INDEX idx_vol_group (group_id)
);

CREATE TABLE IF NOT EXISTS volume_shares (
  id                  BIGINT PRIMARY KEY AUTO_INCREMENT,
  volume_id           BIGINT NOT NULL,
  shared_with_user_id BIGINT,
  shared_with_group_id BIGINT,
  permission          VARCHAR(4) NOT NULL DEFAULT 'ro',  -- ro|rw
  created_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_share_vol (volume_id)
);

CREATE TABLE IF NOT EXISTS datasets (
  id            BIGINT PRIMARY KEY AUTO_INCREMENT,
  name          VARCHAR(128) NOT NULL,
  scope         VARCHAR(16) NOT NULL DEFAULT 'global',   -- global|personal
  owner         VARCHAR(64),
  owner_user_id BIGINT,
  size_class    VARCHAR(16),                             -- Small|Medium|Large
  size_gb       INT,
  hash          VARCHAR(80),
  format        VARCHAR(32),
  description   VARCHAR(255),
  status        VARCHAR(16) NOT NULL DEFAULT 'active',   -- active|pending|private
  created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_ds_scope (scope, status)
);

CREATE TABLE IF NOT EXISTS dataset_node_cache (
  dataset_id BIGINT NOT NULL,
  node       VARCHAR(64) NOT NULL,
  path       VARCHAR(255),
  cached_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (dataset_id, node)
);

CREATE TABLE IF NOT EXISTS dataset_requests (
  id               BIGINT PRIMARY KEY AUTO_INCREMENT,
  name             VARCHAR(128) NOT NULL,
  requester_user_id BIGINT NOT NULL,
  size_class       VARCHAR(16),
  size_gb          INT,
  hash             VARCHAR(80),
  target_scope     VARCHAR(16) NOT NULL DEFAULT 'global', -- global|personal
  status           VARCHAR(16) NOT NULL DEFAULT 'pending', -- pending|approved|rejected
  reviewer_user_id BIGINT,
  created_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  reviewed_at      TIMESTAMP NULL,
  INDEX idx_dsreq_status (status)
);
