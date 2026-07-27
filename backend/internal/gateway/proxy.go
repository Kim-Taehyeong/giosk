package gateway

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// cookieName은 access 교환 후 웹 세션을 유지하는 게이트웨이 쿠키명.
const cookieName = "gw_sess"

// Proxy는 세션별 서브도메인(<iid>-<ch>.<domain>) 웹 리버스 프록시.
type Proxy struct {
	cfg   Config
	nonce *nonceCache
	now   func() time.Time
	// dial은 업스트림(세션 Service) 접속 다이얼러. 기본 net.Dialer; 테스트에서 로컬 리다이렉트.
	dial func(ctx context.Context, network, addr string) (net.Conn, error)
}

// NewProxy는 웹 프록시를 만든다.
func NewProxy(cfg Config) *Proxy {
	d := &net.Dialer{Timeout: 8 * time.Second}
	return &Proxy{cfg: cfg, nonce: newNonceCache(), now: func() time.Time { return time.Now().UTC() }, dial: d.DialContext}
}

// ServeHTTP는 요청을 처리한다: ①?access=<토큰> 교환 → 쿠키 발급·리다이렉트, ②쿠키 검증 → 대상 세션으로 프록시.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// ws 업그레이드만 로깅한다(정적 자원 GET 은 스팸이라 제외). 접속 문제 진단의 관심사는
	// WebSocket 이고, 거절은 아래에서 사유와 함께 별도로 남긴다.
	isWS := strings.EqualFold(r.Header.Get("Upgrade"), "websocket")

	sub, ok := p.subdomain(r.Host)
	if !ok {
		log.Printf("[gateway] DENY host=%s reason=unknown-host", r.Host)
		http.Error(w, "Giosk gateway: unknown host", http.StatusNotFound)
		return
	}

	// ① access 토큰 교환(1회) — 쿠키 세팅 후 URL 에서 토큰 제거하며 "/" 로 리다이렉트.
	if tok := r.URL.Query().Get("access"); tok != "" {
		p.exchange(w, r, sub, tok)
		return
	}

	// ② 세션 쿠키 검증.
	ck, err := r.Cookie(cookieName)
	if err != nil {
		log.Printf("[gateway] DENY host=%s reason=no-cookie ws=%v", r.Host, isWS)
		p.denied(w, "세션이 만료되었거나 접속 링크가 필요합니다. 콘솔에서 다시 열어주세요.")
		return
	}
	claims, err := Verify(ck.Value, p.cfg.Secret, p.now())
	if err != nil || claims.Typ != TypCookie || sub != claims.IID+"-"+claims.Ch {
		log.Printf("[gateway] DENY host=%s reason=bad-cookie verifyErr=%v typ=%v subMatch=%v",
			r.Host, err, func() any { if claims != nil { return claims.Typ }; return "nil" }(),
			claims != nil && sub == claims.IID+"-"+claims.Ch)
		p.denied(w, "세션 쿠키가 유효하지 않습니다. 콘솔에서 다시 열어주세요.")
		return
	}

	if isWS {
		log.Printf("[gateway] ws %s -> %s:%d", r.Host, claims.ServiceHost(), claims.Port)
	}
	p.reverseProxy(claims).ServeHTTP(w, r)
}

