import React, { useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { ChevronDown, Check } from 'lucide-react';
import { nextDropdownId, announceOpen, onDropdownOpen } from './dropdownBus';

// 디자인된 커스텀 드롭다운. 네이티브 <select>의 OS 기본 옵션 목록 대신
// 테마(라이트/다크) 변수를 따르는 팝오버 메뉴를 렌더.
// 메뉴는 body 포털 + position:fixed 로 띄워, 카드 애니메이션(stacking context)·overflow 에
// 잘리지 않게 한다. CSS 변수는 :root 전역이라 포털 밖에서도 테마가 유지된다.
// options: [{ value, label }]
export default function Select({ value, onChange, options = [], placeholder = '선택', width, size, disabled = false, searchable = false }) {
  const [open, setOpen] = useState(false);
  const [rect, setRect] = useState(null);
  const [q, setQ] = useState('');
  const btnRef = useRef(null);
  const menuRef = useRef(null);
  const searchRef = useRef(null);
  const idRef = useRef(null);
  if (idRef.current === null) idRef.current = nextDropdownId();

  // 다른 드롭다운이 열리면 나는 닫는다(동시에 여러 개가 떠 있지 않게).
  useEffect(() => onDropdownOpen((openedId) => {
    if (openedId !== idRef.current) setOpen(false);
  }), []);
  const sel = options.find((o) => String(o.value) === String(value));

  const toggle = () => {
    if (disabled) return;
    if (!open && btnRef.current) {
      setRect(btnRef.current.getBoundingClientRect());
      announceOpen(idRef.current); // 나머지 드롭다운을 닫는다.
      setQ('');
      if (searchable) setTimeout(() => searchRef.current?.focus(), 0);
    }
    setOpen((o) => !o);
  };

  // disabled 로 바뀌면 열린 메뉴를 닫는다.
  useEffect(() => { if (disabled) setOpen(false); }, [disabled]);

  useEffect(() => {
    if (!open) return undefined;
    const onDoc = (e) => {
      if (btnRef.current && btnRef.current.contains(e.target)) return;
      if (menuRef.current && menuRef.current.contains(e.target)) return;
      setOpen(false);
    };
    // 스크롤/리사이즈 시 위치가 어긋나므로 닫는다. 단 메뉴 "내부" 스크롤은 무시(목록 스크롤 중 닫힘 방지).
    const onMove = (e) => {
      if (e && e.target && menuRef.current && menuRef.current.contains(e.target)) return;
      setOpen(false);
    };
    document.addEventListener('mousedown', onDoc);
    window.addEventListener('scroll', onMove, true);
    window.addEventListener('resize', onMove);
    return () => {
      document.removeEventListener('mousedown', onDoc);
      window.removeEventListener('scroll', onMove, true);
      window.removeEventListener('resize', onMove);
    };
  }, [open]);

  const pad = size === 'sm' ? '6px 30px 6px 10px' : '9px 32px 9px 11px';
  const fs = size === 'sm' ? 12.5 : 13;

  return (
    <div style={{ position: 'relative', width: width || (size === 'sm' ? 'auto' : '100%'), minWidth: size === 'sm' ? 120 : 0, display: 'inline-block' }}>
      <button ref={btnRef} type="button" onClick={toggle} disabled={disabled}
        style={{
          width: '100%', textAlign: 'left', padding: pad, fontSize: fs, fontWeight: 500,
          cursor: disabled ? 'not-allowed' : 'pointer', opacity: disabled ? 0.5 : 1,
          border: '1px solid ' + (open ? 'var(--primary)' : 'var(--border)'), borderRadius: 9,
          background: 'var(--surface-2)', color: sel ? 'var(--text)' : 'var(--muted)',
          boxShadow: open ? '0 0 0 3px var(--primary-soft)' : 'none', transition: 'border-color .15s, box-shadow .15s',
          position: 'relative', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
        }}>
        {sel ? sel.label : placeholder}
        <ChevronDown size={15} style={{ position: 'absolute', right: 9, top: '50%', transform: `translateY(-50%) rotate(${open ? 180 : 0}deg)`, transition: 'transform .15s', color: 'var(--muted)' }} />
      </button>
      {open && rect && createPortal(
        (() => {
          // 아래 공간이 부족하면(마지막 행 등) 위로 펼쳐서 뷰포트에 안 짤리게 한다.
          const vh = typeof window !== 'undefined' ? window.innerHeight : 800;
          const desired = Math.min(300, options.length * 40 + 16);
          const openUp = rect.bottom + desired + 8 > vh && rect.top > desired;
          const pos = openUp
            ? { bottom: Math.max(8, vh - rect.top + 6) }
            : { top: rect.bottom + 6 };
          const shown = searchable && q.trim()
            ? options.filter((o) => String(o.label).toLowerCase().includes(q.trim().toLowerCase()))
            : options;
          return (
            <div ref={menuRef} style={{
              position: 'fixed', ...pos, left: rect.left, minWidth: Math.max(rect.width, 180), zIndex: 2000,
              background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 11,
              boxShadow: '0 12px 32px rgba(10,15,28,.22)', padding: 7, maxHeight: Math.min(320, openUp ? rect.top - 12 : vh - rect.bottom - 12), overflowY: 'auto',
            }}>
          {searchable && (
            <input ref={searchRef} value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search…"
              onKeyDown={(e) => e.stopPropagation()}
              style={{ width: '100%', boxSizing: 'border-box', padding: '8px 10px', marginBottom: 6, fontSize: fs,
                color: 'var(--text)', background: 'var(--surface-2)', border: '1px solid var(--border)', borderRadius: 8, outline: 'none' }} />
          )}
          {shown.map((o, oi) => {
            const on = String(o.value) === String(value);
            return (
              <div key={`${o.value}-${oi}`} onClick={() => { onChange(o.value); setOpen(false); }}
                style={{
                  display: 'flex', alignItems: 'center', gap: 9, padding: '10px 12px', borderRadius: 8, cursor: 'pointer',
                  fontSize: fs, fontWeight: on ? 700 : 500,
                  background: on ? 'var(--primary-soft)' : 'transparent', color: on ? 'var(--primary)' : 'var(--text)',
                }}
                onMouseEnter={(e) => { if (!on) e.currentTarget.style.background = 'var(--surface-2)'; }}
                onMouseLeave={(e) => { if (!on) e.currentTarget.style.background = 'transparent'; }}>
                <span style={{ width: 15, flex: '0 0 auto' }}>{on && <Check size={14} />}</span>
                <span style={{ whiteSpace: 'nowrap' }}>{o.label}</span>
              </div>
            );
          })}
          {searchable && shown.length === 0 && <div className="muted" style={{ padding: '8px 12px', fontSize: fs }}>No match</div>}
            </div>
          );
        })(),
        // 전체화면(감시월)에선 body 포털이 화면 밖이라 안 보인다 → 전체화면 요소 안으로 붙인다.
        (typeof document !== 'undefined' && document.fullscreenElement) || document.body,
      )}
    </div>
  );
}
