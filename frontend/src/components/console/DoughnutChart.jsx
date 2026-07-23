import React, { useMemo } from 'react';
import { Chart as ChartJS, ArcElement, Tooltip } from 'chart.js';
import { Doughnut } from 'react-chartjs-2';

ChartJS.register(ArcElement, Tooltip);

// Chart.js 는 CSS 변수(var(--x))를 못 읽으므로 실제 색값으로 변환한다.
function resolveColor(c) {
  if (typeof c === 'string' && c.startsWith('var(') && typeof window !== 'undefined') {
    const name = c.slice(4, -1).trim();
    return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || c;
  }
  return c;
}

// Chart.js 도넛 — segments: [{label,value,color}]. center/sub: 가운데 텍스트.
export default function DoughnutChart({ segments = [], center, sub, size = 160 }) {
  const total = segments.reduce((a, s) => a + (s.value || 0), 0);
  const { data, options } = useMemo(() => ({
    data: {
      labels: segments.map((s) => s.label),
      datasets: [{
        data: segments.map((s) => s.value || 0),
        backgroundColor: segments.map((s) => resolveColor(s.color)),
        borderColor: resolveColor('var(--surface)'),
        borderWidth: 2,
        hoverOffset: 4,
      }],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      cutout: '68%',
      plugins: {
        legend: { display: false },
        tooltip: { callbacks: { label: (c) => ` ${c.label}: ${c.parsed} (${total ? Math.round((c.parsed / total) * 100) : 0}%)` } },
      },
    },
  }), [segments, total]);

  return (
    <div className="flex" style={{ gap: 18, alignItems: 'center', flexWrap: 'wrap' }}>
      <div style={{ position: 'relative', width: size, height: size, flex: '0 0 auto' }}>
        <Doughnut data={data} options={options} />
        <div style={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center', pointerEvents: 'none', textAlign: 'center' }}>
          <div>
            <div style={{ fontSize: size * 0.26, fontWeight: 800, lineHeight: 1 }}>{center ?? total}</div>
            {sub && <div className="muted" style={{ fontSize: size * 0.085, marginTop: 2 }}>{sub}</div>}
          </div>
        </div>
      </div>
      <div className="grid" style={{ gap: 7, minWidth: 130 }}>
        {segments.map((s, i) => (
          <div key={i} className="flex" style={{ gap: 8, alignItems: 'center', fontSize: 13 }}>
            <span style={{ width: 10, height: 10, borderRadius: 3, background: s.color, flex: '0 0 auto' }} />
            <span style={{ flex: 1 }}>{s.label}</span>
            <span style={{ fontWeight: 700 }}>{s.value}</span>
            <span className="muted" style={{ fontSize: 11.5, minWidth: 36, textAlign: 'right' }}>{total ? Math.round(((s.value || 0) / total) * 100) : 0}%</span>
          </div>
        ))}
      </div>
    </div>
  );
}
