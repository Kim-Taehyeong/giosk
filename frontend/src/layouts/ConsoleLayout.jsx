import React from 'react';
import { Outlet, useLocation } from 'react-router-dom';
import Sidebar from '../components/console/Sidebar';
import Topbar from '../components/console/Topbar';
import BrandLogo from '../components/BrandLogo';
import { userNavGroups } from '../config/userNav';
import { adminNavGroups } from '../config/adminNav';
import { useAuth } from '../context/AuthContext';
import { useSystemConfig } from '../context/SystemConfigContext';
import { activeLevelOf } from '../config/consoleRoles';

// Dynamic(선착순) 과금 모드에선 크레딧 개념이 없으므로 크레딧 전용 메뉴를 숨긴다.
//  manageRequests = 조직 관리자의 '요청 승인'(그룹→조직 충전요청) → 순수 크레딧이므로 숨김.
//  groupRequests(그룹 관리자)는 가입 승인도 포함하므로 숨기지 않고, 페이지 안에서 크레딧 섹션만 가린다.
const CREDIT_ONLY_NAV = new Set(['wallet', 'groupBudget', 'billing', 'manageRequests']);
// 승인할 요청(가입/충전)이 하나도 없으면 '요청 승인' 메뉴 자체를 숨긴다.
const filterNav = (groups, { creditMode, approvalsActive, joinOn, datasetsOn, groupReqActive, level, gateByLevel }) => groups
  .map((g) => ({ ...g, items: g.items.filter((it) => {
    // 레벨 게이트(단일 콘솔) — levels 미지정 항목은 platform 전용(인프라·시스템 보호).
    if (gateByLevel && !(it.levels || ['platform']).includes(level)) return false;
    if (!creditMode && CREDIT_ONLY_NAV.has(it.key)) return false;
    if (it.key === 'approvals' && !approvalsActive) return false;
    if (it.key === 'joinGroup' && !joinOn) return false;
    if (it.key === 'datasets' && !datasetsOn) return false;
    // 그룹 관리자 '요청 승인' — 가입 신청도, 멤버 충전 요청도 받지 않으면 탭 자체를 숨긴다.
    if (it.key === 'groupRequests' && !groupReqActive) return false;
    return true;
  }) }))
  .filter((g) => g.items.length > 0);

// 공유 콘솔 셸: CSS-grid(logo/top/nav/main). variant: admin | user | manage.
//  manage = 조직/그룹 관리자 공용 콘솔 (scope 는 로그인 사용자의 레벨로 결정).
export default function ConsoleLayout({ variant = 'user' }) {
  const { user, activeScope, reloadScopeForConsole } = useAuth();
  const { config } = useSystemConfig();
  // 이 콘솔(관리자/사용자)의 스코프를 그 콘솔 전용 키에서 다시 로드 — 다른 콘솔에서 고른 스코프를
  // 상태로 물고 넘어오는 SPA 이동 누수를 막는다(관리자 콘솔이 사용자 팀 스코프를 물려받던 버그).
  React.useEffect(() => { reloadScopeForConsole?.(); }, [variant]); // eslint-disable-line react-hooks/exhaustive-deps
  const creditMode = config.billing.mode === 'credit';
  const approvalsActive = config.features.signupRequest || (creditMode && config.features.creditRequest);
  const joinOn = config.features.groupJoinRequest;
  const datasetsOn = config.features.datasets;
  // 그룹 관리자 '요청 승인' 탭 활성 조건: 그룹 가입 신청 또는 멤버 충전 요청 중 하나라도 허용.
  const groupReqActive = joinOn || (creditMode && config.features.creditRequest);
  const location = useLocation();
  const isAdmin = variant === 'admin';
  const ns = isAdmin ? 'consoleAdmin' : 'consoleUser';
  const level = activeLevelOf(user, activeScope); // 활성 스코프 기준 platform|org|group|user

  let basePath = '/console';
  let navGroups = userNavGroups;
  let badge = 'CONSOLE';
  if (isAdmin) {
    basePath = '/console/admin';
    navGroups = adminNavGroups;
    // 단일 콘솔 — 레벨별 배지(최고관리자/조직관리자/팀장).
    badge = level === 'org' ? 'ORG ADMIN' : level === 'group' ? 'GROUP ADMIN' : 'ADMIN CONSOLE';
  }

  navGroups = filterNav(navGroups, { creditMode, approvalsActive, joinOn, datasetsOn, groupReqActive, level, gateByLevel: isAdmin });

  return (
    <div className="console-root">
      <div className="logo">
        <BrandLogo markSize={27} badge={badge} />
      </div>
      <Topbar variant={variant} ns={ns} />
      <Sidebar navGroups={navGroups} basePath={basePath} ns={ns} />
      <main className="main">
        {/* key=경로+활성스코프 → 페이지 전환/스코프 전환 시 리마운트(스코프 바뀌면 데이터 재조회) */}
        <div className="console-page" key={`${location.pathname}::${activeScope || 'default'}`}>
          <Outlet />
        </div>
      </main>
    </div>
  );
}
