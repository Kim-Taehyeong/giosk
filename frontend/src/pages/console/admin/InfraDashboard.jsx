import React, { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { TrendingUp, BellRing, Cpu, MemoryStick, Server, HeartPulse, Thermometer, Layers, Boxes, Users } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import StatCard from '../../../components/console/StatCard';
import UtilLineChart from '../../../components/console/UtilLineChart';
import BarChart from '../../../components/console/BarChart';
import DoughnutChart from '../../../components/console/DoughnutChart';
import AlertFeed from '../../../components/console/AlertFeed';
import MonitorControls from '../../../components/console/MonitorControls';
import usePoll from '../../../hooks/usePoll';
import { getInfraDashboard } from '../../../api/console/dashboard';

const hhmm = (ts) => { const d = new Date(ts); return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`; };

// 인프라 대시보드(클러스터 하드웨어) — 최고관리자 전용. 감시월: 폴링 주기 선택 + 전체화면.
export default function InfraDashboard() {
  const { t } = useTranslation('consoleAdmin');
  const [intervalMs, setIntervalMs] = useState(15000);
  const wrap = useRef(null);
  const d = usePoll(getInfraDashboard, intervalMs);
  if (!d) return <div className="muted">{t('common.loading')}</div>;
  const k = d.kpis;
  const ss = d.sessionStats || { running: 0, idle: 0, provisioning: 0, stopped: 0, byGpuType: {} };
  const active = Math.max(0, ss.running - ss.idle);
  const users = d.activeUsers || [];

  const snaps = d.snapshots || [];
  const trendLabels = snaps.length ? snaps.map((s) => hhmm(s.ts)) : (d.gpuTrend7d || []).map((x) => x.date);
  const utilVals = snaps.length ? snaps.map((s) => s.gpuUtil) : (d.gpuTrend7d || []).map((x) => x.util);
  const vramVals = snaps.length ? snaps.map((s) => s.vramUsedPct) : [];
  const byType = Object.entries(ss.byGpuType || {}).map(([label, value]) => ({ label: label.replace(/^NVIDIA-/, ''), value }));

  return (
    <div ref={wrap} className="mon-wrap" style={{ background: 'var(--bg)' }}>
      <PageHead crumb={t('badge')} title={t('dash.infraTitle')} subtitle={t('dash.infraSubtitle')}
        actions={<MonitorControls intervalMs={intervalMs} setIntervalMs={setIntervalMs} containerRef={wrap} />} />

      {/* 인프라 KPI (GPU 온도는 값이 있을 때만) */}
      <div className="grid cols-4 mb">
        <StatCard icon={Cpu} tone="gpu" label={t('dash.gpuUtil')} value={`${k.gpuUtil}%`} bar={{ value: k.gpuUtil, max: 100, variant: 'gpu' }} />
        <StatCard icon={MemoryStick} tone="warn" label={t('dash.vramAlloc')} value={`${k.vramAlloc}%`} bar={{ value: k.vramAlloc, max: 100, variant: 'warn' }} />
        <StatCard icon={Boxes} tone="primary" label={t('dash.gpusUsed')} value={`${k.gpusUsed}`} unit={`/ ${k.gpusTotal}`} bar={{ value: k.gpusUsed, max: k.gpusTotal || 1, variant: 'gpu' }} />
        <StatCard icon={Server} tone="free" label={t('dash.nodesUp')} value={`${k.nodesUp}`} unit={`/ ${k.nodesTotal}`} bar={{ value: k.nodesUp, max: k.nodesTotal || 1, variant: 'free' }} />
        <StatCard icon={Layers} tone="primary" label={t('dash.activeSessions')} value={`${k.activeSessions}`} unit={`/ ${k.maxSessions}`} />
        <StatCard icon={BellRing} tone={ss.idle > 0 ? 'warn' : 'free'} label={t('dash.sessionIdle')} value={`${ss.idle}`} />
        <StatCard icon={HeartPulse} tone={k.healthAlerts > 0 ? 'warn' : 'free'} label={t('dash.healthAlerts')} value={`${k.healthAlerts}`} />
        {k.gpuTempMax > 0
          ? <StatCard icon={Thermometer} tone={k.gpuTempMax >= 80 ? 'warn' : 'primary'} label={t('dash.gpuTempMax')} value={`${k.gpuTempMax}°`} />
          : <StatCard icon={Users} tone="primary" label={t('dash.activeUsers')} value={`${users.length}`} />}
      </div>

      {/* 추이 + 세션 상태 도넛 */}
      <div className="grid cols-2 mb">
        <div className="card">
          <h3><TrendingUp size={16} /> {snaps.length ? t('dash.snapshotTrend') : t('dash.gpuTrend')}</h3>
          <UtilLineChart labels={trendLabels} values={utilVals} label={t('dash.gpuUtil')} extra={vramVals.length ? { label: t('dash.vramAlloc'), values: vramVals } : undefined} />
        </div>
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
            {/* 오른쪽 여백에 현재 사용중인 사용자(세션) 리스트 */}
            <div style={{ flex: 1, minWidth: 200 }}>
              <div className="legend" style={{ marginTop: 0, marginBottom: 8, paddingBottom: 7, fontWeight: 700, borderBottom: '2px solid var(--border)' }}>{t('dash.activeUsers')}</div>
              {users.length === 0 ? <div className="muted" style={{ fontSize: 12.5 }}>{t('dash.noActiveUsers')}</div>
                : users.slice(0, 8).map((u, i) => (
                  <div key={i} className="flex" style={{ justifyContent: 'space-between', alignItems: 'center', gap: 8, padding: '7px 0', borderTop: i ? '1px solid var(--border)' : 'none', fontSize: 12.5 }}>
                    <span style={{ fontWeight: 600, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{u.name}</span>
                    <span className="flex" style={{ gap: 8, alignItems: 'center', flex: '0 0 auto' }}>
                      {u.gpuType && <span className="muted mono" style={{ fontSize: 11 }}>{u.gpuType.replace(/^NVIDIA-/, '')}</span>}
                      <span className="badge" style={{ fontWeight: 700 }}>{u.sessions}</span>
                    </span>
                  </div>
                ))}
            </div>
          </div>
        </div>
      </div>

      {/* GPU 타입별 세션 + 통합 경고 피드 */}
      <div className="grid cols-2">
        <div className="card">
          <h3><Boxes size={16} /> {t('dash.byGpuType')}</h3>
          {byType.length ? <BarChart data={byType} /> : <div className="muted" style={{ padding: '18px 0', textAlign: 'center' }}>{t('common.empty')}</div>}
        </div>
        <div className="card">
          <h3><BellRing size={16} /> {t('dash.alertFeed')}</h3>
          <AlertFeed live={d.alerts} events={d.alertFeed} emptyLabel={t('dash.noAlerts')} />
        </div>
      </div>
    </div>
  );
}
