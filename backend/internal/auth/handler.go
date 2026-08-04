package auth

import (
	"errors"

	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// Handler는 auth HTTP 핸들러다. 파싱하고 service 를 부르고 응답하는 정도로 얇게 유지한다.
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Login(c *gin.Context) {
	var req LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "username, password 필요")
		return
	}
	res, err := h.svc.Login(req.Username, req.Password)
	if err != nil {
		h.writeLoginError(c, err)
		return
	}
	httpx.OK(c, gin.H{"sessionkey": res.SessionKey, "user": userView(res.User, h.svc.profile(res.User.ID))})
}

func (h *Handler) Signup(c *gin.Context) {
	var req SignupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "필수 항목 누락")
		return
	}
	if err := h.svc.Signup(req); err != nil {
		h.writeSignupError(c, err)
		return
	}
	httpx.Created(c, gin.H{"pending": true, "message": "submitted"})
}

func (h *Handler) Logout(c *gin.Context) {
	_ = h.svc.Logout(bearer(c.GetHeader("Authorization")))
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) Me(c *gin.Context) {
	u := CurrentUser(c)
	httpx.OK(c, userView(u, h.svc.profile(u.ID)))
}

func (h *Handler) SetSSHKey(c *gin.Context) {
	var body struct {
		PublicKey string `json:"publicKey"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BadRequest(c, "publicKey 필요")
		return
	}
	if err := h.svc.SetSSHKey(CurrentUser(c).ID, body.PublicKey); err != nil {
		if errors.Is(err, ErrBadPublicKey) {
			httpx.Err(c, 400, "bad_public_key", "SSH 공개키 형식이 올바르지 않습니다(ssh-ed25519/ssh-rsa ... 한 줄)")
			return
		}
		httpx.Internal(c, "ssh key 저장 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// GenerateSSHKey는 서버에서 키쌍을 만들어 공개키를 등록하고 개인키를 1회 반환한다.
// 개인키는 서버에 저장하지 않는다. 응답을 놓치면 다시 생성해야 한다.
func (h *Handler) GenerateSSHKey(c *gin.Context) {
	u := CurrentUser(c)
	priv, err := h.svc.GenerateSSHKey(u.ID, "giosk-"+u.Username)
	if err != nil {
		httpx.Internal(c, "ssh 키 생성 실패")
		return
	}
	httpx.OK(c, gin.H{"privateKey": priv, "filename": "giosk-" + u.Username + ".pem"})
}

// ChangePassword는 현재/새 비밀번호로 본인 비밀번호를 변경한다.
func (h *Handler) ChangePassword(c *gin.Context) {
	var body struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.BadRequest(c, "current, new 필요")
		return
	}
	err := h.svc.ChangePassword(CurrentUser(c).ID, body.Current, body.New)
	switch {
	case err == nil:
		httpx.OK(c, gin.H{"ok": true})
	case errors.Is(err, ErrInvalidCredentials):
		httpx.Err(c, 400, "wrong_password", "현재 비밀번호가 올바르지 않습니다")
	case errors.Is(err, ErrWeakPassword):
		httpx.Err(c, 400, "weak_password", "비밀번호는 8자 이상이어야 합니다")
	default:
		httpx.Internal(c, "비밀번호 변경 실패")
	}
}

// writeLoginError는 서비스 에러를 프론트가 기대하는 code 로 매핑.
func (h *Handler) writeLoginError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		httpx.Unauthorized(c, "아이디 또는 비밀번호가 올바르지 않습니다")
	case errors.Is(err, ErrPending):
		httpx.Err(c, 403, "pending_approval", "승인 대기 중입니다")
	case errors.Is(err, ErrSuspended):
		httpx.Err(c, 403, "suspended", "정지된 계정입니다")
	case errors.Is(err, ErrRejected):
		httpx.Err(c, 403, "rejected", "거절된 계정입니다")
	default:
		httpx.Internal(c, "로그인 처리 실패")
	}
}

func (h *Handler) writeSignupError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrUsernameTaken):
		httpx.Err(c, 409, "username_taken", "이미 사용 중인 아이디입니다")
	case errors.Is(err, ErrWeakPassword):
		httpx.Err(c, 400, "weak_password", "비밀번호는 8자 이상이어야 합니다")
	default:
		httpx.Internal(c, "가입 신청 처리 실패")
	}
}
