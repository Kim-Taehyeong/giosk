import React, { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { TrendingUp, FolderKanban, Users, Coins, ChevronRight, Layers, AlertTriangle, UserPlus, BellRing, Clock, Timer } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import StatCard from '../../../components/console/StatCard';
import DataTable from '../../../components/console/DataTable';
import UtilLineChart from '../../../components/console/UtilLineChart';
import DoughnutChart from '../../../components/console/DoughnutChart';
import AlertFeed from '../../../components/console/AlertFeed';
import ActiveUserList from '../../../components/console/ActiveUserList';
import MonitorControls from '../../../components/console/MonitorControls';
import { c, cU } from '../../../lib/credit';
import usePoll from '../../../hooks/usePoll';
import { getOpsDashboard } from '../../../api/console/dashboard';
import { getTopupRequests, getUsers } from '../../../api/console/misc';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { useAuth } from '../../../context/AuthContext';

// ReqCard는 승인 대기 요청 카드(가입·크레딧 충전) — 클릭하면 승인 화면으로.
function ReqCard({ icon, label, n, tone, hint, onClick }) {
  const Icon = icon; // JSX 전용 참조는 지역 변수로(무시 패턴 ^[A-Z] — eslint 설정과 동일 관례)
  return (
    <div className="card" style={{ cursor: 'pointer', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, marginBottom: 0 }}
      onClick={onClick}>
      <span className="flex gap" style={{ alignItems: 'center' }}>
        <span style={{ width: 40, height: 40, borderRadius: 11, display: 'grid', placeItems: 'center', background: tone.bg, color: tone.fg }}><Icon size={20} /></span>
        <span>
          <div style={{ fontWeight: 800, fontSize: 15 }}>{label}</div>
          <div className="muted" style={{ fontSize: 12.5 }}>{hint}</div>
        </span>
      </span>
      <span className="flex gap" style={{ alignItems: 'center' }}>
        {n > 0 && <span style={{ minWidth: 26, height: 26, padding: '0 8px', borderRadius: 13, display: 'grid', placeItems: 'center', background: 'var(--danger)', color: '#fff', fontWeight: 800, fontSize: 13 }}>{n}</span>}
        <ChevronRight size={20} className="muted" />
      </span>
    </div>
  );
}

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
  const toApprovals = () => navigate('/console/admin/approvals');
  const reqHint = (n) => (n > 0 ? t('dash.pendingN', { n }) : t('dash.noPending'));


  // 지금 쓰고 있는 사용자는 세션 현황과 같은 맥락 → 별도 카드가 아니라 세션 도넛 옆에 붙인다(인프라 대시보드와 동일).
  const sessionCard = (
    <div className="card">
      <h3><Layers size={16} /> {t('dash.sessions')}</h3>
      <div className="flex" style={{ gap: 22, alignItems: 'flex-start', flexWrap: 'wrap' }}>
        <div>
          <DoughnutChart center={active + ss.idle + ss.provisioning} sub={t('dash.sessions')} segments={[
            { label: t('dash.sessionActive'), value: active, color: 'var(--free)' },
            { label: t('dash.sessionIdle'), value: ss.idle, color: 'var(--warn)' },
            { label: t('dash.sessionProvisioning'), value: ss.provisioning, color: 'var(--primary)' },
          ]} />
          <div className="legend mt">{t('dash.sessionStopped')}: {ss.stopped}</div>
        </div>
        <ActiveUserList users={users} title={t('dash.activeUsers')} emptyLabel={t('dash.noActiveUsers')} />
      </div>
    </div>
  );

  // 통합 경고 피드는 화면당 1개. 최고관리자는 인프라 대시보드에서 같은 피드를 보므로 여기선 숨긴다
  // (조직·그룹 관리자는 인프라 대시보드가 없어 여기가 유일한 노출 지점).
  const alertCard = (
    <div className="card">
      <h3><BellRing size={16} /> {t('dash.alertFeed')}</h3>
      <AlertFeed events={d.alertFeed} emptyLabel={t('dash.noAlerts')} />
    </div>
  );
  const showAlerts = !isPlatform;

  const modeHintCard = (
    <div className="card">
      <h3><AlertTriangle size={16} /> {t('dash.opsHint')}</h3>
      <div className="legend">{t(`dash.modeHint_${mode}`)}</div>
    </div>
  );

  return (
    <div ref={wrap} className="mon-wrap" style={{ background: 'var(--bg)' }}>
      <PageHead crumb={t('badge')} title={t('dash.opsTitle')} subtitle={t('dash.opsSubtitle')}
        actions={<MonitorControls intervalMs={intervalMs} setIntervalMs={setIntervalMs} containerRef={wrap} />} />

      {/* 승인 대기 요청 */}
      {((topupOn && isPlatform) || signupOn) && (
        <div className={`grid ${topupOn && isPlatform && signupOn ? 'cols-2' : 'cols-1'} mb`}>
          {topupOn && isPlatform && <ReqCard icon={Coins} label={t('dash.topupReq')} n={badges.topup} hint={reqHint(badges.topup)} onClick={toApprovals} tone={{ bg: 'var(--primary-soft)', fg: 'var(--primary)' }} />}
          {signupOn && <ReqCard icon={UserPlus} label={t('dash.signupReq')} n={badges.signup} hint={reqHint(badges.signup)} onClick={toApprovals} tone={{ bg: 'var(--gpu-soft)', fg: 'var(--gpu)' }} />}
        </div>
      )}

      {/* 운영 KPI (유휴는 개수만) */}
      <div className="grid cols-4 mb">
        <StatCard icon={Layers} tone="free" label={t('dash.activeSessions')} value={`${k.activeSessions}`} />
        <StatCard icon={BellRing} tone={ss.idle > 0 ? 'warn' : 'free'} label={t('dash.sessionIdle')} value={`${ss.idle}`} />
        <StatCard icon={Clock} tone="gpu" label={t('dash.gpuHoursMonth')} value={`${d.gpuHours || 0}`} unit="h" />
        {creditMode
          ? <StatCard icon={Coins} tone="gpu" label={t('dash.monthCredit')} value={c(k.monthCredit)} unit="C" />
          : <StatCard icon={Timer} tone="primary" label={t(`dash.mode_${mode}`)} value={t('dash.noCredit')} />}
      </div>

      {/* 세션(도넛+사용 중 유저) + (credit)크레딧 추이 / (그외)경고 피드·모드 안내 */}
      <div className="grid cols-2 mb">
        {sessionCard}
        {creditMode ? (
          <div className="card">
            <h3><TrendingUp size={16} /> {t('dash.creditTrend')}</h3>
            <UtilLineChart percent={false} labels={(d.creditTrend || []).map((x) => x.date)} values={(d.creditTrend || []).map((x) => x.amount)} />
          </div>
        ) : (showAlerts ? alertCard : modeHintCard)}
      </div>

      {/* credit: 상위 그룹·상위 사용자 */}
      {creditMode && (
        <div className="grid cols-2 mb">
          <div className="card">
            <h3><FolderKanban size={16} /> {t('dash.topGroups')}</h3>
            <DataTable rows={d.topGroups} columns={[
              { key: 'name', header: t('dash.group') },
              { key: 'credit', header: t('dash.consumed'), render: (r) => cU(r.credit) },
            ]} />
          </div>
          <div className="card">
            <h3><Users size={16} /> {t('dash.topUsers')}</h3>
            <DataTable rows={d.topUsers} columns={[
              { key: 'name', header: t('dash.user') },
              { key: 'credit', header: t('dash.consumed'), render: (r) => cU(r.credit) },
            ]} />
          </div>
        </div>
      )}

      {/* 마지막 줄: credit 은 경고 피드(노출 대상일 때), 그 외는 모드 안내(위에 경고 피드를 이미 뒀을 때만) */}
      {creditMode ? (showAlerts ? alertCard : null) : (showAlerts ? modeHintCard : null)}
    </div>
  );
}
