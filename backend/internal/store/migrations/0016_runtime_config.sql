-- 0016 런타임 설정 — 운영 중 조정 가능한 단순 정책(유휴 타임아웃·기능 토글)의 영속 저장소.
-- 무거운 항목(과금모델·데이터셋 인프라·쿼터)은 설치시 env 고정이라 여기에 두지 않는다.
CREATE TABLE IF NOT EXISTS runtime_config (
  cfg_key    VARCHAR(64) PRIMARY KEY,
  cfg_value  VARCHAR(255) NOT NULL,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
