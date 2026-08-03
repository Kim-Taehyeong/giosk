import React from 'react';

// ActiveUserList는 "지금 세션을 쓰고 있는 사용자" 목록(운영·인프라 대시보드 공용).
// 세션 도넛 옆 여백에 붙는 컴팩트 리스트 — 별도 카드로 떼지 않는다(세션 현황과 같은 맥락).
export default function ActiveUserList({ users = [], title, emptyLabel, limit = 8 }) {
  return (
    <div style={{ flex: 1, minWidth: 200 }}>
      <div className="legend" style={{ marginTop: 0, marginBottom: 8, paddingBottom: 7, fontWeight: 700, borderBottom: '2px solid var(--border)' }}>
        {title}{users.length ? ` (${users.length})` : ''}
      </div>
      {users.length === 0 ? <div className="muted" style={{ fontSize: 12.5 }}>{emptyLabel}</div>
        : users.slice(0, limit).map((u, i) => (
          <div key={i} className="flex" style={{ justifyContent: 'space-between', alignItems: 'center', gap: 8, padding: '7px 0', borderTop: i ? '1px solid var(--border)' : 'none', fontSize: 12.5 }}>
            <span style={{ fontWeight: 600, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{u.name}</span>
            <span className="flex" style={{ gap: 8, alignItems: 'center', flex: '0 0 auto' }}>
              {u.gpuType && <span className="muted mono" style={{ fontSize: 11 }}>{u.gpuType.replace(/^NVIDIA-/, '')}</span>}
              <span className="badge" style={{ fontWeight: 700 }}>{u.sessions}</span>
            </span>
          </div>
        ))}
    </div>
  );
}
