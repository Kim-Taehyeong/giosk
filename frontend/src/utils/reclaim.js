// 중단 세션 홈 회수(T1) 안내 계산.
//
// 중단된 세션도 홈 디스크를 계속 점유하므로, 방치 기간이 정책(config.reclaim.stoppedTtlDays)을
// 넘기면 회수 후보가 된다. 다만 실제 삭제는 "그 세션이 있는 노드의 디스크가 임계를 넘었을 때"만
// 일어난다 — 그래서 문구는 "삭제 예정일"이 아니라 "회수 대상"이어야 한다. 확정 삭제일처럼 적으면
// 여유 있는 노드에서 몇 달째 멀쩡한 세션에 매일 거짓 경고를 띄우게 된다.
//
// 면책(reclaimExempt)은 백엔드가 내려준다 — 사용자별 가장 최근 중단 세션 1개는 회수하지 않는다.
// 프론트에서 목록으로 계산하면 팀 스코프에 걸러진 상태라 전역 최신과 달라질 수 있다.

// state:
//   'off'    회수 정책 꺼짐 / 대상 아님(실행 중, 물리 세션 등) → 표시 없음
//   'exempt' 면책(가장 최근 중단 세션) → "보관됨"
//   'due'    이미 TTL 초과 → 디스크가 차면 회수될 수 있음
//   'soon'   TTL 임박(잔여 7일 이하) → 남은 일수 표시
//   'ok'     여유 있음 → 남은 일수 표시(경고색 없이)
export function reclaimInfo(sess, config) {
  const ttlDays = config?.reclaim?.stoppedTtlDays ?? 0;
  if (!ttlDays || !sess) return { state: 'off' };
  if (sess.status !== 'stopped' || sess.mode === 'ssh') return { state: 'off' };
  if (sess.reclaimExempt) return { state: 'exempt' };
  if (!sess.stoppedSince) return { state: 'off' }; // 기준 시각 없음(도입 이전 세션) — 다음 정산 틱에 채워진다

  const elapsedDays = (Date.now() - new Date(sess.stoppedSince).getTime()) / 86400000;
  const left = Math.ceil(ttlDays - elapsedDays);
  if (left <= 0) return { state: 'due', days: 0 };
  return { state: left <= 7 ? 'soon' : 'ok', days: left };
}

// Pill variant 매핑 — due=경고(빨강), soon=주의(노랑), 그 외는 중립.
export const RECLAIM_VARIANT = { due: 'danger', soon: 'warn', exempt: 'ok', ok: 'pause' };
