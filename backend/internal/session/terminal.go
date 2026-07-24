package session

import (
	"context"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"giosk/internal/auth"
	"giosk/internal/k8s"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"
)

// Terminal은 세션 웹터미널 websocket 을 처리한다(브라우저 xterm ↔ 여기 ↔ 컨테이너 exec / 물리 노드 SSH).
// 인증: RequireAuth 가 헤더 또는 ?access_token= 쿼리로 사용자를 붙인다(ws 는 헤더 불가 → 쿼리 사용).
func (h *Handler) Terminal(c *gin.Context) {
	u := auth.CurrentUser(c)
	if u == nil {
		return
	}
	instanceID := c.Param("id")
	userID := u.ID
	// x/net/websocket: Handshake 를 통과시켜 same-origin 외 Origin 도 허용(접근은 세션키로 이미 인증됨).
	websocket.Server{
		Handshake: func(*websocket.Config, *http.Request) error { return nil },
		Handler: func(ws *websocket.Conn) {
			h.serveTerminal(ws, instanceID, userID)
		},
	}.ServeHTTP(c.Writer, c.Request)
}

// serveTerminal은 ws 프레임 프로토콜을 stdin/stdout/resize 로 풀어 Service.RunTerminal 에 넘긴다.
//
//	C→S 프레임: 첫 바이트 '0'=입력(나머지=stdin), '1'=리사이즈("cols,rows").
//	S→C 프레임: 원시 출력 바이트(xterm 이 그대로 write).
func (h *Handler) serveTerminal(ws *websocket.Conn, instanceID string, userID int64) {
	ctx, cancel := context.WithCancel(ws.Request().Context())
	defer cancel()

	pr, pw := io.Pipe()
	resize := make(chan k8s.TermSize, 8)
	go func() {
		defer pw.Close()
		defer close(resize)
		for {
			var buf []byte
			if err := websocket.Message.Receive(ws, &buf); err != nil {
				cancel()
				return
			}
			if len(buf) == 0 {
				continue
			}
			switch buf[0] {
			case '1': // 리사이즈 "cols,rows"
				if cols, rows, ok := parseSize(string(buf[1:])); ok {
					select {
					case resize <- k8s.TermSize{Cols: cols, Rows: rows}:
					default:
					}
				}
			default: // '0' 입력(또는 프리픽스 없는 원시 데이터)
				data := buf
				if buf[0] == '0' {
					data = buf[1:]
				}
				if _, err := pw.Write(data); err != nil {
					cancel()
					return
				}
			}
		}
	}()

	log.Printf("terminal: start instance=%s user=%d", instanceID, userID)
	err := h.svc.RunTerminal(ctx, instanceID, userID, pr, &wsWriter{ws}, resize)
	log.Printf("terminal: end instance=%s user=%d err=%v ctxErr=%v", instanceID, userID, err, ctx.Err())
	if err != nil && ctx.Err() == nil {
		_ = websocket.Message.Send(ws, []byte("\r\n["+err.Error()+"]\r\n"))
	}
	_ = ws.Close()
}

func parseSize(s string) (cols, rows uint16, ok bool) {
	parts := strings.SplitN(s, ",", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	cn, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	rn, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || cn <= 0 || rn <= 0 {
		return 0, 0, false
	}
	return uint16(cn), uint16(rn), true
}

// wsWriter는 exec/ssh stdout 을 ws 바이너리 프레임으로 내보낸다.
type wsWriter struct{ ws *websocket.Conn }

func (w *wsWriter) Write(p []byte) (int, error) {
	if err := websocket.Message.Send(w.ws, append([]byte(nil), p...)); err != nil {
		return 0, err
	}
	return len(p), nil
}

// RunTerminal은 세션 웹터미널을 실행한다. 컨테이너=파드 exec(TTY), 물리(SSH)=게이트웨이 키로 노드 SSH.
func (s *Service) RunTerminal(ctx context.Context, instanceID string, userID int64, stdin io.Reader, stdout io.Writer, resize <-chan k8s.TermSize) error {
	sess, err := s.repo.Get(instanceID, userID)
	if err != nil {
		return err
	}
	if sess.Phase != PhaseRunning {
		_, _ = io.WriteString(stdout, "세션이 실행 중이 아닙니다.\r\n")
		return nil
	}
	if sess.Env == "ssh" {
		return s.runPhysicalTerminal(ctx, sess, userID, stdin, stdout, resize)
	}
	// 컨테이너 세션: 홈으로 이동 후 대화형(-i) 로그인(-l) 셸. -i 라야 프롬프트가 뜨고 job control 이 붙어
	// Ctrl+C 가 터미널 전체가 아니라 포그라운드 명령만 끊는다. TERM 지정으로 컬러/편집키 동작.
	cmd := []string{"/bin/sh", "-c", `cd "${HOME:-/home/work}" 2>/dev/null; export TERM=xterm-256color; exec bash -il 2>/dev/null || exec bash -i 2>/dev/null || exec sh -i`}
	return s.prov.ExecTerminal(ctx, s.namespaceOf(sess), sess.InstanceID, "session", cmd, k8s.ExecIO{
		Stdin: stdin, Stdout: stdout, Resize: resize,
	})
}

// runPhysicalTerminal은 물리 세션의 웹터미널 — 게이트웨이 SSH 관리키로 노드의 사용자 계정에 붙는다.
// 노드 authorized_keys 는 이미 게이트웨이 공개키를 신뢰한다(node-agent 가 병기 주입).
func (s *Service) runPhysicalTerminal(ctx context.Context, sess *Session, userID int64, stdin io.Reader, stdout io.Writer, resize <-chan k8s.TermSize) error {
	if len(s.gatewaySSHKey) == 0 {
		_, _ = io.WriteString(stdout, "이 배포에서는 물리 세션 웹 터미널이 비활성화되어 있습니다.\r\n")
		return nil
	}
	signer, err := ssh.ParsePrivateKey(s.gatewaySSHKey)
	if err != nil {
		return err
	}
	cfg := &ssh.ClientConfig{
		User:            s.usernameOf(userID),
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 사내망 노드, 키 고정 목록 없음
		Timeout:         10 * time.Second,
	}
	client, err := ssh.Dial("tcp", sess.Node+":22", cfg)
	if err != nil {
		_, _ = io.WriteString(stdout, "노드 접속 실패: "+err.Error()+"\r\n")
		return nil
	}
	defer client.Close()
	sshSess, err := client.NewSession()
	if err != nil {
		return err
	}
	defer sshSess.Close()
	sshSess.Stdin = stdin
	sshSess.Stdout = stdout
	sshSess.Stderr = stdout
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
	if err := sshSess.RequestPty("xterm-256color", 40, 120, modes); err != nil {
		return err
	}
	go func() {
		for sz := range resize {
			_ = sshSess.WindowChange(int(sz.Rows), int(sz.Cols))
		}
	}()
	go func() { <-ctx.Done(); _ = sshSess.Close(); _ = client.Close() }()
	if err := sshSess.Shell(); err != nil {
		return err
	}
	return sshSess.Wait()
}
