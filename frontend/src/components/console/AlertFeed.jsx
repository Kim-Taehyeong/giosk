import React from 'react';
import { AlertTriangle, AlertOctagon, Info, CheckCircle2 } from 'lucide-react';

const SEV = {
  err: { variant: 'err', icon: AlertOctagon, color: 'var(--danger)' },
  warn: { variant: 'warn', icon: AlertTriangle, color: 'var(--warn)' },
  info: { variant: 'primary', icon: Info, color: 'var(--primary)' },
};

// 간단 상대시각(같은 날=HH:MM, 아니면 M/D HH:MM).
function fmtTime(ts) {
  if (!ts) return '';
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return '';
  const now = new Date();
  const hm = `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
  if (d.toDateString() === now.toDateString()) return hm;
  return `${d.getMonth() + 1}/${d.getDate()} ${hm}`;
}

// 통합 경고 피드 — 운영/인프라 경고를 한 화면에 시간순으로.
// live: 현재상태 경보(AdminAlert[] {type,title,detail,age}), events: 발화 이력(alert_events[]).
export default function AlertFeed({ live = [], events = [], emptyLabel = '경고 없음' }) {
  const rows = [
    ...live.map((a) => ({ severity: a.type === 'err' ? 'err' : 'warn', source: 'infra', message: `${a.title} — ${a.detail}`, time: a.age, live: true })),
    ...events.map((e) => ({ severity: e.severity, source: e.source, message: e.message, time: fmtTime(e.ts), type: e.type })),
  ];
  if (rows.length === 0) {
    return <div className="muted flex gap" style={{ alignItems: 'center', padding: '18px 0', justifyContent: 'center' }}><CheckCircle2 size={16} /> {emptyLabel}</div>;
  }
  return (
    <div className="grid" style={{ gap: 2, maxHeight: 320, overflowY: 'auto' }}>
      {rows.map((r, i) => {
        const s = SEV[r.severity] || SEV.info;
        const Icon = s.icon;
        return (
          <div key={i} className="flex" style={{ gap: 10, alignItems: 'flex-start', padding: '8px 4px', borderBottom: '1px solid var(--border)' }}>
            <Icon size={15} style={{ color: s.color, flex: '0 0 auto', marginTop: 1 }} />
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ fontSize: 13, fontWeight: 600, wordBreak: 'break-word' }}>{r.message}</div>
              <div className="muted" style={{ fontSize: 11, marginTop: 1 }}>
                {r.source === 'ops' ? 'OPS' : 'INFRA'}{r.type ? ` · ${r.type}` : ''}{r.live ? ' · live' : ''}
              </div>
            </div>
            <span className="muted" style={{ fontSize: 11.5, flex: '0 0 auto' }}>{r.time}</span>
          </div>
        );
      })}
    </div>
  );
}
