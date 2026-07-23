-- 0026 데이터셋 노드 로컬 캐시 상태 — 각 물리노드 로컬 디스크(hostPath) 캐시의 진행 상태.
-- caching=복사 중, cached=완료(세션이 hostPath 로 마운트), failed=실패. 기존 행은 cached.
ALTER TABLE dataset_node_cache ADD COLUMN status VARCHAR(16) NOT NULL DEFAULT 'cached';
