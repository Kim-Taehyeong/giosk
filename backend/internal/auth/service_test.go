package auth

import (
	"testing"
	"time"
)

// fakeRepo는 인메모리 Repository 구현(DB 없이 서비스 로직 테스트).
type fakeRepo struct {
	users    map[string]*User
	sessions map[string]*Session
	joins    []int64
	nextID   int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{users: map[string]*User{}, sessions: map[string]*Session{}}
}

func (f *fakeRepo) FindUserByUsername(u string) (*User, error) {
	if x, ok := f.users[u]; ok {
		return x, nil
	}
	return nil, ErrNotFound
}
func (f *fakeRepo) FindUserByID(id int64) (*User, error) {
	for _, u := range f.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, ErrNotFound
}
func (f *fakeRepo) CreateUser(u *User) error {
	f.nextID++
	u.ID = f.nextID
	f.users[u.Username] = u
	return nil
}
func (f *fakeRepo) UpdateSSHKey(id int64, key string) error    { return nil }
func (f *fakeRepo) UpdatePassword(id int64, hash string) error { return nil }
func (f *fakeRepo) CreateJoinRequest(uid, gid int64) error {
	f.joins = append(f.joins, gid)
	return nil
}
func (f *fakeRepo) CreateSession(s *Session) error { f.sessions[s.SessionKey] = s; return nil }
func (f *fakeRepo) FindSession(k string) (*Session, error) {
	if s, ok := f.sessions[k]; ok {
		return s, nil
	}
	return nil, ErrNotFound
}
func (f *fakeRepo) DeleteSession(k string) error { delete(f.sessions, k); return nil }

func newSvc() (*Service, *fakeRepo) {
	r := newFakeRepo()
	return NewService(r, time.Hour), r
}

func TestSignupThenLogin(t *testing.T) {
	svc, repo := newSvc()
	gid := int64(1)
	err := svc.Signup(SignupReq{Username: "hong", Password: "password1", Email: "h@x.io",
		FirstName: "길동", LastName: "홍", GroupID: &gid, TermsAccepted: true})
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	// 가입 직후엔 pending 이라 로그인이 거부된다
	if _, err := svc.Login("hong", "password1"); err != ErrPending {
		t.Fatalf("pending 사용자는 ErrPending 이어야 함, got %v", err)
	}
	if len(repo.joins) != 1 || repo.joins[0] != 1 {
		t.Fatalf("그룹 가입 신청이 생성돼야 함")
	}
	// 승인 후 로그인 성공 + 세션 발급
	repo.users["hong"].Status = StatusApproved
	res, err := svc.Login("hong", "password1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if res.SessionKey == "" || res.User.Username != "hong" {
		t.Fatalf("세션/유저 응답 이상: %+v", res)
	}
	// 세션으로 인증 가능
	if _, err := svc.Authenticate(res.SessionKey); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	svc, repo := newSvc()
	_ = svc.Signup(SignupReq{Username: "a", Password: "password1", Email: "a@x.io", FirstName: "a", LastName: "a"})
	repo.users["a"].Status = StatusApproved
	if _, err := svc.Login("a", "wrongpass"); err != ErrInvalidCredentials {
		t.Fatalf("틀린 비번은 ErrInvalidCredentials, got %v", err)
	}
}

func TestSignupWeakPassword(t *testing.T) {
	svc, _ := newSvc()
	if err := svc.Signup(SignupReq{Username: "a", Password: "short", Email: "a@x.io", FirstName: "a", LastName: "a"}); err != ErrWeakPassword {
		t.Fatalf("8자 미만은 ErrWeakPassword, got %v", err)
	}
}

func TestSignupDuplicate(t *testing.T) {
	svc, _ := newSvc()
	r := SignupReq{Username: "dup", Password: "password1", Email: "d@x.io", FirstName: "d", LastName: "d"}
	_ = svc.Signup(r)
	if err := svc.Signup(r); err != ErrUsernameTaken {
		t.Fatalf("중복 username 은 ErrUsernameTaken, got %v", err)
	}
}
