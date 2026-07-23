-- 0021 node_leases.expires_at 를 TIMESTAMP→DATETIME 으로 확장.
-- TIMESTAMP 는 2038-01-19 까지만 표현 가능해, 자유(free) 모드의 영속 임대(만료 100년)를 담지 못한다.
-- DATETIME 은 9999 년까지 지원 → 영속 임대 표현 가능. started_at 도 함께 맞춘다.
ALTER TABLE node_leases MODIFY COLUMN expires_at DATETIME NOT NULL;
ALTER TABLE node_leases MODIFY COLUMN started_at DATETIME NULL DEFAULT CURRENT_TIMESTAMP;
