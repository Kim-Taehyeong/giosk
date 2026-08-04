import React, { useState, useRef, useEffect } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Building2, FolderKanban, ChevronDown, Check, ShieldCheck } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
import { getAllOrgs, getAllGroups } from '../../api/console/governance';
import { getConsoleScope } from '../../api/client';

// scopeKey 는 백엔드 X-Console-Scope 형식이다(org:10 이나 group:2).
const keyOf = (s) => `${s.level}:${s.level === 'org' ? s.orgId : s.groupId}`;

// RoleSwitcher는 관리자 뷰의 "역할 스코프" 셀렉터.
//   최고관리자(admin)는 조직과 그룹 2드롭다운을 쓴다. 한 사람이 여러 조직이나 그룹을 관리하므로 계층으로 고른다.
//   조직/그룹 관리자: 보유한 스코프 단일 드롭다운.
// 선택 스코프는 X-Console-Scope 로 전송돼 화면/데이터가 그 레벨로 좁혀진다.
export default function RoleSwitcher({ ns }) {
  const { t } = useTranslation(ns);
  const { user, activeScope, setActiveScope } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const isAdmin = user?.role === 'admin';
  const scopes = user?.scopes || [];

  const [adminOrgs, setAdminOrgs] = useState([]);
  const [adminGroups, setAdminGroups] = useState([]);
  useEffect(() => {
    if (!isAdmin) return;
    getAllOrgs().then((d) => setAdminOrgs(d.items)).catch(() => {});
    getAllGroups().then((d) => setAdminGroups(d.items)).catch(() => {});
  }, [isAdmin]);

  const inAdmin = location.pathname.startsWith('/console/admin');
  const goHome = () => { if (inAdmin) navigate('/console/admin/dashboard/ops'); };

  // 현재 스코프 파싱.
  let curLevel = 'platform', curOrgId = null, curGroupId = null;
  if (activeScope?.startsWith('org:')) { curLevel = 'org'; curOrgId = Number(activeScope.slice(4)); }
  else if (activeScope?.startsWith('group:')) { curLevel = 'group'; curGroupId = Number(activeScope.slice(6)); curOrgId = adminGroups.find((g) => g.id === curGroupId)?.orgId ?? null; }

  // 이 뷰의 스코프를 실제 요청 헤더와 동기화(사용자 뷰가 group 으로 덮었을 수 있음). 훅은 항상 호출.
  useEffect(() => {
    if (getConsoleScope() !== (activeScope || null)) setActiveScope(activeScope || null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // ── 비관리자(조직/그룹 매니저): 보유 스코프 단일 드롭다운 ──
  if (!isAdmin) {
    const opts = [];
    scopes.filter((s) => s.level === 'org').forEach((s) => opts.push({ id: keyOf(s), icon: Building2, label: s.orgName || '—', sub: t('topbar.scopeOrg', { defaultValue: '조직' }) }));
    scopes.filter((s) => s.level === 'group').forEach((s) => opts.push({ id: keyOf(s), icon: FolderKanban, label: s.groupName || '—', sub: t('topbar.scopeGroup', { defaultValue: '그룹' }) }));
    if (opts.length === 0) return null;
    const cur = opts.find((o) => o.id === activeScope) || opts[0];
    return <Dropdown label={cur.sub} value={cur.label} icon={cur.icon} items={opts.map((o) => ({ id: o.id, label: o.label, sub: o.sub, active: o.id === cur.id, on: () => { setActiveScope(o.id); goHome(); } }))} single={opts.length === 1} />;
  }

  // ── 최고관리자: 조직과 그룹 2드롭다운 ──
  const effOrgId = curOrgId; // 그룹 드롭다운이 보여줄 조직
  const orgName = (id) => adminOrgs.find((o) => o.id === id)?.displayName || adminOrgs.find((o) => o.id === id)?.name || '—';
  const groupsOfOrg = adminGroups.filter((g) => g.orgId === effOrgId);

  // 조직 드롭다운: 플랫폼 전체 + 각 조직.
  const orgItems = [
    { id: 'platform', label: t('topbar.rolePlatform', { defaultValue: '플랫폼 전체' }), sub: t('topbar.rolePlatformSub', { defaultValue: '최고 관리자' }), icon: ShieldCheck, active: curLevel === 'platform', on: () => { setActiveScope(null); goHome(); } },
    ...adminOrgs.map((o) => ({ id: `org:${o.id}`, label: o.displayName || o.name || '—', sub: t('topbar.scopeOrg', { defaultValue: '조직' }), icon: Building2, active: curOrgId === o.id, on: () => { setActiveScope(`org:${o.id}`); goHome(); } })),
  ];
  const orgValue = curLevel === 'platform' ? t('topbar.rolePlatform', { defaultValue: '플랫폼 전체' }) : orgName(curOrgId);
  const orgIcon = curLevel === 'platform' ? ShieldCheck : Building2;

  // 그룹 드롭다운: 조직 전체 + 그 조직의 그룹(조직 선택 상태에서만).
  const grpItems = [
    { id: 'whole', label: t('topbar.orgWhole', { defaultValue: '조직 전체' }), sub: t('topbar.scopeOrg', { defaultValue: '조직' }), icon: Building2, active: curLevel === 'org', on: () => { setActiveScope(`org:${effOrgId}`); goHome(); } },
    ...groupsOfOrg.map((g) => ({ id: `group:${g.id}`, label: g.displayName || g.name || '—', sub: t('topbar.scopeGroup', { defaultValue: '그룹' }), icon: FolderKanban, active: curGroupId === g.id, on: () => { setActiveScope(`group:${g.id}`); goHome(); } })),
  ];
  const grpValue = curLevel === 'group' ? (adminGroups.find((g) => g.id === curGroupId)?.displayName || '—') : t('topbar.orgWhole', { defaultValue: '조직 전체' });

  return (
    <>
      <Dropdown label={t('topbar.scopeOrg', { defaultValue: '조직' })} value={orgValue} icon={orgIcon} items={orgItems} />
      {effOrgId ? (
        <Dropdown label={t('topbar.scopeGroup', { defaultValue: '그룹' })} value={grpValue} icon={curLevel === 'group' ? FolderKanban : Building2} items={grpItems} />
      ) : null}
    </>
  );
}

// Dropdown은 탑바 .proj 셀과 메뉴를 함께 쓰는 공용 컴포넌트다. items 는 [{id,label,sub,icon,active,on}].
function Dropdown({ label, value, icon: Icon, items, single }) {
  const [open, setOpen] = useState(false);
  const ref = useRef(null);
  useEffect(() => {
    const onDoc = (e) => { if (ref.current && !ref.current.contains(e.target)) setOpen(false); };
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, []);
  const clickable = !single && items.length > 1;
  return (
    <div className="proj" ref={ref} style={{ position: 'relative', cursor: clickable ? 'pointer' : 'default' }}
      onClick={clickable ? () => setOpen((o) => !o) : undefined} role={clickable ? 'button' : undefined}>
      <small>{label}</small>
      <span className="flex gap" style={{ gap: 6, alignItems: 'center' }}>
        {Icon && <Icon size={13} />}{value || '—'}
        {clickable && <ChevronDown size={14} style={{ opacity: 0.6, transform: open ? 'rotate(180deg)' : 'none', transition: 'transform .15s' }} />}
      </span>
      {open && clickable && (
        <div className="scope-menu" style={{
          position: 'absolute', top: '100%', left: 0, marginTop: 6, minWidth: 230, zIndex: 60,
          background: 'var(--surface, #fff)', border: '1px solid var(--border, #e5e7eb)', borderRadius: 10,
          boxShadow: '0 12px 30px rgba(10,15,28,.18)', padding: 6, maxHeight: 360, overflowY: 'auto',
        }}>
          {items.map((it) => {
            const Ic = it.icon;
            return (
              <button key={it.id} className="flex gap" onClick={(e) => { e.stopPropagation(); setOpen(false); it.on(); }}
                style={{ width: '100%', textAlign: 'left', alignItems: 'center', gap: 8, padding: '8px 9px', border: 0, borderRadius: 7,
                  background: it.active ? 'var(--primary-soft)' : 'transparent', color: it.active ? 'var(--primary)' : 'var(--text)', cursor: 'pointer' }}>
                {Ic && <Ic size={14} />}
                <span style={{ flex: 1 }}>
                  <div style={{ fontWeight: it.active ? 700 : 600, fontSize: 13 }}>{it.label}</div>
                  {it.sub && <div className="muted" style={{ fontSize: 11 }}>{it.sub}</div>}
                </span>
                {it.active && <Check size={14} />}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
