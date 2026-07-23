import React from 'react';

// 도넛 차트(SVG, 의존성 없음) — 세션 상태 비율 등.
// segments: [{ label, value, color }]. center: 가운데 큰 값. sub: 가운데 작은 라벨.
export default function Donut({ segments = [], center, sub, size = 150, stroke = 18 }) {
  const total = segments.reduce((a, s) => a + (s.value || 0), 0);
  const r = (size - stroke) / 2;
  const c = 2 * Math.PI * r;
  let offset = 0;
  return (
    <div className="flex" style={{ gap: 16, alignItems: 'center', flexWrap: 'wrap' }}>
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{ flex: '0 0 auto' }}>
        <g transform={`rotate(-90 ${size / 2} ${size / 2})`}>
          <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="var(--border)" strokeWidth={stroke} />
          {total > 0 && segments.map((s, i) => {
            const frac = (s.value || 0) / total;
            const dash = frac * c;
            const el = (
              <circle key={i} cx={size / 2} cy={size / 2} r={r} fill="none"
                stroke={s.color} strokeWidth={stroke}
                strokeDasharray={`${dash} ${c - dash}`} strokeDashoffset={-offset} strokeLinecap="butt" />
            );
            offset += dash;
            return el;
          })}
        </g>
        <text x="50%" y="47%" textAnchor="middle" dominantBaseline="middle"
          style={{ fontSize: size * 0.24, fontWeight: 800, fill: 'var(--text)' }}>{center ?? total}</text>
        {sub && <text x="50%" y="63%" textAnchor="middle" style={{ fontSize: size * 0.09, fill: 'var(--muted)' }}>{sub}</text>}
      </svg>
      <div className="grid" style={{ gap: 6, minWidth: 120 }}>
        {segments.map((s, i) => (
          <div key={i} className="flex" style={{ gap: 8, alignItems: 'center', fontSize: 13 }}>
            <span style={{ width: 10, height: 10, borderRadius: 3, background: s.color, flex: '0 0 auto' }} />
            <span style={{ flex: 1 }}>{s.label}</span>
            <span style={{ fontWeight: 700 }}>{s.value}</span>
            <span className="muted" style={{ fontSize: 11.5, minWidth: 34, textAlign: 'right' }}>
              {total > 0 ? Math.round(((s.value || 0) / total) * 100) : 0}%
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}
