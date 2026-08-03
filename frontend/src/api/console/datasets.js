import { apiGet, apiPost, apiPatch, apiDelete, apiUploadProgress, apiPutRaw } from '../client';

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

// 최고관리자: zip/tar 파일 직접 업로드(진행률%). 서버가 NFS 에 스트리밍 저장 후 해제 Job → PVC 바인딩.
// onProgress(pct)로 브라우저→서버 전송 진행률을 실시간 표시. 전송 완료 후 서버 해제는 데이터셋 목록의 loading 상태로.
export const uploadDataset = ({ file, name, scope }, onProgress) => {
  const fd = new FormData();
  fd.append('file', file);
  fd.append('name', name);
  fd.append('scope', scope || 'global');
  return apiUploadProgress('/admin/datasets/upload', fd, onProgress);
};

export const deleteDataset = (id) => apiDelete(`/datasets/${id}`);

// ── 청크 이어올리기(대용량, Cloudflare 100MB 리밋 우회 + 재개) ──
const q = (o) => Object.entries(o).map(([k, v]) => `${k}=${encodeURIComponent(v)}`).join('&');
export const uploadInit = (name, filename) => apiPost('/admin/datasets/upload/init', { name, filename });
export const uploadChunk = (name, filename, offset, blob) => apiPutRaw(`/admin/datasets/upload/chunk?${q({ name, filename, offset })}`, blob);
export const uploadStatus = (name, filename) => apiGet(`/admin/datasets/upload/status?${q({ name, filename })}`);
export const uploadFinish = (name, scope, filename, size) => apiPost('/admin/datasets/upload/finish', { name, scope, filename, size });

// chunkedUpload는 파일을 청크로 잘라 순차 업로드한다. onProgress(pct, uploaded, total).
// 네트워크 중단(offset 불일치 409)이면 서버 offset 으로 재동기화. signal 로 취소 가능.
const CHUNK = 16 * 1024 * 1024; // 16MB — Cloudflare 100MB 리밋 안쪽
export async function chunkedUpload({ file, name, scope }, onProgress, signal) {
  const filename = file.name;
  const total = file.size;
  let { offset } = await uploadInit(name, filename);
  onProgress && onProgress(Math.round((offset / total) * 100) || 0, offset, total);
  while (offset < total) {
    if (signal?.aborted) throw new Error('aborted');
    const end = Math.min(offset + CHUNK, total);
    try {
      const r = await uploadChunk(name, filename, offset, file.slice(offset, end));
      offset = r.offset;
    } catch (e) {
      if (e.status === 409) { const s = await uploadStatus(name, filename); offset = s.offset; continue; } // 서버 offset 재동기화
      throw e;
    }
    onProgress && onProgress(Math.round((offset / total) * 100), offset, total);
  }
  await uploadFinish(name, scope, filename, total);
}

// 노드 로컬 캐시 토글(관리자) — 없으면 캐시 시작(NFS→노드 로컬 복사), 있으면 해제.
export const toggleDatasetCache = (id, node) => apiPost(`/admin/datasets/${id}/cache`, { node });
export const updateDatasetDescription = (id, description) => apiPatch(`/admin/datasets/${id}`, { description });
export const createDataset = (form) => registerDataset(form);

// 관리자 승인/거절.
export const approveDatasetRequest = (id) => apiPost(`/admin/dataset-requests/${id}/approve`);
export const rejectDatasetRequest = (id) => apiPost(`/admin/dataset-requests/${id}/reject`);
