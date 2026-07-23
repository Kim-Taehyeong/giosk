// 세션 실사용 지표(/metrics/sessions)를 막대로 그리기 위한 공용 규칙.
// 사용자 세션 목록과 관리자 관제가 같은 규칙을 써야 같은 세션이 두 화면에서 다르게 보이지 않는다.

// GPU 지표를 잴 수 없는 세션인지 — 타임슬라이싱(카드 단위 계측이라 세션별로 갈리지 않음),
// 물리 임대(Pod 이 없음), exporter/Prometheus 미가용. CPU 전용 세션은 GPU 가 없는 것이지 못 재는 게 아니다.
export function gpuUnmeasurable(r) {
  return r.gpuSource === 'none' && !!r.gpuReason && r.gpuReason !== 'cpu';
}

const pct = (used, limit) => (limit > 0 ? Math.min(100, (used / limit) * 100) : 0);
const gb = (mb) => (mb / 1024).toFixed(1);

// 잴 수 있는 지표만 막대로 만든다. null 은 "측정 불가"라 막대를 그리지 않는다 — 0% 막대로 그리면
// 놀고 있는 세션과 구분이 안 된다. all=false(목록)면 GPU 위주로 압축하고, 못 재면 CPU/RAM 으로 대체한다.
export function measureRows(r, all = false) {
  const out = [];
  if (r.gpuUtil != null) {
    out.push({ label: 'GPU', pct: r.gpuUtil, txt: `${Math.round(r.gpuUtil)}%`, variant: r.gpuUtil < 10 ? 'warn' : 'gpu' });
  }
  if (r.vramUsedMb != null) {
    const txt = r.vramTotalMb > 0 ? `${gb(r.vramUsedMb)}/${(r.vramTotalMb / 1024).toFixed(0)}G` : `${gb(r.vramUsedMb)}G`;
    out.push({ label: 'VRAM', pct: pct(r.vramUsedMb, r.vramTotalMb), txt, variant: 'gpu' });
  }
  if (all || !out.length) {
    if (r.cpuCores != null) {
      const txt = r.cpuLimitCores > 0 ? `${r.cpuCores.toFixed(1)}/${r.cpuLimitCores}c` : `${r.cpuCores.toFixed(1)}c`;
      out.push({ label: 'CPU', pct: pct(r.cpuCores, r.cpuLimitCores), txt, variant: 'free' });
    }
    if (r.memUsedMb != null) {
      const txt = r.memLimitMb > 0 ? `${gb(r.memUsedMb)}/${(r.memLimitMb / 1024).toFixed(0)}G` : `${gb(r.memUsedMb)}G`;
      out.push({ label: 'RAM', pct: pct(r.memUsedMb, r.memLimitMb), txt, variant: 'free' });
    }
  }
  return out;
}
