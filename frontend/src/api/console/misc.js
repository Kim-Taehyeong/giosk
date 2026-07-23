import { apiGet, apiPost, apiPut } from '../client';

// 빌링 showback — /console 은 호출자 스코프로 필터(platform=전체, org=자기 조직, group=자기 그룹).
export const getBilling = () => apiGet('/console/billing');

// 감사 로그 — /console 은 actor 가 스코프 소속인 로그만(platform=전체).
export const getAudit = () => apiGet('/console/audit').then((d) => ({ items: d.items || [] }));

// 충전 요청(플랫폼 승인 큐: 그룹→조직, 조직→플랫폼) — 실 백엔드.
export const getTopupRequests = () => apiGet('/admin/topup-requests').then((d) => ({ items: d.items || [] }));
export const approveTopup = (id) => apiPost(`/admin/topup-requests/${id}/approve`);
export const rejectTopup = (id) => apiPost(`/admin/topup-requests/${id}/reject`);

// 사용자 관리 — 실 백엔드.
// 사용자 목록 — 서버측 검색/필터/페이징(전체를 내려받지 않는다).
//  params: { q, status, group, org, page, size } → { items, total, page, size }
export const getUsers = (params = {}) => {
  const qs = new URLSearchParams(
    Object.entries(params).filter(([, v]) => v !== undefined && v !== null && v !== '' && v !== 'all'),
  ).toString();
  // 단일 /console 트리 — 백엔드가 호출자 스코프로 필터(platform=전체, org=자기 조직, group=자기 그룹).
  return apiGet('/console/users' + (qs ? `?${qs}` : '')).then((d) => ({
    items: d.items || [], total: d.total ?? (d.items || []).length,
  }));
};
// 사용자 360 상세 — 볼륨/세션/데이터셋/가입요청/지갑/사용량을 한 응답으로.
// /console 트리(스코프 검증) — platform 은 전체, 매니저는 자기 범위 사용자만.
export const getUserDetail = (id) => apiGet(`/console/users/${id}/detail`);
export const grantUserCredit = (id, body) => apiPost(`/admin/users/${id}/grant-credit`, body);
export const setUserRefill = (id, body) => apiPut(`/admin/users/${id}/refill`, body); // 개인 정기 리필
export const updateUserStatus = (id, status) => apiPut(`/admin/users/${id}/status`, { status });
export const createUser = (body) => apiPost('/admin/users', body);
export const bulkCreateUsers = (rows) => apiPost('/admin/users/bulk', { users: rows });
