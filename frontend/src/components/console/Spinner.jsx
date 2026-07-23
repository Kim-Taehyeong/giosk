import React from 'react';

// 간단한 회전 스피너. label 있으면 옆에 텍스트. pad=중앙 여백 박스.
export default function Spinner({ size = 20, label, pad = false }) {
  const ring = (
    <span
      aria-label="loading"
      style={{
        display: 'inline-block', width: size, height: size, flex: '0 0 auto',
        border: `${Math.max(2, size / 8)}px solid var(--border)`,
        borderTopColor: 'var(--primary)', borderRadius: '50%',
        animation: 'giosk-spin 0.7s linear infinite',
      }}
    />
  );
  if (!label && !pad) return ring;
  return (
    <div className="flex" style={{ gap: 10, alignItems: 'center', justifyContent: 'center', color: 'var(--muted)', padding: pad ? '32px 0' : 0 }}>
      {ring}{label && <span style={{ fontSize: 13 }}>{label}</span>}
    </div>
  );
}
