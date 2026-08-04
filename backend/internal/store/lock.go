package store

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ErrLockBusy는 제한 시간 안에 잠금을 얻지 못했을 때.
var ErrLockBusy = errors.New("lock busy")

// NamedLock은 MySQL GET_LOCK 기반의 클러스터 전역 상호배제다.
// API 가 여러 replica 로 떠도 "검사 후 예약"처럼 한 번에 하나만 지나야 하는 구간을 지킨다.
//
// GET_LOCK 은 커넥션에 귀속되므로 풀에서 커넥션 하나를 잡아 두고, 해제할 때 같은 커넥션에서
// RELEASE_LOCK 한 뒤 돌려준다. 커넥션이 끊기면 잠금도 서버가 자동으로 푼다(교착 방지).
func NamedLock(ctx context.Context, db *gorm.DB, name string, wait time.Duration) (release func(), err error) {
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, err
	}
	secs := int(wait.Seconds())
	if secs < 1 {
		secs = 1
	}
	var got *int
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", name, secs).Scan(&got); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if got == nil || *got != 1 {
		_ = conn.Close()
		return nil, ErrLockBusy
	}
	return func() {
		// 해제는 호출자의 ctx 와 무관하게 반드시 수행한다. 요청이 취소돼 ctx 가 죽은 뒤에
		// RELEASE_LOCK 을 못 보내면 커넥션이 풀로 돌아가면서 잠금이 남아 다음 요청을 막는다.
		rel, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(rel, "SELECT RELEASE_LOCK(?)", name)
		_ = conn.Close()
	}, nil
}
