-- 0023 데이터셋 소스 URL — 등록 시 입력한 다운로드 URL. 승인 시 이 URL 을 PVC 로 wget 적재한다.
ALTER TABLE dataset_requests ADD COLUMN source_url VARCHAR(1024) NOT NULL DEFAULT '';
ALTER TABLE datasets ADD COLUMN source_url VARCHAR(1024) NOT NULL DEFAULT '';
