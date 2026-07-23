package config

import "strconv"

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
