package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"strings"

	"golang.org/x/crypto/ssh"
)

// ErrBadPublicKey는 OpenSSH authorized_keys 한 줄로 파싱되지 않는 공개키.
var ErrBadPublicKey = errors.New("invalid ssh public key")

// normalizePublicKey는 사용자가 붙여넣은 공개키를 검증하고 정규화(1줄)한다.
// authorized_keys 에 그대로 들어가므로 개행/여러 줄은 잘라내고 파싱 실패는 거부한다
// (개인키를 잘못 붙여넣는 사고를 여기서 막는다).
func normalizePublicKey(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil // 빈 값 = 키 삭제(호출측이 허용 여부 판단)
	}
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	pub, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(s))
	if err != nil {
		return "", ErrBadPublicKey
	}
	out := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))
	if comment != "" {
		out += " " + comment
	}
	return out, nil
}

// generateKeyPair는 ed25519 키쌍을 만들어 (공개키 authorized_keys 1줄, 개인키 OpenSSH PEM)를 반환한다.
// 개인키는 저장하지 않고 응답으로 1회만 내려준다(EC2 키페어 방식). 분실하면 재발급해야 한다.
func generateKeyPair(comment string) (pubLine, privPEM string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	block, err := ssh.MarshalPrivateKey(priv, comment)
	if err != nil {
		return "", "", err
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return "", "", err
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
	if comment != "" {
		line += " " + comment
	}
	return line, string(pem.EncodeToMemory(block)), nil
}
