-- 0037 NFS 기존 볼륨 임포트 — 도입 환경에 이미 데이터가 있는 NFS 경로를 관리자가 수기로 볼륨에 매핑.
-- 기존 볼륨은 항상 빈 vol-<id> 하위디렉터리를 동적 프로비저닝했다. 임포트 볼륨은 지정한
-- server:path 를 정적 PV+PVC 로 바인딩해 기존 데이터를 그대로 노출한다(nfs_server 비어있으면 일반 동적 볼륨).
ALTER TABLE volumes ADD COLUMN nfs_server VARCHAR(190) NOT NULL DEFAULT '';
ALTER TABLE volumes ADD COLUMN nfs_path   VARCHAR(255) NOT NULL DEFAULT '';
