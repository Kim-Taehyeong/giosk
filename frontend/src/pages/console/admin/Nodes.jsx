import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { Server, Cpu, MemoryStick, HeartPulse, ChevronRight, Info } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import StatCard from '../../../components/console/StatCard';
import Pill from '../../../components/console/Pill';
import Bar from '../../../components/console/Bar';
import { useToast } from '../../../components/console/Toast';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { getDatasets } from '../../../api/console/datasets';
import { getAdminNodes, cordonNode, uncordonNode, getAdminStorage } from '../../../api/console/nodes';
import { getAdminDashboard } from '../../../api/console/dashboard';

// 안전한 퍼센트: max<=0(=지표 없음) 이면 0 을 반환해 NaN%(브라우저가 꽉 채움) 버그를 막는다.
const barPct = (v, m) => (m > 0 ? Math.min(100, Math.max(0, (v / m) * 100)) : 0);

function MiniMon({ n }) {
  const { t } = useTranslation('consoleAdmin');
  const isGpu = n.total !== undefined;
  // GPU 노드인데 VRAM 총량이 없음 = DCGM/Prometheus 미설치로 지표를 못 받는 상태.
  const noMetrics = isGpu && !(n.total > 0);
  if (noMetrics) {
    return <span className="muted" style={{ fontSize: 11.5, fontWeight: 600 }}>{t('nodes.noMetrics', { defaultValue: '지표 없음' })}</span>;
  }
  const rows = isGpu
    ? [{ label: 'GPU', value: n.util, max: 100, txt: `${n.util}%`, variant: 'gpu' },
       { label: 'VRAM', value: n.used, max: n.total, txt: `${n.used}/${n.total}G`, variant: 'warn' }]
    : [{ label: 'CPU', value: n.cpuUtil, max: 100, txt: `${n.cpuUtil}%`, variant: 'free' },
       { label: 'MEM', value: n.memUtil, max: 100, txt: `${n.memUtil}%`, variant: 'gpu' }];
  return (
    <div style={{ minWidth: 130 }}>
      {rows.map((r) => (
        <div key={r.label} style={{ marginBottom: 4 }}>
          <div className="flex" style={{ justifyContent: 'space-between', fontSize: 10.5, color: 'var(--muted)', fontWeight: 600, marginBottom: 1 }}>
            <span>{r.label}</span><span>{r.txt}</span>
          </div>
          <div style={{ height: 6, borderRadius: 4, background: 'var(--surface-2)', overflow: 'hidden' }}>
            <div style={{ height: '100%', width: `${barPct(r.value, r.max)}%`, background: `var(--${r.variant})`, borderRadius: 4 }} />
          </div>
        </div>
      ))}
    </div>
  );
}

