import { apiGet, apiPost, apiPut, apiDelete } from '../client';

// 거버넌스는 단일 /console 트리(레벨 인식형)다. platform, org, group 관리자가 같은 클라이언트를 쓰고
// 백엔드가 호출자 스코프로 데이터를 필터·인가한다.
export const getOrgs = () => apiGet('/console/orgs').then((d) => ({ items: d.items || [] }));
// 스코프와 무관한 전체 목록(최고관리자 전용, /admin)이다. RoleSwitcher 가 현재 채택 스코프와 무관하게
// 전체 조직/그룹을 보여줘야 하므로(스코프가 좁혀지면 /console/* 은 부분만 반환) 이걸 쓴다.
export const getAllOrgs = () => apiGet('/admin/orgs').then((d) => ({ items: d.items || [] }));
export const getAllGroups = () => apiGet('/admin/groups').then((d) => ({ items: d.items || [] }));
export const createOrg = (body) => apiPost('/console/orgs', body);        // 플랫폼 전용(백엔드 가드)
export const updateOrg = (id, body) => apiPut(`/console/orgs/${id}`, body); // 자기 조직(org admin)
export const deleteOrg = (id) => apiDelete(`/console/orgs/${id}`);          // 플랫폼 전용
export const grantOrgCredit = (id, body) => apiPost(`/console/orgs/${id}/grant`, body); // 플랫폼 전용

export const getGroups = () => apiGet('/console/groups').then((d) => ({ items: d.items || [] }));
export const createGroup = (body) => apiPost('/console/groups', body);      // org 는 자기 조직으로 강제된다
export const updateGroup = (id, body) => apiPut(`/console/groups/${id}`, body);
export const deleteGroup = (id) => apiDelete(`/console/groups/${id}`);

// 그룹 멤버는 /console 그룹범위다(org admin 은 자식 그룹까지, group admin 은 자기 그룹만).
export const getMembers = (groupId) => apiGet(`/console/groups/${groupId}/members`).then((d) => ({ items: d.items || [] }));
export const addMember = (groupId, body) => apiPost(`/console/groups/${groupId}/members`, body);
export const updateMember = (groupId, userId, body) => apiPut(`/console/groups/${groupId}/members/${userId}/role`, body);
export const removeMember = (groupId, userId) => apiDelete(`/console/groups/${groupId}/members/${userId}`);
// 그룹 이동(원자적)은 플랫폼 관리자 전용이다. role 을 생략하면 기존 역할을 유지한다.
export const moveMember = (groupId, userId, body) => apiPut(`/console/groups/${groupId}/members/${userId}/move`, body);

export const updateBudget = (groupId, body) => apiPut(`/console/groups/${groupId}/wallet/budget`, body);
export const grantGroupCredit = (groupId, body) => apiPost(`/console/groups/${groupId}/wallet/grant`, body);
export const setGroupRefill = (groupId, body) => apiPut(`/console/groups/${groupId}/wallet/refill`, body); // 팀 정기 리필(스코프)
export const refillGroupNow = (groupId) => apiPost(`/console/groups/${groupId}/wallet/refill/now`); // 팀 즉시 리필
export const getGroupWallet = (groupId) => apiGet(`/console/groups/${groupId}/wallet`); // 팀 지갑(다음 리필일 포함)
export const setMemberRefill = (groupId, userId, body) => apiPut(`/console/groups/${groupId}/members/${userId}/refill`, body); // 멤버별 정기 리필 금액
export const refillMemberNow = (groupId, userId) => apiPost(`/console/groups/${groupId}/members/${userId}/refill/now`); // 멤버 즉시 리필
