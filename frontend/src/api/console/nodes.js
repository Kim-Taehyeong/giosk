import { apiGet, apiPut, apiPost } from '../client';

// 관리자 노드 목록 — 실 백엔드(라이브 K8s + DB 오버레이 + DCGM/Prometheus 지표).
// 모니터링 값(util/vram/temp)은 Prometheus 미연동 시 0 으로 내려온다.
export const getAdminNodes = () => apiGet('/admin/nodes').then((d) => (d.items || []).map(adaptNode));

// 관리자 스토리지 현황(노드 디스크 + NFS 사용 + 사용자별 볼륨 할당).
export const getAdminStorage = () => apiGet('/admin/storage');

// 기존 NFS 경로를 볼륨으로 임포트(도입 환경 어댑션). body: {name, ownerUserId?, groupId?, nfsServer, nfsPath, sizeGiB}.
export const importNfsVolume = (body) => apiPost('/admin/volumes', body);

// 전체 볼륨 목록(플랫폼) — 스토리지 요약 동봉.
export const getAdminVolumes = () => apiGet('/admin/volumes').then((d) => ({ items: d.items || [], storage: d.storage || null }));
// 스코프 볼륨 열람(매니저) — 자기 조직/그룹 볼륨만.
export const getScopedVolumes = () => apiGet('/console/volumes').then((d) => ({ items: d.items || [], storage: null }));

// 노드 설정(공유 모드/분할/홈쿼터) 저장.
export const saveNodeConfig = (name, body) => apiPut(`/admin/nodes/${name}`, body);
export const cordonNode = (name) => apiPost(`/admin/nodes/${name}/cordon`);
export const uncordonNode = (name) => apiPost(`/admin/nodes/${name}/uncordon`);

// 노드 내 GPU별 실측 지표(DCGM). MB→GB 변환해 반환.
export const getNodeGPUs = (name) =>
  apiGet(`/admin/nodes/${name}/gpus`).then((d) => (d.items || []).map((g, i) => ({
    index: g.index ?? String(i),
    util: g.util || 0,
    vramUsed: Math.round((g.vramUsedMb || 0) / 1024),
    vramTotal: Math.round((g.vramTotalMb || 0) / 1024),
    temp: g.temp || 0,
  })));

// 노드 상세 페이지용 시계열 + 인사이트(hours 구간). 미연동 시 points=[], insights.samples=0.
export const getNodeMetrics = (name, hours = 24) =>
  apiGet(`/admin/nodes/${name}/metrics?hours=${hours}`).then((d) => ({
    points: d.points || [],
    insights: d.insights || {},
  }));

// 단일 노드 조회 — 목록에서 이름으로 찾는다(백엔드 단건 API 없음).
export const getAdminNode = (name) => getAdminNodes().then((ns) => ns.find((n) => n.node === name) || null);

function adaptNode(v) {
  const hasGpu = !!v.gpuType;
  const gpuLabel = hasGpu ? `${v.gpuType} ×${v.gpuCapacity}` : '— (CPU 풀)';
  const base = {
    node: v.name,
    cluster: '',
    gpu: gpuLabel,
    cudaVersion: v.cudaVersion || '',
    cpu: v.cpuCores ? `${v.cpuCores} vCPU` : '',
    mem: v.memGb ? `${v.memGb} GB` : '',
    disk: '',
    temp: v.temp || 0,
    cpuUtil: v.cpuUtil || 0,
    memUtil: v.memUtil || 0,
    status: v.cordoned ? 'cordoned' : 'ready',
    sessions: v.sessions || 0,
    datasetQuotaGb: v.datasetQuotaGb || 0,
    hami: !!v.hami,
    shareMode: v.shareMode || 'exclusive',
    splitCount: v.splitCount || 10,
    physicalLease: !!v.physicalLease,
    scratchEnabled: !!v.scratchEnabled,
    leased: !!v.leasedTo,
    leasedTo: v.leasedTo || '',
  };
  // MiniMon 분기: GPU 노드는 util/used/total(VRAM), CPU 풀은 total 미정의 → CPU/MEM 표시.
  if (hasGpu) return { ...base, util: v.util || 0, used: v.vramUsed || 0, total: v.vramTotal || 0 };
  return base;
}
