// 콘솔 레벨 모델. 로그인한 사용자가 어느 콘솔로 갈지 정한다.
//  platform : 최고(플랫폼) 관리자, /console/admin
//  org      : 조직 관리자, /console/manage  (scope=org)
//  group    : 그룹 관리자, /console/manage  (scope=group)
//  user     : 일반 사용자, /console
export const consoleLevelOf = (user) => {
  if (!user) return 'user';
  if (user.role === 'admin') return 'platform';
  if (user.consoleLevel) return user.consoleLevel;
  if (user.membershipRole === 'org_admin') return 'org';
  if (user.membershipRole === 'group_admin') return 'group';
  return 'user';
};

// activeScopeOf는 멀티롤 사용자의 현재 활성 스코프({level,orgId,groupId})다. 전환기 선택이 우선이고,
// 없으면 기본(user.scopes[0]).
//  최고관리자(admin): 기본 platform 이지만, 전환기로 조직/그룹을 고르면(activeScope) 그 스코프로
//  내려가서 관리한다(백엔드도 X-Console-Scope 를 존중한다). nav 와 페이지가 그 레벨로 좁혀진다.
//  admin 은 전권이라 보유 스코프 집합 대조 없이 activeScope 를 그대로 신뢰한다.
export const activeScopeOf = (user, activeScope) => {
  if (!user) return null;
  if (user.role === 'admin') {
    if (activeScope) {
      const [level, idStr] = activeScope.split(':');
      const id = Number(idStr);
      if (level === 'org' && id) return { level: 'org', orgId: id };
      if (level === 'group' && id) return { level: 'group', groupId: id };
    }
    return { level: 'platform' };
  }
  const scopes = user.scopes || [];
  if (activeScope) {
    const [level, idStr] = activeScope.split(':');
    const id = Number(idStr);
    const hit = scopes.find((s) => s.level === level && (level === 'org' ? s.orgId === id : s.groupId === id));
    if (hit) return hit;
  }
  return scopes[0] || null;
};

// activeLevelOf는 활성 스코프 기준 콘솔 레벨이다(nav 와 배지 필터용). 전환기 선택을 반영한다.
export const activeLevelOf = (user, activeScope) => {
  const s = activeScopeOf(user, activeScope);
  return s ? s.level : consoleLevelOf(user);
};

// 단일 콘솔이다. platform, org, group 관리자 모두 /console/admin 으로 가고 레벨별 탭과 데이터는 백엔드 스코프로 갈린다.
export const consolePathFor = (user) => {
  const lvl = consoleLevelOf(user);
  if (lvl === 'platform' || lvl === 'org' || lvl === 'group') return '/console/admin';
  return '/console';
};

// 관리자 콘솔 진입 후 첫 화면이다. 모든 관리 레벨이 자기 범위의 운영 대시보드로 간다. 인프라는 platform 전용 탭이다.
export const consoleHomeFor = () => 'dashboard/ops';

export const isManager = (user) => ['org', 'group'].includes(consoleLevelOf(user));
