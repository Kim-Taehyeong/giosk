import React, { useMemo } from 'react';
import {
  Chart as ChartJS, CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend, Filler,
} from 'chart.js';
import { Line } from 'react-chartjs-2';

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend, Filler);

// 현재 테마(CSS 변수)에서 색을 읽어 차트에 적용.
function cssVar(name, fallback) {
  if (typeof window === 'undefined') return fallback;
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  return v || fallback;
}

// 시계열 라인 차트. labels/values 가 기본 계열.
//  percent(기본 true): y축 0~100% + % 툴팁. false 면 자동 스케일(크레딧 등 절대값).
//  extra: { label, values } 있으면 둘째 계열(경고색) 오버레이 + 범례 표시.
//  label: 기본 계열 범례 이름(extra 있을 때만 표시).
export default function UtilLineChart({ labels, values, height = 220, percent = true, extra, label }) {
  const { data, options } = useMemo(() => {
    const primary = cssVar('--primary', '#4f46e5');
    const warn = cssVar('--warn', '#f59e0b');
    const grid = cssVar('--border', '#e5e7eb');
    const muted = cssVar('--muted', '#66707c');
    const mkFill = (color) => (ctx) => {
      const { ctx: c, chartArea } = ctx.chart;
      if (!chartArea) return 'transparent';
      const g = c.createLinearGradient(0, chartArea.top, 0, chartArea.bottom);
      g.addColorStop(0, `${color}40`);
      g.addColorStop(1, `${color}00`);
      return g;
    };
    // 점이 많으면(예: 24h 스냅샷) 마커를 숨겨 선만 보이게 한다 — 점이 촘촘하면 그래프가 안 보인다.
    const n = (values || []).length;
    const dot = n > 40 ? 0 : n > 20 ? 1.5 : 2.5;
    const base = (color, vals, name) => ({
      label: name, data: vals, borderColor: color, backgroundColor: mkFill(color),
      fill: true, tension: 0.4, borderWidth: 2.5, pointRadius: dot, pointBackgroundColor: color, pointHoverRadius: 5,
    });
    const datasets = [base(primary, values, label || '')];
    if (extra && extra.values) datasets.push({ ...base(warn, extra.values, extra.label), fill: false });
    return {
      data: { labels, datasets },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        interaction: { mode: 'index', intersect: false },
        plugins: {
          legend: { display: !!extra, labels: { color: muted, font: { size: 11 }, boxWidth: 12 } },
          tooltip: { callbacks: { label: (c) => ` ${c.dataset.label ? c.dataset.label + ': ' : ''}${c.parsed.y}${percent ? '%' : ''}` } },
        },
        scales: {
          y: {
            min: 0, ...(percent ? { max: 100 } : {}),
            ticks: { color: muted, font: { size: 11 }, callback: (v) => `${v}${percent ? '%' : ''}`, ...(percent ? { stepSize: 25 } : {}) },
            grid: { color: grid },
          },
          x: { ticks: { color: muted, font: { size: 11 }, maxRotation: 0, autoSkip: true, maxTicksLimit: 8 }, grid: { display: false } },
        },
      },
    };
  }, [labels, values, percent, extra, label]);

  return <div style={{ height }}><Line data={data} options={options} /></div>;
}
