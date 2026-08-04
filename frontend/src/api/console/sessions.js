import { apiGet, apiPost, apiDelete } from '../client';

// createdAt(ISO)을 경과 실행시간 문자열("2h 13m", "13m", "45s")로 바꾼다. 없으면 표기를 비운다.
export function fmtRuntime(createdAt) {
  if (!createdAt) return '—';
  const start = new Date(createdAt).getTime();
  if (Number.isNaN(start)) return '—';
  let sec = Math.max(0, Math.floor((Date.now() - start) / 1000));
  const d = Math.floor(sec / 86400); sec -= d * 86400;
  const h = Math.floor(sec / 3600); sec -= h * 3600;
  const m = Math.floor(sec / 60); sec -= m * 60;
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m`;
  return `${sec}s`;
}

// 백엔드 채널 키를 표시 라벨로 바꾼다(연결 탭과 배지).
const CONN_LABEL = { vscode: 'VSCode', jupyter: 'Jupyter', ssh: 'SSH' };

// 백엔드 Usage 를 목록 행에 머지할 라이브 지표로 바꾼다.
// gpuUtil/vramUsedMb/cpuCores/memUsedMb 는 null 이 "측정 불가"다. 0(안 쓰는 중)과 구분해 UI 에서 갈라 그린다.
// gpuSource=none 이면 gpuReason 이 사유(timeslice=타임슬라이싱은 카드 단위라 세션별 계측 불가 등).
function adaptUsage(u) {
  const total = u.vramTotalMb || 0;
  return {
    cpuCores: u.cpuCores,
    cpuLimitCores: u.cpuLimitCores || 0,
    memUsedMb: u.memUsedMb,
    memLimitMb: u.memLimitMb || 0,
    gpuUtil: u.gpuUtil,
    vramUsedMb: u.vramUsedMb,
    vramTotalMb: total,
    vramUtil: u.vramUsedMb != null && total > 0 ? (u.vramUsedMb / total) * 100 : null,
    gpuSource: u.gpuSource,
    gpuReason: u.gpuReason || '',
  };
}

// 백엔드 Session 을 프론트 세션 목록 shape 으로 변환한다(라이브 지표는 metrics 에서 머지).
function adaptSession(s) {
  const gb = (s.vramMb || 0) / 1024;
  const ssh = s.env === 'ssh'; // 물리노드 임대 세션
  const offering = ssh
    ? `물리 노드 · ${s.node || ''}`
    : s.gpuMode === 'cpu'
      ? 'CPU'
      : s.gpuMode === 'exclusive'
        ? `${s.gpuType || 'GPU'} ×${s.gpuCount || 1}` // 전용=GPU 통째 (VRAM/코어% 미표기)
        : s.gpuMode === 'timeslice'
          ? `${s.gpuType || 'GPU'}` // 타임슬라이스=시분할 (VRAM/코어% 개념 없음)
          : s.gpuType && (s.vramMb || s.corePercent)
            ? `${s.gpuType} · ${gb.toFixed(0)}GB·GPU ${s.corePercent}%` // 분할=VRAM·GPU 점유율
            : (s.gpuType || `${gb.toFixed(0)}GB·GPU ${s.corePercent}%`); // 폴백: 0/0 이면 GPU타입만
  const running = s.status === 'running';
  return {
    id: s.id,
    name: s.name,
    mode: ssh ? 'ssh' : (s.gpuMode || 'shared'),
    offering,
    runtime: running ? fmtRuntime(s.startedAt || s.createdAt) : '—',
    startedAt: s.startedAt || s.createdAt,
    idleMin: 0,
    // 실제 소비 = 정산(billed)과 실시간 누적 중 큰 값(정산 주기 사이에도 반영).
    credit: Math.max(s.billedCredits || 0, running ? accruedCredits(s.startedAt || s.createdAt, s.pricePerHour) : 0),
    pricePerHour: s.pricePerHour || 0,
    status: s.status,
    conn: running ? ((s.channels && s.channels.length ? s.channels : (ssh ? ['ssh'] : ['vscode'])).map((c) => CONN_LABEL[c] || c)) : [],
    node: s.node || '—',
    gpuId: '',
    // 지표는 /metrics/sessions 를 머지하기 전까지 "측정 불가"(null)다. 0 으로 두면 유휴처럼 보인다.
    cpuCores: null,
    cpuLimitCores: s.cpuCores || 0,
    memUsedMb: null,
    memLimitMb: (s.memGb || 0) * 1024,
    gpuUtil: null,
    vramUtil: null,
    vramUsedMb: null,
    vramTotalMb: s.vramMb || 0,
    gpuSource: '',
    gpuReason: '',
    autoStopIdleMin: 0,
    logs: [],
    extensionsUsed: s.extensionsUsed || 0,
    // 원시 사양이다. 중단 세션 자원 변경(GPU 붙이기) 모달이 현재 값에서 시작하도록 그대로 보존한다.
    // (표시용 offering 이나 node 는 빈 값 표기가 섞여 그대로 쓸 수 없다.)
    spec: {
      gpuMode: s.gpuMode || '',
      gpuType: s.gpuType || '',
      gpuCount: s.gpuCount || 0,
      offeringId: s.offeringId || null,
      imageId: s.imageId || null,
      vramMb: s.vramMb || 0,
      corePercent: s.corePercent || 0,
      node: s.node || '', // 홈 PVC 가 묶인 노드(비면 아직 뜬 적 없음 = 노드 제약 없음)
    },
  };
}

// 세션 연결 정보(게이트웨이 URL/명령).
export const getConnection = (id) => apiGet(`/instances/${id}/connection`);

// 게이트웨이 단기 접속 토큰을 열 때마다 새로 발급한다(토큰 포함 웹 URL 과 복붙 SSH 명령).
export const getAccess = (id) => apiPost(`/instances/${id}/access`);

// 세션 활동 기록(감사). 해당 세션을 대상으로 한 audit_logs 다.
export const getSessionAudit = (id) =>
  apiGet(`/instances/${id}/audit`).then((d) => (d.items || []).map((a) => ({
    ts: (a.createdAt || '').replace('T', ' ').slice(0, 19),
    action: a.action,
    actor: a.actorUsername || 'system',
    result: a.result,
  })));

export const getMySessions = () => apiGet('/instances').then((d) => (d.items || []).map(adaptSession));

// 본인 running 세션 실사용 지표(id 별 지표). 세션 수와 무관하게 한 번만 호출한다.
const usageMap = (d) => Object.fromEntries(Object.entries(d.items || {}).map(([id, u]) => [id, adaptUsage(u)]));
export const getMySessionMetrics = () => apiGet('/metrics/sessions').then(usageMap);

// 세션 목록 + 지표를 함께 읽어 머지. 지표 조회가 실패해도 목록은 살린다(Prometheus 장애 ≠ 목록 장애).
export const getMySessionsWithUsage = () =>
  Promise.all([getMySessions(), getMySessionMetrics().catch(() => ({}))])
    .then(([list, usage]) => list.map((r) => (usage[r.id] ? { ...r, ...usage[r.id] } : r)));
export const createSession = (body) => apiPost('/instances', body);
export const stopSession = (id) => apiPost(`/instances/${id}/stop`);
export const startSession = (id) => apiPost(`/instances/${id}/start`);
export const extendSession = (id) => apiPost(`/instances/${id}/extend`, { hours: 1 });
// 중단 세션 자원 변경(GPU 붙이기/떼기). start=true 면 서버가 적용 후 바로 재개한다.
export const reconfigureSession = (id, body) => apiPost(`/instances/${id}/reconfigure`, body);
// 홈을 버리고 다른 노드에서 다시 시작한다. 되돌릴 수 없으므로 호출 전에 반드시 확인을 받는다.
export const reallocateSession = (id, start) => apiPost(`/instances/${id}/reallocate`, { start });
export const deleteSession = (id) => apiDelete(`/instances/${id}`);

// 실시간 누적 크레딧(= 시간당 단가 × 실행시간, 내림). 정산(billed)과 동일 공식이라 더 즉각적.
export function accruedCredits(startedAt, pricePerHour) {
  if (!pricePerHour || pricePerHour <= 0 || !startedAt) return 0;
  const start = new Date(startedAt).getTime();
  if (Number.isNaN(start)) return 0;
  const hours = Math.max(0, (Date.now() - start) / 3600000);
  return Math.floor(pricePerHour * hours);
}

// 백엔드 AdminRow 를 관제 테이블 shape 으로 바꾼다.
function adaptAdminRow(r) {
  const offering = r.env === 'ssh' ? '물리 노드' : r.gpuMode === 'cpu' ? 'CPU' : (r.gpuType || '—');
  const running = r.status === 'running';
  // 실제 소비 = 정산된 billed_credits 와 실시간 누적값 중 큰 값(정산 주기 사이에도 반영).
  const credit = Math.max(r.consumed || 0, running ? accruedCredits(r.startedAt, r.pricePerHour) : 0);
  return {
    id: r.id,
    name: r.name,
    owner: r.owner || '—',
    org: r.org || '',
    group: r.group || '—',
    offering,
    gpu: r.node || '—',
    runtime: running ? fmtRuntime(r.startedAt || r.createdAt) : '—',
    idle: 0,
    credit,
    status: r.status,
    mode: r.env === 'ssh' ? 'ssh' : (r.gpuMode || 'shared'),
    // 지표를 머지하기 전 기본값이다. null 이 측정 불가를 뜻한다.
    cpuCores: null,
    cpuLimitCores: 0,
    memUsedMb: null,
    memLimitMb: 0,
    gpuUtil: null,
    vramUtil: null,
    vramUsedMb: null,
    vramTotalMb: 0,
    gpuSource: '',
    gpuReason: '',
  };
}

// admin 세션 관제.
export const getAllSessions = () =>
  apiGet('/admin/sessions').then((d) => (d.items || []).map(adaptAdminRow));

// admin 관제 목록 + running 세션 실사용 지표 머지(지표 실패해도 목록은 살린다).
export const getAllSessionsWithUsage = () =>
  Promise.all([getAllSessions(), apiGet('/admin/sessions/metrics').then(usageMap).catch(() => ({}))])
    .then(([list, usage]) => list.map((r) => (usage[r.id] ? { ...r, ...usage[r.id] } : r)));
export const forceTerminate = (id) => apiPost(`/admin/sessions/${id}/terminate`);
export const adminStopSession = (id) => apiPost(`/admin/sessions/${id}/stop`);
export const adminDeleteSession = (id) => apiDelete(`/admin/sessions/${id}`);

// 단일 세션 관제 상세. 원본 행(userId 포함)에 adaptAdminRow 파생과 실사용 지표를 머지한다.
export const getAdminSession = (id) =>
  Promise.all([
    apiGet(`/admin/sessions/${id}`),
    apiGet('/admin/sessions/metrics').then(usageMap).catch(() => ({})),
  ]).then(([raw, usage]) => ({ ...adaptAdminRow(raw), userId: raw.userId, image: raw.image, createdAt: raw.createdAt, startedAt: raw.startedAt, pricePerHour: raw.pricePerHour, env: raw.env, gpuMode: raw.gpuMode, ...(usage[id] || {}) }));

// 세션 감사 로그(관제).
export const getAdminSessionAudit = (id) => apiGet(`/admin/sessions/${id}/audit`).then((d) => d.items || []);

// 세션 파드의 쿠버네티스 로그(tail). 관제 상세용이다.
export const getAdminSessionLogs = (id) => apiGet(`/admin/sessions/${id}/logs`).then((d) => d.logs || '');

// 세션 파드 describe(상태, 컨테이너, 조건, 이벤트와 오류). 관제 진단용이다.
export const getAdminSessionDescribe = (id) => apiGet(`/admin/sessions/${id}/describe`).then((d) => d.describe || null);

// 세션 사용률 이력(기본 24h). Prometheus Range 라 DB 에 저장하지 않는다. points[] 와 insights 를 준다.
export const getSessionHistory = (id, hours = 24) =>
  apiGet(`/instances/${id}/history?hours=${hours}`).then((d) => ({ points: d.points || [], insights: d.insights || {} }));
export const getAdminSessionHistory = (id, hours = 24) =>
  apiGet(`/admin/sessions/${id}/history?hours=${hours}`).then((d) => ({ points: d.points || [], insights: d.insights || {} }));
