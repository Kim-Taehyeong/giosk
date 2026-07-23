import { apiGet, apiPost, apiPatch, apiDelete } from '../client';

// 데이터셋 — 실 백엔드. 사용자(/datasets)와 관리자(/admin/dataset-requests)를 합쳐
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

// 노드 로컬 캐시 토글(관리자) — 없으면 캐시 시작(NFS→노드 로컬 복사), 있으면 해제.
export const toggleDatasetCache = (id, node) => apiPost(`/admin/datasets/${id}/cache`, { node });
export const updateDatasetDescription = (id, description) => apiPatch(`/admin/datasets/${id}`, { description });
export const createDataset = (form) => registerDataset(form);

// 관리자 승인/거절.
export const approveDatasetRequest = (id) => apiPost(`/admin/dataset-requests/${id}/approve`);
export const rejectDatasetRequest = (id) => apiPost(`/admin/dataset-requests/${id}/reject`);
