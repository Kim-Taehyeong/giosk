import React from 'react';

// 진행 막대. variant: '' | 'gpu' | 'warn' | 'free'. className 으로 트랙 보강(예: framed=흰 트랙+테두리).
export default function Bar({ value = 0, max = 100, variant = '', className = '' }) {
  const pct = max > 0 ? Math.min(100, Math.round((value / max) * 100)) : 0;
  return (
    <div className={`bar ${variant} ${className}`.trim()}>
      <i style={{ width: `${pct}%` }} />
    </div>
  );
}
