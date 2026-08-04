package gateway

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTokenRoundTrip(t *testing.T) {
	secret := []byte("test-shared-secret")
	now := time.Unix(1_700_000_000, 0)
	in := Claims{IID: "ses-abc", Ch: ChanVSCode, NS: "giosk-grp-1", Port: 8080,
		Sub: 42, Typ: TypWeb, Tgt: TgtContainer, Secret: "pw123", Exp: now.Add(2 * time.Minute).Unix(), Jti: "j1"}
	tok, err := Sign(in, secret)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Verify(tok, secret, now)
	if err != nil {
		t.Fatal(err)
	}
	if out.IID != in.IID || out.Ch != in.Ch || out.Secret != in.Secret || out.Port != in.Port {
		t.Fatalf("claims mismatch: %+v", out)
	}
}

func TestVerifyExpired(t *testing.T) {
	secret := []byte("s")
	now := time.Unix(1_700_000_000, 0)
	tok, _ := Sign(Claims{IID: "x", Exp: now.Add(-time.Second).Unix()}, secret)
	if _, err := Verify(tok, secret, now); err != ErrExpired {
		t.Fatalf("expected ErrExpired, got %v", err)
	}
}

func TestVerifyWrongKey(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tok, _ := Sign(Claims{IID: "x", Exp: now.Add(time.Minute).Unix()}, []byte("k1"))
	if _, err := Verify(tok, []byte("k2"), now); err != ErrToken {
		t.Fatalf("expected ErrToken, got %v", err)
	}
}

func TestSubdomain(t *testing.T) {
	p := NewProxy(Config{Domain: "gw.giosk.local"})
	cases := map[string]string{
		"ses-abc-vscode.gw.giosk.local":      "ses-abc-vscode",
		"ses-abc-jupyter.gw.giosk.local:443": "ses-abc-jupyter",
		"ses-abc-vscode.gw.giosk.local.":     "ses-abc-vscode",
	}
	for host, want := range cases {
		got, ok := p.subdomain(host)
		if !ok || got != want {
			t.Errorf("subdomain(%q)=%q,%v want %q", host, got, ok, want)
		}
	}
	bad := []string{"gw.giosk.local", "a.b.gw.giosk.local", "ses-abc-vscode.other.local", ""}
	for _, host := range bad {
		if _, ok := p.subdomain(host); ok {
			t.Errorf("subdomain(%q) should be rejected", host)
		}
	}
}

// TestExchangeFlow는 ?access 교환이 anti-swap/단일사용을 강제하고 성공 시 쿠키+302 를 내는지 검증한다.
// jupyter 채널은 헤더 주입이라 업스트림 프라임을 건너뛰므로 클러스터 없이 테스트 가능하다.
func TestExchangeFlow(t *testing.T) {
	secret := []byte("test")
	p := NewProxy(Config{Domain: "gw.giosk.local", Secret: secret, CookieTTL: 3600})
	p.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	exp := p.now().Add(2 * time.Minute).Unix()
	mk := func(jti string) string {
		tok, _ := Sign(Claims{IID: "ses-abc", Ch: ChanJupyter, NS: "giosk-grp-1", Port: 8888,
			Typ: TypWeb, Tgt: TgtContainer, Secret: "tok", Exp: exp, Jti: jti}, secret)
		return tok
	}
	host := "ses-abc-jupyter.gw.giosk.local"

	// 성공: 302 + gw_sess 쿠키.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://"+host+"/?access="+mk("j1"), nil)
	req.Host = host
	p.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rr.Code)
	}
	if !hasCookie(rr, cookieName) {
		t.Fatal("gw_sess cookie not set")
	}

	// 재사용(같은 jti): 거부(403).
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "http://"+host+"/?access="+mk("j1"), nil)
	req2.Host = host
	p.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusForbidden {
		t.Fatalf("token reuse should be 403, got %d", rr2.Code)
	}

	// anti-swap: jupyter 토큰을 vscode 서브도메인에 쓰면 거부된다.
	swapHost := "ses-abc-vscode.gw.giosk.local"
	rr3 := httptest.NewRecorder()
	req3 := httptest.NewRequest("GET", "http://"+swapHost+"/?access="+mk("j2"), nil)
	req3.Host = swapHost
	p.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusForbidden {
		t.Fatalf("cross-subdomain reuse should be 403, got %d", rr3.Code)
	}
}

