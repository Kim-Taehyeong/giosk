import React, { useEffect, useMemo, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { ArrowLeft, Server, Cpu, MemoryStick, Activity, Zap, Clock, Thermometer, Gauge, Layers, Scissors, HardDrive, Database } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import StatCard from '../../../components/console/StatCard';
import Pill from '../../../components/console/Pill';
import Bar from '../../../components/console/Bar';
import Toggle from '../../../components/console/Toggle';
import Spinner from '../../../components/console/Spinner';
import UtilLineChart from '../../../components/console/UtilLineChart';
import { useToast } from '../../../components/console/Toast';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { getAdminNode, getNodeGPUs, getNodeMetrics, saveNodeConfig, cordonNode, uncordonNode } from '../../../api/console/nodes';
import { getDatasets } from '../../../api/console/datasets';
import { getImages } from '../../../api/console/resources';

const WINDOWS = [{ h: 6, k: 'w6h' }, { h: 24, k: 'w24h' }, { h: 168, k: 'w7d' }];

// 시각 라벨 — 24h 이하는 시:분, 그 이상은 월/일 시.
function fmtTime(t, hours) {
  const d = new Date(t * 1000);
  const p = (n) => String(n).padStart(2, '0');
  return hours > 48 ? `${d.getMonth() + 1}/${d.getDate()} ${p(d.getHours())}h` : `${p(d.getHours())}:${p(d.getMinutes())}`;
}

export default function NodeDetailPage() {
  const { name } = useParams();
  const navigate = useNavigate();
  const { t } = useTranslation('consoleAdmin');
  const { toast } = useToast();
  const { config } = useSystemConfig();
  const hybrid = config.deploymentMode === 'hybrid';

  const [node, setNode] = useState(null);
  const [perGpu, setPerGpu] = useState([]);
  const [metrics, setMetrics] = useState(null);
  const [hours, setHours] = useState(24);
  const [cachedDs, setCachedDs] = useState([]);
  const [cachedImg, setCachedImg] = useState([]);

  const reload = () => getAdminNode(name).then(setNode);
  useEffect(() => { reload(); }, [name]);
  useEffect(() => { getNodeGPUs(name).then(setPerGpu).catch(() => {}); }, [name]);
  useEffect(() => { setMetrics(null); getNodeMetrics(name, hours).then(setMetrics).catch(() => {}); }, [name, hours]);
  // 이 노드에 캐시된 데이터셋/이미지(빠른 생성 원천).
  useEffect(() => { getDatasets().then((d) => setCachedDs((d.global || []).filter((x) => (x.nodes || []).includes(name)))).catch(() => {}); }, [name]);
  useEffect(() => { getImages().then((d) => setCachedImg((d.items || []).filter((x) => (x.cachedNodes || []).includes(name)))).catch(() => {}); }, [name]);

  // 설정 저장 — 로컬 반영 + 영속.
  const save = (p) => { setNode((n) => ({ ...n, ...p })); saveNodeConfig(name, p).catch(() => {}); };
  const setCordon = async (on) => {
    setNode((n) => ({ ...n, status: on ? 'cordoned' : 'ready' }));
    try { await (on ? cordonNode(name) : uncordonNode(name)); } catch { /* 낙관 유지 */ }
    toast(on ? t('nodes.cordoned') : t('nodes.uncordoned'));
  };

  const setLease = (leased) => { setNode((n) => ({ ...n, leased, status: leased ? 'cordoned' : 'ready' })); toast(leased ? t('nodes.leased') : t('nodes.released')); };
  const stLabel = (st) => t(`nodes.st_${st}`, { defaultValue: st });

  const ins = metrics?.insights || {};
  const labels = useMemo(() => (metrics?.points || []).map((p) => fmtTime(p.t, hours)), [metrics, hours]);
  const series = (key) => (metrics?.points || []).map((p) => p[key]);
  const hasMetrics = (metrics?.points || []).length > 0;

  if (!node) return <Spinner pad label={t('nodeDetail.loading', { defaultValue: '…' })} />;

  const shareMode = node.shareMode || (node.hami ? 'hami' : 'exclusive');
  const splitCount = node.splitCount ?? 10;
  const gpuCount = parseInt((node.gpu.match(/×\s*(\d+)/) || [])[1] || '0', 10);
  const peakHour = ins.peakHour >= 0 ? `${String(ins.peakHour).padStart(2, '0')}:00` : '—';

  return (
    <div>
      <button className="btn sm" style={{ marginBottom: 12 }} onClick={() => navigate('/console/admin/nodes')}>
        <ArrowLeft size={13} /> {t('nodeDetail.back')}
      </button>
      <PageHead
        icon={Server}
        title={node.node}
        subtitle={[node.gpu, node.cpu, node.mem, node.cudaVersion && `CUDA ${node.cudaVersion}`].filter(Boolean).join(' · ')}
        actions={
          <span className="flex" style={{ gap: 8, alignItems: 'center' }}>
            {node.leased && <Pill variant="primary" dot>{t('nodes.leasedTag')}</Pill>}
            <Pill variant={node.status === 'ready' ? 'ok' : 'cordon'} dot>{stLabel(node.status)}</Pill>
            {hybrid && node.physicalLease && (node.leased
              ? <button className="btn sm warn" onClick={() => setLease(false)}>{t('nodes.release')}</button>
              : <button className="btn sm" onClick={() => setLease(true)}>{t('nodes.lease')}</button>)}
            {!node.leased && (node.status === 'cordoned'
              ? <button className="btn sm ok" onClick={() => setCordon(false)}>{t('nodes.uncordon', { defaultValue: 'uncordon' })}</button>
              : <button className="btn sm warn" onClick={() => setCordon(true)}>{t('nodes.cordon', { defaultValue: 'cordon' })}</button>)}
          </span>
        } />

      {/* 인사이트 요약 — 구간 집계 */}
      <div className="grid cols-4 mb">
        <StatCard icon={Activity} tone="gpu" label={t('nodeDetail.avgUtil')} value={`${ins.avgUtil ?? 0}%`} sub={`peak ${ins.peakUtil ?? 0}%`} bar={{ value: ins.avgUtil ?? 0, max: 100, variant: 'gpu' }} />
        <StatCard icon={Clock} tone="free" label={t('nodeDetail.idleTime')} value={`${ins.idlePct ?? 0}%`} sub={t('nodeDetail.ofWindow', { w: ins.window || '' })} bar={{ value: ins.idlePct ?? 0, max: 100, variant: 'free' }} />
        <StatCard icon={Zap} tone="warn" label={t('nodeDetail.avgPower')} value={`${ins.avgPower ?? 0}`} unit="W" sub={`${ins.energyKwh ?? 0} kWh`} />
        <StatCard icon={Thermometer} tone="warn" label={t('nodeDetail.avgTemp')} value={ins.maxTemp ? `${ins.avgTemp ?? 0}°` : '—'} sub={ins.maxTemp ? `max ${ins.maxTemp}°` : t('nodeDetail.noTemp')} />
      </div>
      <div className="grid cols-4 mb">
        <StatCard icon={Gauge} tone="gpu" label={t('nodeDetail.peakUtil')} value={`${ins.peakUtil ?? 0}%`} />
        <StatCard icon={Clock} tone="primary" label={t('nodeDetail.peakHour')} value={peakHour} sub={t('nodeDetail.peakHourHint')} />
        <StatCard icon={Zap} tone="warn" label={t('nodeDetail.energy')} value={`${ins.energyKwh ?? 0}`} unit="kWh" sub={`/ ${ins.window || ''}`} />
        <StatCard icon={Server} tone="free" label={t('nodeDetail.sessions')} value={`${node.sessions ?? 0}`} sub={t('nodeDetail.running')} />
      </div>

      {/* 이력 차트 + 구간 선택 */}
      <div className="card mb">
        <div className="flex" style={{ justifyContent: 'space-between', alignItems: 'center', marginBottom: 10 }}>
          <h3 style={{ margin: 0 }}><Activity size={16} /> {t('nodeDetail.history')}</h3>
          <div style={{ display: 'inline-flex', gap: 3, padding: 3, background: 'var(--surface-2)', borderRadius: 9 }}>
            {WINDOWS.map((w) => {
              const on = w.h === hours;
              return (
                <button key={w.h} onClick={() => setHours(w.h)}
                  style={{ padding: '6px 13px', borderRadius: 7, border: 'none', cursor: 'pointer', fontSize: 12.5, fontWeight: on ? 700 : 600,
                    background: on ? 'var(--surface)' : 'transparent', color: on ? 'var(--primary)' : 'var(--muted)', boxShadow: on ? '0 1px 2px rgba(0,0,0,.1)' : 'none' }}>
                  {t(`nodeDetail.${w.k}`)}
                </button>
              );
            })}
          </div>
        </div>
        {!metrics ? (
          <Spinner pad label={t('nodeDetail.loadingWindow', { defaultValue: '메트릭 불러오는 중…' })} />
        ) : !hasMetrics ? (
          <div className="legend" style={{ padding: 30, textAlign: 'center' }}>{t('nodeDetail.noMetrics')}</div>
        ) : (
          <div className="grid cols-2" style={{ gap: 20 }}>
            <div>
              <div className="legend" style={{ marginBottom: 6, fontWeight: 700 }}>{t('nodeDetail.chartUtilVram')}</div>
              <UtilLineChart labels={labels} values={series('util')} label={t('nodeDetail.util')}
                extra={{ label: t('nodeDetail.vram'), values: series('vramPct') }} height={200} />
            </div>
            <div>
              <div className="legend" style={{ marginBottom: 6, fontWeight: 700 }}>{t('nodeDetail.chartCpuMem')}</div>
              <UtilLineChart labels={labels} values={series('cpu')} label="CPU"
                extra={{ label: 'RAM', values: series('mem') }} height={200} />
            </div>
            <div>
              <div className="legend" style={{ marginBottom: 6, fontWeight: 700 }}>{t('nodeDetail.chartPower')}</div>
              <UtilLineChart labels={labels} values={series('power')} label="W" percent={false} height={200} />
            </div>
            <div>
              <div className="legend" style={{ marginBottom: 6, fontWeight: 700 }}>{t('nodeDetail.chartTemp')}</div>
              <UtilLineChart labels={labels} values={series('temp')} label="°C" percent={false} height={200} />
            </div>
          </div>
        )}
      </div>

      <div className="grid cols-2" style={{ gap: 18, alignItems: 'start' }}>
        {/* 현재 GPU 장치별 실측 */}
        <div className="card">
          <h3><Cpu size={16} /> {t('nodeDetail.perGpu')}</h3>
          {gpuCount === 0 ? (
            <div className="legend">{t('nodes.noGpuShare')}</div>
          ) : perGpu.length === 0 ? (
            <div className="legend">{t('nodeDetail.noGpuLive')}</div>
          ) : (
            <div>
              <div className="flex" style={{ alignItems: 'center', gap: 8, fontSize: 10.5, fontWeight: 700, color: 'var(--muted)', textTransform: 'uppercase', letterSpacing: '.03em', paddingBottom: 5 }}>
                <span style={{ width: 48 }}>{t('nodes.colDevice')}</span>
                <span style={{ flex: 1 }}>{t('nodes.colUtil')}</span>
                <span style={{ width: 30 }} />
                <span style={{ width: 64, textAlign: 'right' }}>{t('nodes.mVram')}</span>
                <span style={{ width: 34, textAlign: 'right' }}>{t('nodes.mTemp')}</span>
              </div>
              {perGpu.map((g, i) => (
                <div key={i} className="flex" style={{ alignItems: 'center', gap: 8, padding: '6px 0', borderTop: '1px solid var(--border)' }}>
                  <span style={{ width: 48, fontWeight: 700, fontSize: 12.5 }}>GPU#{i}</span>
                  <div style={{ flex: 1 }}><Bar value={g.util} max={100} variant={g.util >= 85 ? 'warn' : 'gpu'} /></div>
                  <span style={{ width: 30, textAlign: 'right', fontSize: 12 }}>{g.util}%</span>
                  <span className="muted" style={{ width: 64, textAlign: 'right', fontSize: 12 }}>{g.vramUsed}/{g.vramTotal}G</span>
                  <span style={{ width: 34, textAlign: 'right', fontSize: 12, color: g.temp >= 80 ? 'var(--warn)' : 'var(--muted)' }}>{g.temp}°</span>
                </div>
              ))}
            </div>
          )}
          <div className="flex" style={{ gap: 16, marginTop: 14, paddingTop: 12, borderTop: '1px solid var(--border)' }}>
            <div style={{ flex: 1 }}>
              <div className="flex" style={{ justifyContent: 'space-between', fontSize: 11.5, fontWeight: 700, color: 'var(--muted)', marginBottom: 3 }}><span><Cpu size={11} /> CPU</span><span>{node.cpuUtil ?? 0}%</span></div>
              <Bar value={node.cpuUtil ?? 0} max={100} variant="free" />
            </div>
            <div style={{ flex: 1 }}>
              <div className="flex" style={{ justifyContent: 'space-between', fontSize: 11.5, fontWeight: 700, color: 'var(--muted)', marginBottom: 3 }}><span><MemoryStick size={11} /> RAM</span><span>{node.memUtil ?? 0}%</span></div>
              <Bar value={node.memUtil ?? 0} max={100} variant="gpu" />
            </div>
          </div>
        </div>

        {/* 설정 — 공유 모드 / 스크래치 / 물리임대 */}
        <div className="card">
          <h3><Layers size={16} /> {t('nodeDetail.settings')}</h3>
          {gpuCount > 0 ? (
            <>
              <div className="flex" style={{ gap: 6, alignItems: 'center', fontWeight: 700, fontSize: 13.5 }}><Layers size={14} style={{ color: 'var(--primary)' }} /> {t('nodes.shareMode')}</div>
              <div className="legend" style={{ marginTop: 2, marginBottom: 8 }}>{t('nodes.shareModeHint')}</div>
              <div style={{ display: 'inline-flex', gap: 3, padding: 3, background: 'var(--surface-2)', borderRadius: 9 }}>
                {[{ k: 'exclusive', label: t('nodes.modeExclusive') }, { k: 'hami', label: t('nodes.modeHami') }].map((m) => {
                  const on = shareMode === m.k;
                  return (
                    <button key={m.k} onClick={() => save({ shareMode: m.k, hami: m.k === 'hami' })}
                      style={{ padding: '7px 14px', borderRadius: 7, border: 'none', cursor: 'pointer', fontSize: 12.5, fontWeight: on ? 700 : 600,
                        background: on ? 'var(--surface)' : 'transparent', color: on ? 'var(--primary)' : 'var(--muted)', boxShadow: on ? '0 1px 2px rgba(0,0,0,.1)' : 'none' }}>
                      {m.label}
                    </button>
                  );
                })}
              </div>
              {shareMode === 'hami' && (
                <div style={{ marginTop: 14 }}>
                  <label className="fld" style={{ marginTop: 0, marginBottom: 5 }}>
                    <span className="flex" style={{ gap: 5, alignItems: 'center' }}><Scissors size={12} /> {t('nodes.splitCount')}</span>
                  </label>
                  <span className="flex" style={{ gap: 6, alignItems: 'center' }}>
                    <input type="number" min={1} max={10} value={splitCount} style={{ width: 90 }}
                      onChange={(e) => save({ splitCount: Math.max(1, Math.min(10, Number(e.target.value))) })} />
                    <span className="muted" style={{ fontSize: 13, fontWeight: 600 }}>{t('nodes.splitCountUnit')}</span>
                  </span>
                  <div className="legend" style={{ marginTop: 4 }}>{t('nodes.sliceNote')}</div>
                </div>
              )}
            </>
          ) : (
            <div className="legend">{t('nodes.noGpuShare')}</div>
          )}

          <div style={{ marginTop: 16, borderTop: '1px solid var(--border)', paddingTop: 6 }}>
            {hybrid && (
              <div className="flex" style={{ justifyContent: 'space-between', alignItems: 'flex-start', gap: 14, padding: '10px 0' }}>
                <div><div style={{ fontWeight: 700, fontSize: 13.5 }} className="flex"><Server size={13} /> {t('nodes.physicalLease')}</div><div className="legend" style={{ marginTop: 2 }}>{t('nodes.physicalLeaseHint')}</div></div>
                <Toggle checked={node.physicalLease} onChange={(v) => save({ physicalLease: v })} />
              </div>
            )}
            <div className="flex" style={{ justifyContent: 'space-between', alignItems: 'flex-start', gap: 14, padding: '10px 0' }}>
              <div><div style={{ fontWeight: 700, fontSize: 13.5 }} className="flex"><HardDrive size={13} /> {t('nodes.scratch')}</div><div className="legend" style={{ marginTop: 2 }}>{t('nodes.scratchHint')}</div></div>
              <Toggle checked={node.scratchEnabled} onChange={(v) => { save({ scratchEnabled: v }); toast(v ? t('nodes.scratchOn') : t('nodes.scratchOff')); }} />
            </div>
          </div>
        </div>
      </div>

      {/* 이 노드에 캐시된 데이터셋 / 이미지 — 빠른 세션 생성 원천 */}
      <div className="grid cols-2" style={{ gap: 18, alignItems: 'start', marginTop: 18 }}>
        <div className="card">
          <h3><Database size={16} /> {t('nodeDetail.cachedDs')} <span className="muted" style={{ fontSize: 12.5, fontWeight: 600 }}>({cachedDs.length})</span></h3>
          {cachedDs.length === 0 ? <div className="legend">{t('nodeDetail.noCachedDs')}</div> : cachedDs.map((d, i) => (
            <div key={d.id} className="flex" style={{ justifyContent: 'space-between', alignItems: 'center', gap: 10, padding: '8px 0', borderTop: i ? '1px solid var(--border)' : 'none' }}>
              <span className="flex" style={{ gap: 8, alignItems: 'center', minWidth: 0 }}>
                {d.sizeClass && <Pill variant={sizeVariant[d.sizeClass] || 'pause'}>{d.sizeClass}</Pill>}
                <span style={{ fontWeight: 600, fontSize: 13.5 }}>{d.name}</span>
              </span>
              <span className="muted" style={{ fontSize: 12, whiteSpace: 'nowrap' }}>{d.sizeGb ? `${d.sizeGb} GB` : ''}</span>
            </div>
          ))}
        </div>
        <div className="card">
          <h3><Layers size={16} /> {t('nodeDetail.cachedImg')} <span className="muted" style={{ fontSize: 12.5, fontWeight: 600 }}>({cachedImg.length})</span></h3>
          {cachedImg.length === 0 ? <div className="legend">{t('nodeDetail.noCachedImg')}</div> : cachedImg.map((im, i) => (
            <div key={im.id} className="flex" style={{ justifyContent: 'space-between', alignItems: 'center', gap: 10, padding: '8px 0', borderTop: i ? '1px solid var(--border)' : 'none' }}>
              <span style={{ fontWeight: 600, fontSize: 13.5 }} className="mono">{im.name}</span>
              {im.cudaVersion && <span className="muted" style={{ fontSize: 12, whiteSpace: 'nowrap' }}>CUDA {im.cudaVersion}</span>}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

const sizeVariant = { Large: 'primary', Medium: 'gpu', Small: 'free' };
