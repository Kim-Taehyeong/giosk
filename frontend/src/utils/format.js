// 초를 남은 시간 문자열(ETA)로 바꾼다. 0 이거나 측정 불가면 빈 값을 준다. 예를 들어 45s, 3:12, 1:02:30 이다.
export function formatEta(sec) {
  const n = Math.round(Number(sec) || 0);
  if (n <= 0) return '—';
  if (n < 60) return `${n}s`;
  const h = Math.floor(n / 3600);
  const m = Math.floor((n % 3600) / 60);
  const s = n % 60;
  const pad = (x) => String(x).padStart(2, '0');
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`;
}

// 바이트를 사람이 읽기 좋은 크기 문자열로 바꾼다. 0 이거나 측정 불가면 빈 값을 준다.
export function formatBytes(bytes) {
  const n = Number(bytes) || 0;
  if (n <= 0) return '—';
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i += 1; }
  return `${v >= 100 || i === 0 ? Math.round(v) : v.toFixed(1)} ${units[i]}`;
}
