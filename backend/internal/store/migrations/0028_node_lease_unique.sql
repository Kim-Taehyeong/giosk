-- 0028 노드 임대 선착순 원자성 — 생성컬럼 + 유니크 인덱스로 경합을 DB가 보장.
-- shared: 자유모드=1(노드당 사용자별 1건 허용), 비-자유=0(노드당 1건=전용).
-- lease_key: 활성 임대만 키를 가짐(released/expired 는 NULL→유니크 무시).
--   비-자유(전용): key=node           → 노드당 활성 1건만(둘째는 1062 중복=ErrNodeBusy)
--   자유(공유):   key=node#user_id    → (노드,사용자)당 1건(둘째는 멱등 재사용)
-- 거의 동시 INSERT 도 유니크 제약으로 처음 1건만 성공 → 갭락/데드락 없이 선착순 보장.
ALTER TABLE node_leases ADD COLUMN shared TINYINT NOT NULL DEFAULT 0;
ALTER TABLE node_leases ADD COLUMN lease_key VARCHAR(160)
  GENERATED ALWAYS AS (
    CASE WHEN status <> 'active' THEN NULL
         WHEN shared = 1 THEN CONCAT(node, '#', user_id)
         ELSE node END
  ) STORED;
CREATE UNIQUE INDEX uniq_node_leases_active ON node_leases (lease_key);
