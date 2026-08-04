import React, { useMemo } from 'react';
import { Chart as ChartJS, CategoryScale, LinearScale, BarElement, Tooltip } from 'chart.js';
import { Bar } from 'react-chartjs-2';

ChartJS.register(CategoryScale, LinearScale, BarElement, Tooltip);

function cssVar(name, fb) {
  if (typeof window === 'undefined') return fb;
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fb;
}

// Chart.js 막대. data 는 [{label,value}] 이고 horizontal 옵션이 있다. 단일 색(테마 primary 나 gpu)을 쓴다.
export default function BarChart({ data = [], height = 200, horizontal = false, color = '--gpu' }) {
  const { chartData, options } = useMemo(() => {
    const c = cssVar(color, '#7c6cf0');
    const muted = cssVar('--muted', '#66707c');
    const grid = cssVar('--border', '#e5e7eb');
    return {
      chartData: {
        labels: data.map((d) => d.label),
        datasets: [{ data: data.map((d) => d.value), backgroundColor: c, borderRadius: 6, maxBarThickness: 46 }],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        indexAxis: horizontal ? 'y' : 'x',
        plugins: { legend: { display: false }, tooltip: { callbacks: { label: (x) => ` ${x.parsed[horizontal ? 'x' : 'y']}` } } },
        scales: {
          x: { ticks: { color: muted, font: { size: 11 }, precision: 0 }, grid: { display: !horizontal, color: grid } },
          y: { beginAtZero: true, ticks: { color: muted, font: { size: 11 }, precision: 0 }, grid: { display: horizontal, color: grid } },
        },
      },
    };
  }, [data, horizontal, color]);

  return <div style={{ height }}><Bar data={chartData} options={options} /></div>;
}
