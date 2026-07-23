-- 0002 자원 정의 — 오퍼링/프리셋/이미지 (설계 문서 §3.2 C)
CREATE TABLE IF NOT EXISTS offerings (
  id            BIGINT PRIMARY KEY AUTO_INCREMENT,
  name          VARCHAR(64) NOT NULL,
  gpu_type      VARCHAR(32),
  vram_mb       INT NOT NULL DEFAULT 0,
  core_percent  INT NOT NULL DEFAULT 0,
  mode          VARCHAR(16) NOT NULL DEFAULT 'fractional', -- fractional|exclusive|mig
  price_per_hour INT NOT NULL DEFAULT 0,
  is_active     BOOLEAN NOT NULL DEFAULT TRUE,
  created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS presets (
  id           BIGINT PRIMARY KEY AUTO_INCREMENT,
  name         VARCHAR(64) NOT NULL,
  display_name VARCHAR(128),
  icon         VARCHAR(16),
  vram_mb      INT NOT NULL DEFAULT 0,
  core_percent INT NOT NULL DEFAULT 0,
  sort_order   INT NOT NULL DEFAULT 0,
  is_active    BOOLEAN NOT NULL DEFAULT TRUE,
  created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS images (
  id          BIGINT PRIMARY KEY AUTO_INCREMENT,
  name        VARCHAR(64) NOT NULL,
  tag         VARCHAR(64),
  base_image  VARCHAR(128),
  dockerfile  TEXT,
  description VARCHAR(255),
  channels    JSON,                       -- {"vscode":bool,"jupyter":bool,"ssh":true}
  tags        JSON,
  gpu         BOOLEAN NOT NULL DEFAULT TRUE,
  scan_status VARCHAR(16),                -- pass|high|building
  signed      BOOLEAN NOT NULL DEFAULT FALSE,
  status      VARCHAR(16) NOT NULL DEFAULT 'draft', -- active|draft|building
  created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
