import { apiGet, apiPost, apiDelete } from '../client';

// 사용자 소속 컨텍스트. 실 백엔드이며 응답 {org, myGroups, directory, joinRequests} 이 프론트 shape 과 같다.
export const getMembershipContext = () => apiGet('/me/membership-context');

// 볼륨 공유 대상은 내 그룹과 동료 사용자다(실 백엔드). {users:[{username,name}], groups:[{id,displayName}]}
export const getShareTargets = () => apiGet('/me/share-targets').then((d) => ({ users: d.users || [], groups: d.groups || [] }));

// 그룹 가입 신청은 해당 그룹 관리자의 요청 승인 큐로 간다.
export const requestJoinGroup = (groupId) => apiPost('/me/join-requests', { groupId });

// 본인 대기중 가입 신청 취소.
export const cancelJoinRequest = (reqId) => apiDelete(`/me/join-requests/${reqId}`);

// 조직 가입 신청은 현재 백엔드가 지원하지 않아 noop 이다.
export const requestJoinOrg = () => Promise.resolve({ ok: true });
