package auth

import (
	"time"

	"golang.org/x/crypto/bcrypt"
)

// checkStatus는 로그인 가능한 상태인지 검사한다.
func checkStatus(status string) error {
	switch status {
	case StatusApproved:
		return nil
	case StatusPending:
		return ErrPending
	case StatusSuspended:
		return ErrSuspended
	case StatusRejected:
		return ErrRejected
	default:
		return ErrInvalidCredentials
	}
}

func hashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

func passwordMatches(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// newPendingUser는 가입 신청 요청에서 status=pending 사용자를 만든다.
func newPendingUser(req SignupReq, hash string) *User {
	now := time.Now().UTC()
	u := &User{
		Username:     req.Username,
		PasswordHash: hash,
		Email:        req.Email,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		FirstNameEn:  req.FirstNameEn,
		LastNameEn:   req.LastNameEn,
		SponsorName:  req.SponsorName,
		Role:          RoleMember,
		Status:        StatusPending,
		SignupGroupID: req.GroupID, // 승인 시 이 그룹의 active 멤버십을 만든다
	}
	if req.TermsAccepted {
		u.TermsAcceptedAt = &now
	}
	return u
}
