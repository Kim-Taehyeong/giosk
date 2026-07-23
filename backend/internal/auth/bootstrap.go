package auth

import "errors"

// EnsureAdmin은 플랫폼 관리자 계정이 없으면 생성한다(멱등).
// password 가 비어 있으면 건너뛴다(설정 누락 시 부트스트랩 생략).
func (s *Service) EnsureAdmin(username, password string) error {
	if username == "" || password == "" {
		return nil
	}
	if _, err := s.repo.FindUserByUsername(username); err == nil {
		return nil // 이미 존재
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	return s.repo.CreateUser(&User{
		Username:     username,
		PasswordHash: hash,
		Role:         RoleAdmin,
		Status:       StatusApproved,
	})
}
