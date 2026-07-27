import { apiGet, apiPost, apiPut, apiDelete } from '../client';

// 거버넌스 — 단일 /console 트리(레벨 인식형). platform/org/group 관리자가 같은 클라이언트를 쓰고
// 백엔드가 호출자 스코프로 데이터를 필터·인가한다.
export const getOrgs = () => apiGet('/console/orgs').then((d) => ({ items: d.items || [] }));
// 스코프 무관 전체 목록(최고관리자 전용, /admin) — RoleSwitcher 가 현재 채택 스코프와 무관하게
// 전체 조직/그룹을 보여줘야 하므로(스코프가 좁혀지면 /console/* 은 부분만 반환) 이걸 쓴다.
export const getAllOrgs = () => apiGet('/admin/orgs').then((d) => ({ items: d.items || [] }));
export const getAllGroups = () => apiGet('/admin/groups').then((d) => ({ items: d.items || [] }));
export const createOrg = (body) => apiPost('/console/orgs', body);        // 플랫폼 전용(백엔드 가드)
export const updateOrg = (id, body) => apiPut(`/console/orgs/${id}`, body); // 자기 조직(org admin)
export const deleteOrg = (id) => apiDelete(`/console/orgs/${id}`);          // 플랫폼 전용
export const grantOrgCredit = (id, body) => apiPost(`/console/orgs/${id}/grant`, body); // 플랫폼 전용

export const getGroups = () => apiGet('/console/groups').then((d) => ({ items: d.items || [] }));
export const createGroup = (body) => apiPost('/console/groups', body);      // org→자기 조직 강제
export const updateGroup = (id, body) => apiPut(`/console/groups/${id}`, body);
export const deleteGroup = (id) => apiDelete(`/console/groups/${id}`);

// 그룹 멤버 — /console 그룹범위(org admin 은 자식 그룹까지, group admin 은 자기 그룹).
export const getMembers = (groupId) => apiGet(`/console/groups/${groupId}/members`).then((d) => ({ items: d.items || [] }));
export const addMember = (groupId, body) => apiPost(`/console/groups/${groupId}/members`, body);
export const updateMember = (groupId, userId, body) => apiPut(`/console/groups/${groupId}/members/${userId}/role`, body);
export const removeMember = (groupId, userId) => apiDelete(`/console/groups/${groupId}/members/${userId}`);
// 그룹 이동(원자적) — 플랫폼 관리자 전용. role 생략 시 기존 역할 유지.
export const moveMember = (groupId, userId, body) => apiPut(`/console/groups/${groupId}/members/${userId}/move`, body);

export const updateBudget = (groupId, body) => apiPut(`/console/groups/${groupId}/wallet/budget`, body);
export const grantGroupCredit = (groupId, body) => apiPost(`/console/groups/${groupId}/wallet/grant`, body);
export const setGroupRefill = (groupId, body) => apiPut(`/admin/groups/${groupId}/wallet/refill`, body); // 팀 정기 리필
