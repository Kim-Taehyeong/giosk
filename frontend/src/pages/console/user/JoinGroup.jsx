import React, { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { UserPlus, Search, Building2, FolderKanban, Clock, CheckCheck } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import Pill from '../../../components/console/Pill';
import { useToast } from '../../../components/console/Toast';
import { getMembershipContext, requestJoinGroup, cancelJoinRequest } from '../../../api/console/membership';

// 그룹 가입 신청 — 전체 그룹(내 조직/타 조직)을 검색해 가입을 신청하고 신청 상태를 확인한다.
export default function JoinGroup() {
  const { t } = useTranslation('consoleUser');
  const { toast } = useToast();
  const [ctx, setCtx] = useState(null);
  const [q, setQ] = useState('');
  const [tab, setTab] = useState('search'); // search | mine
  const [requests, setRequests] = useState([]);
  const [sent, setSent] = useState({}); // 낙관적 표시: groupId -> true

  const reload = () => getMembershipContext().then((d) => { setCtx(d); setRequests(d.joinRequests || []); });
  useEffect(() => { reload(); }, []);

  const onCancel = async (r) => {
    await cancelJoinRequest(r.id);
    setSent((p) => { const n = { ...p }; delete n[r.groupId]; return n; });
    toast(t('joingrp.canceled', { name: r.groupName }));
    reload();
  };

  const dir = ctx?.directory || [];
  const myOrgId = ctx?.org?.id;
  const pendingIds = useMemo(
    () => new Set(requests.filter((r) => r.status === 'pending').map((r) => r.groupId)),
    [requests],
  );

  const filtered = dir.filter((g) => {
    const s = q.trim().toLowerCase();
    if (!s) return true;
    return g.displayName.toLowerCase().includes(s)
      || g.name.toLowerCase().includes(s)
      || (g.orgName || '').toLowerCase().includes(s);
  });

  const apply = async (g) => {
    await requestJoinGroup(g.id);
    setSent((p) => ({ ...p, [g.id]: true }));
    // 낙관적 표시(검색 목록의 '신청됨')는 즉시. 단, '나의 요청'의 취소 버튼은 서버 id(숫자)가 있어야
    // 뜨므로 reload 로 실 요청을 받아 tmp 항목을 교체한다(예전엔 tmp 문자열 id라 취소가 안 떴다).
    toast(t('joingrp.sent', { name: g.displayName }));
    reload();
  };

  const statusPill = (s) => {
    if (s === 'approved') return <Pill variant="ok" dot>{t('joingrp.statusApproved')}</Pill>;
    if (s === 'rejected') return <Pill variant="err" dot>{t('joingrp.statusRejected')}</Pill>;
    return <Pill variant="wait" dot>{t('joingrp.statusPending')}</Pill>;
  };

  const pendingCount = requests.filter((r) => r.status === 'pending').length;

  return (
    <div>
      <PageHead icon={UserPlus} title={t('joingrp.title')} subtitle={t('joingrp.subtitle')} />

      {/* 탭 분할 — 검색(가입 가능한 그룹) / 나의 요청(신청 현황·취소) */}
      <div className="subtabs mb" style={{ display: 'flex', gap: 4 }}>
        <span className={`st${tab === 'search' ? ' active' : ''}`} onClick={() => setTab('search')}>
          <Search size={13} /> {t('joingrp.tabSearch')}
        </span>
        <span className={`st${tab === 'mine' ? ' active' : ''}`} onClick={() => setTab('mine')}>
          <Clock size={13} /> {t('joingrp.tabMine')}{pendingCount > 0 ? ` (${pendingCount})` : ''}
        </span>
      </div>

      {/* 검색 + 가입 가능한 그룹 */}
      {tab === 'search' && (
      <div className="card">
        <h3><Search size={16} /> {t('joingrp.available')}</h3>
        <div style={{ position: 'relative', margin: '4px 0 14px' }}>
          <Search size={15} style={{ position: 'absolute', left: 11, top: '50%', transform: 'translateY(-50%)', color: 'var(--muted)' }} />
          <input
            type="text"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder={t('joingrp.searchPh')}
            style={{ paddingLeft: 34, width: '100%' }}
          />
        </div>
        {filtered.length === 0 && (
          <div className="muted" style={{ padding: '18px 0', textAlign: 'center' }}>{t('joingrp.none')}</div>
        )}
        {filtered.map((g) => {
          const home = g.orgId === myOrgId;
          const isPending = pendingIds.has(g.id) || sent[g.id];
          return (
            <div className="avail" key={g.id}>
              <span className="flex gap" style={{ alignItems: 'center', flexWrap: 'wrap' }}>
                <FolderKanban size={15} style={{ color: 'var(--primary)' }} />
                <strong>{g.displayName}</strong>
                <span className="muted flex gap" style={{ gap: 4, fontSize: 12, alignItems: 'center' }}>
                  <Building2 size={12} /> {g.orgName}
                </span>
                <Pill variant={home ? 'primary' : ''}>{home ? t('joingrp.homeOrg') : t('joingrp.otherOrg')}</Pill>
              </span>
              <span style={{ flex: '0 0 auto' }}>
                {g.joined
                  ? <Pill variant="ok" dot>{t('joingrp.joined')}</Pill>
                  : isPending
                    ? <Pill variant="wait" dot>{t('joingrp.requested')}</Pill>
                    : g.acceptsJoin === false
                      ? <Pill variant="pause" dot>{t('joingrp.closed')}</Pill>
                      : <button className="btn sm primary" onClick={() => apply(g)}><UserPlus size={13} /> {t('joingrp.apply')}</button>}
              </span>
            </div>
          );
        })}
      </div>
      )}

      {/* 내 가입 신청 현황 */}
      {tab === 'mine' && (
      <div className="card">
        <h3><Clock size={16} /> {t('joingrp.myRequests')}</h3>
        {requests.length === 0 && (
          <div className="muted" style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '18px 0', justifyContent: 'center' }}>
            <CheckCheck size={18} /> {t('joingrp.noRequests')}
          </div>
        )}
        {requests.map((r) => (
          <div className="avail" key={r.id}>
            <span className="flex gap" style={{ alignItems: 'center', flexWrap: 'wrap' }}>
              <FolderKanban size={15} />
              <strong>{r.groupName}</strong>
              <span className="muted" style={{ fontSize: 12 }}>{r.orgName}</span>
              {r.requestedAt && <span className="muted" style={{ fontSize: 12 }}>· {t('joingrp.reqAt', { d: r.requestedAt })}</span>}
            </span>
            <span className="flex gap" style={{ alignItems: 'center' }}>
              {statusPill(r.status)}
              {r.status === 'pending' && typeof r.id === 'number' && (
                <button className="btn sm danger" onClick={() => onCancel(r)}>{t('joingrp.cancel')}</button>
              )}
            </span>
          </div>
        ))}
      </div>
      )}
    </div>
  );
}
