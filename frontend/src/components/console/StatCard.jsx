import React from 'react';
import Bar from './Bar';

// 통계 카드: (아이콘) + 라벨 + 값(+단위) + 선택적 진행막대.
// icon: lucide 아이콘 컴포넌트. tone: 아이콘 배지 색상 키(gpu|free|warn|primary).
const TONE = {
  primary: { bg: 'var(--primary-soft)', fg: 'var(--primary)' },
  gpu: { bg: 'var(--gpu-soft)', fg: 'var(--gpu)' },
  free: { bg: 'var(--free-soft)', fg: 'var(--free)' },
  warn: { bg: 'var(--warn-soft, rgba(245,158,11,.14))', fg: 'var(--warn)' },
};

// unit: 값 옆 짧은 단위(%, C, W…). sub: 값 아래 설명 캡션("최근 24h 기준" 등 — 숫자 옆에 붙으면 어색한 것).
export default function StatCard({ label, value, unit, sub, bar, icon: Icon, tone = 'primary' }) {
  const c = TONE[tone] || TONE.primary;
  return (
    <div className="card stat">
      <div className="flex" style={{ gap: 10, alignItems: 'center' }}>
        {Icon && (
          <span style={{ width: 32, height: 32, flex: '0 0 auto', borderRadius: 'var(--r-inset)', display: 'grid', placeItems: 'center', background: c.bg, color: c.fg }}>
            <Icon size={17} strokeWidth={2.2} />
          </span>
        )}
        <div className="label">{label}</div>
      </div>
      <div className="value">
        {value}
        {unit && <small> {unit}</small>}
      </div>
      {sub && <div className="muted" style={{ fontSize: 11.5, marginTop: 2, fontWeight: 500 }}>{sub}</div>}
      {bar && <Bar value={bar.value} max={bar.max} variant={bar.variant} />}
    </div>
  );
}
