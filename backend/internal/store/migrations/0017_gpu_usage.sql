-- 0017 GPU 사용량 원장 — 과금 정산(settle) 시 사용 시간(초)을 적립.
-- 세션이 삭제돼도 누적 사용시간(대시보드 '이번달 GPU 사용 시간')이 유지되도록 별도 원장에 기록.
CREATE TABLE IF NOT EXISTS gpu_usage (
  id          BIGINT PRIMARY KEY AUTO_INCREMENT,
  user_id     BIGINT NOT NULL,
  session_ref VARCHAR(64),
  gpu_count   INT NOT NULL DEFAULT 1,
  seconds     INT NOT NULL,
  created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_gpu_usage_user (user_id, created_at)
);
