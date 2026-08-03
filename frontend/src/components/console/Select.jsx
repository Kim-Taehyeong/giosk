import React, { useEffect, useId, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { ChevronDown, Check } from 'lucide-react';
import { nextDropdownId, announceOpen, onDropdownOpen } from './dropdownBus';

// 디자인된 커스텀 드롭다운. 네이티브 <select>의 OS 기본 옵션 목록 대신
// 테마(라이트/다크) 변수를 따르는 팝오버 메뉴를 렌더.
// 메뉴는 body 포털 + position:fixed 로 띄워, 카드 애니메이션(stacking context)·overflow 에
// 잘리지 않게 한다. CSS 변수는 :root 전역이라 포털 밖에서도 테마가 유지된다.
// options: [{ value, label }]
export default function Select({ value, onChange, options = [], placeholder, width, size, disabled = false, searchable = false, ariaLabel, ariaLabelledBy }) {
  const { t } = useTranslation('common');
  const [open, setOpen] = useState(false);
  const [rect, setRect] = useState(null);
  const [q, setQ] = useState('');
  // 키보드로 이동 중인 옵션(aria-activedescendant). -1 = 없음.
  const [active, setActive] = useState(-1);
  const btnRef = useRef(null);
  const menuRef = useRef(null);
  const listRef = useRef(null);
  const searchRef = useRef(null);
  const idRef = useRef(null);
  const domId = useId();
  if (idRef.current === null) idRef.current = nextDropdownId();
  const ph = placeholder ?? t('select.placeholder', { defaultValue: '선택' });

  // 다른 드롭다운이 열리면 나는 닫는다(동시에 여러 개가 떠 있지 않게).
  useEffect(() => onDropdownOpen((openedId) => {
    if (openedId !== idRef.current) setOpen(false);
  }), []);
  const sel = options.find((o) => String(o.value) === String(value));

  // 검색어로 걸러진 목록. 키보드 이동이 이 배열의 인덱스를 쓴다.
  const shown = searchable && q.trim()
    ? options.filter((o) => String(o.label).toLowerCase().includes(q.trim().toLowerCase()))
    : options;

  const openMenu = () => {
    if (disabled || !btnRef.current) return;
    setRect(btnRef.current.getBoundingClientRect());
    announceOpen(idRef.current); // 나머지 드롭다운을 닫는다.
    setQ('');
    // 열자마자 현재 선택 항목에 커서를 둔다(없으면 첫 항목).
    const cur = options.findIndex((o) => String(o.value) === String(value));
    setActive(cur >= 0 ? cur : 0);
    setOpen(true);
    // 검색형은 입력창, 아니면 목록 자체에 포커스를 줘 방향키를 받는다.
    setTimeout(() => (searchable ? searchRef.current : listRef.current)?.focus(), 0);
  };

  // 닫으면서 포커스를 트리거로 되돌린다(키보드 사용자가 미아가 되지 않게).
  const closeMenu = ({ refocus = true } = {}) => {
    setOpen(false);
    setActive(-1);
    if (refocus) btnRef.current?.focus();
  };

  const toggle = () => {
    if (disabled) return;
    if (open) closeMenu();
    else openMenu();
  };

  const pick = (o) => { onChange(o.value); closeMenu(); };

  // 트리거·목록 공통 키 조작(WAI-ARIA combobox 관행).
  const onKeyNav = (e) => {
    if (disabled) return;
    const { key } = e;
    if (!open) {
      if (key === 'ArrowDown' || key === 'Enter' || key === ' ') { e.preventDefault(); openMenu(); }
      return;
    }
    if (key === 'Escape') { e.preventDefault(); closeMenu(); return; }
    if (key === 'Tab') { closeMenu({ refocus: false }); return; }
    if (!shown.length) return;
    if (key === 'ArrowDown') { e.preventDefault(); setActive((i) => (i + 1) % shown.length); }
    else if (key === 'ArrowUp') { e.preventDefault(); setActive((i) => (i <= 0 ? shown.length - 1 : i - 1)); }
    else if (key === 'Home') { e.preventDefault(); setActive(0); }
    else if (key === 'End') { e.preventDefault(); setActive(shown.length - 1); }
    else if (key === 'Enter' || (key === ' ' && !searchable)) {
      e.preventDefault();
      if (shown[active]) pick(shown[active]);
    }
  };

  // 커서가 목록 밖으로 나가면 스크롤로 따라간다.
  useEffect(() => {
    if (!open || active < 0) return;
    menuRef.current?.querySelector(`#${CSS.escape(domId)}-opt-${active}`)?.scrollIntoView({ block: 'nearest' });
  }, [active, open, domId]);

  // 검색어가 바뀌면 커서를 첫 결과로 되돌린다(이펙트가 아니라 입력 시점에 — 연쇄 렌더 방지).
  const onSearch = (e) => { setQ(e.target.value); setActive(0); };

  // disabled 로 바뀌면 열린 메뉴를 닫는다.
  useEffect(() => { if (disabled) setOpen(false); }, [disabled]);

  useEffect(() => {
    if (!open) return undefined;
    const onDoc = (e) => {
      if (btnRef.current && btnRef.current.contains(e.target)) return;
      if (menuRef.current && menuRef.current.contains(e.target)) return;
      closeMenu({ refocus: false }); // 바깥 클릭은 그쪽으로 포커스가 가야 자연스럽다
    };
    // 스크롤/리사이즈 시 위치가 어긋나므로 닫는다. 단 메뉴 "내부" 스크롤은 무시(목록 스크롤 중 닫힘 방지).
    const onMove = (e) => {
      if (e && e.target && menuRef.current && menuRef.current.contains(e.target)) return;
      closeMenu({ refocus: false });
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
      <button ref={btnRef} type="button" onClick={toggle} onKeyDown={onKeyNav} disabled={disabled}
        role="combobox" aria-expanded={open} aria-haspopup="listbox"
        aria-controls={open ? `${domId}-list` : undefined}
        aria-label={ariaLabel} aria-labelledby={ariaLabelledBy}
        style={{
          width: '100%', textAlign: 'left', padding: pad, fontSize: fs, fontWeight: 500,
          cursor: disabled ? 'not-allowed' : 'pointer', opacity: disabled ? 0.5 : 1,
          border: '1px solid ' + (open ? 'var(--primary)' : 'var(--border)'), borderRadius: 'var(--r-control)',
          background: 'var(--surface-2)', color: sel ? 'var(--text)' : 'var(--muted)',
          boxShadow: open ? '0 0 0 3px var(--primary-soft)' : 'none', transition: 'border-color .15s, box-shadow .15s',
          position: 'relative', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
        }}>
        {sel ? sel.label : ph}
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
          return (
            <div ref={menuRef} style={{
              position: 'fixed', ...pos, left: rect.left, minWidth: Math.max(rect.width, 180), zIndex: 2000,
              background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--r-container)',
              boxShadow: '0 12px 32px rgba(10,15,28,.22)', padding: 7, maxHeight: Math.min(320, openUp ? rect.top - 12 : vh - rect.bottom - 12), overflowY: 'auto',
            }}>
          {searchable && (
            <input ref={searchRef} value={q} onChange={onSearch}
              placeholder={t('select.search', { defaultValue: 'Search…' })}
              aria-label={t('select.search', { defaultValue: 'Search…' })}
              aria-controls={`${domId}-list`}
              aria-activedescendant={active >= 0 ? `${domId}-opt-${active}` : undefined}
              onKeyDown={onKeyNav}
              style={{ width: '100%', boxSizing: 'border-box', padding: '8px 10px', marginBottom: 6, fontSize: fs,
                color: 'var(--text)', background: 'var(--surface-2)', border: '1px solid var(--border)', borderRadius: 'var(--r-control)', outline: 'none' }} />
          )}
          {/* 목록: 검색형이 아니면 목록 자체가 포커스를 받아 방향키를 처리한다. */}
          <div ref={listRef} id={`${domId}-list`} role="listbox" aria-label={ariaLabel} aria-labelledby={ariaLabelledBy}
            tabIndex={searchable ? -1 : 0}
            aria-activedescendant={active >= 0 ? `${domId}-opt-${active}` : undefined}
            onKeyDown={searchable ? undefined : onKeyNav}
            style={{ outline: 'none' }}>
            {shown.map((o, oi) => {
              const on = String(o.value) === String(value);
              const cursor = oi === active;
              return (
                <div key={`${o.value}-${oi}`} id={`${domId}-opt-${oi}`} role="option" aria-selected={on}
                  onClick={() => pick(o)}
                  onMouseMove={() => setActive(oi)}
                  style={{
                    display: 'flex', alignItems: 'center', gap: 9, padding: '10px 12px', borderRadius: 'var(--r-control)', cursor: 'pointer',
                    fontSize: fs, fontWeight: on ? 700 : 500,
                    // 선택됨(체크)과 커서 위치(호버/방향키)는 다른 상태다 — 커서는 배경만 바뀐다.
                    background: on ? 'var(--primary-soft)' : cursor ? 'var(--surface-2)' : 'transparent',
                    color: on ? 'var(--primary)' : 'var(--text)',
                    boxShadow: cursor && !on ? 'inset 0 0 0 1px var(--border)' : 'none',
                  }}>
                  <span style={{ width: 15, flex: '0 0 auto' }}>{on && <Check size={14} />}</span>
                  <span style={{ whiteSpace: 'nowrap' }}>{o.label}</span>
                </div>
              );
            })}
            {searchable && shown.length === 0 && (
              <div className="muted" style={{ padding: '8px 12px', fontSize: fs }}>{t('select.noMatch', { defaultValue: 'No match' })}</div>
            )}
          </div>
            </div>
          );
        })(),
        // 전체화면(감시월)에선 body 포털이 화면 밖이라 안 보인다 → 전체화면 요소 안으로 붙인다.
        (typeof document !== 'undefined' && document.fullscreenElement) || document.body,
      )}
    </div>
  );
}
