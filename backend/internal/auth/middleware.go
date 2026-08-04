package auth

import (
	"strings"

	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

const ctxUserKey = "giosk.user"

// RequireAuth는 Authorization 헤더의 세션키로 사용자를 인증해 컨텍스트에 넣는다.
func (h *Handler) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := bearer(c.GetHeader("Authorization"))
		if key == "" {
			// WebSocket(브라우저)은 Authorization 헤더를 못 붙이므로 쿼리 파라미터 access_token 으로 폴백한다.
			// (웹터미널 /instances/:id/terminal 등 ws 라우트 전용. HTTP 요청은 계속 헤더 사용.)
			key = strings.TrimSpace(c.Query("access_token"))
		}
		if key == "" {
			httpx.Unauthorized(c, "missing session key")
			return
		}
		u, err := h.svc.Authenticate(key)
		if err != nil {
			httpx.Unauthorized(c, "invalid session")
			return
		}
		c.Set(ctxUserKey, u)
		c.Next()
	}
}

// CurrentUser는 컨텍스트의 인증 사용자를 반환한다.
func CurrentUser(c *gin.Context) *User {
	v, ok := c.Get(ctxUserKey)
	if !ok {
		return nil
	}
	u, _ := v.(*User)
	return u
}

// bearer는 "Bearer <key>" 또는 raw 키를 추출한다.
func bearer(header string) string {
	if header == "" {
		return ""
	}
	if after, ok := strings.CutPrefix(header, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return strings.TrimSpace(header)
}
