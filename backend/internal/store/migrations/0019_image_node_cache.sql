-- 0019 이미지 노드 캐시 — 특정 이미지를 특정 노드에 미리 풀(prefetch)해 세션 기동 지연 감소.
CREATE TABLE IF NOT EXISTS image_node_cache (
  image_id   BIGINT NOT NULL,
  node       VARCHAR(190) NOT NULL,
  status     VARCHAR(16) NOT NULL DEFAULT 'pulling', -- pulling | cached | failed
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (image_id, node)
);
