-- 가입 신청 시 선택한 그룹을 보관(승인 시 이 그룹의 active 멤버십을 생성).
-- newPendingUser 가 저장, users.SetStatus('approved') 가 멤버십 생성 후 NULL 로 비운다.
ALTER TABLE users ADD COLUMN signup_group_id BIGINT NULL;
