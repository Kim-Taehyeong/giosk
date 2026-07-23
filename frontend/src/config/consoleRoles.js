// 콘솔 레벨 모델 — 로그인한 사용자가 어느 콘솔로 가는지 결정.
//  platform : 최고(플랫폼) 관리자  → /console/admin
//  org      : 조직 관리자          → /console/manage  (scope=org)
//  group    : 그룹 관리자          → /console/manage  (scope=group)
//  user     : 일반 사용자          → /console
export const consoleLevelOf = (user) => {
  if (!user) return 'user';
  if (user.role === 'admin') return 'platform';
  if (user.consoleLevel) return user.consoleLevel;
  if (user.membershipRole === 'org_admin') return 'org';
  if (user.membershipRole === 'group_admin') return 'group';
  return 'user';
};

// activeScopeOf — 멀티롤 사용자의 현재 활성 스코프({level,orgId,groupId}). 전환기 선택 우선,
// 없으면 기본(user.scopes[0]). platform 관리자는 항상 platform(전환 불필요).
export const activeScopeOf = (user, activeScope) => {
  if (!user) return null;
  if (user.role === 'admin') return { level: 'platform' };
  const scopes = user.scopes || [];
  if (activeScope) {
    const [level, idStr] = activeScope.split(':');
    const id = Number(idStr);
    const hit = scopes.find((s) => s.level === level && (level === 'org' ? s.orgId === id : s.groupId === id));
    if (hit) return hit;
  }
  return scopes[0] || null;
};

// activeLevelOf — 활성 스코프 기준 콘솔 레벨(nav/배지 필터용). 전환기 반영.
export const activeLevelOf = (user, activeScope) => {
  const s = activeScopeOf(user, activeScope);
  return s ? s.level : consoleLevelOf(user);
};

// 단일 콘솔 — platform/org/group 관리자 모두 /console/admin(레벨별 탭·데이터는 백엔드 스코프로 분기).
export const consolePathFor = (user) => {
  const lvl = consoleLevelOf(user);
  if (lvl === 'platform' || lvl === 'org' || lvl === 'group') return '/console/admin';
  return '/console';
};

// 관리자 콘솔 진입 후 첫 화면 — 전 관리 레벨이 운영 대시보드로(자기 범위). 인프라는 platform 전용 탭.
export const consoleHomeFor = () => 'dashboard/ops';

export const isManager = (user) => ['org', 'group'].includes(consoleLevelOf(user));
