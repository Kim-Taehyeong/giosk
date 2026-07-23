-- 0035 인프라 메트릭 스냅샷 — 대시보드 감시용 시계열.
-- Prometheus/k8s 는 라이브만 주고 우리 DB엔 인프라 이력이 없었다. 샘플러가 주기적으로 이 표에 적재해
-- Prometheus 보존기간/가동과 무관하게 장기 추이·유휴 비율을 대시보드가 그린다.
CREATE TABLE IF NOT EXISTS metric_snapshots (
  id               BIGINT PRIMARY KEY AUTO_INCREMENT,
  ts               TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  gpu_util         INT NOT NULL DEFAULT 0,  -- 평균 GPU 사용률 %
  vram_used_pct    INT NOT NULL DEFAULT 0,  -- VRAM 사용률 %
  gpu_temp_avg     INT NOT NULL DEFAULT 0,  -- 평균 GPU 온도 °C
  gpu_temp_max     INT NOT NULL DEFAULT 0,  -- 최고 GPU 온도 °C
  nodes_up         INT NOT NULL DEFAULT 0,
  nodes_total      INT NOT NULL DEFAULT 0,
  gpus_used        INT NOT NULL DEFAULT 0,  -- 점유 GPU 수(running 세션 기준)
  gpus_total       INT NOT NULL DEFAULT 0,  -- 전체 GPU 수(노드 capacity 합)
  running_sessions INT NOT NULL DEFAULT 0,
  idle_sessions    INT NOT NULL DEFAULT 0,  -- running 이지만 저사용(GPU util<5%)
  INDEX idx_metric_snap_ts (ts)
);
