import React, { useState, useRef, useEffect } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Building2, FolderKanban, ChevronsUpDown, Check, ShieldCheck, LayoutGrid } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';

// scopeKey — 백엔드 X-Console-Scope 형식("org:10"|"group:2").
const keyOf = (s) => `${s.level}:${s.level === 'org' ? s.orgId : s.groupId}`;

// 통합 역할/뷰 전환기 — 최고관리자·조직/그룹 관리자·개인 뷰를 한 셀렉터로 묶는다.
//   - 최고관리자(role=admin): [플랫폼 관리자] + [내 콘솔]  (백엔드가 admin 은 항상 platform 스코프로
//     고정 → org/group 드릴인은 데이터가 안 걸려 오해만 주므로 넣지 않는다)
//   - 조직/그룹 관리자(복수 org·복수 group 가능): [각 조직…] + [각 그룹…] + [내 콘솔]
//   - 순수 사용자(스코프 없음): 전환 대상이 없어 렌더하지 않음(Topbar 가 OrgGroupSelector 로 대체).
// 개인 뷰는 /console, 관리 뷰는 /console/admin(+X-Console-Scope 헤더)로 전환한다.
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
  const orgs = scopes.filter((s) => s.level === 'org');
  const groups = scopes.filter((s) => s.level === 'group');

  // 옵션 목록: [플랫폼?] + [조직들] + [그룹들] + [내 콘솔].
  const opts = [];
  if (isAdmin) opts.push({ id: 'platform', kind: 'platform', icon: ShieldCheck, label: t('topbar.rolePlatform', { defaultValue: '플랫폼 관리자' }), sub: t('topbar.rolePlatformSub', { defaultValue: '전체 플랫폼' }) });
  orgs.forEach((s) => opts.push({ id: keyOf(s), kind: 'scope', scope: s, icon: Building2, label: s.orgName || '—', sub: t('topbar.scopeOrg', { defaultValue: '조직 관리' }) }));
  groups.forEach((s) => opts.push({ id: keyOf(s), kind: 'scope', scope: s, icon: FolderKanban, label: s.groupName || '—', sub: t('topbar.scopeGroup', { defaultValue: '그룹 관리' }) }));
  opts.push({ id: 'user', kind: 'user', icon: LayoutGrid, label: t('topbar.roleUser', { defaultValue: '내 콘솔' }), sub: t('topbar.roleUserSub', { defaultValue: '사용자' }) });

  // 현재 활성 옵션 판정 — 경로(관리/사용자) + 활성 스코프 기준.
  const inAdmin = location.pathname.startsWith('/console/admin');
  let currentId;
  if (!inAdmin) currentId = 'user';
  else if (activeScope && opts.some((o) => o.id === activeScope)) currentId = activeScope;
  else if (isAdmin) currentId = 'platform';
  else currentId = orgs[0] ? keyOf(orgs[0]) : (groups[0] ? keyOf(groups[0]) : 'user');
  const current = opts.find((o) => o.id === currentId) || opts[0];

  const pick = (o) => {
    setOpen(false);
    if (o.kind === 'user') { navigate('/console'); return; }
    setActiveScope(o.kind === 'platform' ? null : o.id); // platform=스코프 없음(admin 기본)
    navigate('/console/admin/dashboard/ops'); // 새 스코프에 유효한 홈으로(현재 탭이 스코프 밖일 수 있음)
  };

  // 전환 대상이 하나뿐이면(내 콘솔만) 셀렉터 불필요.
  if (opts.length <= 1) return null;

  const CurIcon = current?.icon;
  return (
    <div className="proj" ref={ref} style={{ position: 'relative', cursor: 'pointer' }} onClick={() => setOpen((o) => !o)} role="button">
      <small>{current?.sub || ''}</small>
      <span className="flex gap" style={{ gap: 6, alignItems: 'center' }}>
        {CurIcon && <CurIcon size={13} />}{current?.label || '—'}
        <ChevronsUpDown size={13} style={{ opacity: 0.6 }} />
      </span>
      {open && (
        <div className="scope-menu" style={{
          position: 'absolute', top: '100%', left: 0, marginTop: 6, minWidth: 240, zIndex: 50,
          background: 'var(--surface, #fff)', border: '1px solid var(--border, #e5e7eb)', borderRadius: 8,
          boxShadow: '0 8px 24px rgba(0,0,0,.12)', padding: 6,
        }}>
          <div className="muted" style={{ fontSize: 11, padding: '4px 8px' }}>{t('topbar.switchRole', { defaultValue: '역할 / 화면 전환' })}</div>
          {opts.map((o) => {
            const active = o.id === currentId;
            const Icon = o.icon;
            return (
              <button key={o.id} className="scope-item flex gap" onClick={(e) => { e.stopPropagation(); pick(o); }}
                style={{
                  width: '100%', textAlign: 'left', alignItems: 'center', gap: 8, padding: '7px 8px',
                  border: 0, borderRadius: 6, background: active ? 'var(--hover, #f3f4f6)' : 'transparent', cursor: 'pointer',
                }}>
                {Icon && <Icon size={14} />}
                <span style={{ flex: 1 }}>
                  <div style={{ fontWeight: 600, fontSize: 13 }}>{o.label}</div>
                  {o.sub && <div className="muted" style={{ fontSize: 11 }}>{o.sub}</div>}
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
