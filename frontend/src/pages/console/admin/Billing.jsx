import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Receipt, Coins, Wallet as WalletIcon, Percent } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import StatCard from '../../../components/console/StatCard';
import Pill from '../../../components/console/Pill';
import PagedTable from '../../../components/console/PagedTable';
import { getBilling } from '../../../api/console/misc';
import { downloadCsv } from '../../../utils/csv';
import { cU } from '../../../lib/credit';
import { tabbable } from '../../../utils/a11y';

// 기간 프리셋 → {from,to} (YYYY-MM-DD; to 는 배타). 'all'=전체.
function periodRange(key) {
  const now = new Date();
  const ymd = (dt) => dt.toISOString().slice(0, 10);
  if (key === 'month') { const f = new Date(now.getFullYear(), now.getMonth(), 1); const t2 = new Date(now.getFullYear(), now.getMonth() + 1, 1); return { from: ymd(f), to: ymd(t2) }; }
  if (key === 'lastmonth') { const f = new Date(now.getFullYear(), now.getMonth() - 1, 1); const t2 = new Date(now.getFullYear(), now.getMonth(), 1); return { from: ymd(f), to: ymd(t2) }; }
  if (key === '7d') { const f = new Date(now.getTime() - 7 * 86400000); const t2 = new Date(now.getTime() + 86400000); return { from: ymd(f), to: ymd(t2) }; }
  return {};
}

