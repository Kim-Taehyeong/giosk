import React, { useEffect, useRef, useState } from 'react';
import { ChevronDown, Check, Building2, FolderKanban } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useConsole } from '../../context/ConsoleContext';
import { useAuth } from '../../context/AuthContext';
import { useSystemConfig } from '../../context/SystemConfigContext';
import { getMembershipContext } from '../../api/console/membership';
import { clickable as clickableProps } from '../../utils/a11y';

// 탑바 도메인 셀렉터(사용자 뷰).
//  조직 → 팀 2뎁스. 사용자는 여러 조직·여러 팀에 속할 수 있으므로 먼저 조직을 고르고,
//  그 조직 안의 내 팀을 고른다. 선택한 팀이 활성 컨텍스트(X-Console-Scope group:N)가 되어
//  세션·크레딧(D 멤버십 지갑)이 그 팀에 귀속된다.
//  admin 클러스터 표시는 이제 관리자 뷰의 RoleSwitcher(역할 설정)가 담당 — 여기선 사용자 뷰만.
export default function OrgGroupSelector({ variant, ns }) {
  const { t } = useTranslation(ns);
  const { activeGroup, setActiveGroup, activeCluster } = useConsole();
  const { setActiveScope } = useAuth();
  const { config } = useSystemConfig();

  // 팀 선택 = 표시 컨텍스트(activeGroup) + 공유 스코프(activeScope) 동시 갱신.
  // activeScope 를 함께 바꿔야 ConsoleLayout 이 리마운트(페이지 재조회)되고 탑바 크레딧 배지가
  // 새 팀 지갑으로 갱신된다(안 그러면 헤더만 바뀌고 화면이 그대로라 "안 바뀌는" 것처럼 보임).
  const selectTeam = (g) => {
    setActiveGroup({ id: g.id, name: g.name, displayName: g.displayName, orgId: g.orgId, orgName: g.orgName });
    setActiveScope(`group:${g.id}`);
  };
  const [ctx, setCtx] = useState(null);
  const [openOrg, setOpenOrg] = useState(false);
  const [openTeam, setOpenTeam] = useState(false);
  const [orgId, setOrgId] = useState(null); // 선택된 조직(1뎁스)
  const orgRef = useRef(null);
  const teamRef = useRef(null);

  useEffect(() => { if (variant !== 'admin') getMembershipContext().then(setCtx); }, [variant]);
  useEffect(() => {
    const onDoc = (e) => {
      if (orgRef.current && !orgRef.current.contains(e.target)) setOpenOrg(false);
      if (teamRef.current && !teamRef.current.contains(e.target)) setOpenTeam(false);
    };
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, []);

  // 관리자 뷰에서 실수로 렌더되면 클러스터 이름만(안전망). 정상 경로는 RoleSwitcher.
  if (variant === 'admin') {
    return (
      <div className="proj" role="button" style={{ cursor: 'default' }}>
        <small>{t('topbar.cluster')}</small>
        <span>{activeCluster?.name || config?.branding?.name || 'Giosk'}</span>
      </div>
    );
  }

  const myGroups = ctx?.myGroups || [];
  // 내가 속한 조직 목록(팀들의 소속 조직에서 유일 추출).
  const orgs = [];
  myGroups.forEach((g) => { if (g.orgId && !orgs.some((o) => o.id === g.orgId)) orgs.push({ id: g.orgId, name: g.orgName || '—' }); });

  // 현재 팀 — 저장된 활성 팀이 내 팀 목록에 있으면 그대로, 아니면 첫 팀.
  const current = (activeGroup && myGroups.find((g) => g.id === activeGroup.id)) || myGroups[0] || null;
  // 선택 조직 — 로컬 상태 우선, 없으면 현재 팀의 조직.
  const curOrgId = orgId || current?.orgId || orgs[0]?.id || null;
  const curOrg = orgs.find((o) => o.id === curOrgId) || null;
  const teamsInOrg = myGroups.filter((g) => g.orgId === curOrgId);
  const curTeam = (current && current.orgId === curOrgId) ? current : teamsInOrg[0] || null;

  // ctx 로드 후 활성 팀이 아직 없으면 현재 팀으로 스코프 시딩(요청이 팀 스코프를 달고 나가게).
  useEffect(() => {
    if (ctx && curTeam && (!activeGroup || activeGroup.id !== curTeam.id)) {
      selectTeam(curTeam);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ctx]);

  const pickOrg = (o) => {
    setOrgId(o.id);
    setOpenOrg(false);
    // 조직을 바꾸면 그 조직의 첫 팀으로 컨텍스트 이동.
    const first = myGroups.find((g) => g.orgId === o.id);
    if (first) selectTeam(first);
  };
  const pickTeam = (g) => {
    selectTeam(g);
    setOpenTeam(false);
  };

  const cell = (labelKey, labelDefault, icon, valueNode, opened, onToggle, clickable) => {
    const Icon = icon;
    return (
      <div className="proj" style={{ cursor: clickable ? 'pointer' : 'default' }}
        aria-expanded={clickable ? !!opened : undefined} aria-haspopup={clickable ? 'menu' : undefined}
        {...clickableProps(clickable ? onToggle : undefined)}>
        <small>{t(labelKey, { defaultValue: labelDefault })}</small>
        <span className="flex gap" style={{ gap: 6, alignItems: 'center' }}>
          <Icon size={13} /> {valueNode}
          {clickable && <ChevronDown size={14} style={{ transform: opened ? 'rotate(180deg)' : 'none', transition: 'transform .15s' }} />}
        </span>
      </div>
    );
  };

  const menu = (items, activeId, onPick, emptyKey, emptyDefault, headKey, headDefault) => (
    <div role="menu" style={{
      position: 'absolute', top: 'calc(100% + 6px)', left: 0, minWidth: 240, zIndex: 70,
      background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--r-container)',
      boxShadow: '0 16px 40px rgba(10,15,28,.22)', padding: 8,
    }}>
      <div className="muted" style={{ fontSize: 10.5, fontWeight: 700, letterSpacing: '.04em', padding: '4px 8px' }}>{t(headKey, { defaultValue: headDefault })}</div>
      {items.length === 0 && <div className="muted" style={{ fontSize: 12, padding: '6px 10px' }}>{t(emptyKey, { defaultValue: emptyDefault })}</div>}
      {items.map((it) => {
        const on = it.id === activeId;
        return (
          <div key={it.id} {...clickableProps(() => onPick(it), { role: 'menuitem' })} aria-current={on || undefined}
            style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '8px 10px', borderRadius: 'var(--r-control)', cursor: 'pointer', background: on ? 'var(--primary-soft)' : 'transparent', color: on ? 'var(--primary)' : 'var(--text)', fontWeight: on ? 700 : 500 }}>
            {it.icon}
            <span style={{ flex: 1, fontSize: 13 }}>{it.label}</span>
            {on && <Check size={14} />}
          </div>
        );
      })}
    </div>
  );

  const multiOrg = orgs.length > 1;
  return (
    <>
      {/* 1뎁스 — 조직 (여러 조직이면 전환 드롭다운, 하나면 정적) */}
      <div ref={orgRef} style={{ position: 'relative' }}>
        {cell('topbar.org', '조직', Building2, curOrg?.name || '—', openOrg, () => { setOpenOrg((o) => !o); setOpenTeam(false); }, multiOrg)}
        {openOrg && multiOrg && menu(
          orgs.map((o) => ({ id: o.id, label: o.name, icon: <Building2 size={14} /> })),
          curOrgId, pickOrg, 'topbar.noOrg', '소속 조직 없음', 'topbar.myOrgs', '내 조직',
        )}
      </div>

      {/* 2뎁스 — 팀 (선택 조직 안의 내 팀 전환) */}
      <div ref={teamRef} style={{ position: 'relative' }}>
        {cell('topbar.selectGroup', '팀 선택', FolderKanban, curTeam?.displayName || curTeam?.name || t('topbar.noGroup', { defaultValue: '팀 없음' }), openTeam, () => { setOpenTeam((o) => !o); setOpenOrg(false); }, teamsInOrg.length > 0)}
        {openTeam && teamsInOrg.length > 0 && menu(
          teamsInOrg.map((g) => ({ id: g.id, label: g.displayName || g.name, icon: <FolderKanban size={14} /> })),
          curTeam?.id, pickTeam, 'topbar.noGroupJoined', '가입한 팀이 없습니다', 'topbar.myGroups', '내 팀',
        )}
      </div>
    </>
  );
}
