package gateway

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"time"
)

// 토큰 검증 오류.
var (
	ErrToken   = errors.New("gateway: invalid token")
	ErrExpired = errors.New("gateway: token expired")
)

// deriveKey는 공유 비밀에서 AES-256 키(32B)를 파생한다(SHA-256).
func deriveKey(secret []byte) []byte {
	sum := sha256.Sum256(secret)
	return sum[:]
}

// gcmFor는 파생 키로 AES-GCM AEAD 를 만든다.
func gcmFor(secret []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(deriveKey(secret))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Sign은 Claims 를 AES-GCM 으로 암호화·인증한 불투명 URL-safe 토큰으로 만든다.
// 출력 = base64url(nonce || ciphertext||tag). 키 자체가 인증(AEAD)이므로 별도 HMAC 불필요.
func Sign(c Claims, secret []byte) (string, error) {
	gcm, err := gcmFor(secret)
	if err != nil {
		return "", err
	}
	pt, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, pt, nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Verify는 토큰을 복호화·검증하고 만료를 확인한다(now 이후면 ErrExpired).
func Verify(tok string, secret []byte, now time.Time) (*Claims, error) {
	raw, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		return nil, ErrToken
	}
	gcm, err := gcmFor(secret)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(raw) < ns {
		return nil, ErrToken
	}
	pt, err := gcm.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return nil, ErrToken
	}
	var c Claims
	if err := json.Unmarshal(pt, &c); err != nil {
		return nil, ErrToken
	}
	if c.Exp > 0 && now.Unix() > c.Exp {
		return nil, ErrExpired
	}
	return &c, nil
}
