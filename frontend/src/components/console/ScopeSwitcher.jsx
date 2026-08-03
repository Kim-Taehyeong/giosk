import React, { useState, useRef, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Building2, FolderKanban, ChevronsUpDown, Check } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
import { activeScopeOf } from '../../config/consoleRoles';
import { clickable } from '../../utils/a11y';

// scopeKey — 백엔드 X-Console-Scope 형식("org:10"|"group:2").
const keyOf = (s) => `${s.level}:${s.level === 'org' ? s.orgId : s.groupId}`;

// 멀티롤 컨텍스트 전환기. 사용자가 조직 관리자 + 그룹 관리자를 동시에 가질 때
// 어느 역할로 콘솔을 운영할지 고른다. 스코프가 1개면 정적 라벨만(전환 UI 없음).
export default function ScopeSwitcher({ ns }) {
  const { t } = useTranslation(ns);
  const { user, activeScope, setActiveScope } = useAuth();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const ref = useRef(null);

  useEffect(() => {
    const onDoc = (e) => { if (ref.current && !ref.current.contains(e.target)) setOpen(false); };
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, []);

  const scopes = user?.scopes || [];
  const current = activeScopeOf(user, activeScope);
  const label = (s) => (s.level === 'org' ? s.orgName : s.groupName) || '—';
  const roleLabel = (s) => (s.level === 'org' ? t('topbar.scopeOrg') : t('topbar.scopeGroup'));

  const pick = (s) => {
    setActiveScope(keyOf(s));
    setOpen(false);
    navigate('/console/admin/dashboard/ops'); // 새 스코프에 유효한 홈으로(현재 탭이 스코프 밖일 수 있음)
  };

  // 스코프가 1개면 전환 필요 없음 — 기존 정적 라벨.
  if (scopes.length <= 1) {
    const s = current || scopes[0];
    return (
      <div className="proj" style={{ cursor: 'default' }}>
        <small>{s ? roleLabel(s) : t('topbar.scopeGroup')}</small>
        <span className="flex gap" style={{ gap: 6 }}>
          {s?.level === 'org' ? <Building2 size={13} /> : <FolderKanban size={13} />}{s ? label(s) : '—'}
        </span>
      </div>
    );
  }

  return (
    <div className="proj" ref={ref} style={{ position: 'relative', cursor: 'pointer' }}
      aria-expanded={open} aria-haspopup="menu" {...clickable(() => setOpen((o) => !o))}>
      <small>{current ? roleLabel(current) : '—'}</small>
      <span className="flex gap" style={{ gap: 6, alignItems: 'center' }}>
        {current?.level === 'org' ? <Building2 size={13} /> : <FolderKanban size={13} />}
        {current ? label(current) : '—'}
        <ChevronsUpDown size={13} style={{ opacity: 0.6 }} />
      </span>
      {open && (
        <div className="scope-menu" role="menu" style={{
          position: 'absolute', top: '100%', left: 0, marginTop: 6, minWidth: 220, zIndex: 50,
          background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--r-container)',
          boxShadow: '0 8px 24px rgba(0,0,0,.12)', padding: 6,
        }}>
          <div className="muted" style={{ fontSize: 11, padding: '4px 8px' }}>{t('topbar.switchScope')}</div>
          {scopes.map((s) => {
            const active = current && keyOf(current) === keyOf(s);
            return (
              <button key={keyOf(s)} className="scope-item flex gap"
                onClick={(e) => { e.stopPropagation(); pick(s); }}
                style={{
                  width: '100%', textAlign: 'left', alignItems: 'center', gap: 8, padding: '7px 8px',
                  border: 0, borderRadius: 'var(--r-control)', background: active ? 'var(--surface-2)' : 'transparent', cursor: 'pointer',
                }}>
                {s.level === 'org' ? <Building2 size={14} /> : <FolderKanban size={14} />}
                <span style={{ flex: 1 }}>
                  <div style={{ fontWeight: 600, fontSize: 13 }}>{label(s)}</div>
                  <div className="muted" style={{ fontSize: 11 }}>{roleLabel(s)}</div>
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