// TestVSCodeFullFlow는 가짜 code-server 업스트림으로 교환, 로그인 프라임, 프록시 전 과정을 실제로 구동한다.
// 사용자는 비밀을 모르고 게이트웨이가 /login 을 대신 수행해 인증 쿠키를 브라우저에 전달, 이후 프록시가 200 을 낸다.
func TestVSCodeFullFlow(t *testing.T) {
	const pw = "secret-pw"
	// 가짜 code-server: 쿠키 없으면 401, POST /login {password} 맞으면 cs=ok 쿠키+302, 쿠키 있으면 200.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			if r.FormValue("password") == pw {
				http.SetCookie(w, &http.Cookie{Name: "cs", Value: "ok", Path: "/"})
				w.WriteHeader(http.StatusFound)
			} else {
				w.WriteHeader(http.StatusUnauthorized)
			}
			return
		}
		if c, err := r.Cookie("cs"); err == nil && c.Value == "ok" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("EDITOR-OK"))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer upstream.Close()
	upAddr := upstream.Listener.Addr().String()

	secret := []byte("test")
	p := NewProxy(Config{Domain: "gw.giosk.local", Secret: secret, CookieTTL: 3600})
	p.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	p.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, upAddr) // svc DNS 를 로컬 업스트림으로 리다이렉트
	}
	host := "ses-abc-vscode.gw.giosk.local"
	tok, _ := Sign(Claims{IID: "ses-abc", Ch: ChanVSCode, NS: "giosk-grp-1", Port: 8080,
		Typ: TypWeb, Tgt: TgtContainer, Secret: pw, Exp: p.now().Add(2 * time.Minute).Unix(), Jti: "v1"}, secret)

	// 1) 교환하면 302 와 함께 cs 쿠키(업스트림 로그인 결과), gw_sess 가 온다.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://"+host+"/?access="+tok, nil)
	req.Host = host
	p.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("exchange expected 302, got %d", rr.Code)
	}
	var gwCookie, csCookie *http.Cookie
	for _, c := range rr.Result().Cookies() {
		switch c.Name {
		case cookieName:
			gwCookie = c
		case "cs":
			csCookie = c
		}
	}
	if gwCookie == nil || csCookie == nil {
		t.Fatalf("expected gw_sess and cs cookies, got %v", rr.Result().Cookies())
	}

	// 2) 후속 요청(gw_sess 와 cs 쿠키)은 프록시 200 EDITOR-OK 를 받는다(비밀 노출 없이 인증됨).
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "http://"+host+"/", nil)
	req2.Host = host
	req2.AddCookie(gwCookie)
	req2.AddCookie(csCookie)
	p.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK || rr2.Body.String() != "EDITOR-OK" {
		t.Fatalf("proxy expected 200 EDITOR-OK, got %d %q", rr2.Code, rr2.Body.String())
	}
}

func hasCookie(rr *httptest.ResponseRecorder, name string) bool {
	for _, c := range rr.Result().Cookies() {
		if c.Name == name {
			return true
		}
	}
	return false
}

func TestNonceSingleUse(t *testing.T) {
	n := newNonceCache()
	now := time.Unix(1_700_000_000, 0)
	exp := now.Add(time.Minute).Unix()
	if !n.use("j1", exp, now) {
		t.Fatal("first use should succeed")
	}
	if n.use("j1", exp, now) {
		t.Fatal("second use should fail")
	}
	if !n.use("", exp, now) {
		t.Fatal("empty jti always allowed")
	}
}
