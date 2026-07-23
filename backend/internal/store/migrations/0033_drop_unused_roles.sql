-- billing_admin·guest 역할 제거(사용처 없음).
--
--  billing_admin: 등급이 project_admin 과 같아(rank 2) 이름과 달리 멤버 추가/삭제/역할변경까지
--                 가능했다. 정산 전용 권한은 코드에 존재한 적이 없다 → member 로 내린다.
--  guest        : member 와 구분하는 검사가 한 곳도 없어 사실상 동일했다 → member 로 통일한다.
--
-- 남아 있던 billing_admin 계정은 그룹 관리 권한을 잃는다(원래 갖지 말았어야 할 권한).
-- 팀 관리가 필요한 계정은 관리자가 그룹 상세에서 '팀장'으로 다시 지정해야 한다.
UPDATE memberships SET role = 'member' WHERE role IN ('billing_admin', 'guest');
