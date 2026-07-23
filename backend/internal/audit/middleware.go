package audit

import (
	"strings"

	"giosk/internal/auth"

	"github.com/gin-gonic/gin"
)

// Middleware는 인증된 변경 요청(POST/PUT/DELETE)을 audit_logs 에 적재한다.
// 값을 먼저 추출한 뒤 비동기로 기록해 응답 지연을 피한다.
func Middleware(repo Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if !mutating(c.Request.Method) {
			return
		}
		u := auth.CurrentUser(c)
		l := Log{
			Action: actionOf(c),
			Target: targetOf(c),
			Result: resultOf(c.Writer.Status()),
			IP:     c.ClientIP(),
		}
		if u != nil {
			l.ActorID = &u.ID
			l.ActorUsername = u.Username
		}
		go func() { _ = repo.Insert(&l) }()
	}
}

func mutating(m string) bool {
	return m == "POST" || m == "PUT" || m == "DELETE" || m == "PATCH"
}

// actionOf는 "METHOD route-pattern"(/api 접두 제거)을 액션명으로 만든다.
func actionOf(c *gin.Context) string {
	p := strings.TrimPrefix(c.FullPath(), "/api")
	if p == "" {
		p = c.Request.URL.Path
	}
	return c.Request.Method + " " + p
}

// targetOf는 라우트 파라미터(id|name)에서 대상 식별자를 고른다.
func targetOf(c *gin.Context) string {
	for _, k := range []string{"id", "name", "username"} {
		if v := c.Param(k); v != "" {
			return v
		}
	}
	return ""
}

func resultOf(status int) string {
	switch {
	case status == 403:
		return "denied"
	case status >= 400:
		return "failed"
	default:
		return "success"
	}
}
