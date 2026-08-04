// 하드 리소스 제한(크레딧과 무관한 절대 상한)이다. 개인, 그룹, 조직, 전역 순으로 계층 해석한다.
// 각 한계값은 nullable 이다. null(생략)이면 이 레벨은 미지정이라 상위로 폴백한다.
import { apiGet, apiPut } from '../client';

// 정책 화면은 전역(설치 고정)과 상한이 설정된 범위를 보여준다. /console 은 호출자 스코프로 자동 필터한다
// (platform=전체, org=자기 조직+산하, group=자기 그룹). 편집/부여는 platform 전용(아래 /admin).
export const getPolicies = () =>
  apiGet('/console/limits').then((d) => ({ items: d.items || [], global: d.global || {} }));

// 사용자에게 최종 적용되는(계층 해석된) 하드 상한.
export const getUserEffectiveLimits = (id) => apiGet(`/admin/limits/user/${id}/effective`);

// 계층에 직접 설정된(미해석) 하드 제한(편집 폼 초기값). 미지정 항목은 null.
export const getUserLimits = (id) => apiGet(`/admin/limits/user/${id}`);
export const getGroupLimits = (id) => apiGet(`/admin/limits/group/${id}`);
export const getOrgLimits = (id) => apiGet(`/admin/limits/org/${id}`);

// 계층별 하드 제한 설정({maxGpu, maxVramGb, maxVolumeGib, maxConcurrentSessions}; 각 필드 null=미지정).
export const setUserLimits = (id, body) => apiPut(`/admin/limits/user/${id}`, body);
export const setGroupLimits = (id, body) => apiPut(`/admin/limits/group/${id}`, body);
export const setOrgLimits = (id, body) => apiPut(`/admin/limits/org/${id}`, body);
// 전역 상한(모든 사용자의 폴백)은 런타임에 조정한다. 전역은 폴백이라 null(미지정)이 없다.
export const setGlobalLimits = (body) => apiPut('/admin/limits/global', body);

// 스코프 편집(매니저)은 /console 트리다. 백엔드가 대상이 내 스코프인지, 상위 상한을 넘지 않는지를 강제한다.
export const getUserLimitsScoped = (id) => apiGet(`/console/limits/user/${id}`);
export const getGroupLimitsScoped = (id) => apiGet(`/console/limits/group/${id}`);
export const getOrgLimitsScoped = (id) => apiGet(`/console/limits/org/${id}`);
export const setUserLimitsScoped = (id, body) => apiPut(`/console/limits/user/${id}`, body);
export const setGroupLimitsScoped = (id, body) => apiPut(`/console/limits/group/${id}`, body);
export const setOrgLimitsScoped = (id, body) => apiPut(`/console/limits/org/${id}`, body);
// 설정 가능한 최대치(상위 유효 상한)다. 편집 폼의 최대 N 힌트로 쓴다.
export const getParentLimits = (scope, id) => apiGet(`/console/limits/${scope}/${id}/parent`);
