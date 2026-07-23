-- 0025 데이터셋 적재 상태 — NFS 정규경로(/dataset/<name>) 다운로드 진행 상태.
-- loading=NFS 다운로드 중, ready=완료(세션 마운트 가능), failed=실패. 기존 행은 ready.
ALTER TABLE datasets ADD COLUMN load_status VARCHAR(16) NOT NULL DEFAULT 'ready';
