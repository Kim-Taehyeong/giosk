import { apiGet, apiPost, apiPatch, apiDelete } from '../client';

// 데이터셋. 실 백엔드다. 사용자(/datasets)와 관리자(/admin/dataset-requests)를 합쳐
// 한 응답으로 제공(관리자만 전체 pending, 일반 사용자는 본인 pending).
export const getDatasets = async () => {
  const d = await apiGet('/datasets');
  let requests = (d.mine || []).filter((m) => m.status === 'pending');
  try {
    const r = await apiGet('/admin/dataset-requests'); // 관리자만 성공
    if (r?.items) requests = r.items;
  } catch { /* 비관리자 403 → 본인 pending 유지 */ }
  return {
    global: (d.global || []).map((g) => ({ ...g, nodes: g.nodes || [] })),
    mine: d.mine || [],
    requests,
    cache: [],
    allNodes: [],
  };
};

// 사용자: URL 등록 신청(기본 전역). 승인 시 sourceUrl 을 PVC 로 wget 적재.
export const registerDataset = (form) =>
  apiPost('/datasets/register', {
    name: form.name, sizeClass: form.sizeClass, sizeGb: form.sizeGb || 0,
    sourceUrl: form.url || form.sourceUrl || '', targetScope: form.targetScope || 'global',
  });

export const deleteDataset = (id) => apiDelete(`/datasets/${id}`);

// ── 관리자 데이터셋 등록: ① NFS 인박스(SCP 복사 후 선택)  ② URL(wget) ──
// 인박스: SCP 안내 경로 + 그 폴더에 올라온 아카이브 목록.
export const getDatasetInbox = () => apiGet('/admin/datasets/inbox');
// 인박스 파일을 데이터셋으로 등록(등록 후 인박스에서 제거 + 자동 해제).
export const registerDatasetNFS = ({ name, filename, scope, sizeClass }) => apiPost('/admin/datasets/register-nfs', { name, filename, scope: scope || 'global', sizeClass: sizeClass || '' });
// URL(wget)로 등록하면 서버가 직접 내려받는다.
export const registerDatasetURL = ({ name, url, scope, sizeClass }) => apiPost('/admin/datasets/register-url', { name, url, scope: scope || 'global', sizeClass: sizeClass || '' });

// 노드 로컬 캐시 토글(관리자). 없으면 캐시를 시작하고(NFS 에서 노드 로컬로 복사) 있으면 해제한다.
export const toggleDatasetCache = (id, node) => apiPost(`/admin/datasets/${id}/cache`, { node });
export const updateDatasetDescription = (id, description) => apiPatch(`/admin/datasets/${id}`, { description });
// 설명 + 크기 클래스 함께 저장(상세 편집).
export const updateDatasetMeta = (id, { description, sizeClass }) => apiPatch(`/admin/datasets/${id}`, { description: description ?? '', sizeClass: sizeClass || '' });
export const createDataset = (form) => registerDataset(form);

// 관리자 승인/거절.
export const approveDatasetRequest = (id) => apiPost(`/admin/dataset-requests/${id}/approve`);
export const rejectDatasetRequest = (id) => apiPost(`/admin/dataset-requests/${id}/reject`);
