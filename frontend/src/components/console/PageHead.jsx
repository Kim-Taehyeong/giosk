import React from 'react';
import { useLocation } from 'react-router-dom';
import { userNavGroups } from '../../config/userNav';
import { adminNavGroups } from '../../config/adminNav';

// 사이드바 네비게이션의 아이콘을 경로별 아이콘 맵으로 평탄화한다.
const NAV_ICONS = [...userNavGroups, ...adminNavGroups]
  .flatMap((g) => g.items)
  .reduce((acc, it) => { acc[it.path.split('/')[0]] = it.icon; return acc; }, {});

// 페이지 헤더: (사이드바 아이콘) + 브레드크럼 + 제목 + 부제 + 우측 액션 슬롯.
// icon: 명시 전달 시 그 아이콘 사용. 미전달 시 현재 경로로 사이드바 아이콘 자동 매칭.
export default function PageHead({ crumb, title, subtitle, actions, icon }) {
  const { pathname } = useLocation();
  const segs = pathname.split('/').filter(Boolean);
  const autoKey = [...segs].reverse().find((s) => NAV_ICONS[s]);
  const Icon = icon || (autoKey && NAV_ICONS[autoKey]);

  return (
    <div className="page-head">
      <div>
        {crumb && <div className="crumb">{crumb}</div>}
        <h1 style={{ display: 'flex', alignItems: 'center', gap: 9 }}>
          {Icon && <Icon size={22} strokeWidth={2.2} style={{ flex: '0 0 auto', color: 'var(--primary)' }} />}
          <span>{title}</span>
        </h1>
        {subtitle && <p>{subtitle}</p>}
      </div>
      {actions && <div className="flex gap wrap">{actions}</div>}
    </div>
  );
}
