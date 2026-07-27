import React, { useState, useRef, useEffect } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Building2, FolderKanban, ChevronsUpDown, Check, ShieldCheck } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
import { getAllOrgs, getAllGroups } from '../../api/console/governance';
import { getConsoleScope } from '../../api/client';

// scopeKey — 백엔드 X-Console-Scope 형식("org:10"|"group:2").
const keyOf = (s) => `${s.level}:${s.level === 'org' ? s.orgId : s.groupId}`;

// RoleSwitcher는 "조직/역할 스코프" 셀렉터다(모드 전환 아님 — 모드는 별도 ModeToggle).
// 관리자·사용자 어느 화면에서도 항상 표시해 "지금 어느 조직 컨텍스트인가"를 보여준다
// (멀티 org 사용자가 '내 콘솔'에서도 소속 조직을 알 수 있게). 스코프를 바꿔도 현재 모드(관리/사용자)는 유지한다.
//   - 최고관리자(admin): [플랫폼] + 보유한 org/group
//   - 조직/그룹 관리자(복수 org·group 가능): 보유한 org/group 전부
// 백엔드는 admin 을 항상 platform 스코프로 고정하므로, admin 의 org/group 선택은 화면 컨텍스트 표시용이다.
export default function RoleSwitcher({ ns }) {
  const { t } = useTranslation(ns);
  const { user, activeScope, setActiveScope } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const [open, setOpen] = useState(false);
  const ref = useRef(null);

  useEffect(() => {
    const onDoc = (e) => { if (ref.current && !ref.current.contains(e.target)) setOpen(false); };
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, []);

  const isAdmin = user?.role === 'admin';
  const scopes = user?.scopes || [];

  // 최고관리자는 임의 조직/그룹을 "역할"로 채택할 수 있어야 하므로 전체 목록을 불러온다
  // (본인 scopes 는 비어 있음). 일반 매니저는 보유 scopes 만.
  const [adminOrgs, setAdminOrgs] = useState([]);
  const [adminGroups, setAdminGroups] = useState([]);
  useEffect(() => {
    if (!isAdmin) return;
    getAllOrgs().then((d) => setAdminOrgs(d.items)).catch(() => {});
    getAllGroups().then((d) => setAdminGroups(d.items)).catch(() => {});
  }, [isAdmin]);

  const opts = [];
  if (isAdmin) {
    opts.push({ id: 'platform', icon: ShieldCheck, label: t('topbar.rolePlatform', { defaultValue: '플랫폼 전체' }), sub: t('topbar.rolePlatformSub', { defaultValue: '최고 관리자' }) });
    adminOrgs.forEach((o) => opts.push({ id: `org:${o.id}`, icon: Building2, label: o.displayName || o.name || '—', sub: t('topbar.scopeOrg', { defaultValue: '조직' }) }));
    adminGroups.forEach((g) => opts.push({ id: `group:${g.id}`, orgId: g.orgId, icon: FolderKanban, label: g.displayName || g.name || '—', sub: (g.orgName ? `${t('topbar.scopeGroup', { defaultValue: '그룹' })} · ${g.orgName}` : t('topbar.scopeGroup', { defaultValue: '그룹' })) }));
  } else {
    scopes.filter((s) => s.level === 'org').forEach((s) => opts.push({ id: keyOf(s), scope: s, icon: Building2, label: s.orgName || '—', sub: t('topbar.scopeOrg', { defaultValue: '조직' }) }));
    scopes.filter((s) => s.level === 'group').forEach((s) => opts.push({ id: keyOf(s), scope: s, icon: FolderKanban, label: s.groupName || '—', sub: t('topbar.scopeGroup', { defaultValue: '그룹' }) }));
  }

  // 최고관리자 드롭다운은 조직→그룹 2뎁스: 플랫폼 + (조직 헤더 + 그 조직의 그룹 들여쓰기).
  // rows: [{opt, indent}]. 조직 없는(무소속) 그룹은 '기타'로 맨 끝.
  const adminRows = [];
  if (isAdmin) {
    adminRows.push({ opt: opts.find((o) => o.id === 'platform'), indent: 0 });
    adminOrgs.forEach((o) => {
      adminRows.push({ opt: opts.find((x) => x.id === `org:${o.id}`), indent: 0 });
      opts.filter((x) => String(x.id).startsWith('group:') && x.orgId === o.id).forEach((g) => adminRows.push({ opt: g, indent: 1 }));
    });
    opts.filter((x) => String(x.id).startsWith('group:') && !adminOrgs.some((o) => o.id === x.orgId)).forEach((g) => adminRows.push({ opt: g, indent: 1 }));
  }

  // 현재 스코프.
  let currentId;
  if (activeScope && opts.some((o) => o.id === activeScope)) currentId = activeScope;
  else if (isAdmin) currentId = 'platform';
  else currentId = opts[0]?.id;

  // 이 뷰(관리자)의 선택 스코프를 실제 요청 헤더와 동기화한다. 사용자 뷰(OrgGroupSelector)가
  // 같은 스코프 키를 group:N 으로 덮어썼을 수 있으므로, 관리자 뷰로 돌아오면 표시=헤더가 어긋나지 않게 맞춘다.
  useEffect(() => {
    if (!currentId) return;
    const want = currentId === 'platform' ? null : currentId;
    if (getConsoleScope() !== (want || null)) setActiveScope(want);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentId]);

  if (opts.length === 0) return null; // 스코프 없는 순수 사용자 → OrgGroupSelector 가 대신 표시
  const current = opts.find((o) => o.id === currentId) || opts[0];

  const inAdmin = location.pathname.startsWith('/console/admin');
  const pick = (o) => {
    setOpen(false);
    setActiveScope(o.id === 'platform' ? null : o.id); // platform=스코프 없음(admin 기본)
    // 모드는 유지 — 관리 화면이면 그 스코프의 관리 홈으로(현재 탭이 스코프 밖일 수 있음), 사용자 화면이면 그대로.
    if (inAdmin) navigate('/console/admin/dashboard/ops');
  };

  const single = opts.length === 1;
  const CurIcon = current?.icon;
  return (
    <div className="proj" ref={single ? null : ref} style={{ position: 'relative', cursor: single ? 'default' : 'pointer' }}
      onClick={single ? undefined : () => setOpen((o) => !o)} role={single ? undefined : 'button'}>
      <small>{current?.sub || ''}</small>
      <span className="flex gap" style={{ gap: 6, alignItems: 'center' }}>
        {CurIcon && <CurIcon size={13} />}{current?.label || '—'}
        {!single && <ChevronsUpDown size={13} style={{ opacity: 0.6 }} />}
      </span>
      {open && !single && (
        <div className="scope-menu" style={{
          position: 'absolute', top: '100%', left: 0, marginTop: 6, minWidth: 240, zIndex: 50,
          background: 'var(--surface, #fff)', border: '1px solid var(--border, #e5e7eb)', borderRadius: 8,
          boxShadow: '0 8px 24px rgba(0,0,0,.12)', padding: 6, maxHeight: 380, overflowY: 'auto',
        }}>
          <div className="muted" style={{ fontSize: 11, padding: '4px 8px' }}>{t('topbar.switchScope', { defaultValue: '조직 / 스코프 전환' })}</div>
          {(isAdmin ? adminRows : opts.map((o) => ({ opt: o, indent: 0 }))).map(({ opt: o, indent }) => {
            if (!o) return null;
            const active = o.id === currentId;
            const Icon = o.icon;
            return (
              <button key={o.id} className="scope-item flex gap" onClick={(e) => { e.stopPropagation(); pick(o); }}
                style={{
                  width: '100%', textAlign: 'left', alignItems: 'center', gap: 8,
                  padding: '7px 8px', paddingLeft: 8 + indent * 18,
                  border: 0, borderRadius: 6, background: active ? 'var(--hover, #f3f4f6)' : 'transparent', cursor: 'pointer',
                }}>
                {Icon && <Icon size={14} style={{ opacity: indent ? 0.75 : 1 }} />}
                <span style={{ flex: 1 }}>
                  <div style={{ fontWeight: indent ? 500 : 600, fontSize: 13 }}>{o.label}</div>
                  {o.sub && indent === 0 && <div className="muted" style={{ fontSize: 11 }}>{o.sub}</div>}
                </span>
                {active && <Check size={14} />}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
