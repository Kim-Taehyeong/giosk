import React from 'react';
import { createPortal } from 'react-dom';
import { X } from 'lucide-react';

// 테마 인지 모달. body 포털로 렌더 → 조상 요소의 transform/filter(스태킹 컨텍스트)에 갇히지 않고
// position:fixed 백드롭이 화면 전체를 덮는다(오퍼링 카드처럼 변형된 조상 안에서도 정상).
export default function Modal({ open, title, onClose, children, footer, width = 760 }) {
  if (!open) return null;
  return createPortal(
    <div className="console-modal-bg" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose?.(); }}>
      <div className="modal" style={{ width }} onMouseDown={(e) => e.stopPropagation()}>
        <div className="modal-head">
          <h3>{title}</h3>
          <button className="icon-btn" onClick={onClose} aria-label="close"><X size={16} /></button>
        </div>
        <div className="modal-body">{children}</div>
        {footer && <div className="modal-foot">{footer}</div>}
      </div>
    </div>,
    document.body,
  );
}
