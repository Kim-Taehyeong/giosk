-- 전용 GPU 시간당 단가(gpuType별). 공유/CPU 는 offerings.price_per_hour 사용.
CREATE TABLE IF NOT EXISTS gpu_pricing (
  gpu_type       VARCHAR(64)  NOT NULL,
  price_per_hour INT          NOT NULL DEFAULT 0,
  updated_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (gpu_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
