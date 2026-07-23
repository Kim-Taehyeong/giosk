-- 0024 데이터셋 실제 용량(바이트) — 등록 시 소스 URL 의 Content-Length 로 측정해 저장한다.
-- 표시는 사람이 읽기 좋은 단위(KB/MB/GB)로 환산. size_gb 는 PVC 용량 산정에 계속 사용.
ALTER TABLE datasets ADD COLUMN size_bytes BIGINT NOT NULL DEFAULT 0;
ALTER TABLE dataset_requests ADD COLUMN size_bytes BIGINT NOT NULL DEFAULT 0;
