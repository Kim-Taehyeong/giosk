-- 0004 크레딧/예산 — 개인/그룹 거래 원장 + 그룹 지갑 + 충전 요청 (설계 §3.2 E)
-- (user_wallets 는 0001 에 이미 존재)

CREATE TABLE IF NOT EXISTS credit_transactions (
  id            BIGINT PRIMARY KEY AUTO_INCREMENT,
  user_id       BIGINT NOT NULL,
  type          VARCHAR(20) NOT NULL,            -- topup|hold|consume|settle|refund
  amount        INT NOT NULL,
  balance_after INT NOT NULL,
  ref           VARCHAR(64),                     -- instance_id 등
  description   VARCHAR(255),
  actor_user_id BIGINT,
  created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_ctx_user (user_id, created_at)
);

CREATE TABLE IF NOT EXISTS group_wallets (
  group_id            BIGINT PRIMARY KEY,
  balance             INT NOT NULL DEFAULT 0,
  reserved            INT NOT NULL DEFAULT 0,
  budget_cap          INT NOT NULL DEFAULT 0,    -- 0 = 무제한
  alert_threshold_pct INT NOT NULL DEFAULT 80,
  default_member_budget INT NOT NULL DEFAULT 0,
  cycle_started_at    TIMESTAMP NULL,
  updated_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS group_credit_transactions (
  id             BIGINT PRIMARY KEY AUTO_INCREMENT,
  group_id       BIGINT NOT NULL,
  type           VARCHAR(20) NOT NULL,           -- topup|hold|consume|settle|refund|budget_alert|admin_grant
  amount         INT NOT NULL,
  balance_after  INT NOT NULL,
  reserved_after INT NOT NULL DEFAULT 0,
  actor_user_id  BIGINT,
  session_ref    VARCHAR(64),
  created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_gtx_group (group_id, created_at)
);

CREATE TABLE IF NOT EXISTS topup_requests (
  id                BIGINT PRIMARY KEY AUTO_INCREMENT,
  requester_user_id BIGINT NOT NULL,
  target_type       VARCHAR(16) NOT NULL,        -- user|group|org
  target_id         BIGINT NOT NULL,
  amount            INT NOT NULL,
  reason            VARCHAR(255),
  status            VARCHAR(16) NOT NULL DEFAULT 'pending', -- pending|approved|rejected
  reviewer_user_id  BIGINT,
  review_note       VARCHAR(255),
  created_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  reviewed_at       TIMESTAMP NULL,
  INDEX idx_topup_status (status),
  INDEX idx_topup_target (target_type, target_id)
);
