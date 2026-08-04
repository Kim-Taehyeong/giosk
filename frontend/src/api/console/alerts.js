import { apiGet, apiPost, apiDelete } from '../client';

// 워크로드 알림. 자원(GPU)이나 노드(SSH)가 가용해지면 알려주는 구독이다(사용자별 실 백엔드).
export const getAlerts = () => apiGet('/alerts').then((d) => ({ items: d.items || [] }));
export const createAlert = (body) => apiPost('/alerts', body);
export const deleteAlert = (id) => apiDelete(`/alerts/${id}`);
export const toggleAlert = (id) => apiPost(`/alerts/${id}/toggle`);
