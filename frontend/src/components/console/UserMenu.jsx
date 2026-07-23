import React, { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Bell, LogOut, Settings, ChevronDown, Sun, Moon } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
import { useTheme } from '../../context/ThemeContext';

function initials(user) {
  const f = user?.firstName || user?.username || '?';
  return f.slice(0, 2).toUpperCase();
}

function Item({ icon, label, hint, onClick }) {
  return (
    <div role="button" onClick={onClick}
      style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '9px 10px', borderRadius: 8, cursor: 'pointer', fontSize: 13 }}
      onMouseEnter={(e) => { e.currentTarget.style.background = 'var(--surface-2)'; }}
      onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent'; }}>
      {icon}
      <span style={{ flex: 1 }}>{label}</span>
      {hint && <span className="muted" style={{ fontSize: 11, fontWeight: 700 }}>{hint}</span>}
    </div>
  );
}

// 탑바 우측 사용자 메뉴 — 알림 센터·내 정보·언어·테마·로그아웃을 아바타 드롭다운으로 모음.
export default function UserMenu({ ns }) {
  const { t } = useTranslation(ns);
  const { user, logout } = useAuth();
  const { theme, toggle } = useTheme();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const ref = useRef(null);

  useEffect(() => {
    if (!open) return undefined;
    const onDoc = (e) => { if (ref.current && !ref.current.contains(e.target)) setOpen(false); };
    const onEsc = (e) => { if (e.key === 'Escape') setOpen(false); };
    document.addEventListener('mousedown', onDoc);
    document.addEventListener('keydown', onEsc);
    return () => { document.removeEventListener('mousedown', onDoc); document.removeEventListener('keydown', onEsc); };
  }, [open]);

  const go = (path) => { setOpen(false); navigate(path); };
  const onLogout = async () => { setOpen(false); await logout(); navigate('/login'); };

  return (
    <div ref={ref} style={{ position: 'relative' }}>
      <div className="user" onClick={() => setOpen((o) => !o)} title={user?.username || ''}>
        <span className="avatar">{initials(user)}</span>
        <ChevronDown size={14} style={{ transform: open ? 'rotate(180deg)' : 'none', transition: 'transform .15s' }} />
      </div>

      {open && (
        <div style={{
          position: 'absolute', top: 'calc(100% + 8px)', right: 0, minWidth: 236, zIndex: 70,
          background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 12,
          boxShadow: '0 16px 40px rgba(10,15,28,.22)', padding: 8,
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '8px 10px 10px' }}>
            <span className="avatar">{initials(user)}</span>
            <div style={{ minWidth: 0 }}>
              <div style={{ fontSize: 13, fontWeight: 700, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {user?.firstName || user?.username || '—'}
              </div>
              <div className="muted" style={{ fontSize: 11, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {user?.email || user?.username || ''}
              </div>
            </div>
          </div>
          <div style={{ height: 1, background: 'var(--border)', margin: '2px 0 6px' }} />

          <Item icon={<Bell size={15} />} label={t('topbar.notifications')} onClick={() => go('/console/notifications')} />
          <Item icon={<Settings size={15} />} label={t('topbar.account')} onClick={() => go('/console/account')} />
          <Item icon={theme === 'dark' ? <Sun size={15} /> : <Moon size={15} />} label={t('topbar.theme')}
            hint={theme === 'dark' ? t('topbar.themeDark') : t('topbar.themeLight')} onClick={toggle} />

          <div style={{ height: 1, background: 'var(--border)', margin: '6px 0' }} />
          <Item icon={<LogOut size={15} />} label={t('topbar.logout')} onClick={onLogout} />
        </div>
      )}
    </div>
  );
}
