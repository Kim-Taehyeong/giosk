import React from 'react';

// 디자인된 on/off 토글 스위치 (네이티브 체크박스 대체). 테마 변수 사용.
export default function Toggle({ checked, onChange, size = 26 }) {
  const w = size * 1.75;
  const knob = size - 6;
  return (
    <button type="button" role="switch" aria-checked={checked} onClick={() => onChange(!checked)}
      style={{
        width: w, height: size, flex: '0 0 auto', borderRadius: size, border: 'none', cursor: 'pointer', padding: 0,
        position: 'relative', transition: 'background .18s ease',
        background: checked ? 'var(--primary)' : 'var(--surface-2)',
        boxShadow: checked ? 'none' : 'inset 0 0 0 1px var(--border)',
      }}>
      <span style={{
        position: 'absolute', top: 3, left: checked ? w - knob - 3 : 3, width: knob, height: knob, borderRadius: '50%',
        background: '#fff', boxShadow: '0 1px 3px rgba(0,0,0,.25)', transition: 'left .18s ease',
      }} />
    </button>
  );
}
