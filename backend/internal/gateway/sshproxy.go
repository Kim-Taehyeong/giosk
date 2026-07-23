package gateway

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHProxy는 username=1회 토큰으로 인증하고 백엔드(컨테이너 sshd/물리 노드)로 재접속해
// 모든 채널을 범용 포워딩하는 SSH 바스티온이다. 사용자는 실제 키 없이 게이트웨이에만 인증한다.
type SSHProxy struct {
	cfg       Config
	nonce     *nonceCache
	hostKey   ssh.Signer // 게이트웨이 SSH 서버 호스트키
	clientKey ssh.Signer // 백엔드 재접속용 게이트웨이 관리키
	now       func() time.Time
}

// NewSSHProxy는 호스트키/클라이언트키를 준비해 SSH 프록시를 만든다.
func NewSSHProxy(cfg Config) (*SSHProxy, error) {
	hostKey, err := loadOrGenSigner(cfg.SSHHostKey)
	if err != nil {
		return nil, fmt.Errorf("host key: %w", err)
	}
	var clientKey ssh.Signer
	if len(cfg.SSHClientKey) > 0 {
		if clientKey, err = ssh.ParsePrivateKey(cfg.SSHClientKey); err != nil {
			return nil, fmt.Errorf("client key: %w", err)
		}
	}
	return &SSHProxy{cfg: cfg, nonce: newNonceCache(), hostKey: hostKey, clientKey: clientKey,
		now: func() time.Time { return time.Now().UTC() }}, nil
}

// Listen은 SSH 프록시를 리스닝한다(블로킹).
func (p *SSHProxy) Listen(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("[gateway] SSH 프록시 리스닝 %s", addr)
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go p.handle(c)
	}
}

// serverConfig는 매 연결마다 claims 를 담을 수 있도록 새로 만든다(username 토큰 검증).
// ⚠️ 여기서는 토큰의 유효성(서명·만료·타입)만 검사하고 nonce(1회성)는 소비하지 않는다.
//
//	publickey 인증은 콜백을 2번 호출한다(질의 단계 → 서명 단계). 콜백에서 nonce 를 소비하면
//	질의 단계가 먹어버려 서명 단계가 "이미 사용됨"으로 실패한다(클라가 실키를 제시할 때 재현).
//	→ nonce 는 핸드셰이크 성공 후 handle 에서 정확히 1회만 소비한다.
func (p *SSHProxy) serverConfig() (*ssh.ServerConfig, *Claims) {
	var claims Claims
	verify := func(user string) error {
		c, err := Verify(user, p.cfg.Secret, p.now())
		if err != nil {
			return err
		}
		if c.Typ != TypSSH {
			return errors.New("not an ssh token")
		}
		// 만료 등으로 이미 소비 불가한 토큰이면 여기서 걸러 인증 자체를 실패시킨다(소비는 안 함).
		if p.nonce.used(c.Jti, p.now()) {
			return errors.New("token already used")
		}
		claims = *c
		return nil
	}
	cfg := &ssh.ServerConfig{
		// username = 토큰. 인증 방식 무관(none/password/publickey 모두 토큰만 검사).
		NoClientAuth: false,
		PasswordCallback: func(conn ssh.ConnMetadata, _ []byte) (*ssh.Permissions, error) {
			return nil, verify(conn.User())
		},
		PublicKeyCallback: func(conn ssh.ConnMetadata, _ ssh.PublicKey) (*ssh.Permissions, error) {
			return nil, verify(conn.User())
		},
		KeyboardInteractiveCallback: func(conn ssh.ConnMetadata, _ ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			return nil, verify(conn.User())
		},
	}
	cfg.AddHostKey(p.hostKey)
	return cfg, &claims
}

