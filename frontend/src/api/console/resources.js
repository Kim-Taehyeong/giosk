import { apiGet, apiPost, apiPut, apiDelete } from '../client';

// 오퍼링/프리셋/GPU타입/이미지 — 실 백엔드 연결.

export const getOfferings = () => apiGet('/offerings?active=true').then((d) => ({ items: d.items || [] }));
// 노드 라벨에서 수집된 GPU 타입 + 단가(전용/시간, 분할 GB·시간, 분할 코어%·시간).
export const getGpuTypes = () =>
  apiGet('/gpu-types').then((d) => ({ cpuPricePerHour: d.cpuPricePerHour || 0, items: (d.items || []).map((g) => ({
    name: g.name, nodes: g.nodes,
    fullPricePerHour: g.pricePerHour || 0,
    pricePerGb: g.pricePerGb || 0,
    pricePerCore: g.pricePerCore || 0,
    // 타임셰어링(노드가 GPU 를 slots 개로 광고) — 새 세션에서 슬롯 옵션 노출용.
    timeslicing: !!g.timeslicing,
    slots: g.slots || 0,
    // 최소 후보 노드 사양 — CPU·메모리 최소 보장 = nodeCpu × (GPU 지분). 가상 기준값 하드코딩 금지.
    nodeCpu: g.nodeCpu || 0,
    nodeMemGb: g.nodeMemGb || 0,
    nodeGpus: g.nodeGpus || 0,
    cudaVersion: g.cudaVersion || '',
    cudaMin: g.cudaMin || '',
    cudaMax: g.cudaMax || '',
  })) }));

// GPU 타입별 단가(전용/시간, 분할 VRAM GB/시간, 코어%/시간) — 과금 모델 관리자.
export const getGpuPricing = () => apiGet('/admin/gpu-pricing').then((d) => (d.items || []));
export const setGpuPricing = (body) => apiPut('/admin/gpu-pricing', body); // {gpuType, pricePerHour, pricePerGb, pricePerCore}
export const getAdminGpuTypes = () => apiGet('/admin/gpu-types').then((d) => (d.items || []));
export const getPresets = () => apiGet('/presets?active=true').then((d) => ({ items: d.items || [] }));

// GPU 가용 현황(세션 마법사·대시보드 공용 단일 소스). byType[]·byNode[].
// 클러스터 노드 capacity − running 세션 기반(실시간).
export const getAvailability = () =>
  apiGet('/resources/availability').then((d) => ({ byType: d.byType || [], byNode: d.byNode || [] }));

// 관리자 CRUD (id 유무로 생성/수정).
export const saveOffering = (o) => (o.id ? apiPut(`/admin/offerings/${o.id}`, o) : apiPost('/admin/offerings', o));
export const deleteOffering = (id) => apiDelete(`/admin/offerings/${id}`);
export const savePreset = (p) => (p.id ? apiPut(`/admin/presets/${p.id}`, p) : apiPost('/admin/presets', p));
export const deletePreset = (id) => apiDelete(`/admin/presets/${id}`);

// 이미지 카탈로그 — 백엔드 image → 프론트 image(shape: id/name/base/desc/conns/tags/gpu).
export const getImages = () => apiGet('/images?active=true').then((d) => ({ items: (d.items || []).map(adaptImage) }));

// 관리자 이미지 레지스트리 — 실 백엔드(빌드/게시/삭제).
export const getAdminImages = () => apiGet('/admin/images').then((d) => (d.items || []).map(adaptAdminImage));
// 가이드형 빌드: 베이스 + apt/pip 패키지만 보내면 서버가 Dockerfile 생성 → Kaniko 빌드.
export const buildImage = (form) => apiPost('/admin/images', {
  name: form.name,
  desc: form.desc || '',
  base: form.base,
  apt: form.apt || '',
  pip: form.pip || '',
  channels: { vscode: !!form.channels.VSCode, jupyter: !!form.channels.Jupyter, ssh: true },
  gpu: true,
});
// 외부 이미지 등록: 빌드 없이 Docker Hub/외부 레지스트리 ref 를 그대로 등록.
// 웹 채널(vscode/jupyter/커스텀 포트)이 하나도 없으면 세션은 SSH 전용.
export const registerExternalImage = (form) => apiPost('/admin/images/external', {
  ref: form.ref,
  desc: form.desc || '',
  gpu: true,
  channels: {
    vscode: !!form.channels.VSCode,
    jupyter: !!form.channels.Jupyter,
    ssh: true,
    ...(form.webPort ? { web: { name: form.webName || 'app', port: Number(form.webPort) } } : {}),
  },
});
export const publishImage = (id) => apiPost(`/admin/images/${id}/status`, { status: 'active' });
export const rebuildImage = (id) => apiPost(`/admin/images/${id}/rebuild`, {});
export const deleteImage = (id) => apiDelete(`/admin/images/${id}`);

// 빌드/스캔 Job 로그(실패 진단).
export const getImageLogs = (id) => apiGet(`/admin/images/${id}/logs`).then((d) => d.logs || '');

// 노드별 이미지 캐싱(프리페치).
export const getImageCache = (id) => apiGet(`/admin/images/${id}/cache`).then((d) => d.items || []);
export const cacheImageOnNode = (id, node) => apiPost(`/admin/images/${id}/cache`, { node });
export const uncacheImageOnNode = (id, node) => apiDelete(`/admin/images/${id}/cache?node=${encodeURIComponent(node)}`);

// 백엔드 channels({vscode,jupyter,ssh}) → 표시용 배열(['VSCode',...]).
function adaptAdminImage(im) {
  const ch = im.channels || {};
  const channels = [];
  if (ch.vscode) channels.push('VSCode');
  if (ch.jupyter) channels.push('Jupyter');
  if (ch.ssh) channels.push('SSH');
  if (ch.web?.port) channels.push(ch.web.name || 'web');
  return {
    id: im.id,
    name: im.name,
    tag: im.tag || '',
    base: im.base || '',
    channels: channels.length ? channels : ['SSH'],
    dockerfile: im.dockerfile || '',  // 있으면 빌드 이미지(재빌드 가능), 없으면 외부
    external: !im.dockerfile,          // 외부 이미지 라벨용
    webPort: ch.web?.port || 0,
    scan: im.scan || 'pass',
    sign: !!im.sign,
    status: im.status || 'active',
  };
}

function adaptImage(im) {
  const ch = im.channels || {};
  const conns = [];
  if (ch.vscode) conns.push('VSCode');
  if (ch.jupyter) conns.push('Jupyter');
  if (ch.web?.port) conns.push(ch.web.name || 'web'); // 커스텀 웹 채널
  if (ch.ssh) conns.push('SSH');
  return {
    id: im.id,
    name: im.tag ? `${im.name}:${im.tag}` : im.name,
    base: im.base || '',
    desc: im.desc || '',
    conns: conns.length ? conns : ['SSH'],
    tags: im.tags || [],
    gpu: !!im.gpu,
    cachedNodes: im.cachedNodes || [], // 캐시 완료 노드 — 빠른 생성 배지
  };
}
