import React, { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { ArrowLeft, Code2, NotebookPen, TerminalSquare, Square, RotateCcw, Trash2, Clock, Activity, Power, ScrollText, Server, Coins } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import StatCard from '../../../components/console/StatCard';
import Pill from '../../../components/console/Pill';
import Bar from '../../../components/console/Bar';
import Spinner from '../../../components/console/Spinner';
import PagedTable from '../../../components/console/PagedTable';
import ConnectionModal from '../../../components/console/ConnectionModal';
import SessionHistoryChart from '../../../components/console/SessionHistoryChart';
import { useToast } from '../../../components/console/Toast';
import { useConfirm } from '../../../components/console/Confirm';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { c } from '../../../lib/credit';
import { getMySessionsWithUsage, stopSession, startSession, deleteSession, extendSession, getSessionAudit, getSessionHistory } from '../../../api/console/sessions';
import { measureRows, gpuUnmeasurable } from '../../../utils/sessionUsage';
import { reclaimInfo } from '../../../utils/reclaim';

const CONN_ICON = { vscode: Code2, jupyter: NotebookPen, ssh: TerminalSquare };

export default function UserSessionDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { t } = useTranslation('consoleUser');
  const { config } = useSystemConfig();
  const creditMode = config.billing.mode === 'credit';
  const dynamicMode = config.billing.mode === 'dynamic';
  const lease = config.lease || { extensionHours: 0, maxExtensions: 0 };
  const { toast } = useToast();
  const confirm = useConfirm();

  const [r, setR] = useState(null);
  const [notFound, setNotFound] = useState(false);
  const [conn, setConn] = useState(false);
  const [activity, setActivity] = useState([]);

  const load = () => getMySessionsWithUsage().then((list) => {
    const found = (list || []).find((x) => x.id === id);
    if (!found) setNotFound(true); else setR(found);
  }).catch(() => setNotFound(true));
  useEffect(() => { load(); getSessionAudit(id).then(setActivity).catch(() => {}); /* eslint-disable-next-line */ }, [id]);
  // 종료 상태가 아니면 폴링해 준비중→실행 전이를 라이브로 반영(준비중엔 더 촘촘히).
  useEffect(() => {
    const terminal = ['stopped', 'failed', 'terminated', 'deleted', 'error'].includes(r?.status);
    if (!r || terminal) return undefined;
    const t = setInterval(load, r.status === 'running' ? 4000 : 2000);
    return () => clearInterval(t);
    /* eslint-disable-next-line */
  }, [r?.status]);

  if (notFound) return <div className="card">{t('sdetail.notFound', { defaultValue: '세션을 찾을 수 없습니다.' })}</div>;
  if (!r) return <Spinner pad label={t('sdetail.loading', { defaultValue: '…' })} />;

  const act = async (fn, msg) => { await fn(id); toast(msg); load(); };
  const doDelete = async () => {
    if (!await confirm({ title: t('session.delete'), message: t('confirmDelete'), confirmText: t('session.delete') })) return;
    await deleteSession(id);
    toast(t('session.deleted'));
    navigate('/console/sessions'); // 삭제 후엔 상세에 머무르지 말고 목록으로(세션이 사라졌으므로)
  };
  const doExtend = async () => { try { await extendSession(id); toast(t('session.extended', { h: lease.extensionHours || 1 })); } catch { toast(t('session.extendFailed')); } load(); };
  const canExtend = dynamicMode && r?.mode !== 'cpu' && r?.status === 'running' && (r?.extensionsUsed || 0) < lease.maxExtensions;
  const statusPill = (s) => {
    const map = { running: ['run', 'running'], provisioning: ['wait', 'provisioning'], queued: ['wait', 'queued'], paused: ['pause', 'paused'], stopped: ['pause', 'paused'], terminated: ['pause', 'terminated'] };
    const [v, k] = map[s] || ['pause', null];
    return <Pill variant={v} dot>{k ? t('session.' + k) : s}</Pill>;
  };
  const rows = measureRows(r, true);
  const isGpu = r.mode !== 'cpu' && r.mode !== 'ssh';
  // 자동 종료 임계 = 전역 유휴 정책(config.idle.timeoutMin). 유휴 리퍼는 GPU 세션만 회수(CPU 제외).
  const autoStopMin = isGpu ? (r.autoStopIdleMin || config.idle?.timeoutMin || 0) : 0;
  const canConnect = r.status === 'running' && (r.conn || []).length > 0;
  // 중단 세션 홈 회수 안내(중단 상태에서만 의미; 그 외엔 state='off').
  const reclaim = reclaimInfo(r, config);

  return (
    <div>
      <button className="btn sm" style={{ marginBottom: 12 }} onClick={() => navigate('/console/sessions')}>
        <ArrowLeft size={13} /> {t('sdetail.back', { defaultValue: '세션' })}
      </button>
      <PageHead
        icon={Server}
        title={r.name}
        subtitle={<span className="flex" style={{ gap: 10, alignItems: 'center', flexWrap: 'wrap' }}>{statusPill(r.status)}<span className="muted" style={{ fontSize: 12 }}>{t('session.detId')}</span><span className="mono" style={{ fontSize: 12.5 }}>{r.id}</span></span>}
        actions={
          <span className="flex" style={{ gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
            {canConnect && <button className="btn primary" onClick={() => setConn(true)}><TerminalSquare size={14} /> {t('session.conn')}</button>}
            {canExtend && <button className="btn" onClick={doExtend}><Clock size={14} /> {t('session.extend')} ({r.extensionsUsed || 0}/{lease.maxExtensions})</button>}
            {r.status === 'running' && <button className="btn" onClick={() => act(stopSession, t('session.stopped'))}><Square size={14} /> {t('session.stop')}</button>}
            {r.status === 'stopped' && <button className="btn" onClick={() => act(startSession, t('session.restarted'))}><RotateCcw size={14} /> {t('session.restart')}</button>}
            {r.status !== 'running' && <button className="btn danger" onClick={doDelete}><Trash2 size={14} /> {t('session.delete')}</button>}
          </span>
        } />

      <div className="grid cols-4 mb">
        <StatCard icon={Clock} tone="primary" label={t('session.runtime')} value={r.runtime || '—'} />
        <StatCard icon={Server} tone="free" label={t('session.detNode')} value={r.node || '—'} />
        <StatCard icon={Activity} tone="gpu" label={t('session.offering')} value={r.offering || '—'} />
        {creditMode && <StatCard icon={Coins} tone="warn" label={t('session.credit')} value={r.pricePerHour > 0 ? c(r.credit) : t('session.free')} unit={r.pricePerHour > 0 ? 'C' : ''} />}
      </div>

      {/* 좌: 실사용량 + 자동 종료(세로 스택) / 우: 사용률 이력 24h — 좌우 높이를 맞춰 여백을 줄인다 */}
      <div className="grid cols-2 mb" style={{ gap: 18, alignItems: 'start' }}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 18 }}>
          <div className="card">
            <h3 style={{ marginTop: 0 }}><Activity size={16} /> {t('session.detUsage')}</h3>
            {r.status !== 'running' ? (
              <div className="muted" style={{ fontSize: 12.5 }}>{t('sdetail.metricsIdle', { defaultValue: '세션이 실행 중일 때만 실사용량을 볼 수 있습니다.' })}</div>
            ) : rows.length > 0 ? (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                {rows.map((x) => (
                  <div key={x.label}>
                    <div className="flex" style={{ justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 5, fontSize: 12.5 }}>
                      <span style={{ fontWeight: 700, color: 'var(--muted)' }}>{x.label}</span>
                      <span><b style={{ fontSize: 14 }}>{Math.round(x.pct)}%</b> <span className="muted mono" style={{ fontSize: 11 }}>{x.txt}</span></span>
                    </div>
                    <Bar value={x.pct} max={100} variant={x.variant} className="framed" />
                  </div>
                ))}
              </div>
            ) : gpuUnmeasurable(r) ? <div className="muted" style={{ fontSize: 12.5 }}>{t(`session.gpuReason.${r.gpuReason}`)}</div>
              : <div className="muted" style={{ fontSize: 12.5 }}>{t('session.usageUnavailable')}</div>}
          </div>

          <div className="card">
            <h3 style={{ marginTop: 0 }}><Power size={16} /> {t('session.detAutoStop')}</h3>
            {autoStopMin ? (
              <>
                <div className="muted" style={{ fontSize: 12.5 }}>{t('session.detAutoStopDesc', { n: autoStopMin, defaultValue: `GPU 유휴가 ${autoStopMin}분 이상 지속되면 세션이 자동 종료됩니다.` })}</div>
                {r.status === 'stopped' && <div style={{ fontSize: 12.5, color: 'var(--danger)', fontWeight: 600, marginTop: 6 }}>{t('session.detStoppedIdle')}</div>}
              </>
            ) : <div className="muted" style={{ fontSize: 12.5 }}>{t('session.detNoAutoStop')}</div>}

            {/* 중단 세션 홈 회수 — 중단 중에도 홈 디스크를 계속 점유한다는 사실과 회수 조건을 알린다. */}
            {reclaim.state !== 'off' && (
              <div style={{ marginTop: 12, paddingTop: 12, borderTop: '1px solid var(--border)' }}>
                <div style={{ fontWeight: 700, fontSize: 12.5, marginBottom: 4 }}>{t('session.detReclaimTitle')}</div>
                {reclaim.state === 'exempt' ? (
                  <div className="muted" style={{ fontSize: 12.5 }}>{t('session.reclaimKeptHint')}</div>
                ) : (
                  <>
                    <div style={{ fontSize: 12.5, fontWeight: 600, color: reclaim.state === 'due' ? 'var(--danger)' : reclaim.state === 'soon' ? 'var(--warn)' : 'inherit' }}>
                      {reclaim.state === 'due' ? t('session.reclaimDue') : t('session.reclaimIn', { n: reclaim.days })}
                    </div>
                    <div className="muted" style={{ fontSize: 12.5, marginTop: 4, lineHeight: 1.6 }}>
                      {t('session.reclaimHint', { n: config.reclaim.stoppedTtlDays })}<br />
                      {t('session.reclaimResumeHint')}
                    </div>
                  </>
                )}
              </div>
            )}
          </div>
        </div>

        <SessionHistoryChart id={r.id} fetch={getSessionHistory} />
      </div>

      {/* 활동 기록 — 전체 폭 */}
      <div className="card">
        <h3 style={{ marginTop: 0 }}><ScrollText size={16} /> {t('session.detActivity')} <span className="muted" style={{ fontSize: 12.5, fontWeight: 600 }}>({activity.length})</span></h3>
        {activity.length === 0 ? <div className="muted" style={{ fontSize: 12.5 }}>{t('session.detNoActivity')}</div> : (
          <PagedTable rows={activity} pageSize={8} rowKey={(a, i) => i}
            columns={[
              { key: 'ts', header: t('session.detId', { defaultValue: '시각' }), className: 'mono', render: (a) => a.ts },
              { key: 'action', header: '', className: 'mono', render: (a) => a.action },
              { key: 'result', header: '', render: (a) => <span className="muted" style={{ fontSize: 12 }}>{a.actor} · {a.result}</span> },
            ]} />
        )}
      </div>

      <ConnectionModal session={conn ? r : null} onClose={() => setConn(false)} />
    </div>
  );
}
