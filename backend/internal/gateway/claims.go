// Package gateway는 세션 접속 게이트웨이(리버스 프록시 + SSH 프록시)의 데이터 플레인과,
// API 가 발급하는 자체완결(stateless) 접속 토큰의 서명/검증을 담는다.
//
// 신뢰 모델: API 와 게이트웨이가 공유하는 대칭키(GIOSK_GATEWAY_SECRET)로 토큰을 AES-GCM
// 암호화·인증한다. 토큰은 사용자에게 완전 불투명하고(비밀 주입값 포함), 짧은 수명 + 단일사용(jti)
// + 즉시 쿠키 교환으로 재생을 막는다. SSH 는 게이트웨이 관리키로 백엔드에 재접속하므로 토큰에
// 비밀을 담지 않는다.
package gateway

// 접속 채널 이름.
const (
	ChanVSCode  = "vscode"
	ChanJupyter = "jupyter"
	ChanSSH     = "ssh"
)

// 토큰 종류.
const (
	TypWeb    = "web"    // 웹(vscode/jupyter) 1회 access 토큰
	TypSSH    = "ssh"    // SSH 접속 1회 토큰(username 으로 제시)
	TypCookie = "cookie" // 웹 세션 쿠키(access 교환 후 재사용, jti 없음)
)

// 접속 대상 종류.
const (
	TgtContainer = "container" // 컨테이너 세션(인클러스터 Service 로 라우팅)
	TgtPhysical  = "physical"  // 물리노드 임대(노드 IP:22 로 SSH)
)

// Claims는 접속 토큰/쿠키가 담는 라우팅·인증 정보. API 가 서명, 게이트웨이가 검증.
type Claims struct {
	IID    string `json:"iid"`            // 인스턴스 ID(= 세션 서비스명)
	Ch     string `json:"ch"`             // vscode | jupyter | ssh
	NS     string `json:"ns"`             // 세션 네임스페이스
	Port   int    `json:"port"`           // 대상 컨테이너 포트
	Sub    int64  `json:"sub"`            // 소유 사용자 ID
	Typ    string `json:"typ"`            // web | ssh | cookie
	Tgt    string `json:"tgt"`            // container | physical
	Host   string `json:"host,omitempty"` // 물리 SSH 대상 노드 호스트/IP
	User   string `json:"user,omitempty"` // SSH 로그인 계정(물리=username, 컨테이너=work)
	Secret string `json:"sec,omitempty"`  // 주입 비밀(code-server 비번 / jupyter 토큰). SSH 는 비움.
	Exp    int64  `json:"exp"`            // 만료(unix)
	Jti    string `json:"jti,omitempty"`  // 단일사용 nonce(cookie 는 비움)
}

// Service는 컨테이너 세션의 인클러스터 Service DNS(FQDN)를 반환한다.
func (c Claims) ServiceHost() string {
	return c.IID + "." + c.NS + ".svc.cluster.local"
}
