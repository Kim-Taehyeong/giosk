-- 0039 사용자 인앱 알림 수신함 — 알림 엔진이 사용자 규칙(크레딧/예산/볼륨/유휴) 위반 시 여기에 적재한다.
-- 사용자는 알림센터에서 받은 알림을 보고, 토픽바 종 배지로 미읽음 수를 확인한다.
CREATE TABLE IF NOT EXISTS user_notifications (
  id         BIGINT PRIMARY KEY AUTO_INCREMENT,
  user_id    BIGINT       NOT NULL,
  severity   VARCHAR(16)  NOT NULL DEFAULT 'warn',  -- info | warn | err
  metric     VARCHAR(32)  NOT NULL DEFAULT '',      -- 발화 지표(중복 억제용)
  title      VARCHAR(190) NOT NULL,
  body       VARCHAR(500) NOT NULL DEFAULT '',
  read_at    TIMESTAMP    NULL,
  created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_user_notifications_user (user_id, id)
);
