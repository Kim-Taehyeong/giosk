import React, { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { TrendingUp, FolderKanban, Users, Coins, ChevronRight, Layers, AlertTriangle, UserPlus, BellRing, Clock, Timer, Activity } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import StatCard from '../../../components/console/StatCard';
import DataTable from '../../../components/console/DataTable';
import Pill from '../../../components/console/Pill';
import UtilLineChart from '../../../components/console/UtilLineChart';
import DoughnutChart from '../../../components/console/DoughnutChart';
import AlertFeed from '../../../components/console/AlertFeed';
import MonitorControls from '../../../components/console/MonitorControls';
import usePoll from '../../../hooks/usePoll';
import { getOpsDashboard } from '../../../api/console/dashboard';
import { getTopupRequests, getUsers } from '../../../api/console/misc';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { useAuth } from '../../../context/AuthContext';

// 운영 대시보드(사용·거버넌스) — 전 관리 레벨, 과금모드 인식, 폴링 주기 선택 + 전체화면.
export default function OpsDashboard() {
  const { t } = useTranslation('consoleAdmin');
  const { config } = useSystemConfig();
  const { user } = useAuth();
  const isPlatform = user?.role === 'admin';
  const signupOn = config.features.signupRequest;
  const navigate = useNavigate();
  const [intervalMs, setIntervalMs] = useState(15000);
  const wrap = useRef(null);

  const d = usePoll(getOpsDashboard, intervalMs);
  const badges = usePoll(async () => {
    const out = { signup: 0, topup: 0 };
    try { out.signup = (await getUsers({ status: 'pending', size: 1 })).total; } catch { /* ignore */ }
    if (isPlatform) { try { out.topup = ((await getTopupRequests()).items || []).filter((x) => x.status === 'pending').length; } catch { /* ignore */ } }
    return out;
  }, 20000, [isPlatform]) || { signup: 0, topup: 0 };

  if (!d) return <div className="muted">{t('common.loading')}</div>;
  const k = d.kpis;
  const mode = d.billingMode || config.billing.mode;
  const creditMode = mode === 'credit';
  const topupOn = creditMode && config.features.creditRequest;
  const ss = d.sessionStats || { running: 0, idle: 0, provisioning: 0, stopped: 0 };
  const active = Math.max(0, ss.running - ss.idle);
  const users = d.activeUsers || [];

  const ReqCard = ({ icon: Icon, label, n, tone }) => (
    <div className="card" style={{ cursor: 'pointer', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, marginBottom: 0 }}
      onClick={() => navigate('/console/admin/approvals')}>
      <span className="flex gap" style={{ alignItems: 'center' }}>
        <span style={{ width: 40, height: 40, borderRadius: 11, display: 'grid', placeItems: 'center', background: tone.bg, color: tone.fg }}><Icon size={20} /></span>
        <span>
          <div style={{ fontWeight: 800, fontSize: 15 }}>{label}</div>
          <div className="muted" style={{ fontSize: 12.5 }}>{n > 0 ? t('dash.pendingN', { n }) : t('dash.noPending')}</div>
        </span>
      </span>
      <span className="flex gap" style={{ alignItems: 'center' }}>
        {n > 0 && <span style={{ minWidth: 26, height: 26, padding: '0 8px', borderRadius: 13, display: 'grid', placeItems: 'center', background: 'var(--danger)', color: '#fff', fontWeight: 800, fontSize: 13 }}>{n}</span>}
        <ChevronRight size={20} className="muted" />
      </span>
    </div>
  );

  const activeUsersCard = (
    <div className="card">
      <h3><Activity size={16} /> {t('dash.activeUsers')} <span className="muted" style={{ fontSize: 12 }}>({users.length})</span></h3>
      {users.length ? (
        <DataTable rows={users} rowKey={(r) => r.name} columns={[
          { key: 'name', header: t('dash.user'), render: (r) => <span style={{ fontWeight: 600 }}>{r.name}</span> },
          { key: 'gpuType', header: t('dash.byGpuType'), render: (r) => (r.gpuType ? <Pill variant="gpu">{r.gpuType.replace(/^NVIDIA-/, '')}</Pill> : <span className="muted">CPU</span>) },
          { key: 'sessions', header: t('dash.activeSessions'), render: (r) => `${r.sessions}` },
        ]} />
      ) : <div className="muted" style={{ padding: '18px 0', textAlign: 'center' }}>{t('dash.noActiveUsers')}</div>}
    </div>
  );

  return (
    <div ref={wrap} className="mon-wrap" style={{ background: 'var(--bg)' }}>
      <PageHead crumb={t('badge')} title={t('dash.opsTitle')} subtitle={t('dash.opsSubtitle')}
        actions={<MonitorControls intervalMs={intervalMs} setIntervalMs={setIntervalMs} containerRef={wrap} />} />

      {/* 승인 대기 요청 */}
      {((topupOn && isPlatform) || signupOn) && (
        <div className={`grid ${topupOn && isPlatform && signupOn ? 'cols-2' : 'cols-1'} mb`}>
          {topupOn && isPlatform && <ReqCard icon={Coins} label={t('dash.topupReq')} n={badges.topup} tone={{ bg: 'var(--primary-soft)', fg: 'var(--primary)' }} />}
          {signupOn && <ReqCard icon={UserPlus} label={t('dash.signupReq')} n={badges.signup} tone={{ bg: 'var(--gpu-soft)', fg: 'var(--gpu)' }} />}
        </div>
      )}

      {/* 운영 KPI (유휴는 개수만) */}
      <div className="grid cols-4 mb">
        <StatCard icon={Layers} tone="free" label={t('dash.activeSessions')} value={`${k.activeSessions}`} />
        <StatCard icon={BellRing} tone={ss.idle > 0 ? 'warn' : 'free'} label={t('dash.sessionIdle')} value={`${ss.idle}`} />
        <StatCard icon={Clock} tone="gpu" label={t('dash.gpuHoursMonth')} value={`${d.gpuHours || 0}`} unit="h" />
        {creditMode
          ? <StatCard icon={Coins} tone="gpu" label={t('dash.monthCredit')} value={`${k.monthCredit}`} unit="C" />
          : <StatCard icon={Timer} tone="primary" label={t(`dash.mode_${mode}`)} value={t('dash.noCredit')} />}
      </div>

      {/* 세션 도넛 + (credit)크레딧 추이 / (그외)경고 피드 */}
      <div className="grid cols-2 mb">
        <div className="card">
          <h3><Layers size={16} /> {t('dash.sessions')}</h3>
          <DoughnutChart center={active + ss.idle + ss.provisioning} sub={t('dash.sessions')} segments={[
            { label: t('dash.sessionActive'), value: active, color: 'var(--free)' },
            { label: t('dash.sessionIdle'), value: ss.idle, color: 'var(--warn)' },
            { label: t('dash.sessionProvisioning'), value: ss.provisioning, color: 'var(--primary)' },
          ]} />
          <div className="legend mt">{t('dash.sessionStopped')}: {ss.stopped}</div>
        </div>
        {creditMode ? (
          <div className="card">
            <h3><TrendingUp size={16} /> {t('dash.creditTrend')}</h3>
            <UtilLineChart percent={false} labels={(d.creditTrend || []).map((x) => x.date)} values={(d.creditTrend || []).map((x) => x.amount)} />
          </div>
        ) : (
          <div className="card">
            <h3><BellRing size={16} /> {t('dash.alertFeed')}</h3>
            <AlertFeed events={d.alertFeed} emptyLabel={t('dash.noAlerts')} />
          </div>
        )}
      </div>

      {/* 사용 중 유저 + (credit)상위 그룹 / (그외)경고 피드 */}
      <div className="grid cols-2 mb">
        {activeUsersCard}
        {creditMode ? (
          <div className="card">
            <h3><FolderKanban size={16} /> {t('dash.topGroups')}</h3>
            <DataTable rows={d.topGroups} columns={[
              { key: 'name', header: t('dash.group') },
              { key: 'credit', header: t('dash.consumed'), render: (r) => `${r.credit} C` },
            ]} />
          </div>
        ) : (
          <div className="card">
            <h3><BellRing size={16} /> {t('dash.alertFeed')}</h3>
            <AlertFeed events={d.alertFeed} emptyLabel={t('dash.noAlerts')} />
          </div>
        )}
      </div>

      {creditMode ? (
        <div className="card">
          <h3><Users size={16} /> {t('dash.topUsers')}</h3>
          <DataTable rows={d.topUsers} columns={[
            { key: 'name', header: t('dash.user') },
            { key: 'credit', header: t('dash.consumed'), render: (r) => `${r.credit} C` },
          ]} />
        </div>
      ) : (
        <div className="card">
          <h3><AlertTriangle size={16} /> {t('dash.opsHint')}</h3>
          <div className="legend">{t(`dash.modeHint_${mode}`)}</div>
        </div>
      )}
    </div>
  );
}
