import React from 'react';

// CSS 막대 차트. data: [{ label, value }]. max 미지정 시 자동.
export default function MiniChart({ data, height = 180 }) {
  const max = Math.max(1, ...data.map((d) => d.value));
  return (
    <div className="chart" style={{ height }}>
      {data.map((d, i) => (
        <div className="col" key={i}>
          <div className="cbar" style={{ height: `${(d.value / max) * 100}%` }} title={`${d.value}`} />
          <div className="cx">{d.label}</div>
        </div>
      ))}
    </div>
  );
}
