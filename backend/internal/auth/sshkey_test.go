package auth

import (
	"strings"
	"testing"
)

func TestNormalizePublicKey(t *testing.T) {
	pub, _, err := generateKeyPair("giosk-test")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// 생성한 키는 그대로 통과하고 코멘트가 보존된다.
	got, err := normalizePublicKey("  " + pub + "\n")
	if err != nil || got != pub {
		t.Fatalf("roundtrip: got=%q err=%v want=%q", got, err, pub)
	}
	// 빈 값은 "키 삭제" 로 허용.
	if got, err := normalizePublicKey("   "); err != nil || got != "" {
		t.Fatalf("empty: got=%q err=%v", got, err)
	}
	// 개인키를 잘못 붙여넣거나 형식이 깨지면 거부.
	for _, bad := range []string{"not-a-key", "-----BEGIN OPENSSH PRIVATE KEY-----", "ssh-ed25519 !!!!"} {
		if _, err := normalizePublicKey(bad); err != ErrBadPublicKey {
			t.Fatalf("bad key %q: err=%v", bad, err)
		}
	}
}

func TestGenerateKeyPair(t *testing.T) {
	pub, priv, err := generateKeyPair("giosk-alice")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.HasPrefix(pub, "ssh-ed25519 ") || !strings.HasSuffix(pub, " giosk-alice") {
		t.Fatalf("public line: %q", pub)
	}
	if !strings.HasPrefix(priv, "-----BEGIN OPENSSH PRIVATE KEY-----") {
		t.Fatalf("private pem: %q", priv[:min(40, len(priv))])
	}
}
