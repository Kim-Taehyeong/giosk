-- 0001 init — 인증/거버넌스 핵심 (설계 문서 §3.2 A·B)
CREATE TABLE IF NOT EXISTS users (
  id              BIGINT PRIMARY KEY AUTO_INCREMENT,
  username        VARCHAR(64) NOT NULL UNIQUE,
  password_hash   VARCHAR(255),
  email           VARCHAR(128),
  first_name      VARCHAR(64),
  last_name       VARCHAR(64),
  first_name_en   VARCHAR(64),
  last_name_en    VARCHAR(64),
  sponsor_name    VARCHAR(128),
  ssh_public_key  TEXT,
  role            VARCHAR(16) NOT NULL DEFAULT 'member',     -- admin | member
  status          VARCHAR(16) NOT NULL DEFAULT 'pending',    -- pending|approved|rejected|suspended
  terms_accepted_at TIMESTAMP NULL,
  created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS organizations (
  id                   BIGINT PRIMARY KEY AUTO_INCREMENT,
  name                 VARCHAR(64) NOT NULL UNIQUE,
  display_name         VARCHAR(128),
  description          VARCHAR(255),
  credit_pool          INT NOT NULL DEFAULT 0,
  default_group_budget INT NOT NULL DEFAULT 0,
  status               VARCHAR(16) NOT NULL DEFAULT 'active', -- active | archived
  created_at           TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at           TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS `groups` (
  id           BIGINT PRIMARY KEY AUTO_INCREMENT,
  org_id       BIGINT NOT NULL,
  name         VARCHAR(64) NOT NULL,
  display_name VARCHAR(128),
  cluster      VARCHAR(64),
  accepts_join BOOLEAN NOT NULL DEFAULT TRUE,
  status       VARCHAR(16) NOT NULL DEFAULT 'active',
  created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uniq_group_org_name (org_id, name),
  INDEX idx_group_org (org_id)
);

CREATE TABLE IF NOT EXISTS memberships (
  id         BIGINT PRIMARY KEY AUTO_INCREMENT,
  group_id   BIGINT NOT NULL,
  user_id    BIGINT NOT NULL,
  role       VARCHAR(20) NOT NULL DEFAULT 'member',  -- org_admin|project_admin|billing_admin|member|guest
  budget     INT NOT NULL DEFAULT 0,
  consumed   INT NOT NULL DEFAULT 0,
  status     VARCHAR(16) NOT NULL DEFAULT 'active',  -- active|invited|pending|removed
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uniq_member (group_id, user_id),
  INDEX idx_member_user (user_id)
);

CREATE TABLE IF NOT EXISTS group_join_requests (
  id               BIGINT PRIMARY KEY AUTO_INCREMENT,
  user_id          BIGINT NOT NULL,
  group_id         BIGINT NOT NULL,
  status           VARCHAR(16) NOT NULL DEFAULT 'pending',  -- pending|approved|rejected
  message          VARCHAR(255),
  reviewer_user_id BIGINT NULL,
  requested_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  reviewed_at      TIMESTAMP NULL,
  INDEX idx_join_group_status (group_id, status),
  INDEX idx_join_user (user_id)
);

-- 인증 세션키(로컬 인증). 미들웨어가 Authorization 헤더의 키로 사용자 조회.
-- (컴퓨트 세션 테이블 `sessions`(0003)과 이름 충돌 회피 위해 auth_sessions)
CREATE TABLE IF NOT EXISTS auth_sessions (
  session_key VARCHAR(128) PRIMARY KEY,
  user_id     BIGINT NOT NULL,
  created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  expires_at  TIMESTAMP NOT NULL,
  INDEX idx_sessions_user (user_id)
);

-- 개인 지갑 (설계 §3.2 E)
CREATE TABLE IF NOT EXISTS user_wallets (
  user_id          BIGINT PRIMARY KEY,
  balance          INT NOT NULL DEFAULT 0,
  reserved         INT NOT NULL DEFAULT 0,
  monthly_cap      INT NOT NULL DEFAULT 0,
  cycle_started_at TIMESTAMP NULL,
  updated_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
