package gateway

import (
	"sync"
	"time"
)

// nonceCache는 access 토큰 jti 의 단일사용을 보장하는 인메모리 캐시(단일 레플리카 게이트웨이).
// 만료(exp)까지만 보관하고 주기적으로 청소한다. 다중 레플리카로 확장 시 공유 저장소 필요.
type nonceCache struct {
	mu   sync.Mutex
	seen map[string]int64 // jti 별 exp(unix)
}

func newNonceCache() *nonceCache {
	n := &nonceCache{seen: make(map[string]int64)}
	return n
}

// use는 jti 를 소비한다. 처음이면 true, 이미 사용됐으면 false(재생 차단).
// exp 는 이 jti 를 캐시에 유지할 시각(토큰 만료). 빈 jti 는 항상 true(단일사용 미요구).
func (n *nonceCache) use(jti string, exp int64, now time.Time) bool {
	if jti == "" {
		return true
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	nowU := now.Unix()
	if e, ok := n.seen[jti]; ok && e > nowU {
		return false
	}
	n.seen[jti] = exp
	// 편승 청소(캐시가 커질 때만).
	if len(n.seen) > 1024 {
		for k, e := range n.seen {
			if e <= nowU {
				delete(n.seen, k)
			}
		}
	}
	return true
}

// used는 jti 가 이미 소비됐는지만 조회한다(소비하지 않는 읽기 전용).
// 인증 콜백은 이걸로 사전 차단만 하고, 실제 소비는 핸드셰이크 성공 후 use 로 1회 한다
// (publickey 2단계 콜백이 질의 단계에서 nonce 를 먹어버리는 것을 피하기 위함).
func (n *nonceCache) used(jti string, now time.Time) bool {
	if jti == "" {
		return false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	e, ok := n.seen[jti]
	return ok && e > now.Unix()
}
