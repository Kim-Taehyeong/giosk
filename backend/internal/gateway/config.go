package gateway

import (
	"os"
	"strconv"
)

// Config는 게이트웨이 바이너리의 자체 설정(대형 API config 를 import 하지 않음 — 데이터 플레인 최소 의존).
type Config struct {
	Domain       string // 접속 도메인(예: gw.dku.ac.kr). 서브도메인 = <iid>-<ch>.<Domain>
	Secret       []byte // API 와 공유하는 토큰 대칭키(GIOSK_GATEWAY_SECRET)
	SSHPort      int    // 외부 SSH 프록시 리스닝 포트(기본 2222)
	SSHHostKey   []byte // 게이트웨이 SSH 서버 호스트키(PEM). 빈값이면 기동 시 임시 생성.
	SSHClientKey []byte // 백엔드(컨테이너 sshd/물리 계정)로 재접속할 게이트웨이 관리 개인키(PEM)
	TLSCert      string // TLS 인증서 PEM(파일 경로). 빈값이면 HTTP 만.
	TLSKey       string // TLS 개인키 PEM(파일 경로)
	HTTPAddr     string // HTTP 리스닝(기본 :80)
	HTTPSAddr    string // HTTPS 리스닝(기본 :443)
	CookieTTL    int    // 웹 세션 쿠키 수명(초, 기본 8h)
	ConsoleURL   string // 콘솔 URL(접근 거부 페이지의 "콘솔로 가기" 버튼용). 빈값이면 버튼 생략.
}

// Load는 환경변수에서 게이트웨이 설정을 읽는다.
func Load() Config {
	c := Config{
		Domain:       env("GIOSK_GATEWAY_DOMAIN", "gw.giosk.local"),
		Secret:       []byte(os.Getenv("GIOSK_GATEWAY_SECRET")),
		SSHPort:      envInt("GIOSK_GATEWAY_SSH_PORT", 2222),
		SSHHostKey:   []byte(os.Getenv("GIOSK_GATEWAY_SSH_HOST_KEY")),
		SSHClientKey: []byte(os.Getenv("GIOSK_GATEWAY_SSH_KEY")),
		TLSCert:      os.Getenv("GIOSK_GATEWAY_TLS_CERT"),
		TLSKey:       os.Getenv("GIOSK_GATEWAY_TLS_KEY"),
		HTTPAddr:     env("GIOSK_GATEWAY_HTTP_ADDR", ":80"),
		HTTPSAddr:    env("GIOSK_GATEWAY_HTTPS_ADDR", ":443"),
		CookieTTL:    envInt("GIOSK_GATEWAY_COOKIE_TTL", 8*3600),
		ConsoleURL:   os.Getenv("GIOSK_GATEWAY_CONSOLE_URL"),
	}
	return c
}

// TLSEnabled는 인증서/키 경로가 모두 설정됐는지 여부.
func (c Config) TLSEnabled() bool { return c.TLSCert != "" && c.TLSKey != "" }

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
