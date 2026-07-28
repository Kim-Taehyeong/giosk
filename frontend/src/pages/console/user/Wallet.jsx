import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { TrendingUp, ReceiptText, PieChart, Wallet as WalletIcon, Hourglass, CalendarDays } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import StatCard from '../../../components/console/StatCard';
import Pill from '../../../components/console/Pill';
import DataTable from '../../../components/console/DataTable';
import Heatmap from '../../../components/console/Heatmap';
import Calendar from '../../../components/console/Calendar';
import Modal from '../../../components/console/Modal';
import { useToast } from '../../../components/console/Toast';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { getWallet, requestTopup } from '../../../api/console/wallet';
import { cU } from '../../../lib/credit';

const txVariant = { topup: 'ok', hold: 'wait', consume: 'gpu', settle: 'pause', refund: 'free' };

// 거래 설명 — 백엔드가 저장한 한국어 desc("GPU 사용(ref)") 대신 type+ref 로 현지화해서 렌더한다.
// 소비(consume)만 자동 라벨이라 번역 대상, 그 외(충전 메모 등)는 저장된 desc 를 그대로 둔다.
function txDesc(r, t) {
  const ref = (String(r.desc || '').match(/\(([^)]+)\)/) || [])[1] || '';
  if (r.type === 'consume' && ref) {
    return ref.startsWith('vol-')
      ? t('wallet.desc_storage', { ref, defaultValue: `Storage (${ref})` })
      : t('wallet.desc_gpu', { ref, defaultValue: `GPU usage (${ref})` });
  }
  return r.desc || t(`wallet.tx_${r.type}`, { defaultValue: r.type });
}

