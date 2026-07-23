-- 0027 노드 임대 조회 인덱스 — (node, status). 노드별 활성 임대 조회/cordon 판정 가속.
CREATE INDEX idx_node_leases_node_status ON node_leases (node, status);
