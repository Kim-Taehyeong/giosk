-- HAMi 분할 GPU 단위 단가(gpuType별): VRAM 1GB/시간, 코어 1%/시간.
-- price_per_hour(기존)는 전용(exclusive) 1 GPU/시간.
ALTER TABLE gpu_pricing ADD COLUMN price_per_gb INT NOT NULL DEFAULT 0;
ALTER TABLE gpu_pricing ADD COLUMN price_per_core INT NOT NULL DEFAULT 0;
