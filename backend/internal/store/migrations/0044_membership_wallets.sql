-- 크레딧 지갑을 멤버십(유저×팀) 단위로 전환한다. group_id = 팀(그룹) id.
-- 팀 없는 세션은 금지하므로 group_id 는 항상 실제 팀(개인지갑 특례 없음).
-- 기존 단일 잔액은 사용자의 대표(최소 membership id) 활성 팀으로 귀속한다(없으면 0=미귀속, 세션 불가).
ALTER TABLE user_wallets ADD COLUMN group_id BIGINT NOT NULL DEFAULT 0;

UPDATE user_wallets uw
   SET group_id = COALESCE(
     (SELECT m.group_id FROM memberships m
       WHERE m.user_id = uw.user_id AND m.status = 'active'
       ORDER BY m.id LIMIT 1), 0);

ALTER TABLE user_wallets DROP PRIMARY KEY, ADD PRIMARY KEY (user_id, group_id);

-- 원장도 팀 귀속(팀별 소비 showback). 기존 행은 0(미귀속).
ALTER TABLE credit_transactions ADD COLUMN group_id BIGINT NOT NULL DEFAULT 0;
CREATE INDEX idx_ctx_user_group ON credit_transactions (user_id, group_id, created_at);