export default function Billing() {
  const { t } = useTranslation('consoleAdmin');
  const [d, setD] = useState(null);
  const [tab, setTab] = useState('group');
  const [period, setPeriod] = useState('all');
  useEffect(() => { getBilling(periodRange(period)).then(setD); }, [period]);
  if (!d) return <div className="muted">{t('common.loading')}</div>;

  const PERIODS = [
    { k: 'all', label: t('billing.periodAll', { defaultValue: '전체' }) },
    { k: 'month', label: t('billing.periodMonth', { defaultValue: '이번 달' }) },
    { k: 'lastmonth', label: t('billing.periodLastMonth', { defaultValue: '지난 달' }) },
    { k: '7d', label: t('billing.period7d', { defaultValue: '최근 7일' }) },
  ];

  // 현재 탭 기준으로 CSV 내보내기(클라이언트 생성).
  const exportCsv = () => {
    let rows;
    if (tab === 'org') {
      rows = [['조직', '그룹수', '사용자수', 'GPU시간', '소모(C)', '할당(C)']];
      (d.byOrg || []).forEach((r) => rows.push([r.name, r.groups, r.users, r.gpuHours, r.consumed, r.creditPool]));
    } else if (tab === 'group') {
      rows = [['그룹', '조직', '세션수', 'GPU시간', '소모(C)', '예산(C)']];
      (d.byGroup || []).forEach((r) => rows.push([r.name, r.orgName, r.sessions, r.gpuHours, r.consumed, r.budgetCap]));
    } else {
      rows = [['사용자', '조직', '그룹', '세션수', 'GPU시간', '소모(C)']];
      (d.byUser || []).forEach((r) => rows.push([r.name, r.org, r.group, r.sessions, r.gpuHours, r.consumed]));
    }
    downloadCsv(`billing-${tab}.csv`, rows);
  };

  return (
    <div>
      <PageHead title={t('billing.title')} subtitle={t('billing.subtitle')}
        actions={<span className="flex gap" style={{ gap: 8, alignItems: 'center' }}>
          <span className="flex" style={{ gap: 2, background: 'var(--surface-2)', borderRadius: 8, padding: 3 }}>
            {PERIODS.map((p) => (
              <button key={p.k} className="btn sm" onClick={() => setPeriod(p.k)}
                style={{ background: period === p.k ? 'var(--primary)' : 'transparent', color: period === p.k ? '#fff' : 'var(--text)', border: 0 }}>
                {p.label}</button>
            ))}
          </span>
          <button className="btn" onClick={exportCsv}>{t('billing.csv')}</button>
        </span>} />
      {period !== 'all' && <div className="legend mb">{t('billing.periodNote', { defaultValue: '표시된 소모·GPU시간은 선택한 기간에 발생한 값입니다(원장 기준). 예산·풀은 기간 무관.' })}</div>}
      <div className="grid cols-3 mb">
        <StatCard icon={Coins} tone="gpu" label={t('billing.totalConsumed')} value={cU(d.totalConsumed)} />
        <StatCard icon={WalletIcon} tone="primary" label={t('billing.totalBudget')} value={cU(d.totalBudget)} />
        <StatCard icon={Percent} tone="warn" label={t('billing.vsBudget')} value={d.totalBudget ? `${Math.round(d.totalConsumed / d.totalBudget * 100)}%` : '—'} />
      </div>

      <div className="subtabs">
        <span className={`st${tab === 'org' ? ' active' : ''}`} {...tabbable(() => setTab('org'), tab === 'org')}>{t('billing.tabOrg')}</span>
        <span className={`st${tab === 'group' ? ' active' : ''}`} {...tabbable(() => setTab('group'), tab === 'group')}>{t('billing.tabGroup')}</span>
        <span className={`st${tab === 'user' ? ' active' : ''}`} {...tabbable(() => setTab('user'), tab === 'user')}>{t('billing.tabUser')}</span>
      </div>

      <div className="card">
        <h3><Receipt size={16} /> {tab === 'org' ? t('billing.byOrg') : tab === 'group' ? t('billing.byGroup') : t('billing.byUser')}</h3>

        {tab === 'org' && (
          <PagedTable rows={d.byOrg} pageSize={12} rowKey={(r) => r.id} columns={[
            { key: 'name', header: t('billing.org') },
            { key: 'groups', header: t('billing.groupCount') },
            { key: 'users', header: t('billing.userCount') },
            { key: 'gpuHours', header: t('billing.gpuHours'), render: (r) => `${r.gpuHours} h` },
            { key: 'consumed', header: t('billing.consumed'), render: (r) => cU(r.consumed) },
            { key: 'pool', header: t('billing.vsAlloc'), render: (r) => <Pill variant={r.consumed >= r.creditPool ? 'err' : 'ok'}>{Math.round(r.consumed / r.creditPool * 100)}%</Pill> },
          ]} />
        )}

        {tab === 'group' && (
          <PagedTable rows={d.byGroup} pageSize={12} rowKey={(r) => r.id} columns={[
            { key: 'name', header: t('billing.group') },
            { key: 'orgName', header: t('billing.org') },
            { key: 'sessions', header: t('billing.sessions') },
            { key: 'gpuHours', header: t('billing.gpuHours'), render: (r) => `${r.gpuHours} h` },
            { key: 'consumed', header: t('billing.consumed'), render: (r) => cU(r.consumed) },
            { key: 'budgetCap', header: t('billing.budget'), render: (r) => (r.budgetCap ? cU(r.budgetCap) : t('billing.notSet')) },
            { key: 'usagePct', header: t('billing.vsBudget2'), render: (r) => <Pill variant={r.usagePct >= 100 ? 'err' : r.usagePct >= 80 ? 'warn' : 'ok'}>{Math.round(r.usagePct)}%</Pill> },
          ]} />
        )}

        {tab === 'user' && (
          <PagedTable rows={d.byUser} pageSize={12} rowKey={(r) => `${r.id}-${r.groupId}`} columns={[
            { key: 'name', header: t('billing.user') },
            { key: 'org', header: t('billing.org', { defaultValue: '조직' }), render: (r) => (r.org ? <Pill variant="gpu">{r.org}</Pill> : <span className="muted">—</span>) },
            { key: 'group', header: t('billing.group'), render: (r) => (r.group ? <Pill variant="primary">{r.group}</Pill> : <span className="muted">—</span>) },
            { key: 'sessions', header: t('billing.sessions') },
            { key: 'gpuHours', header: t('billing.gpuHours'), render: (r) => `${r.gpuHours} h` },
            { key: 'consumed', header: t('billing.consumed'), render: (r) => cU(r.consumed) },
          ]} />
        )}
      </div>
    </div>
  );
}