// exchange는 access 토큰을 검증하고(anti-swap·단일사용) 백엔드 인증을 프라임한 뒤
// 세션 쿠키를 발급하고 "/" 로 리다이렉트한다.
func (p *Proxy) exchange(w http.ResponseWriter, r *http.Request, sub, tok string) {
	claims, err := Verify(tok, p.cfg.Secret, p.now())
	if err != nil {
		p.denied(w, "접속 토큰이 만료되었거나 유효하지 않습니다.")
		return
	}
	// anti-swap: 요청 서브도메인 == 토큰 클레임(iid-ch). vscode 토큰을 jupyter 서브도메인에 못 씀.
	if claims.Typ != TypWeb || sub != claims.IID+"-"+claims.Ch {
		p.denied(w, "접속 토큰이 이 주소와 일치하지 않습니다.")
		return
	}
	if !p.nonce.use(claims.Jti, claims.Exp, p.now()) {
		p.denied(w, "이미 사용된 접속 링크입니다. 콘솔에서 다시 열어주세요.")
		return
	}

	// 백엔드(code-server/jupyter) 인증을 서버측에서 프라임 → 브라우저에 그 인증 쿠키를 전달
	// (사용자에게 원본 비밀 미노출). jupyter 는 Authorization 헤더 주입으로 처리하므로 쿠키 불필요.
	for _, c := range p.primeCookies(claims) {
		http.SetCookie(w, c)
	}

	// 라우팅 쿠키(gw_sess) — 이후 요청의 대상 해석용. 비밀 포함, HttpOnly.
	cookieClaims := *claims
	cookieClaims.Typ = TypCookie
	cookieClaims.Jti = ""
	cookieClaims.Exp = p.now().Add(time.Duration(p.cfg.CookieTTL) * time.Second).Unix()
	val, err := Sign(cookieClaims, p.cfg.Secret)
	if err != nil {
		http.Error(w, "gateway sign error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: val, Path: "/",
		HttpOnly: true, Secure: p.cfg.TLSEnabled(), SameSite: http.SameSiteLaxMode,
		MaxAge: p.cfg.CookieTTL,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

// reverseProxy는 claims 대상 세션 Service 로 향하는 리버스 프록시를 만든다(WebSocket 대응).
func (p *Proxy) reverseProxy(claims *Claims) *httputil.ReverseProxy {
	target := &url.URL{Scheme: "http", Host: fmt.Sprintf("%s:%d", claims.ServiceHost(), claims.Port)}
	rp := httputil.NewSingleHostReverseProxy(target)
	rp.Transport = &http.Transport{DialContext: p.dial} // 테스트 다이얼 시드 재사용
	rp.FlushInterval = -1                               // 스트리밍(터미널·커널 WS) 즉시 flush
	base := rp.Director
	rp.Director = func(req *http.Request) {
		base(req)
		// Host 헤더는 원본(외부 서브도메인)을 그대로 둔다 — code-server·jupyter 는 WebSocket
		// 업그레이드 때 Origin 을 Host 와 대조하는 CSRF 검사를 한다. Host 를 내부 서비스 주소로
		// 덮으면 브라우저가 보낸 Origin(외부 호스트)과 불일치해 백엔드가 403 을 반환하고, 브라우저는
		// 이를 ws 1006 으로 본다(HTTP 자원은 Origin 이 없어 통과 → 페이지만 뜨고 ws 만 죽는 증상).
		// 업스트림 접속은 req.URL.Host(target)로 다이얼하므로 Host 헤더를 바꿀 필요가 없다
		// (표준 nginx `proxy_set_header Host $host` 와 동일).
		injectRequestAuth(claims, req) // jupyter: Authorization 헤더 주입
	}
	rp.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		log.Printf("[gateway] proxy %s error: %v", claims.IID, err)
		http.Error(w, "세션에 연결할 수 없습니다(기동 중이거나 종료됨).", http.StatusBadGateway)
	}
	return rp
}

// subdomain은 Host 에서 <iid>-<ch> 서브도메인 라벨을 추출한다(포트·도메인 접미사 제거).
func (p *Proxy) subdomain(host string) (string, bool) {
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	host = strings.TrimSuffix(host, ".")
	suffix := "." + p.cfg.Domain
	if !strings.HasSuffix(host, suffix) {
		return "", false
	}
	sub := strings.TrimSuffix(host, suffix)
	if sub == "" || strings.Contains(sub, ".") { // 단일 라벨(<iid>-<ch>)만 허용
		return "", false
	}
	return sub, true
}

// denied는 접근 거부 안내를 렌더한다(가입대기 페이지와 같은 톤: 아이콘·문구·콘솔 버튼).
// ConsoleURL 이 설정돼 있으면 "콘솔에서 다시 열기" 버튼을 함께 보여준다(막다른 길 방지).
func (p *Proxy) denied(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	btn := ""
	if p.cfg.ConsoleURL != "" {
		btn = fmt.Sprintf(`<a href="%s" style="display:inline-flex;align-items:center;gap:8px;margin-top:24px;padding:11px 20px;background:#4f46e5;color:#fff;border-radius:10px;text-decoration:none;font-weight:700;font-size:14px">`+
			`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4"/><polyline points="10 17 15 12 10 7"/><line x1="15" y1="12" x2="3" y2="12"/></svg>`+
			`콘솔에서 다시 열기</a>`, p.cfg.ConsoleURL)
	}
	fmt.Fprintf(w, `<!doctype html><html lang="ko"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Giosk</title></head>`+
		`<body style="margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;font-family:system-ui,-apple-system,'Segoe UI',Roboto,sans-serif;background:#f8fafc;color:#0f172a">`+
		`<div style="max-width:440px;padding:40px 32px;text-align:center">`+
		`<div style="width:64px;height:64px;margin:0 auto 20px;border-radius:16px;background:#fef2f2;display:flex;align-items:center;justify-content:center">`+
		`<svg width="30" height="30" viewBox="0 0 24 24" fill="none" stroke="#dc2626" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg></div>`+
		`<h1 style="margin:0 0 10px;font-size:20px;font-weight:800">접속할 수 없습니다</h1>`+
		`<p style="margin:0;color:#64748b;font-size:14px;line-height:1.6">%s</p>%s</div></body></html>`, msg, btn)
}
