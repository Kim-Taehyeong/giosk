package auth

import (
	"errors"
	"time"

	"giosk/pkg/idgen"
)

// 서비스 레벨 에러 — 핸들러가 HTTP 코드/프론트 code 로 매핑.
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrPending            = errors.New("pending approval")
	ErrSuspended          = errors.New("suspended")
	ErrRejected           = errors.New("rejected")
	ErrUsernameTaken      = errors.New("username taken")
	ErrWeakPassword       = errors.New("password too short")
)

const minPasswordLen = 8

// Service는 auth 비즈니스 로직. Repository 인터페이스에만 의존(테스트 용이).
type Service struct {
	repo       Repository
	sessionTTL time.Duration
	enricher   Enricher
}

// NewService는 auth 서비스를 만든다. sessionTTL 은 호출측(설정)에서 주입.
func NewService(repo Repository, sessionTTL time.Duration) *Service {
	return &Service{repo: repo, sessionTTL: sessionTTL}
}

// WithEnricher는 멤버십 컨텍스트 enricher 를 주입한다(콘솔 라우팅 필드용).
func (s *Service) WithEnricher(e Enricher) *Service { s.enricher = e; return s }

// profile은 사용자 멤버십 컨텍스트를 조회한다(enricher 미설정 시 빈 값).
func (s *Service) profile(userID int64) ProfileExtra {
	if s.enricher == nil {
		return ProfileExtra{}
	}
	return s.enricher.Profile(userID)
}

// Login은 자격증명을 검증하고 세션을 발급한다.
func (s *Service) Login(username, password string) (*LoginRes, error) {
	u, err := s.repo.FindUserByUsername(username)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if err := checkStatus(u.Status); err != nil {
		return nil, err
	}
	if !passwordMatches(u.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	return s.issueSession(u)
}

// Signup은 자체 가입 신청을 접수한다(자동 로그인 없음, status=pending).
// 그룹을 선택했으면 가입 신청(group_join_requests)도 생성.
func (s *Service) Signup(req SignupReq) error {
	if len(req.Password) < minPasswordLen {
		return ErrWeakPassword
	}
	if _, err := s.repo.FindUserByUsername(req.Username); err == nil {
		return ErrUsernameTaken
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		return err
	}
	u := newPendingUser(req, hash)
	if err := s.repo.CreateUser(u); err != nil {
		return err
	}
	if req.GroupID != nil {
		return s.repo.CreateJoinRequest(u.ID, *req.GroupID)
	}
	return nil
}

// Authenticate는 세션키로 사용자를 조회한다(미들웨어용).
func (s *Service) Authenticate(sessionKey string) (*User, error) {
	sess, err := s.repo.FindSession(sessionKey)
	if err != nil {
		return nil, err
	}
	return s.repo.FindUserByID(sess.UserID)
}

// Logout은 세션을 폐기한다.
func (s *Service) Logout(sessionKey string) error { return s.repo.DeleteSession(sessionKey) }

// SetSSHKey는 사용자의 단일 SSH 공개키를 갱신한다.
func (s *Service) SetSSHKey(userID int64, key string) error {
	return s.repo.UpdateSSHKey(userID, key)
}

// ChangePassword는 현재 비밀번호 확인 후 새 비밀번호로 교체한다(8자 이상).
func (s *Service) ChangePassword(userID int64, current, next string) error {
	u, err := s.repo.FindUserByID(userID)
	if err != nil {
		return err
	}
	if !passwordMatches(u.PasswordHash, current) {
		return ErrInvalidCredentials
	}
	if len(next) < minPasswordLen {
		return ErrWeakPassword
	}
	hash, err := hashPassword(next)
	if err != nil {
		return err
	}
	return s.repo.UpdatePassword(userID, hash)
}

// issueSession은 세션 레코드를 만들고 응답을 구성한다.
func (s *Service) issueSession(u *User) (*LoginRes, error) {
	now := time.Now().UTC()
	sess := &Session{
		SessionKey: idgen.SessionKey(),
		UserID:     u.ID,
		CreatedAt:  now,
		ExpiresAt:  now.Add(s.sessionTTL),
	}
	if err := s.repo.CreateSession(sess); err != nil {
		return nil, err
	}
	return &LoginRes{SessionKey: sess.SessionKey, User: u}, nil
}
