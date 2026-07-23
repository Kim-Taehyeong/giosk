-- 0020 노드 스크래치 활성 플래그 — 관리자가 노드별로 스크래치(노드로컬 hostPath)를 켜고 끈다.
-- 켜면 노드에 라벨 giosk.io/scratch=true 부여 → scratch-cleaner DaemonSet 이 해당 노드에서 동작.
ALTER TABLE nodes ADD COLUMN scratch_enabled BOOLEAN NOT NULL DEFAULT FALSE;