export default function Wallet() {
  const { t } = useTranslation('consoleUser');
  const { config } = useSystemConfig();
  const canRequest = config.features.creditRequest;
  const [w, setW] = useState(null);
  const [open, setOpen] = useState(false);
  const [amount, setAmount] = useState(100);
  const [reason, setReason] = useState('');
  const [page, setPage] = useState(0); // 내역 페이지네이션
  const [q, setQ] = useState('');       // 내역 검색(세션 ref/유형/내용)
  const [grouped, setGrouped] = useState(false); // 세션별 묶기 토글
  const { toast } = useToast();

  useEffect(() => { getWallet().then(setW); }, []);
  if (!w) return <div className="muted">{t('common.loading')}</div>;

  const PAGE_SIZE = 12;
  // 검색 — 유형/내용/세션 ref 를 소문자 부분일치.
  const ql = q.trim().toLowerCase();
  const matchTx = (r) => !ql || `${r.type} ${r.desc || ''} ${txDesc(r, t)}`.toLowerCase().includes(ql);
  const filteredHistory = w.history.filter(matchTx);
  const pageCount = Math.max(1, Math.ceil(filteredHistory.length / PAGE_SIZE));
  const curPage = Math.min(page, pageCount - 1);
  const pagedHistory = filteredHistory.slice(curPage * PAGE_SIZE, curPage * PAGE_SIZE + PAGE_SIZE);
  // 세션별 묶기 — bySession(세션 단위 소비 합계)을 검색 필터해 표로.
  const groupedRows = [...w.bySession]
    .filter((s) => !ql || `${s.name || ''} ${s.ref || ''}`.toLowerCase().includes(ql))
    .sort((a, b) => b.credit - a.credit);

  const submit = async () => {
    await requestTopup({ amount: Number(amount), reason });
    setOpen(false); toast(t('wallet.topupSent'));
  };

  const maxC = Math.max(...w.bySession.map((s) => s.credit), 1);

  // 정기 재충전 — 다음 충전일 = 마지막 사이클 + 주기.
  const rc = config.billing?.credit?.recharge || {};
  const nextRecharge = (rc.enabled && w.cycleStartedAt)
    ? new Date(new Date(w.cycleStartedAt).getTime() + (rc.intervalDays || 30) * 86400000).toLocaleDateString()
    : (rc.enabled ? t('wallet.rechargeSoon', { defaultValue: '곧' }) : '—');

  return (
    <div>
      <PageHead title={t('wallet.title')} subtitle={t('wallet.subtitle')}
        actions={canRequest ? <button className="btn primary" onClick={() => setOpen(true)}>{t('wallet.topupReq')}</button> : null} />

      <div className={`grid ${rc.enabled ? 'cols-4' : 'cols-3'} mb`}>
        <StatCard icon={WalletIcon} tone="gpu" label={t('wallet.balance')} value={cU(w.balance)} unit={t('wallet.reservedIncl', { n: w.reserved })} bar={{ value: w.balance, max: w.cap, variant: 'gpu' }} />
        <StatCard icon={Hourglass} tone="warn" label={t('wallet.eta')} value={t('wallet.etaDays', { n: w.etaDays, burn: w.burn })} />
        <StatCard icon={CalendarDays} tone="primary" label={t('wallet.monthUsed')} value={cU(w.monthUsed)} />
        {rc.enabled && <StatCard icon={CalendarDays} tone="free" label={t('wallet.nextRecharge', { defaultValue: '다음 리필' })} value={nextRecharge} sub={t('wallet.rechargeEvery', { n: rc.intervalDays, defaultValue: `${rc.intervalDays}일 주기` })} />}
      </div>

      <div className="card mb">
        <h3><TrendingUp size={16} /> {t('wallet.trend')}</h3>
        <div style={{ display: 'flex', gap: 18, alignItems: 'stretch', flexWrap: 'wrap' }}>
          <div style={{ flex: '1 1 420px', minWidth: 0, padding: 16, borderRadius: 12, border: '1px solid var(--border)', background: 'rgba(34,197,94,0.045)' }}>
            <div className="muted" style={{ fontSize: 12.5, fontWeight: 700, marginBottom: 10 }}>{t('wallet.jandi')}</div>
            <Heatmap rows={7} cell={28} data={w.trend.slice(-210).map((tt) => ({ label: `${tt.day}`, value: tt.amount }))} unit="C" />
          </div>
          <div style={{ flex: '0 0 360px', padding: 18, borderRadius: 12, border: '1px solid var(--border)', background: 'rgba(79,70,229,0.045)' }}>
            <div className="muted" style={{ fontSize: 12.5, fontWeight: 700, marginBottom: 10 }}>{t('wallet.calendar')}</div>
            <Calendar byMonth={w.byMonth} />
          </div>
        </div>
      </div>

      <div className="grid cols-2">
        <div className="card">
          <div className="flex" style={{ justifyContent: 'space-between', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
            <h3 style={{ margin: 0 }}><ReceiptText size={16} /> {t('wallet.history')}</h3>
            <div className="flex" style={{ gap: 8, alignItems: 'center' }}>
              <input type="text" value={q} onChange={(e) => { setQ(e.target.value); setPage(0); }}
                placeholder={t('wallet.searchPh', { defaultValue: '검색(세션·유형·내용)' })}
                style={{ width: 200, fontSize: 12.5, padding: '5px 9px' }} />
              <button className="btn sm" onClick={() => setGrouped((g) => !g)}>
                {grouped ? t('wallet.viewAll', { defaultValue: '전체 내역' }) : t('wallet.viewBySession', { defaultValue: '세션별 묶기' })}
              </button>
            </div>
          </div>
          {grouped ? (
            <DataTable
              rows={groupedRows}
              rowKey={(r, i) => r.ref || i}
              columns={[
                { key: 'name', header: t('wallet.session', { defaultValue: '세션' }), render: (r) => <span style={{ fontWeight: 600 }}>{r.name}</span> },
                { key: 'ref', header: 'ID', className: 'mono', render: (r) => <span className="muted" style={{ fontSize: 11.5 }}>{r.ref}</span> },
                { key: 'credit', header: t('wallet.consumedCol', { defaultValue: '소모' }), render: (r) => (r.credit ? cU(r.credit) : t('wallet.free')) },
              ]}
            />
          ) : (<>
          <DataTable
            rows={pagedHistory}
            columns={[
              { key: 'type', header: t('wallet.type'), render: (r) => <Pill variant={txVariant[r.type] || 'pause'}>{t(`wallet.tx_${r.type}`, { defaultValue: r.type })}</Pill> },
              { key: 'desc', header: t('wallet.desc'), render: (r) => txDesc(r, t) },
              { key: 'amount', header: t('wallet.amount'), render: (r) => (r.amount > 0 ? `+${r.amount}` : r.amount) },
              { key: 'balance', header: t('wallet.balanceCol') },
            ]}
          />
          {pageCount > 1 && (
            <div className="flex" style={{ justifyContent: 'flex-end', alignItems: 'center', gap: 10, marginTop: 12 }}>
              <button className="btn sm" disabled={curPage === 0} onClick={() => setPage(curPage - 1)}>{t('wallet.prev')}</button>
              <span className="muted" style={{ fontSize: 12.5 }}>{curPage + 1} / {pageCount}</span>
              <button className="btn sm" disabled={curPage >= pageCount - 1} onClick={() => setPage(curPage + 1)}>{t('wallet.next')}</button>
            </div>
          )}
          </>)}
        </div>
        <div className="card">
          <h3><PieChart size={16} /> {t('wallet.bySession')}</h3>
          {w.bySession.length === 0 && <div className="muted" style={{ padding: '16px 0', fontSize: 13 }}>{t('wallet.noSpend')}</div>}
          {[...w.bySession].sort((a, b) => b.credit - a.credit).map((s, i) => {
            const pct = maxC ? Math.round((s.credit / maxC) * 100) : 0;
            return (
              <div key={i} style={{ padding: '12px 0', borderBottom: '1px solid var(--border)' }}>
                <div className="flex" style={{ justifyContent: 'space-between', marginBottom: 7 }}>
                  <span style={{ fontWeight: 600, fontSize: 13.5 }}>{s.name}</span>
                  <span style={{ fontWeight: 800, fontSize: 14 }}>{s.credit ? cU(s.credit) : t('wallet.free')}</span>
                </div>
                <div style={{ height: 12, borderRadius: 6, background: 'var(--surface-2)', overflow: 'hidden' }}>
                  <div style={{ height: '100%', width: `${pct}%`, borderRadius: 6, background: s.credit ? 'var(--gpu)' : 'var(--free)' }} />
                </div>
              </div>
            );
          })}
        </div>
      </div>

      <Modal open={open} title={t('wallet.topupTitle')} onClose={() => setOpen(false)} width={440}
        footer={<>
          <span className="legend">{t('wallet.topupNote')}</span>
          <button className="btn primary" onClick={submit}>{t('wallet.topupSend')}</button>
        </>}>
        <label className="fld" style={{ marginTop: 0 }}>{t('wallet.amountLabel')}</label>
        <input type="number" value={amount} onChange={(e) => setAmount(e.target.value)} />
        <label className="fld">{t('wallet.reason')}</label>
        <textarea value={reason} onChange={(e) => setReason(e.target.value)} placeholder={t('wallet.reasonPh')} />
      </Modal>
    </div>
  );
}
