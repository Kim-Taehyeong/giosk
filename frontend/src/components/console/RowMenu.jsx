import React, { useEffect, useId, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { MoreHorizontal } from 'lucide-react';
import { nextDropdownId, announceOpen, onDropdownOpen } from './dropdownBus';

// 표 행의 부가 작업 메뉴(케밥).
// 되돌릴 수 없는 작업(삭제 등)을 행에 그대로 노출하면, 바로 옆의 자주 쓰는 버튼과 붙어 오클릭이 나고
// 빨간 아이콘이 상태 배지보다 시끄러워진다. 한 겹 뒤로 보내되 목록을 떠나지 않게 한다.
//
// items: [{ key, label, icon, tone?: 'danger', onSelect, disabled? }]
// 메뉴는 body 포털에 position:fixed 로 띄운다. 표의 overflow-x:auto 에 잘리지 않게 하기 위해서다.
export default function RowMenu({ items = [], label }) {
  const { t } = useTranslation('common');
  const [open, setOpen] = useState(false);
  const [rect, setRect] = useState(null);
  const [active, setActive] = useState(-1);
  const btnRef = useRef(null);
  const menuRef = useRef(null);
  const idRef = useRef(null);
  const domId = useId();
  if (idRef.current === null) idRef.current = nextDropdownId();

  const usable = items.filter(Boolean);

  useEffect(() => onDropdownOpen((openedId) => { if (openedId !== idRef.current) setOpen(false); }), []);

  const close = ({ refocus = true } = {}) => {
    setOpen(false);
    setActive(-1);
    if (refocus) btnRef.current?.focus();
  };

  const openMenu = () => {
    if (!btnRef.current) return;
    setRect(btnRef.current.getBoundingClientRect());
    announceOpen(idRef.current);
    setActive(0);
    setOpen(true);
    setTimeout(() => menuRef.current?.focus(), 0);
  };

  useEffect(() => {
    if (!open) return undefined;
    const onDoc = (e) => {
      if (btnRef.current?.contains(e.target) || menuRef.current?.contains(e.target)) return;
      close({ refocus: false });
    };
    // 스크롤/리사이즈로 위치가 어긋나면 닫는다(행 메뉴는 짧게 쓰는 물건이다).
    const onMove = () => close({ refocus: false });
    document.addEventListener('mousedown', onDoc);
    window.addEventListener('scroll', onMove, true);
    window.addEventListener('resize', onMove);
    return () => {
      document.removeEventListener('mousedown', onDoc);
      window.removeEventListener('scroll', onMove, true);
      window.removeEventListener('resize', onMove);
    };
  }, [open]);

  const pick = (it) => { if (it.disabled) return; close({ refocus: false }); it.onSelect?.(); };

  const onKey = (e) => {
    if (!open) {
      if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') { e.preventDefault(); openMenu(); }
      return;
    }
    if (e.key === 'Escape') { e.preventDefault(); close(); return; }
    if (e.key === 'Tab') { close({ refocus: false }); return; }
    if (!usable.length) return;
    if (e.key === 'ArrowDown') { e.preventDefault(); setActive((i) => (i + 1) % usable.length); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setActive((i) => (i <= 0 ? usable.length - 1 : i - 1)); }
    else if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); if (usable[active]) pick(usable[active]); }
  };

  if (!usable.length) return null;

  return (
    <>
      <button ref={btnRef} type="button" className="icon-btn"
        onClick={(e) => { e.stopPropagation(); if (open) close(); else openMenu(); }}
        onKeyDown={onKey}
        aria-haspopup="menu" aria-expanded={open} aria-controls={open ? `${domId}-menu` : undefined}
        aria-label={label || t('rowMenu.more', { defaultValue: '더 보기' })}
        style={{ width: 30, height: 30 }}>
        <MoreHorizontal size={16} />
      </button>
      {open && rect && createPortal(
        (() => {
          const vh = typeof window !== 'undefined' ? window.innerHeight : 800;
          const desired = usable.length * 38 + 14;
          const up = rect.bottom + desired + 8 > vh && rect.top > desired;
          return (
            <div ref={menuRef} id={`${domId}-menu`} role="menu" tabIndex={-1} onKeyDown={onKey}
              className="dropdown"
              style={{
                position: 'fixed', top: up ? undefined : rect.bottom + 6, bottom: up ? vh - rect.top + 6 : undefined,
                // 메뉴는 버튼의 오른쪽 끝에 맞춘다(행 끝에 붙어 있으므로 왼쪽으로 펼쳐야 화면을 안 넘는다).
                left: Math.max(8, rect.right - 176), minWidth: 176, right: 'auto', zIndex: 2000, outline: 'none',
              }}>
              {usable.map((it, i) => {
                const Icon = it.icon;
                return (
                  <div key={it.key} role="menuitem" aria-disabled={it.disabled || undefined}
                    className="dd-item"
                    onClick={(e) => { e.stopPropagation(); pick(it); }}
                    onMouseMove={() => setActive(i)}
                    style={{
                      color: it.tone === 'danger' ? 'var(--danger)' : 'var(--text)',
                      background: i === active ? 'var(--surface-2)' : 'transparent',
                      opacity: it.disabled ? 0.5 : 1,
                      cursor: it.disabled ? 'not-allowed' : 'pointer',
                    }}>
                    {Icon && <Icon size={15} />}
                    <span>{it.label}</span>
                  </div>
                );
              })}
            </div>
          );
        })(),
        (typeof document !== 'undefined' && document.fullscreenElement) || document.body,
      )}
    </>
  );
}
