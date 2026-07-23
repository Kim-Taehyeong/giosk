-- 0036 경고 이벤트 이력 — 운영/인프라 경고를 시간순으로 한 화면(감시월)에 띄우기 위한 영속 피드.
-- 기존엔 notify 엔진이 웹훅/메일만 쏘고 로그만 남겨 이력이 없었고, adminAlerts 는 매 요청 현재상태에서 즉석 생성했다.
CREATE TABLE IF NOT EXISTS alert_events (
  id       BIGINT PRIMARY KEY AUTO_INCREMENT,
  ts       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  source   VARCHAR(16)  NOT NULL DEFAULT 'infra',  -- infra | ops
  severity VARCHAR(8)   NOT NULL DEFAULT 'warn',    -- info | warn | err
  type     VARCHAR(32)  NOT NULL DEFAULT '',        -- node_down | gpu_temp | gpu_util | disk_usage | budget | credit
  target   VARCHAR(190) NOT NULL DEFAULT '',
  message  VARCHAR(255) NOT NULL DEFAULT '',
  INDEX idx_alert_events_ts (ts)
);
