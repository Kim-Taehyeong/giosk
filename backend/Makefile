# Giosk backend — 개발 태스크
.PHONY: build test vet run tidy

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

# 로컬 API 실행 (kind + mysql 이 떠 있어야 함 — scripts/dev-up.sh)
run:
	GIOSK_DB_HOST=127.0.0.1 GIOSK_DB_USER=giosk GIOSK_DB_PASS=giosk go run ./cmd/api
