package session

import (
	"context"
	"log"
	"strings"
	"time"

	"giosk/internal/audit"
)

// 세션 홈(sh-*) 회수 파라미터.
const (
	// orphanGrace는 고아 판정 유예. 세션 생성은 홈 PVC 를 먼저 만들고 DB 행을 나중에 쓰므로
	// (Create 는 ensureSessionHome, provision, repo.Create 순서다) 갓 만들어진 PVC 는 "행이 아직 없을 뿐"이다.
	// 유예 없이 지우면 생성 중인 세션의 홈을 리퍼가 뺏어간다.
	orphanGrace = time.Hour
)

// RunHomeReaper는 중단 세션이 물고 있는 홈 PVC(노드 로컬 디스크)를 회수한다.
//
//	T0 고아 정리 : 세션 레코드가 없는 sh-* PVC 삭제. 잃을 사용자가 없으므로 압박과 무관하게 항상.
//	T1 방치 회수 : TTL 초과 중단 세션 삭제. 디스크가 임계를 넘은 노드에서만, 임계 아래로 내려갈 만큼만.
//
// T1 을 "TTL 지나면 무조건"이 아니라 압박 조건부로 둔 것은 scratch·로컬홈 정리 DaemonSet 과 같은
// 계약이다. 여유가 있으면 오래된 중단 세션도 그대로 둔다. 실제 정리 압력은 회수가 아니라
// 중단 스토리지 과금(settleStorage)이 먼저 만든다.
//
// 그 위(실행 중 세션의 홈, 면책 세션)는 자동으로 건드리지 않는다. 회수 가능한 후보를 다 써도
// 임계를 못 넘기면 로그·감사로 남기고 멈춘다. 거기서부터는 관리자 판단 영역이다.
func (s *Service) RunHomeReaper(ctx context.Context, interval time.Duration) {
	log.Printf("[home-reaper] started (interval=%s, ttl·임계=live)", interval)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.reapOrphanHomes(ctx)
			s.reapStaleHomes(ctx)
		}
	}
}

// reapOrphanHomes(T0)는 대응하는 세션 레코드가 없는 홈 PVC 를 지운다.
// 세션 삭제 도중 API 가 죽거나 DB 만 정리된 경우 남는 잔재로, 누구의 데이터도 아니다.
func (s *Service) reapOrphanHomes(ctx context.Context) {
	pvcs, err := s.prov.ListPVCsByPrefix(ctx, homePVCPrefix)
	if err != nil {
		return
	}
	rows, err := s.repo.ListAll()
	if err != nil {
		return // 세션 목록을 못 읽으면 전부 고아로 보일 수 있으니 아무것도 지우지 않는다
	}
	live := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		live[r.ID] = struct{}{}
	}
	now := s.now()
	for _, p := range pvcs {
		if now.Sub(p.CreatedAt) < orphanGrace {
			continue
		}
		id := strings.TrimPrefix(p.Name, homePVCPrefix)
		if _, ok := live[id]; ok {
			continue
		}
		if err := s.prov.DeletePVC(ctx, p.Namespace, p.Name); err != nil {
			continue
		}
		log.Printf("[home-reaper] T0 고아 홈 삭제 %s/%s", p.Namespace, p.Name)
		s.recordReap("session_home_orphan_reap", id)
	}
}

// reapStaleHomes(T1)는 오래 방치된 중단 세션의 홈을 회수한다.
//
// 예전에는 노드 디스크가 임계를 넘었을 때만 돌았다. 홈에 하드 쿼터가 없던 시절에는 그게
// 유일한 방어선이었지만, 부작용이 컸다. 회수 여부가 "그 노드에 누가 무엇을 쓰고 있느냐"에
// 달려 있어서 사용자는 자기 세션이 언제 사라질지 알 수 없었고, 남의 사용량 때문에 내 데이터가
// 지워졌다.
//
// 홈이 이미지 기반이 되면서 각 세션은 자기 몫만 쓴다. 노드가 차는 것은 쿼터와 입주 심사가
// 막으므로, 회수는 "방치"라는 한 가지 이유로만 한다. 며칠 뒤에 사라지는지 미리 알 수 있고
// 남의 사용량에 영향받지 않는다.
//
// 방치 일수는 매 틱 라이브로 읽는다(관리자가 운영 중 바꾼 값이 바로 반영된다).
func (s *Service) reapStaleHomes(ctx context.Context) {
	if s.homeReap == nil {
		return
	}
	ttlDays := s.homeReap()
	if ttlDays <= 0 {
		return // 회수 비활성(관리자가 0 으로 꺼둔 상태 포함)
	}
	rows, err := s.repo.ListStopped()
	if err != nil {
		return
	}
	now := s.now()
	keep := s.stoppedProfile(rows, now)

	ttl := time.Duration(ttlDays) * 24 * time.Hour
	for i := range rows {
		sess := &rows[i]
		if sess.StoppedSince == nil {
			continue // 기준시각이 없으면 stoppedProfile 이 방금 찍었다. 다음 틱부터 센다.
		}
		if now.Sub(*sess.StoppedSince) < ttl {
			continue
		}
		if keep[sess.UserID] == sess.InstanceID {
			continue // 면책: 사용자별 가장 최근 중단 세션 1개는 회수하지 않는다
		}
		if err := s.deleteSession(ctx, sess); err != nil {
			continue
		}
		log.Printf("[home-reaper] T1 회수 %s (방치 %.0f일, user=%d)",
			sess.InstanceID, now.Sub(*sess.StoppedSince).Hours()/24, sess.UserID)
		s.recordReap("session_home_reap", sess.InstanceID)
	}
}

// stoppedProfile은 사용자별 면책 세션(가장 최근 중단 1개)을 고른다.
//
// 면책이 없으면 방치 기간만으로 전부 털려서 "잠깐 멈춰 둔 것"까지 사라진다. 한 개는
// 남겨 두면 사용자가 돌아왔을 때 이어서 쓸 자리가 있다.
//
// 기준시각이 없는(도입 이전) 세션은 여기서 지금으로 찍는다. 그러지 않으면 방치 기간이
// 무한대로 계산돼 즉시 회수 대상이 된다.
func (s *Service) stoppedProfile(rows []Session, now time.Time) (keep map[int64]string) {
	keep = map[int64]string{}
	newest := map[int64]time.Time{}
	for i := range rows {
		sess := &rows[i]
		if sess.StoppedSince == nil {
			_ = s.repo.MarkStopped(sess.InstanceID, now)
			continue
		}
		if t, ok := newest[sess.UserID]; !ok || sess.StoppedSince.After(t) {
			newest[sess.UserID] = *sess.StoppedSince
			keep[sess.UserID] = sess.InstanceID
		}
	}
	return keep
}

// recordReap은 회수 동작을 감사 로그에 남긴다(누구 것을 왜 지웠는지 사후 추적 가능해야 한다).
func (s *Service) recordReap(action, target string) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Insert(&audit.Log{ActorUsername: "system", Action: action, Target: target, Result: "applied"})
}
