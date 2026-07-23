# Giosk 백엔드 — api / node-agent / gateway / sshd 멀티타깃.
#   docker/nerdctl build --target api        -t giosk-api:<tag> .
#   docker/nerdctl build --target node-agent -t giosk-node-agent:<tag> .
#   docker/nerdctl build --target gateway    -t giosk-gateway:<tag> .
#   docker/nerdctl build --target sshd       -t giosk-sshd:<tag> .   # 세션 SSH 사이드카
ARG GO_VERSION=1.26

# ── 빌드 ──────────────────────────────────────────────────
FROM golang:${GO_VERSION} AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/node-agent ./cmd/node-agent
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/gateway ./cmd/gateway

# ── 공통 런타임 ───────────────────────────────────────────
FROM alpine:3.20 AS base
RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Seoul

# ── API (비루트) ─────────────────────────────────────────
FROM base AS api
RUN adduser -D -u 10001 giosk
USER giosk
COPY --from=build /out/api /usr/local/bin/api
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/api"]

# ── node-agent (호스트 계정/마운트 작업 → root, DaemonSet privileged) ──
# util-linux: nsenter(호스트 PID1 네임스페이스 진입, ShellExecutor 가 사용).
FROM base AS node-agent
RUN apk add --no-cache util-linux
COPY --from=build /out/node-agent /usr/local/bin/node-agent
ENTRYPOINT ["/usr/local/bin/node-agent"]

# ── gateway (세션 접속 리버스 프록시 + SSH 프록시; 비루트) ──
# 80/443(웹)·2222(SSH) 리스닝. stateless — DB·k8s API 불요.
FROM base AS gateway
RUN adduser -D -u 10002 giosk
USER giosk
COPY --from=build /out/gateway /usr/local/bin/gateway
EXPOSE 80 443 2222
ENTRYPOINT ["/usr/local/bin/gateway"]

# ── sshd 사이드카 (컨테이너 세션 SSH; root — 22 바인드·계정 생성) ──
# 게이트웨이 SSH 프록시만 상대한다: 신뢰 키 = GATEWAY_PUBKEY 하나. Go 바이너리 불요(빌드 스테이지 미참조).
FROM alpine:3.20 AS sshd
RUN apk add --no-cache openssh-server openssh-sftp-server tzdata && mkdir -p /etc/ssh/sshd_config.d
ENV TZ=Asia/Seoul
COPY build/sshd_config /etc/ssh/sshd_config
COPY build/sshd-entrypoint.sh /usr/local/bin/sshd-entrypoint.sh
RUN chmod +x /usr/local/bin/sshd-entrypoint.sh
EXPOSE 22
ENTRYPOINT ["/usr/local/bin/sshd-entrypoint.sh"]
