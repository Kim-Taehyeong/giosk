import { apiGet, apiPut, apiPost } from '../client';

// 인앱 알림 수신함. 알림 엔진이 사용자 규칙(크레딧 등) 위반 시 적재한 알림이다.
export const getInbox = () => apiGet('/inbox').then((d) => ({ items: d.items || [], unread: d.unread || 0 }));
export const getInboxUnread = () => apiGet('/inbox/unread').then((d) => d.unread || 0);
export const markInboxRead = (id) => apiPost(`/inbox/${id}/read`);
export const markInboxAllRead = () => apiPost('/inbox/read-all');

// 알림 규칙과 채널(이메일, 웹훅). 실 백엔드이며 scope 별로 전체를 교체 저장한다.
// 규칙이 없으면 백엔드가 scope 기본 규칙을 내려준다(기본값 단일 출처=백엔드).
export const getUserNotify = () => apiGet('/notify');
export const saveUserNotify = (body) => apiPut('/notify', body);
export const getAdminNotify = () => apiGet('/admin/notify');
export const saveAdminNotify = (body) => apiPut('/admin/notify', body);
