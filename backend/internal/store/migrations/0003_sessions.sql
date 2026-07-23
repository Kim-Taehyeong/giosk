-- 0003 세션 — 인스턴스 + 볼륨 마운트 + 캐시 데이터셋 (설계 문서 §3.2 D)
CREATE TABLE IF NOT EXISTS sessions (
  id           BIGINT PRIMARY KEY AUTO_INCREMENT,
  instance_id  VARCHAR(50) NOT NULL UNIQUE,           -- ses_xxx
  user_id      BIGINT NOT NULL,
  group_id     BIGINT,
  name         VARCHAR(100),
  env          VARCHAR(16) NOT NULL DEFAULT 'container', -- container | ssh
  gpu_mode     VARCHAR(16),                            -- shared | exclusive | cpu
  offering_id  BIGINT,
  preset_id    BIGINT,
  image_id     BIGINT,
  vram_mb      INT NOT NULL DEFAULT 0,
  core_percent INT NOT NULL DEFAULT 0,
  cpu_cores    INT NOT NULL DEFAULT 0,
  mem_gb       INT NOT NULL DEFAULT 0,
  gpu_count    INT NOT NULL DEFAULT 1,
  gpu_type     VARCHAR(32),
  node         VARCHAR(64),
  ip_address   VARCHAR(40),
  phase        VARCHAR(20) NOT NULL DEFAULT 'provisioning', -- queued|provisioning|running|stopped|paused|terminated
  created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_sessions_user (user_id),
  INDEX idx_sessions_group (group_id)
);

CREATE TABLE IF NOT EXISTS session_volumes (
  session_id BIGINT NOT NULL,
  volume_id  BIGINT NOT NULL,
  mount_path VARCHAR(255),
  perm       VARCHAR(4),                              -- ro | rw
  PRIMARY KEY (session_id, volume_id)
);

CREATE TABLE IF NOT EXISTS session_datasets (
  session_id BIGINT NOT NULL,
  dataset_id BIGINT NOT NULL,
  node       VARCHAR(64),
  PRIMARY KEY (session_id, dataset_id)
);
