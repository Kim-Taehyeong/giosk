package session

import (
	"context"
	"log"
	"sort"
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

// reapStaleHomes(T1)는 디스크 압박 노드에서 방치된 중단 세션을 회수한다.
// 조건(방치 일수·디스크 임계)은 매 틱 라이브로 읽는다. 관리자가 운영 중 바꾼 값이 바로 반영된다.
func (s *Service) reapStaleHomes(ctx context.Context) {
	if s.homeReap == nil {
		return
	}
	ttlDays, thresholdPct := s.homeReap()
	if ttlDays <= 0 || thresholdPct <= 0 {
		return // 회수 비활성(관리자가 0 으로 꺼둔 상태 포함)
	}
	need := s.pressuredNodes(ctx, thresholdPct)
	if len(need) == 0 {
		return // 여유가 있으면 방치 세션도 건드리지 않는다
	}
	rows, err := s.repo.ListStopped()
	if err != nil {
		return
	}
	now := s.now()
	occupancy, keep := s.stoppedProfile(rows, now)

	// 후보 선별: 압박 노드 + TTL 초과 + 면책 대상 아님.
	ttl := time.Duration(ttlDays) * 24 * time.Hour
	byNode := map[string][]*Session{}
	for i := range rows {
		sess := &rows[i]
		if sess.StoppedSince == nil || sess.Node == "" {
			continue
		}
		if _, hot := need[sess.Node]; !hot {
			continue
		}
		if now.Sub(*sess.StoppedSince) < ttl {
			continue
		}
		if keep[sess.UserID] == sess.InstanceID {
			continue // 면책: 사용자별 가장 최근 중단 세션 1개는 회수하지 않는다
		}
		byNode[sess.Node] = append(byNode[sess.Node], sess)
	}

	for node, want := range need {
		cands := byNode[node]
		// 점수 = 방치일수 × 그 사용자의 중단 세션 수. 오래 방치할수록, 많이 물고 있을수록 먼저.
		// 절대 점유량만 보면 큰 데이터를 정상적으로 쓰는 사용자가 매번 걸리고,
		// 방치기간만 보면 한 사람이 여러 개를 쌓아둔 과독점이 그대로 남는다.
		sort.SliceStable(cands, func(i, j int) bool {
			return reapScore(cands[i], occupancy, now) > reapScore(cands[j], occupancy, now)
		})
		freed := 0
		for _, sess := range cands {
			if freed >= want {
				break
			}
			if err := s.deleteSession(ctx, sess); err != nil {
				continue
			}
			freed += homeSizeGiB
			log.Printf("[home-reaper] T1 회수 %s (node=%s, 방치 %.0f일, user=%d)",
				sess.InstanceID, node, now.Sub(*sess.StoppedSince).Hours()/24, sess.UserID)
			s.recordReap("session_home_reap", sess.InstanceID)
		}
		if freed < want {
			// 자동으로 할 수 있는 건 여기까지다. 남은 압박은 실행 중 세션이나 면책 세션이 원인이다.
			// (디스크 사용률 알림 규칙 disk_usage 가 이미 관리자에게 울고 있는 상태)
			log.Printf("[home-reaper] %s: %dGiB 필요, %dGiB 회수. 자동 회수 한계라 관리자 개입이 필요하다", node, want, freed)
			s.recordReap("session_home_reap_insufficient", node)
		}
	}
}

// stoppedProfile은 회수 판단에 필요한 사용자별 프로필을 만든다.
//   - occupancy: 사용자별 중단 세션 수(과독점 가중치)
//   - keep     : 사용자별 가장 최근 중단 세션의 instanceID(면책 대상)
//
// 면책이 없으면 노드가 붐빌 때 소량 사용자까지 털려서 "언제 날아갈지 모르는 플랫폼"이 된다.
// 기준시각이 없는(도입 이전) 세션은 여기서 지금으로 찍어 즉시 회수 대상이 되는 것을 막는다.
func (s *Service) stoppedProfile(rows []Session, now time.Time) (occupancy map[int64]int, keep map[int64]string) {
	occupancy, keep = map[int64]int{}, map[int64]string{}
	newest := map[int64]time.Time{}
	for i := range rows {
		sess := &rows[i]
		if sess.StoppedSince == nil {
			_ = s.repo.MarkStopped(sess.InstanceID, now)
			continue
		}
		occupancy[sess.UserID]++
		if t, ok := newest[sess.UserID]; !ok || sess.StoppedSince.After(t) {
			newest[sess.UserID] = *sess.StoppedSince
			keep[sess.UserID] = sess.InstanceID
		}
	}
	return occupancy, keep
}

// reapScore는 회수 우선순위 점수(높을수록 먼저 회수).
func reapScore(sess *Session, occupancy map[int64]int, now time.Time) float64 {
	days := now.Sub(*sess.StoppedSince).Hours() / 24
	n := occupancy[sess.UserID]
	if n < 1 {
		n = 1
	}
	return days * float64(n)
}

// pressuredNodes는 루트 디스크가 임계를 넘은 노드와, 임계 아래로 되돌리는 데 필요한 용량(GiB)을 반환한다.
// 메트릭 미연동이면 빈 맵 = 회수하지 않는다(판정 불가 시 보수적).
func (s *Service) pressuredNodes(ctx context.Context, thresholdPct int) map[string]int {
	out := map[string]int{}
	if s.met == nil || !s.met.Enabled() {
		return out
	}
	const join = ` * on(pod,namespace) group_left(node) kube_pod_info`
	size, ok1 := s.met.VectorByLabel(ctx, `node_filesystem_size_bytes{mountpoint="/"}`+join, "node")
	avail, ok2 := s.met.VectorByLabel(ctx, `node_filesystem_avail_bytes{mountpoint="/"}`+join, "node")
	if !ok1 || !ok2 {
		return out
	}
	const giB = 1024 * 1024 * 1024
	for node, total := range size {
		free, ok := avail[node]
		if !ok || total <= 0 {
			continue // 한쪽만 있으면 사용률을 못 구한다. 100% 로 오판하지 않게 건너뛴다
		}
		used := total - free
		if used/total*100 < float64(thresholdPct) {
			continue
		}
		// 임계 아래로 내리는 데 필요한 바이트를 GiB 로 올린다(홈 하나 단위로 지우므로 1GiB 미만도 1로).
		if excess := used - total*float64(thresholdPct)/100; excess > 0 {
			out[node] = int(excess/giB) + 1
		}
	}
	return out
}

// recordReap은 회수 동작을 감사 로그에 남긴다(누구 것을 왜 지웠는지 사후 추적 가능해야 한다).
func (s *Service) recordReap(action, target string) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Insert(&audit.Log{ActorUsername: "system", Action: action, Target: target, Result: "applied"})
}
