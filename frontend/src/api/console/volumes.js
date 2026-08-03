import { apiGet, apiPost, apiPut, apiDelete } from '../client';

// 볼륨 — 실 백엔드 연결. 백엔드 owned/shared/quota → 프론트 shape 보정.
export const getVolumes = () =>
  apiGet('/volumes').then((d) => ({
    owned: (d.owned || []).map((v) => ({ ...v, perm: 'owner', sharedWith: v.sharedWith || [] })),
    // sharedBy = 공유자(소유자) 이름(백엔드 ownerName).
    shared: (d.shared || []).map((v) => ({ ...v, perm: v.perm || 'rw', sharedBy: v.ownerName || v.sharedBy || '' })),
    localHomes: d.localHomes || [],
    quota: d.quota || { allocatedGb: 0, totalGb: 0 },
  }));

export const createVolume = (form) => apiPost('/volumes', { name: form.name, sizeGib: form.sizeGib });
export const shareVolume = (id, body) => apiPost(`/volumes/${id}/share`, body);
export const changeVolumeTeam = (id, groupId) => apiPut(`/volumes/${id}/team`, { groupId }); // 귀속 팀 변경
export const deleteVolume = (id) => apiDelete(`/volumes/${id}`);
