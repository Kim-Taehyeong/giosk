-- 물리노드 스텁 폐기(설계 정립): 독점 전용 확정으로 대여 중 초대(temp_invites)·고정계정(local_accounts)
-- 개념 제거. 로컬홈 용량제한(home_quota_gb)은 ~/nfs 개인볼륨의 볼륨 GiB 하드쿼터로 통합되어 불필요.
ALTER TABLE nodes DROP COLUMN home_quota_gb;
ALTER TABLE nodes DROP COLUMN local_accounts;
ALTER TABLE nodes DROP COLUMN temp_invites;