export default function Nodes() {
  const { t } = useTranslation('consoleAdmin');
  const { toast } = useToast();
  const navigate = useNavigate();
  const { config } = useSystemConfig();
  const hybrid = config.deploymentMode === 'hybrid';
  const datasetsOn = config.features.datasets;

  const [nodes, setNodes] = useState([]);
  const [datasets, setDatasets] = useState([]);
  const [kpis, setKpis] = useState(null);
  const [storage, setStorage] = useState(null);

  // GPU 노드가 있는데 전부 VRAM 총량이 없으면 = DCGM/Prometheus 미설치로 지표 미가용.
  const gpuNodes = nodes.filter((r) => r.total !== undefined);
  const metricsOff = gpuNodes.length > 0 && gpuNodes.every((r) => !(r.total > 0));

  // 데이터셋 기능이 꺼진 배포에선 라우트 자체가 없다(404) → 켜져 있을 때만 조회.
  useEffect(() => {
    if (!datasetsOn) return;
    getDatasets().then((d) => setDatasets(d.global || [])).catch(() => {});
  }, [datasetsOn]);
  useEffect(() => { getAdminNodes().then(setNodes); }, []);
  useEffect(() => { getAdminDashboard().then((d) => setKpis(d.kpis)); }, []);
  useEffect(() => { getAdminStorage().then(setStorage).catch(() => {}); }, []);
  const cachedFor = (node) => datasets.filter((d) => d.nodes.includes(node));

  // 인라인 편집은 노드 state 에 즉시 반영.
  const patch = (node, p) => setNodes((ns) => ns.map((n) => (n.node === node ? { ...n, ...p } : n)));
  const setLease = (node, leased) => { patch(node, { leased, status: leased ? 'cordoned' : 'ready' }); toast(leased ? t('nodes.leased') : t('nodes.released')); };
  const setCordon = async (node, cordon) => {
    patch(node, { status: cordon ? 'cordoned' : 'ready' });
    try { await (cordon ? cordonNode(node) : uncordonNode(node)); } catch { /* 낙관적 UI 유지 */ }
    toast(cordon ? t('nodes.cordoned') : t('nodes.uncordoned'));
  };


  // 노드별 디스크(스토리지 현황) — 노드 표에 합쳐 한 번에 본다.
  const diskOf = (node) => (storage?.nodes || []).find((n) => n.node === node);
  const diskCell = (r) => {
    const d = diskOf(r.node);
    if (!d) return <span className="muted" style={{ fontSize: 12 }}>—</span>;
    const over = d.totalGb && d.usedGb / d.totalGb > 0.85;
    return (
      <div style={{ minWidth: 110 }}>
        <div className="flex" style={{ justifyContent: 'space-between', fontSize: 10.5, color: 'var(--muted)', fontWeight: 600, marginBottom: 2 }}>
          <span>DISK</span><span style={{ color: over ? 'var(--warn)' : 'inherit' }}>{d.usedGb}/{d.totalGb}G</span>
        </div>
        <Bar value={d.usedGb} max={Math.max(d.totalGb, 1)} variant={over ? 'warn' : 'gpu'} />
      </div>
    );
  };

  // 공유 모드 — 자세히 보기를 열지 않아도 목록에서 바로 보이게 한다.
  const shareCell = (r) => {
    const mode = r.shareMode || (r.hami ? 'hami' : 'exclusive');
    const label = { exclusive: t('nodes.modeExclusive'), hami: t('nodes.modeHami'), timeslicing: t('nodes.modeTimeslicing') }[mode];
    const n = r.splitCount ?? 10;
    return (
      <span className="flex" style={{ gap: 4, flexWrap: 'wrap' }}>
        <Pill variant={mode === 'exclusive' ? 'pause' : 'gpu'}>{label}</Pill>
        {mode !== 'exclusive' && <span className="muted" style={{ fontSize: 11.5, fontWeight: 600 }}>×{n}</span>}
        {r.scratchEnabled && <Pill variant="free">{t('nodes.scratchTag')}</Pill>}
      </span>
    );
  };

  const statusCell = (r) => (
    <span className="flex" style={{ gap: 4, flexWrap: 'wrap' }}>
      {r.leased && <Pill variant="primary" dot>{t('nodes.leasedTag')}</Pill>}
      <Pill variant={r.status === 'ready' ? 'ok' : 'cordon'} dot>{r.status}</Pill>
    </span>
  );

  return (
    <div>
      <PageHead title={t('nodes.title')} subtitle={t('nodes.subtitle')} />

      {metricsOff && (
        <div className="card mb" style={{ display: 'flex', alignItems: 'center', gap: 10, borderLeft: '3px solid var(--warn)' }}>
          <Info size={18} style={{ color: 'var(--warn)', flex: '0 0 auto' }} />
          <span style={{ fontSize: 13, color: 'var(--muted)' }}>
            {t('nodes.metricsOffNotice', { defaultValue: '모니터링(DCGM·Prometheus)이 설치되지 않아 GPU 사용률·VRAM 지표를 표시할 수 없습니다. 모니터링을 활성화하면 실시간 지표가 나타납니다.' })}
          </span>
        </div>
      )}

      <div className="grid cols-4 mb">
        <StatCard icon={Cpu} tone="gpu" label={t('nodes.gpuUtil')} value={`${kpis?.gpuUtil ?? 0}%`} bar={{ value: kpis?.gpuUtil ?? 0, max: 100, variant: 'gpu' }} />
        <StatCard icon={MemoryStick} tone="warn" label={t('nodes.vramAlloc')} value={`${kpis?.vramAlloc ?? 0}%`} bar={{ value: kpis?.vramAlloc ?? 0, max: 100, variant: 'warn' }} />
        <StatCard icon={Server} tone="free" label={t('nodes.nodesUp')} value={`${kpis?.nodesUp ?? 0}`} unit={`/ ${kpis?.nodesTotal ?? 0}`} bar={{ value: kpis?.nodesUp ?? 0, max: kpis?.nodesTotal || 1, variant: 'free' }} />
        <StatCard icon={HeartPulse} tone="warn" label={t('nodes.healthAlerts')} value={`${kpis?.healthAlerts ?? 0}`} unit="(xid auto-cordon)" />
      </div>

      {/* NFS/사용자별 스토리지 현황과 볼륨 임포트는 "볼륨" 탭으로 이관(노드 표엔 디스크 열만 유지) */}

      <div className="card">
        <h3><Server size={16} /> {t('nodes.nodesH')}</h3>
        <table>
          <thead>
            <tr>
              <th>{t('nodes.node')}</th><th>{t('nodes.gpu')}</th><th>{t('nodes.share')}</th><th>{t('nodes.mon')}</th><th>{t('nodes.disk')}</th>
              {datasetsOn && <th>{t('nodes.dsCache')}</th>}<th>{t('nodes.status')}</th><th>{t('nodes.action')}</th>
            </tr>
          </thead>
          <tbody>
            {nodes.map((r) => (
              <React.Fragment key={r.node}>
                <tr className="row-link" style={{ cursor: 'pointer' }}
                  onClick={(e) => { if (e.target.closest('button, a, input, select, textarea')) return; navigate(`/console/admin/nodes/${r.node}`); }}>
                  <td style={{ fontWeight: 600 }}>{r.node}</td>
                  <td>{r.gpu}
                    {r.cudaVersion && <div className="muted" style={{ fontSize: 11 }}>CUDA {r.cudaVersion}</div>}
                  </td>
                  <td>{shareCell(r)}</td>
                  <td><MiniMon n={r} /></td>
                  <td>{diskCell(r)}</td>
                  {datasetsOn && <td><span style={{ fontWeight: 600 }}>{t('nodes.cachedN', { n: cachedFor(r.node).length })}</span></td>}
                  <td>{statusCell(r)}</td>
                  <td>
                    <button className="btn sm" title={t('nodes.detail')} aria-label={t('nodes.detail')}
                      onClick={() => navigate(`/console/admin/nodes/${r.node}`)}>
                      <ChevronRight size={15} />
                    </button>
                  </td>
                </tr>
              </React.Fragment>
            ))}
          </tbody>
        </table>
      </div>

    </div>
  );
}
