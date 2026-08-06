import React, { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { ArrowLeft, Square, Power, Trash2, User, Building2, Users, Cpu, Coins, Clock, Server, FileClock, Terminal, RefreshCw, Boxes } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import StatCard from '../../../components/console/StatCard';
import PagedTable from '../../../components/console/PagedTable';
import Pill from '../../../components/console/Pill';
import UsageMeters from '../../../components/console/UsageMeters';
import Spinner from '../../../components/console/Spinner';
import { useToast } from '../../../components/console/Toast';
import { useConfirm } from '../../../components/console/Confirm';
import { c } from '../../../lib/credit';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { getAdminSession, getAdminSessionAudit, getAdminSessionLogs, getAdminSessionDescribe, getAdminSessionHistory, forceTerminate, adminStopSession, adminDeleteSession } from '../../../api/console/sessions';
import SessionHistoryChart from '../../../components/console/SessionHistoryChart';
import { measureRows, gpuUnmeasurable } from '../../../utils/sessionUsage';

const ST = {
  running: ['run', 'stRunning'], provisioning: ['wait', 'stProvisioning'],
  paused: ['pause', 'stPaused'], stopped: ['pause', 'stStopped'],
  terminated: ['pause', 'stTerminated'], failed: ['err', 'stFailed'],
};
const auditResultVariant = { applied: 'ok', success: 'ok', allowed: 'ok', denied: 'err', mfa: 'wait' };
const fmtDate = (s) => (s ? new Date(s).toLocaleString() : '—');

// 메타데이터 한 줄(라벨/값).
function Meta({ label, children }) {
  return (
    <div className="flex" style={{ justifyContent: 'space-between', gap: 12, padding: '7px 0', borderTop: '1px solid var(--border)', fontSize: 13 }}>
      <span className="muted">{label}</span>
      <span style={{ fontWeight: 600, textAlign: 'right' }}>{children}</span>
    </div>
  );
}

export default function SessionDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { t } = useTranslation('consoleAdmin');
  const { config } = useSystemConfig();
  const creditMode = config.billing.mode === 'credit';
  const { toast } = useToast();
  const confirm = useConfirm();

  const [s, setS] = useState(null);
  const [audit, setAudit] = useState([]);
  const [logs, setLogs] = useState(null);
  const [logsBusy, setLogsBusy] = useState(false);
  const [desc, setDesc] = useState(null);
  const [err, setErr] = useState(false);

  const load = () => getAdminSession(id).then(setS).catch(() => setErr(true));
  const loadLogs = () => { setLogsBusy(true); getAdminSessionLogs(id).then(setLogs).catch(() => setLogs('')).finally(() => setLogsBusy(false)); };
  const loadDesc = () => getAdminSessionDescribe(id).then(setDesc).catch(() => setDesc(null));
  useEffect(() => { setS(null); setErr(false); setLogs(null); setDesc(null); load(); getAdminSessionAudit(id).then(setAudit).catch(() => {}); loadDesc(); /* eslint-disable-next-line */ }, [id]);
  const physical = s?.env === 'ssh'; // 물리 노드 대여라 파드가 없고 k8s 로그나 describe 도 없다
  // running 이면 describe 도 주기 갱신(이벤트/재시작 실시간). 물리는 파드가 없어 스킵.
  useEffect(() => { if (s?.status !== 'running' || physical) return undefined; const t = setInterval(loadDesc, 10000); return () => clearInterval(t); /* eslint-disable-next-line */ }, [s?.status, physical]);
  // 실행 중이면 5초 폴링(실행시간/크레딧 갱신).
  useEffect(() => { if (s?.status !== 'running') return undefined; const t = setInterval(load, 5000); return () => clearInterval(t); /* eslint-disable-next-line */ }, [s?.status]);
  // 컨테이너 세션이면 쿠버네티스 로그를 자동으로 불러온다(물리는 파드가 없어 스킵).
  useEffect(() => { if (s && !physical && s.status === 'running' && logs == null && !logsBusy) loadLogs(); /* eslint-disable-next-line */ }, [s, physical]);

  if (err) return <div className="card">{t('sdetail.notFound')}</div>;
  if (!s) return <Spinner pad label={t('sdetail.loading', { defaultValue: '…' })} />;

  const [variant, key] = ST[s.status] || ['wait', null];
  const running = s.status === 'running';
  const stoppable = running || s.status === 'provisioning' || s.status === 'paused';
  const rows = measureRows(s, true);

  const doStop = async () => { if (!(await confirm({ title: t('monitor.stop'), message: t('monitor.confirmStop'), confirmText: t('monitor.stop'), danger: false }))) return; await adminStopSession(id); toast(t('monitor.stopped')); load(); };
  const doKill = async () => { if (!(await confirm({ title: t('monitor.force'), message: t('confirmTerminate'), confirmText: t('monitor.force'), danger: true }))) return; await forceTerminate(id); toast(t('monitor.forced')); load(); };
  const doDelete = async () => { if (!(await confirm({ title: t('sdetail.delete'), message: t('sdetail.confirmDelete'), confirmText: t('sdetail.delete'), danger: true }))) return; await adminDeleteSession(id); toast(t('sdetail.deleted')); navigate('/console/admin/sessions'); };

  return (
    <div>
      <button className="btn sm" style={{ marginBottom: 12 }} onClick={() => navigate('/console/admin/sessions')}>
        <ArrowLeft size={13} /> {t('sdetail.back')}
      </button>
      <PageHead
        icon={Server}
        title={s.name || id}
        subtitle={
          <span className="flex" style={{ gap: 10, alignItems: 'center', flexWrap: 'wrap' }}>
            <Pill variant={variant} dot>{key ? t('monitor.' + key) : s.status}</Pill>
            <span className="muted" style={{ fontSize: 12 }}>{t('sdetail.idLabel')}</span>
            <span className="mono" style={{ fontSize: 12.5 }}>{id}</span>
          </span>
        }
        actions={
          <span className="flex" style={{ gap: 8, alignItems: 'center' }}>
            {stoppable && <button className="btn" onClick={doStop}><Square size={14} /> {t('monitor.stop')}</button>}
            {running && <button className="btn warn" onClick={doKill}><Power size={14} /> {t('monitor.force')}</button>}
            <button className="btn danger" onClick={doDelete}><Trash2 size={14} /> {t('sdetail.delete')}</button>
          </span>
        } />

      {/* KPI */}
      <div className="grid cols-4 mb">
        <StatCard icon={Clock} tone="primary" label={t('sdetail.runtime')} value={s.runtime || '—'} />
        <StatCard icon={Server} tone="free" label={t('sdetail.node')} value={s.gpu || '—'} />
        <StatCard icon={Cpu} tone="gpu" label={t('sdetail.offering')} value={s.offering || '—'} />
        {creditMode && <StatCard icon={Coins} tone="warn" label={t('sdetail.credit')} value={c(s.credit)} unit="C" />}
      </div>

      <div className="grid cols-2" style={{ gap: 18, alignItems: 'start' }}>
        {/* 메타데이터 */}
        <div className="card">
          <h3><FileClock size={16} /> {t('sdetail.meta')}</h3>
          <Meta label={t('sdetail.created')}>{fmtDate(s.createdAt)}</Meta>
          <Meta label={t('sdetail.started')}>{fmtDate(s.startedAt)}</Meta>
          <Meta label={t('sdetail.mode')}>{physical ? t('sdetail.physical', { defaultValue: '물리 노드 대여' }) : (s.mode || '—')}</Meta>
          {!physical && <Meta label={t('sdetail.image', { defaultValue: '이미지' })}><span className="mono">{s.image || '—'}</span></Meta>}
          <Meta label={t('sdetail.offering')}>{s.offering || '—'}</Meta>
          <Meta label={t('sdetail.node')}>{s.gpu || '—'}</Meta>
          {creditMode && <Meta label={t('sdetail.pricePerHour')}>{c(s.pricePerHour)} C/h</Meta>}
        </div>

        {/* 사용자 / 조직 / 팀 */}
        <div className="card">
          <h3><User size={16} /> {t('sdetail.ownerSection')}</h3>
          <div className="flex" style={{ justifyContent: 'space-between', alignItems: 'center', padding: '7px 0' }}>
            <span className="flex muted" style={{ gap: 6, alignItems: 'center' }}><User size={13} /> {t('sdetail.owner')}</span>
            {s.userId
              ? <button className="btn sm" onClick={() => navigate(`/console/admin/users/${s.userId}`)}>{s.owner} →</button>
              : <span style={{ fontWeight: 600 }}>{s.owner}</span>}
          </div>
          <Meta label={<span className="flex" style={{ gap: 6, alignItems: 'center' }}><Building2 size={13} /> {t('sdetail.org')}</span>}>{s.org || '—'}</Meta>
          <Meta label={<span className="flex" style={{ gap: 6, alignItems: 'center' }}><Users size={13} /> {t('sdetail.group')}</span>}>{s.group || '—'}</Meta>
        </div>
      </div>

      {/* 실사용 메트릭 */}
      <div className="card mt">
        <h3><Cpu size={16} /> {t('sdetail.metrics')}</h3>
        {!running ? (
          <div className="legend">{t('sdetail.metricsIdle')}</div>
        ) : rows.length === 0 && !gpuUnmeasurable(s) ? (
          <div className="legend">{t('sdetail.metricsNone')}</div>
        ) : (
          <div>
            <UsageMeters rows={rows} />
            {gpuUnmeasurable(s) && <div className="muted" style={{ fontSize: 12, marginTop: 10 }}>GPU {t('monitor.notMeasurable')}</div>}
          </div>
        )}
      </div>

      {/* 사용률 이력(기본 24h) — Prometheus Range, DB 미저장 */}
      <div className="mt">
        <SessionHistoryChart id={id} fetch={getAdminSessionHistory} />
      </div>

      {/* 쿠버네티스 상세(describe) — 파드 상태·컨테이너·이벤트/오류 */}
      {desc && (
        <div className="card mt">
          <h3><Boxes size={16} /> {t('sdetail.describe')}</h3>

          {/* 파드 요약 — 라벨/값 그리드 */}
          <div className="grid cols-4" style={{ gap: 12, marginBottom: 4 }}>
            {[['Phase', desc.phase || '—', false], ['Node', desc.node || '—', true], ['IP', desc.ip || '—', true], ['Start', desc.startTime ? fmtDate(desc.startTime) : '—', false]].map(([k, v, mono]) => (
              <div key={k} style={{ padding: '9px 12px', borderRadius: 9, background: 'var(--surface-2)', border: '1px solid var(--border)' }}>
                <div className="muted" style={{ fontSize: 11, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '.03em', marginBottom: 3 }}>{k}</div>
                <div className={mono ? 'mono' : ''} style={{ fontSize: 13, fontWeight: 600 }}>{v}</div>
              </div>
            ))}
          </div>
          {(desc.reason || desc.message) && (
            <div style={{ margin: '10px 0 0', padding: '9px 12px', borderRadius: 9, background: 'color-mix(in srgb, var(--danger) 9%, transparent)', color: 'var(--danger)', fontSize: 12.5, fontWeight: 600 }}>
              {desc.reason} {desc.message ? `— ${desc.message}` : ''}
            </div>
          )}

          {/* 컨테이너 상태 — waiting/terminated 사유(CrashLoopBackOff·OOMKilled 등)가 핵심 */}
          {(desc.containers || []).length > 0 && (
            <div style={{ marginTop: 16 }}>
              <div className="legend" style={{ fontWeight: 700, marginBottom: 6, marginTop: 0 }}>{t('sdetail.containers')}</div>
              {desc.containers.map((c) => (
                <div key={c.name} style={{ padding: '10px 12px', borderRadius: 9, background: 'var(--surface-2)', border: '1px solid var(--border)', marginBottom: 8 }}>
                  <div className="flex" style={{ alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
                    <span className="mono" style={{ fontWeight: 700, fontSize: 13 }}>{c.name}</span>
                    <Pill variant={c.state === 'running' ? 'run' : c.state === 'waiting' ? 'wait' : 'err'} dot>{c.state}{c.reason ? `: ${c.reason}` : ''}</Pill>
                    <Pill variant={c.ready ? 'ok' : 'pause'}>{c.ready ? 'ready' : 'not ready'}</Pill>
                    <span style={{ marginLeft: 'auto', display: 'flex', gap: 12, fontSize: 12 }}>
                      <span style={{ color: c.restartCount > 0 ? 'var(--warn)' : 'var(--muted)', fontWeight: c.restartCount > 0 ? 700 : 500 }}>restarts {c.restartCount}</span>
                      {c.exitCode ? <span className="muted">exit {c.exitCode}</span> : null}
                    </span>
                  </div>
                  {(c.state !== 'running' || c.restartCount > 0) && c.message && <div className="muted" style={{ fontSize: 11.5, marginTop: 6 }}>{c.message}</div>}
                </div>
              ))}
            </div>
          )}

          {/* 이벤트 — Warning(오류) 강조 */}
          <div style={{ marginTop: 12 }}>
            <div className="legend" style={{ fontWeight: 700, marginBottom: 6, marginTop: 0 }}>{t('sdetail.events')} <span className="muted">({(desc.events || []).length})</span></div>
            {(desc.events || []).length === 0 ? <div className="muted" style={{ fontSize: 12.5 }}>{t('sdetail.noEvents')}</div> : (
              <div style={{ maxHeight: 240, overflow: 'auto', border: '1px solid var(--border)', borderRadius: 9 }}>
                {[...desc.events].reverse().map((e, i) => {
                  const warn = e.type === 'Warning';
                  return (
                    <div key={i} className="flex" style={{ gap: 10, alignItems: 'baseline', padding: '8px 11px', borderTop: i ? '1px solid var(--border)' : 'none',
                      background: warn ? 'color-mix(in srgb, var(--danger) 7%, transparent)' : 'transparent', fontSize: 12 }}>
                      <Pill variant={warn ? 'err' : 'pause'}>{e.reason}</Pill>
                      <span style={{ flex: 1, color: warn ? 'var(--danger)' : 'var(--text)' }}>{e.message}</span>
                      {e.count > 1 && <span className="muted">×{e.count}</span>}
                      <span className="muted mono" style={{ fontSize: 11, whiteSpace: 'nowrap' }}>{e.last ? fmtDate(e.last) : ''}</span>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>
      )}

      {/* 로그(감사) */}
      <div className="card mt">
        <h3><FileClock size={16} /> {t('sdetail.logs')} <span className="muted" style={{ fontSize: 12.5, fontWeight: 600 }}>({audit.length})</span></h3>
        {audit.length === 0 ? <div className="legend">{t('sdetail.noLogs')}</div> : (
          <PagedTable rows={audit} pageSize={10} rowKey={(r, i) => i}
            columns={[
              { key: 'createdAt', header: t('sdetail.time'), className: 'mono', render: (r) => fmtDate(r.createdAt) },
              { key: 'action', header: t('sdetail.action'), className: 'mono' },
              { key: 'actorUsername', header: t('sdetail.actor'), render: (r) => r.actorUsername || '—' },
              { key: 'result', header: t('sdetail.result'), render: (r) => <Pill variant={auditResultVariant[r.result] || 'pause'} dot>{t(`audit.result_${r.result}`, { defaultValue: r.result })}</Pill> },
            ]} />
        )}
      </div>

      {/* 물리 노드 대여는 파드가 없다 — 쿠버네티스 로그/describe 대신 안내 */}
      {physical ? (
        <div className="card mt">
          <h3><Server size={16} /> {t('sdetail.physicalTitle', { defaultValue: '물리 노드 대여' })}</h3>
          <div className="legend">{t('sdetail.physicalHint', { defaultValue: '이 세션은 컨테이너가 아니라 물리 노드를 직접 대여한 세션입니다. 파드가 없어 쿠버네티스 로그/describe 는 없습니다. 노드에 직접 접속(SSH)해 확인하세요.' })}</div>
          <div className="grid cols-2" style={{ gap: 12, marginTop: 10 }}>
            <div style={{ padding: '10px 12px', borderRadius: 9, background: 'var(--surface-2)', border: '1px solid var(--border)' }}>
              <div className="muted" style={{ fontSize: 11, fontWeight: 700, textTransform: 'uppercase', marginBottom: 3 }}>{t('sdetail.node')}</div>
              <div className="mono" style={{ fontWeight: 600 }}>{s.gpu || '—'}</div>
            </div>
            <div style={{ padding: '10px 12px', borderRadius: 9, background: 'var(--surface-2)', border: '1px solid var(--border)' }}>
              <div className="muted" style={{ fontSize: 11, fontWeight: 700, textTransform: 'uppercase', marginBottom: 3 }}>{t('sdetail.owner')}</div>
              <div style={{ fontWeight: 600 }}>{s.owner}</div>
            </div>
          </div>
        </div>
      ) : (
        <div className="card mt">
          <div className="flex" style={{ justifyContent: 'space-between', alignItems: 'center' }}>
            <h3 style={{ margin: 0 }}><Terminal size={16} /> {t('sdetail.k8sLogs')}</h3>
            <button className="btn sm" onClick={loadLogs} disabled={logsBusy}>
              <RefreshCw size={13} /> {logs == null ? t('sdetail.loadLogs') : t('sdetail.reloadLogs')}
            </button>
          </div>
          {logsBusy && logs == null ? <Spinner pad label={t('sdetail.loadingLogs', { defaultValue: '…' })} />
            : logs == null ? <div className="legend">{t('sdetail.logsHint')}</div>
            : logs.trim() === '' ? <div className="legend">{t('sdetail.noK8sLogs')}</div>
            : <pre style={{ maxHeight: 360, overflow: 'auto', background: 'var(--surface-2)', border: '1px solid var(--border)', borderRadius: 10, padding: '12px 14px', fontSize: 12, lineHeight: 1.5, margin: '10px 0 0', whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>{logs}</pre>}
        </div>
      )}
    </div>
  );
}
