-- 0015 데이터셋 스토리지 — 데이터셋을 RWX NFS PVC 로 실체화(세션에 RO 마운트).
-- 데이터셋은 전 사용자가 같은 데이터를 읽기전용 공유하므로 NFS(RWX) 고정.
ALTER TABLE datasets ADD COLUMN pvc_name VARCHAR(128);
ALTER TABLE datasets ADD COLUMN pvc_namespace VARCHAR(128);
