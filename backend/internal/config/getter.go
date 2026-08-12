package config

import (
	"strconv"
	"strings"
)

// getter는 환경변수 lookup을 감싸 타입별 기본값 조회를 제공한다.
type getter struct {
	lookup func(string) (string, bool)
}

func newGetter(lookup func(string) (string, bool)) getter {
	return getter{lookup: lookup}
}

func (g getter) str(key, def string) string {
	if v, ok := g.lookup(key); ok && v != "" {
		return v
	}
	return def
}

func (g getter) intv(key string, def int) int {
	v, ok := g.lookup(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// list는 쉼표로 구분한 목록 환경변수를 읽는다(공백 제거, 빈 항목 무시).
// 값이 "none" 이면 빈 목록으로 본다. 기본값이 있는 항목을 설정으로 비우는 유일한 방법이다
// (빈 문자열은 미설정과 구분되지 않아 기본값으로 되돌아간다).
func (g getter) list(key string, def []string) []string {
	v, ok := g.lookup(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	if strings.TrimSpace(v) == "none" {
		return nil
	}
	out := []string{}
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (g getter) boolv(key string, def bool) bool {
	v, ok := g.lookup(key)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