func (p *SSHProxy) handle(nConn net.Conn) {
	defer nConn.Close()
	cfg, claims := p.serverConfig()
	conn, chans, reqs, err := ssh.NewServerConn(nConn, cfg)
	if err != nil {
		return // 인증 실패/핸드셰이크 오류
	}
	defer conn.Close()
	// 핸드셰이크 성공 = 이 토큰으로 인증 성립 → 지금 정확히 1회 소비한다(재사용 방지).
	// 경쟁(동시 2연결)에서 진 쪽은 여기서 걸러 즉시 닫는다.
	if !p.nonce.use(claims.Jti, claims.Exp, p.now()) {
		return
	}
	go ssh.DiscardRequests(reqs)

	client, err := p.dialBackend(claims)
	if err != nil {
		log.Printf("[gateway] SSH 백엔드 접속 실패 %s: %v", claims.IID, err)
		rejectAll(chans)
		return
	}
	defer client.Close()

	for nc := range chans {
		go proxyChannel(client, nc)
	}
}

// dialBackend는 claims 대상(컨테이너 Service:22 / 물리 노드:22)으로 게이트웨이 관리키로 접속한다.
func (p *SSHProxy) dialBackend(c *Claims) (*ssh.Client, error) {
	if p.clientKey == nil {
		return nil, errors.New("gateway ssh client key not configured")
	}
	host := c.Host
	if c.Tgt == TgtContainer {
		host = c.ServiceHost()
	}
	user := c.User
	if user == "" {
		user = "work"
	}
	cc := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(p.clientKey)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 인클러스터/신뢰망 — 백엔드 호스트키 미고정
		Timeout:         8 * time.Second,
	}
	return ssh.Dial("tcp", net.JoinHostPort(host, "22"), cc)
}

// proxyChannel은 사용자 채널을 백엔드의 동종 채널로 열어 데이터·요청을 양방향 포워딩한다
// (session=shell/exec/pty/subsystem, direct-tcpip 등 채널 타입 무관).
func proxyChannel(client *ssh.Client, nc ssh.NewChannel) {
	backendCh, backendReqs, err := client.OpenChannel(nc.ChannelType(), nc.ExtraData())
	if err != nil {
		var oce *ssh.OpenChannelError
		if errors.As(err, &oce) {
			_ = nc.Reject(oce.Reason, oce.Message)
		} else {
			_ = nc.Reject(ssh.ConnectionFailed, err.Error())
		}
		return
	}
	userCh, userReqs, err := nc.Accept()
	if err != nil {
		backendCh.Close()
		return
	}
	// 대역외 요청 포워딩(pty-req·window-change·shell·exec·subsystem·exit-status 등).
	go forwardRequests(userReqs, backendCh)
	go forwardRequests(backendReqs, userCh)
	// 데이터 스트림 양방향 복사. 양쪽 다 끝나면 채널을 완전히 닫는다 —
	// CloseWrite(EOF)만 하면 클라이언트가 exec 종료 후 채널 close 를 기다리며 멈춘다(hang).
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); bridge(userCh, backendCh) }()
	go func() { defer wg.Done(); bridge(backendCh, userCh) }()
	wg.Wait()
	_ = userCh.Close()
	_ = backendCh.Close()
}

// forwardRequests는 채널 요청을 대상 채널로 그대로 전달하고 응답을 되돌린다.
func forwardRequests(in <-chan *ssh.Request, target ssh.Channel) {
	for req := range in {
		ok, err := target.SendRequest(req.Type, req.WantReply, req.Payload)
		if req.WantReply {
			_ = req.Reply(ok && err == nil, nil)
		}
	}
}

// bridge는 src→dst 로 데이터를 복사하고 완료 시 쓰기 종료(CloseWrite)한다.
func bridge(dst, src ssh.Channel) {
	_, _ = io.Copy(dst, src)
	_ = dst.CloseWrite()
}

func rejectAll(chans <-chan ssh.NewChannel) {
	for nc := range chans {
		_ = nc.Reject(ssh.ConnectionFailed, "backend unavailable")
	}
}

// loadOrGenSigner는 PEM 개인키를 파싱하거나, 없으면 임시 ed25519 호스트키를 생성한다.
func loadOrGenSigner(pem []byte) (ssh.Signer, error) {
	if len(pem) > 0 {
		return ssh.ParsePrivateKey(pem)
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return ssh.NewSignerFromKey(priv)
}
