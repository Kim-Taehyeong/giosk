package gateway

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"
)

// primeClient는 백엔드 인증 프라임용 클라이언트다. 리다이렉트를 따라가지 않고 Set-Cookie 만 수확한다.
// 프록시의 다이얼 시드를 재사용해 테스트에서 로컬 업스트림으로 리다이렉트 가능.
func (p *Proxy) primeClient() *http.Client {
	return &http.Client{
		Timeout:       5 * time.Second,
		Transport:     &http.Transport{DialContext: p.dial},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// primeCookies는 채널별로 백엔드 인증을 서버측에서 수행하고, 브라우저에 심을 인증 쿠키를 반환한다.
//   - vscode(code-server): POST /login {password} 로 code-server 가 세팅하는 세션 쿠키를 그대로 전달한다.
//     (쿠키명이 버전마다 달라도 "받은 것을 전달"하므로 무관.)
//   - jupyter: 요청마다 Authorization: token 헤더를 주입하므로(injectRequestAuth) 쿠키 프라임 불필요.
func (p *Proxy) primeCookies(claims *Claims) []*http.Cookie {
	if claims.Ch != ChanVSCode || claims.Secret == "" {
		return nil
	}
	base := fmt.Sprintf("http://%s:%d", claims.ServiceHost(), claims.Port)
	form := url.Values{"password": {claims.Secret}, "base": {"."}}
	resp, err := p.primeClient().PostForm(base+"/login", form)
	if err != nil {
		log.Printf("[gateway] vscode prime %s 실패: %v", claims.IID, err)
		return nil
	}
	defer resp.Body.Close()
	// code-server 가 로그인 성공 시 세팅한 쿠키를 브라우저용으로 재설정한다(Domain 을 안 주면 현재 서브도메인 host-only 다).
	var out []*http.Cookie
	for _, c := range resp.Cookies() {
		out = append(out, &http.Cookie{
			Name: c.Name, Value: c.Value, Path: "/",
			HttpOnly: c.HttpOnly, SameSite: http.SameSiteLaxMode,
		})
	}
	return out
}

// injectRequestAuth는 프록시되는 매 요청에 채널별 인증을 주입한다.
//   - jupyter: Authorization: token <secret> (초기 문서·API·WS 핸드셰이크 모두에 유효).
//   - vscode 는 쿠키 프라임(primeCookies)으로 처리하므로 요청 단위 주입이 없다.
func injectRequestAuth(claims *Claims, req *http.Request) {
	if claims.Ch == ChanJupyter && claims.Secret != "" {
		if req.Header.Get("Authorization") == "" {
			req.Header.Set("Authorization", "token "+claims.Secret)
		}
	}
	// 업스트림이 X-Forwarded-* 로 원 스킴을 인식하도록 힌트(code-server 리다이렉트 정상화).
	req.Header.Set("X-Forwarded-Proto", "https")
}
