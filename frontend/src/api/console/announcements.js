import { apiGet, apiPost, apiPut, apiDelete } from '../client';

// 공지. 실 백엔드다.
//  getAnnouncements    : 활성 공지(대시보드 배너). 모든 사용자가 보되 전역과 내 조직·그룹 타겟만 온다
//  getAllAnnouncements : 관리자 목록(/console). 호출자 스코프로 필터하며 platform 은 전체를 본다
export const getAnnouncements = () => apiGet('/announcements').then((d) => ({ items: d.items || [] }));
export const getAllAnnouncements = () => apiGet('/console/announcements').then((d) => ({ items: d.items || [] }));

// 작성과 수정은 /console 이다. 타겟(targetOrgId/targetGroupId)은 platform 만 자유롭고 매니저는 백엔드가 자기 범위로 강제한다.
export const createAnnouncement = (body) => apiPost('/console/announcements', body);
export const updateAnnouncement = (id, body) => apiPut(`/console/announcements/${id}`, body);
export const deleteAnnouncement = (id) => apiDelete(`/console/announcements/${id}`);
export const toggleAnnouncement = (id) => apiPost(`/console/announcements/${id}/toggle`);
