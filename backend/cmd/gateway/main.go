// Command gateway는 Giosk 세션 접속 게이트웨이(웹 리버스 프록시 + SSH 프록시)를 실행한다.
// 세션별 서브도메인(<iid>-<ch>.<domain>) 웹 접속과 복붙 SSH(username=1회 토큰)를 단일 접점으로 제공한다.
// 상태를 갖지 않으며(DB·k8s API 불요), API 와 공유하는 대칭키로 접속 토큰을 검증한다.
package main

import (
	"crypto/tls"
	"log"
	"net/http"
	"strconv"

	"giosk/internal/gateway"
)

func main() {
	cfg := gateway.Load()
	if len(cfg.Secret) == 0 {
		log.Fatal("GIOSK_GATEWAY_SECRET 미설정: API 와 공유하는 토큰 키가 필요합니다")
	}
	log.Printf("[gateway] domain=%s tls=%v sshPort=%d", cfg.Domain, cfg.TLSEnabled(), cfg.SSHPort)

	// SSH 프록시(백그라운드).
	if sp, err := gateway.NewSSHProxy(cfg); err != nil {
		log.Printf("[gateway] SSH 프록시 비활성: %v", err)
	} else {
		go func() {
			addr := ":" + strconv.Itoa(cfg.SSHPort)
			if err := sp.Listen(addr); err != nil {
				log.Fatalf("[gateway] SSH 프록시 종료: %v", err)
			}
		}()
	}

	// 웹 리버스 프록시.
	proxy := gateway.NewProxy(cfg)
	if cfg.TLSEnabled() {
		go serveHTTPRedirect(cfg.HTTPAddr, cfg.HTTPSAddr)
		srv := &http.Server{Addr: cfg.HTTPSAddr, Handler: proxy, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
		log.Printf("[gateway] HTTPS 리스닝 %s", cfg.HTTPSAddr)
		log.Fatal(srv.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey))
	}
	log.Printf("[gateway] HTTP 리스닝 %s (TLS 비활성)", cfg.HTTPAddr)
	log.Fatal(http.ListenAndServe(cfg.HTTPAddr, proxy))
}

// serveHTTPRedirect는 평문 HTTP 요청을 HTTPS 로 리다이렉트한다(TLS 활성 시).
func serveHTTPRedirect(httpAddr, _ string) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		u.Scheme, u.Host = "https", r.Host
		http.Redirect(w, r, u.String(), http.StatusMovedPermanently)
	})
	_ = http.ListenAndServe(httpAddr, h)
}
