import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { TrendingUp, BellRing, FolderKanban, Users, Coins, ChevronRight, Cpu, MemoryStick, Layers, Server, HeartPulse, UserPlus } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import StatCard from '../../../components/console/StatCard';
import Pill from '../../../components/console/Pill';
import DataTable from '../../../components/console/DataTable';
import UtilLineChart from '../../../components/console/UtilLineChart';
import { getAdminDashboard } from '../../../api/console/dashboard';
import { getTopupRequests, getUsers } from '../../../api/console/misc';
import { useSystemConfig } from '../../../context/SystemConfigContext';

export default function AdminDashboard() {
  const { t } = useTranslation('consoleAdmin');
  const { config } = useSystemConfig();
  const creditMode = config.billing.mode === 'credit';
  const signupOn = config.features.signupRequest;
  const topupOn = creditMode && config.features.creditRequest;
  const navigate = useNavigate();
  const [d, setD] = useState(null);
  const [pending, setPending] = useState(0);
  const [signupPending, setSignupPending] = useState(0);
  useEffect(() => {
    getAdminDashboard().then(setD);
    getTopupRequests().then((r) => setPending((r.items || []).filter((x) => x.status === 'pending').length));
    // 개수만 필요하므로 서버에 세게 하고 1건만 받는다(전체를 내려받지 않는다).
    getUsers({ status: 'pending', size: 1 }).then((r) => setSignupPending(r.total));
  }, []);
  if (!d) return <div className="muted">{t('common.loading')}</div>;
  const k = d.kpis;

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

  return (
    <div>
      <PageHead crumb={t('badge')} title={t('dash.title')} subtitle={t('dash.subtitle')} />

      {/* 최상단: 승인 대기 요청. 충전 요청 허용 시=충전 / 가입신청 허용 시=가입 */}
      {(topupOn || signupOn) && (
        <div className={`grid ${topupOn && signupOn ? 'cols-2' : 'cols-1'} mb`}>
          {topupOn && <ReqCard icon={Coins} label={t('dash.topupReq')} n={pending} tone={{ bg: 'var(--primary-soft)', fg: 'var(--primary)' }} />}
          {signupOn && <ReqCard icon={UserPlus} label={t('dash.signupReq')} n={signupPending} tone={{ bg: 'var(--gpu-soft)', fg: 'var(--gpu)' }} />}
        </div>
      )}

      {/* 운영 KPI — 모드 무관 5개 박스 한 줄 */}
      <div className="grid cols-5 mb">
        <StatCard icon={Cpu} tone="gpu" label={t('dash.gpuUtil')} value={`${k.gpuUtil}%`} bar={{ value: k.gpuUtil, max: 100, variant: 'gpu' }} />
        <StatCard icon={MemoryStick} tone="warn" label={t('dash.vramAlloc')} value={`${k.vramAlloc}%`} bar={{ value: k.vramAlloc, max: 100, variant: 'warn' }} />
        <StatCard icon={Layers} tone="free" label={t('dash.activeSessions')} value={`${k.activeSessions}`} unit={`/ ${k.maxSessions}`} />
        <StatCard icon={Server} tone="free" label={t('dash.nodesUp')} value={`${k.nodesUp}`} unit={`/ ${k.nodesTotal}`} bar={{ value: k.nodesUp, max: k.nodesTotal, variant: 'free' }} />
        <StatCard icon={HeartPulse} tone="warn" label={t('dash.healthAlerts')} value={`${k.healthAlerts}`} />
      </div>

      {creditMode && (
        <div className="card mb">
          <h3><TrendingUp size={16} /> {t('dash.creditTrend')}</h3>
          <UtilLineChart labels={(d.creditTrend || []).map((x) => x.date)} values={(d.creditTrend || []).map((x) => x.amount)} />
        </div>
      )}

      <div className="grid cols-2 mb">
        <div className="card">
          <h3><TrendingUp size={16} /> {t('dash.gpuTrend')}</h3>
          <UtilLineChart labels={(d.gpuTrend7d || []).map((x) => x.date)} values={(d.gpuTrend7d || []).map((x) => x.util)} />
        </div>
        <div className="card">
          <h3><BellRing size={16} /> {t('dash.alerts')} <span className="muted right" style={{ fontSize: 12 }}>{t('dash.alertsManaged')}</span></h3>
          {d.alerts.map((a, i) => (
            <div className="avail" key={i}>
              <span className="flex gap"><Pill variant={a.type}>{a.title}</Pill><span className="muted">{a.detail}</span></span>
              <span className="muted">{a.age}</span>
            </div>
          ))}
        </div>
      </div>

      <div className="grid cols-2">
        <div className="card">
          <h3><FolderKanban size={16} /> {t('dash.topGroups')}</h3>
          <DataTable rows={d.topGroups} columns={[
            { key: 'name', header: t('dash.group') },
            { key: 'credit', header: creditMode ? t('dash.consumed') : t('dash.gpuHours'), render: (r) => (creditMode ? `${r.credit} C` : `${r.credit} GPU·h`) },
          ]} />
        </div>
        <div className="card">
          <h3><Users size={16} /> {t('dash.topUsers')}</h3>
          <DataTable rows={d.topUsers} columns={[
            { key: 'name', header: t('dash.user') },
            { key: 'credit', header: creditMode ? t('dash.consumed') : t('dash.gpuHours'), render: (r) => (creditMode ? `${r.credit} C` : `${r.credit} GPU·h`) },
          ]} />
        </div>
      </div>
    </div>
  );
}
