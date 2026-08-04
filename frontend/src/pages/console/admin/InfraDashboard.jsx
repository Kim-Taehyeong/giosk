import React, { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { TrendingUp, BellRing, Cpu, MemoryStick, Server, HeartPulse, Thermometer, Layers, Boxes, Users } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import StatCard from '../../../components/console/StatCard';
import UtilLineChart from '../../../components/console/UtilLineChart';
import BarChart from '../../../components/console/BarChart';
import DoughnutChart from '../../../components/console/DoughnutChart';
import AlertFeed from '../../../components/console/AlertFeed';
import ActiveUserList from '../../../components/console/ActiveUserList';
import MetricsOffNotice from '../../../components/console/MetricsOffNotice';
import MonitorControls from '../../../components/console/MonitorControls';
import usePoll from '../../../hooks/usePoll';
import { getInfraDashboard } from '../../../api/console/dashboard';

const hhmm = (ts) => { const d = new Date(ts); return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`; };

// 인프라 대시보드(클러스터 하드웨어)는 최고관리자 전용이다. 감시월에서 폴링 주기 선택과 전체화면을 쓴다.
// downsample은 시계열을 최대 target 포인트로 버킷 평균한다(촘촘한 원시 샘플을 부드러운 선으로).
const avg = (xs) => (xs.length ? Math.round(xs.reduce((a, b) => a + (b || 0), 0) / xs.length) : 0);
function downsample(arr, target) {
  if (arr.length <= target) return arr;
  const bucket = Math.ceil(arr.length / target);
  const out = [];
  for (let i = 0; i < arr.length; i += bucket) {
    const chunk = arr.slice(i, i + bucket);
    out.push({ ts: chunk[chunk.length - 1].ts, gpuUtil: avg(chunk.map((c) => c.gpuUtil)), vramUsedPct: avg(chunk.map((c) => c.vramUsedPct)) });
  }
  return out;
}

export default function InfraDashboard() {
  const { t } = useTranslation('consoleAdmin');
  const [intervalMs, setIntervalMs] = useState(15000);
  const [rangeH, setRangeH] = useState(24); // 추이 시간 범위(시간)
  const wrap = useRef(null);
  const d = usePoll(getInfraDashboard, intervalMs);
  if (!d) return <div className="muted">{t('common.loading')}</div>;
  const k = d.kpis;
  const ss = d.sessionStats || { running: 0, idle: 0, provisioning: 0, stopped: 0, byGpuType: {} };
  const active = Math.max(0, ss.running - ss.idle);
  const users = d.activeUsers || [];

  const allSnaps = d.snapshots || [];
  // 선택 범위로 자르고(최근 rangeH 시간) 최대 60포인트로 버킷 평균해 촘촘함을 완화한다.
  const nowMs = Date.now();
  const snaps = downsample(allSnaps.filter((s) => nowMs - new Date(s.ts).getTime() <= rangeH * 3600e3), 60);
  const trendLabels = snaps.length ? snaps.map((s) => hhmm(s.ts)) : (d.gpuTrend7d || []).map((x) => x.date);
  const utilVals = snaps.length ? snaps.map((s) => s.gpuUtil) : (d.gpuTrend7d || []).map((x) => x.util);
  const vramVals = snaps.length ? snaps.map((s) => s.vramUsedPct) : [];
  const RANGES = [6, 12, 24];
  const byType = Object.entries(ss.byGpuType || {}).map(([label, value]) => ({ label: label.replace(/^NVIDIA-/, ''), value }));
  // 지표 스택이 없으면 GPU 사용률, VRAM, 온도는 0 이 아니라 빈 값으로 두고 안내를 띄운다
  // (0% 를 그대로 보여주면 GPU 가 놀고 있는 것으로 오해된다).
  const mon = d.metrics || { prometheus: false, dcgm: false };
  const gpuMetricsOn = mon.dcgm;
  const pct = (v) => (gpuMetricsOn ? `${v}%` : '—');

  return (
    <div ref={wrap} className="mon-wrap" style={{ background: 'var(--bg)' }}>
      <PageHead crumb={t('badge')} title={t('dash.infraTitle')} subtitle={t('dash.infraSubtitle')}
        actions={<MonitorControls intervalMs={intervalMs} setIntervalMs={setIntervalMs} containerRef={wrap} />} />

      {/* 지표 스택 미설치 안내 — 값이 0 인 게 아니라 수집기가 없다는 걸 명시(노드 화면과 같은 문구) */}
      {!gpuMetricsOn && <MetricsOffNotice dcgmOnly={mon.prometheus} />}

      {/* 인프라 KPI (GPU 온도는 값이 있을 때만) */}
      <div className="grid cols-4 mb">
        <StatCard icon={Cpu} tone="gpu" label={t('dash.gpuUtil')} value={pct(k.gpuUtil)} bar={gpuMetricsOn ? { value: k.gpuUtil, max: 100, variant: 'gpu' } : undefined} />
        <StatCard icon={MemoryStick} tone="warn" label={t('dash.vramAlloc')} value={pct(k.vramAlloc)} bar={gpuMetricsOn ? { value: k.vramAlloc, max: 100, variant: 'warn' } : undefined} />
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
          <div className="flex" style={{ justifyContent: 'space-between', alignItems: 'center', marginBottom: 4 }}>
            <h3 style={{ margin: 0 }}><TrendingUp size={16} /> {snaps.length ? t('dash.snapshotTrend') : t('dash.gpuTrend')}</h3>
            {allSnaps.length > 0 && (
              <span className="flex" style={{ gap: 2, background: 'var(--surface-2)', borderRadius: 8, padding: 3 }}>
                {RANGES.map((h) => (
                  <button key={h} className="btn sm" onClick={() => setRangeH(h)}
                    style={{ background: rangeH === h ? 'var(--primary)' : 'transparent', color: rangeH === h ? '#fff' : 'var(--text)', border: 0, padding: '3px 10px' }}>
                    {h}h</button>
                ))}
              </span>
            )}
          </div>
          {gpuMetricsOn || utilVals.some((v) => v > 0)
            ? <UtilLineChart labels={trendLabels} values={utilVals} label={t('dash.gpuUtil')} extra={vramVals.length ? { label: t('dash.vramAlloc'), values: vramVals } : undefined} />
            : <div className="muted" style={{ padding: '48px 0', textAlign: 'center' }}>{t('dash.metricsOffTitle')}</div>}
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
            {/* 오른쪽 여백에 현재 사용중인 사용자(세션) 리스트 — 운영 대시보드와 동일 컴포넌트 */}
            <ActiveUserList users={users} title={t('dash.activeUsers')} emptyLabel={t('dash.noActiveUsers')} />
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
