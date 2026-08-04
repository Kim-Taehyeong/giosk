import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ChevronDown, ChevronRight } from 'lucide-react';

// Advanced는 선택 입력을 접어두는 섹션.
//
// 폼에 필수와 선택이 같은 무게로 나열되면 "다 채워야 하나?" 싶어 부담스럽다(조직 만들려는데
// 관리자 계정부터 찾아야 하는 것처럼 보였다). 필수만 펼쳐두고 선택은 여기로 접는다.
//  defaultOpen: 수정 폼처럼 기존 값이 있으면 펼쳐서 보여준다.
export default function Advanced({ title, children, defaultOpen = false, hint }) {
  const { t } = useTranslation('consoleAdmin');
  const [open, setOpen] = useState(defaultOpen);
  return (
    <div style={{ marginTop: 18, border: '1px solid var(--border)', borderRadius: 10, overflow: 'hidden' }}>
      <button type="button" onClick={() => setOpen((v) => !v)}
        style={{
          width: '100%', display: 'flex', alignItems: 'center', gap: 7, padding: '11px 13px',
          background: 'var(--surface-2)', border: 'none', cursor: 'pointer', color: 'var(--text)',
          fontWeight: 700, fontSize: 13, textAlign: 'left',
        }}>
        {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        {title || t('common.advanced')}
        <span className="muted" style={{ fontWeight: 500, fontSize: 11.5 }}>{t('common.optional')}</span>
      </button>
      {open && (
        <div style={{ padding: '4px 13px 14px' }}>
          {hint && <div className="legend" style={{ marginTop: 8 }}>{hint}</div>}
          {children}
        </div>
      )}
    </div>
  );
}

// Req는 필수 항목 표시(*)다. 라벨 옆에 붙여 필수와 선택을 한눈에 나눈다.
export const Req = () => <span style={{ color: 'var(--danger)', marginLeft: 3 }}>*</span>;
