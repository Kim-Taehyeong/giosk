import React from 'react';
import { NavLink } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

// 그룹화된 사이드바 네비. navGroups + basePath(/console | /console/admin) + ns(i18n 네임스페이스).
export default function Sidebar({ navGroups, basePath, ns }) {
  const { t } = useTranslation(ns);
  return (
    <nav className="nav" id="console-nav" aria-label={t('nav.label', { defaultValue: '콘솔 메뉴' })}>
      {navGroups.map((group, gi) => (
        <div key={gi}>
          {group.title && <div className="group">{t(`nav.${group.title}`)}</div>}
          {group.items.map((item) => {
            const Icon = item.icon;
            return (
              <NavLink
                key={item.key}
                to={`${basePath}/${item.path}`}
                className={({ isActive }) => (isActive ? 'active' : undefined)}
              >
                <span className="i"><Icon size={17} /></span>
                <span>{t(`nav.${item.key}`)}</span>
                {item.tag && <span className="tag">{item.tag}</span>}
              </NavLink>
            );
          })}
        </div>
      ))}
    </nav>
  );
}
