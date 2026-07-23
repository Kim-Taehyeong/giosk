// Package idgen은 충돌 가능성이 낮은 랜덤 식별자를 생성한다(세션키·인스턴스ID 등).
package idgen

import (
	"crypto/rand"
	"encoding/hex"
)

// Token은 prefix + n바이트 hex 랜덤 문자열을 만든다.
func Token(prefix string, n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

// SessionKey는 세션키용 32바이트 토큰.
func SessionKey() string { return Token("sk_", 32) }
