-- 0007 공지 (설계 §3.2 G)
CREATE TABLE IF NOT EXISTS announcements (
  id         BIGINT PRIMARY KEY AUTO_INCREMENT,
  level      VARCHAR(16) NOT NULL DEFAULT 'info',   -- info | warning | critical
  title      VARCHAR(200) NOT NULL,
  body       TEXT NOT NULL,
  active     BOOLEAN NOT NULL DEFAULT TRUE,
  pinned     BOOLEAN NOT NULL DEFAULT FALSE,
  created_by BIGINT,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_ann_active (active)
);
