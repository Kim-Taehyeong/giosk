import React, { useEffect, useRef, useState } from 'react';
import { ChevronDown, Check, Building2, FolderKanban } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useConsole } from '../../context/ConsoleContext';
import { useSystemConfig } from '../../context/SystemConfigContext';
import { getMembershipContext } from '../../api/console/membership';

// 탑바 도메인 셀렉터.
//  admin : 클러스터 표시(정적)
//  user  : 조직(읽기 전용 칩) + 그룹 전환 드롭다운.
//          그룹은 내가 가입한 그룹만 전환 — 신규 가입은 사이드바 '그룹 가입 신청'에서.
export default function OrgGroupSelector({ variant, ns }) {
  const { t } = useTranslation(ns);
  const { activeGroup, setActiveGroup, activeCluster } = useConsole();
  const { config } = useSystemConfig();
  const [open, setOpen] = useState(false);
  const [ctx, setCtx] = useState(null);
  const ref = useRef(null);

  useEffect(() => { if (variant !== 'admin') getMembershipContext().then(setCtx); }, [variant]);
  useEffect(() => {
    if (!open) return undefined;
    const onDoc = (e) => { if (ref.current && !ref.current.contains(e.target)) setOpen(false); };
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, [open]);

  if (variant === 'admin') {
    return (
      <div className="proj" role="button" style={{ cursor: 'default' }}>
        <small>{t('topbar.cluster')}</small>
        {/* 클러스터 이름: 활성 클러스터 > 설치 브랜드명. 하드코딩('gpu-dc') 제거. */}
        <span>{activeCluster?.name || config?.branding?.name || 'Giosk'}</span>
      </div>
    );
  }

  const myGroups = ctx?.myGroups || [];
  // 저장된 활성 그룹이 현재 내 그룹 목록에 있으면 그대로, 아니면 첫 그룹으로.
  const current = (activeGroup && myGroups.find((g) => g.id === activeGroup.id)) || myGroups[0] || null;
  const curGroupName = current?.displayName || current?.name || t('topbar.noGroup');
  const curOrg = current?.orgName || ctx?.org?.name || '—';

  const pickGroup = (g) => {
    setActiveGroup({ id: g.id, name: g.name, displayName: g.displayName, orgId: g.orgId, orgName: g.orgName });
    setOpen(false);
  };

  return (
    <>
      {/* 조직 — 읽기 전용 (현재 선택된 그룹의 소속 조직) */}
      <div className="proj" role="button" style={{ cursor: 'default' }}>
        <small>{t('topbar.org')}</small>
        <span className="flex gap" style={{ gap: 6 }}><Building2 size={13} /> {curOrg}</span>
      </div>

      {/* 그룹 — 가입한 그룹 간 전환 드롭다운 */}
      <div ref={ref} style={{ position: 'relative' }}>
        <div className="proj" role="button" onClick={() => setOpen((o) => !o)}>
          <small>{t('topbar.selectGroup')}</small>
          <span className="flex gap" style={{ gap: 6 }}>
            <FolderKanban size={13} /> {curGroupName}
            <ChevronDown size={14} style={{ transform: open ? 'rotate(180deg)' : 'none', transition: 'transform .15s' }} />
          </span>
        </div>

        {open && ctx && (
          <div style={{
            position: 'absolute', top: 'calc(100% + 6px)', left: 0, minWidth: 252, zIndex: 70,
            background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 12,
            boxShadow: '0 16px 40px rgba(10,15,28,.22)', padding: 8,
          }}>
            <div className="muted" style={{ fontSize: 10.5, fontWeight: 700, letterSpacing: '.04em', padding: '4px 8px' }}>{t('topbar.myGroups')}</div>
            {myGroups.length === 0 && <div className="muted" style={{ fontSize: 12, padding: '6px 10px' }}>{t('topbar.noGroupJoined')}</div>}
            {myGroups.map((g) => {
              const on = current?.id === g.id;
              const homeOrg = g.orgId === ctx.org?.id;
              return (
                <div key={g.id} onClick={() => pickGroup(g)}
                  style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '8px 10px', borderRadius: 8, cursor: 'pointer', background: on ? 'var(--primary-soft)' : 'transparent', color: on ? 'var(--primary)' : 'var(--text)', fontWeight: on ? 700 : 500 }}>
                  <FolderKanban size={14} />
                  <span style={{ flex: 1, fontSize: 13 }}>{g.displayName}</span>
                  <span className="muted" style={{ fontSize: 11 }}>{homeOrg ? t('topbar.homeOrg') : g.orgName}</span>
                  {on && <Check size={14} />}
                </div>
              );
            })}
          </div>
        )}
      </div>
    </>
  );
}
