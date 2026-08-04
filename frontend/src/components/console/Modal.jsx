import React, { useEffect, useId, useRef } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { X } from 'lucide-react';

// 포커스를 받을 수 있는 요소들(포커스 트랩 계산용).
const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

// 테마 인지 모달이다. body 포털로 렌더해 조상 요소의 transform/filter(스태킹 컨텍스트)에 갇히지 않고
// position:fixed 백드롭이 화면 전체를 덮는다(오퍼링 카드처럼 변형된 조상 안에서도 정상).
//
// 접근성: role=dialog + aria-modal 로 보조기술에 "지금은 이 안이 전부"라고 알리고,
// Escape 로 닫히며, 포커스는 열릴 때 안으로 들어와 Tab 이 밖으로 새지 않고, 닫히면 이전 요소로 돌아간다.
export default function Modal({ open, title, onClose, children, footer, width = 760 }) {
  const { t } = useTranslation('common');
  const panelRef = useRef(null);
  const lastFocused = useRef(null);
  const titleId = useId();

  useEffect(() => {
    if (!open) return undefined;
    lastFocused.current = document.activeElement;
    // 열리면 패널 안 첫 컨트롤로 포커스를 옮긴다(없으면 패널 자체).
    const panel = panelRef.current;
    const first = panel?.querySelector(FOCUSABLE);
    (first || panel)?.focus();

    const onKey = (e) => {
      if (e.key === 'Escape') { e.stopPropagation(); onClose?.(); return; }
      if (e.key !== 'Tab' || !panel) return;
      // 포커스 트랩: 모달 밖으로 나가려 하면 반대쪽 끝으로 되돌린다.
      const items = [...panel.querySelectorAll(FOCUSABLE)].filter((el) => el.offsetParent !== null);
      if (!items.length) { e.preventDefault(); panel.focus(); return; }
      const firstEl = items[0];
      const lastEl = items[items.length - 1];
      if (!e.shiftKey && document.activeElement === lastEl) { e.preventDefault(); firstEl.focus(); }
      else if (e.shiftKey && document.activeElement === firstEl) { e.preventDefault(); lastEl.focus(); }
    };
    document.addEventListener('keydown', onKey);
    // 뒤 배경이 스크롤되지 않게(모바일에서 특히 어색하다).
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.removeEventListener('keydown', onKey);
      document.body.style.overflow = prevOverflow;
      // 닫히면 열기 전 요소로 포커스 복귀.
      if (lastFocused.current instanceof HTMLElement) lastFocused.current.focus();
    };
  }, [open, onClose]);

  if (!open) return null;
  return createPortal(
    <div className="console-modal-bg" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose?.(); }}>
      <div ref={panelRef} className="modal" style={{ width }} onMouseDown={(e) => e.stopPropagation()}
        role="dialog" aria-modal="true" aria-labelledby={title ? titleId : undefined} tabIndex={-1}>
        <div className="modal-head">
          <h3 id={titleId}>{title}</h3>
          <button className="icon-btn" onClick={onClose} aria-label={t('close', { defaultValue: 'Close' })}><X size={16} /></button>
        </div>
        <div className="modal-body">{children}</div>
        {footer && <div className="modal-foot">{footer}</div>}
      </div>
    </div>,
    document.body,
  );
}
