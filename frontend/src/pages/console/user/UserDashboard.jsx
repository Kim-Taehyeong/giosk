import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { KeyRound, Rocket, Globe, Boxes, X, Zap, HardDriveDownload, FolderPlus, Coins, Layers, Clock, Hourglass, Server, Timer, Megaphone, AlertTriangle, Info, Code2, NotebookPen, TerminalSquare } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import StatCard from '../../../components/console/StatCard';
import Bar from '../../../components/console/Bar';
import Pill from '../../../components/console/Pill';
import DataTable from '../../../components/console/DataTable';
import ConnectionModal from '../../../components/console/ConnectionModal';
import { getUserDashboard } from '../../../api/console/dashboard';
import { getAvailability } from '../../../api/console/resources';
import { getAnnouncements } from '../../../api/console/announcements';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { useAuth } from '../../../context/AuthContext';

// 공지 레벨별 스타일. critical 은 사용자가 닫을 수 없음(고정 노출).
const NOTICE_STYLE = {
  info: { bg: 'var(--primary-soft)', fg: 'var(--primary)', Icon: Info },
  warning: { bg: 'var(--warn-soft)', fg: 'var(--warn)', Icon: AlertTriangle },
  critical: { bg: 'var(--danger-soft)', fg: 'var(--danger)', Icon: Megaphone },
};
const DISMISS_KEY = 'giosk_dismissed_notices';

// 세션 접속 채널 → 버튼 아이콘·라벨. 세션의 conn 배열(VSCode/Jupyter/SSH)마다 버튼을 만든다.
const CONN_CH = {
  vscode: { icon: Code2, label: 'VSCode' },
  jupyter: { icon: NotebookPen, label: 'Jupyter' },
  terminal: { icon: TerminalSquare, label: 'SSH' }, // 웹터미널(컨테이너) — SSH 로 표기
  ssh: { icon: TerminalSquare, label: 'SSH' },
};

export default function UserDashboard() {
  const { t } = useTranslation('consoleUser');
  const { config } = useSystemConfig();
  const { user } = useAuth();
  const hybrid = config.deploymentMode === 'hybrid';
  const creditMode = config.billing.mode === 'credit';
  const freeMode = config.billing.mode === 'free';
  const dyn = config.billing.dynamic;
  const creditLimit = config.billing.credit.maxConcurrentSessions;
  // free 모드라도 전역 정책 쿼터(최대 동시 세션)는 유효하다 → ∞ 대신 이 상한을 보여준다.
  const quotaSessions = config.quota?.maxConcurrentSessions;
  const [d, setD] = useState(null);
  const [connSession, setConnSession] = useState(null); // 접속 모달 대상 세션
  const [connTab, setConnTab] = useState(null); // 클릭한 채널(모달 초기 탭)
  const openConn = (r, tab) => { setConnTab(tab); setConnSession(r); };
  const [avail, setAvail] = useState({ byType: [], byNode: [] }); // 실시간 GPU 가용(단일 소스)
  const [notices, setNotices] = useState([]);
  const [dismissed, setDismissed] = useState(() => { try { return JSON.parse(localStorage.getItem(DISMISS_KEY)) || []; } catch { return []; } });
  const [bannerClosed, setBannerClosed] = useState(() => localStorage.getItem('giosk_welcome_closed') === '1');
  const navigate = useNavigate();
  useEffect(() => { getUserDashboard().then(setD); }, []);
  useEffect(() => { getAnnouncements().then((r) => setNotices(r.items || [])); }, []);
  useEffect(() => { getAvailability().then(setAvail); }, []);
  const dismissNotice = (id) => { const next = [...dismissed, id]; setDismissed(next); localStorage.setItem(DISMISS_KEY, JSON.stringify(next)); };
  const visibleNotices = notices.filter((n) => n.active && (n.level === 'critical' || !dismissed.includes(n.id)));
  if (!d) return <div className="muted">{t('common.loading')}</div>;
  const k = d.kpis;
  const hasSsh = !!user?.sshPublicKey; // 계정에 등록된 공개키(서버 권위 — 예전 localStorage 판단은 기기마다 달랐다)
  const closeBanner = () => { localStorage.setItem('giosk_welcome_closed', '1'); setBannerClosed(true); };

  const quick = [
    { icon: Zap, color: 'var(--gpu-soft)', fg: 'var(--gpu)', title: t('dash.qsGpu'), desc: t('dash.qsGpuDesc'), to: '/console/sessions/new?preset=standard' },
    { icon: HardDriveDownload, color: 'var(--free-soft)', fg: 'var(--free)', title: t('dash.qsCpu'), desc: t('dash.qsCpuDesc'), to: '/console/sessions/new?preset=cpu' },
    { icon: FolderPlus, color: 'var(--primary-soft)', fg: 'var(--primary)', title: t('dash.qsVol'), desc: t('dash.qsVolDesc'), to: '/console/volumes' },
  ];

  return (
    <div>
      <PageHead crumb={`${config.branding?.name?.trim() || 'Giosk'} Console`} title={t('dash.title')} subtitle={t('dash.subtitle')} />

      {visibleNotices.length > 0 && (
        <div className="grid mb" style={{ gap: 10 }}>
          {[...visibleNotices]
            .sort((a, b) => (b.pinned - a.pinned) || (({ critical: 3, warning: 2, info: 1 }[b.level] || 0) - ({ critical: 3, warning: 2, info: 1 }[a.level] || 0)))
            .map((n) => {
              const s = NOTICE_STYLE[n.level] || NOTICE_STYLE.info;
              const Icon = s.Icon;
              const closable = n.level !== 'critical';
              return (
                <div key={n.id} style={{ display: 'flex', gap: 12, alignItems: 'flex-start', padding: '14px 16px', borderRadius: 12, background: s.bg, border: `1px solid ${s.fg}33` }}>
                  <span style={{ color: s.fg, flex: '0 0 auto', marginTop: 1 }}><Icon size={19} /></span>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div className="flex" style={{ gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
                      <span style={{ fontWeight: 800, fontSize: 14.5, color: s.fg }}>{n.title}</span>
                      {n.pinned && <span style={{ fontSize: 11, fontWeight: 700, color: s.fg, opacity: .7 }}>· {t('dash.noticePinned')}</span>}
                      <span className="muted" style={{ fontSize: 12 }}>{n.createdAt ? new Date(n.createdAt).toLocaleString() : ''}</span>
                    </div>
                    {n.body && <div style={{ fontSize: 13, marginTop: 3, lineHeight: 1.55 }}>{n.body}</div>}
                  </div>
                  {closable && (
                    <button onClick={() => dismissNotice(n.id)} aria-label="dismiss"
                      style={{ flex: '0 0 auto', width: 26, height: 26, borderRadius: 7, border: 'none', background: 'transparent', color: s.fg, cursor: 'pointer', display: 'grid', placeItems: 'center' }}>
                      <X size={15} />
                    </button>
                  )}
                </div>
              );
            })}
        </div>
      )}

      {!hasSsh && !bannerClosed && (
        <div className="mb" style={{
          position: 'relative', display: 'flex', gap: 24, alignItems: 'center', padding: '28px 32px', borderRadius: 16,
          background: 'linear-gradient(120deg, #4f46e5 0%, #6366f1 55%, #7c8aff 100%)',
          color: '#fff', boxShadow: '0 10px 30px rgba(79,70,229,.28)',
        }}>
          <button onClick={closeBanner} aria-label="close" style={{
            position: 'absolute', top: 14, right: 14, width: 30, height: 30, borderRadius: 8, border: 'none',
            background: 'rgba(255,255,255,.18)', color: '#fff', cursor: 'pointer', display: 'grid', placeItems: 'center',
          }}><X size={16} /></button>
          <div style={{ width: 72, height: 72, borderRadius: 18, flex: '0 0 auto', display: 'grid', placeItems: 'center', background: 'rgba(255,255,255,.18)' }}>
            <Rocket size={34} />
          </div>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ fontSize: 25, fontWeight: 800, letterSpacing: '-.5px' }}>{t('dash.welcomeTitle', { brand: `${config.branding?.name?.trim() || 'Giosk'} Console` })} 👋</div>
            <p style={{ fontSize: 14.5, opacity: 0.92, margin: '6px 0 16px' }}>{t('dash.welcomeSub')}</p>
            <div className="flex gap wrap">
              <button className="btn" style={{ background: '#fff', color: 'var(--primary)', border: 'none', fontWeight: 800 }}
                onClick={() => navigate('/console/sessions/new')}><Rocket size={15} /> {t('dash.firstSession')}</button>
              {!hasSsh && <button className="btn" style={{ background: 'rgba(255,255,255,.15)', color: '#fff', borderColor: 'rgba(255,255,255,.35)' }}
                onClick={() => navigate('/console/account')}><KeyRound size={15} /> {t('dash.registerSsh')}</button>}
            </div>
          </div>
        </div>
      )}

      <div className="mb" style={{ padding: 20, borderRadius: 16, background: 'var(--surface-2)', border: '1px solid var(--border)' }}>
        <div className="flex" style={{ alignItems: 'center', gap: 8, marginBottom: 14 }}>
          <span style={{ width: 34, height: 34, borderRadius: 10, flex: '0 0 auto', display: 'grid', placeItems: 'center', background: 'var(--gpu-soft)', color: 'var(--gpu)' }}>
            <Rocket size={19} />
          </span>
          <span style={{ fontSize: 19, fontWeight: 800, letterSpacing: '-.4px' }}>{t('dash.quickStart')}</span>
        </div>
        <div className="grid cols-3">
          {quick.map((c, i) => {
            const Icon = c.icon;
            return (
              <div className="task-card" key={i} onClick={() => navigate(c.to)}>
                {c.badge && <div className="ribbon">{c.badge}</div>}
                <div className="ico" style={{ background: c.color, color: c.fg }}><Icon size={22} /></div>
                <h4>{c.title}</h4>
                <p>{c.desc}</p>
              </div>
            );
          })}
        </div>
      </div>

      <div className="grid cols-4 mb">
        {freeMode ? (
          <>
            <StatCard icon={Layers} tone="free" label={t('dash.kpiConcurrent')} value={`${k.activeSessions}`}
              unit={quotaSessions ? `/ ${quotaSessions}` : '/ ∞'}
              bar={quotaSessions ? { value: k.activeSessions, max: quotaSessions, variant: 'free' } : undefined} />
            <StatCard icon={Clock} tone="gpu" label={t('dash.kpiGpuHours')} value={`${k.gpuHoursMonth}`} unit={t('dash.hoursUnit')} />
            <StatCard icon={Zap} tone="free" label={t('dash.kpiBilling')} value={t('dash.kpiFree')} />
            <StatCard icon={Server} tone="gpu" label={t('dash.kpiNodes')} value={`${avail.byNode.length}`} />
          </>
        ) : creditMode ? (
          <>
            <StatCard icon={Coins} tone="gpu" label={t('dash.kpiBalance')} value={`${k.balance}`} unit={`/ ${k.cap} C`} bar={{ value: k.balance, max: k.cap, variant: 'gpu' }} />
            <StatCard icon={Layers} tone="free" label={t('dash.kpiSessions')} value={`${k.activeSessions}`} unit={`/ ${creditLimit}`} bar={{ value: k.activeSessions, max: creditLimit, variant: 'free' }} />
            <StatCard icon={Clock} tone="gpu" label={t('dash.kpiGpuHours')} value={`${k.gpuHoursMonth}`} unit={t('dash.hoursUnit')} />
            <StatCard icon={Hourglass} tone="warn" label={t('dash.kpiEta')} value={t('dash.kpiEtaVal', { n: k.etaDays })} unit={`(${k.burn} C/day)`} />
          </>
        ) : (
          <>
            <StatCard icon={Layers} tone="free" label={t('dash.kpiConcurrent')} value={`${k.activeSessions}`} unit={`/ ${dyn.maxConcurrentSessions}`} bar={{ value: k.activeSessions, max: dyn.maxConcurrentSessions, variant: 'free' }} />
            <StatCard icon={Hourglass} tone="gpu" label={t('dash.kpiMaxLease')} value={`${dyn.maxLeaseHours}`} unit={t('dash.hoursUnit')} />
            <StatCard icon={Clock} tone="gpu" label={t('dash.kpiGpuHours')} value={`${k.gpuHoursMonth}`} unit={t('dash.hoursUnit')} />
            <StatCard icon={Timer} tone="warn" label={t('dash.kpiCooldown')} value={`${dyn.cooldownHours}`} unit={t('dash.hoursUnit')} />
          </>
        )}
      </div>

      <div className="card mb">
          {/* 물리노드(하이브리드) 모드: 노드별 상세 박스 + 그 안의 GPU 가용성. 그 외: 지역 단위 행. */}
          <h3>{hybrid ? <><Server size={16} /> {t('dash.nodeAvail')}</> : <><Globe size={16} /> {t('dash.regions')}</>}</h3>
          {hybrid ? (
            <div className="grid" style={{ gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(max(220px, calc(25% - 9px)), 1fr))' }}>
              {avail.byNode.map((n) => (
                <div key={n.node} style={{ padding: 14, borderRadius: 12, border: '1px solid var(--border)', background: 'var(--surface-2)' }}>
                  <div className="flex" style={{ justifyContent: 'space-between', alignItems: 'center' }}>
                    <span className="flex" style={{ gap: 8, minWidth: 0 }}>
                      <Server size={15} style={{ color: 'var(--primary)', flex: '0 0 auto' }} />
                      <span style={{ fontWeight: 800, fontSize: 15 }}>{n.node}</span>
                    </span>
                    <Pill variant={n.gpuFree > 0 ? 'ok' : 'wait'}>{n.gpuFree > 0 ? t('dash.available') : t('dash.unavailable')}</Pill>
                  </div>
                  <div className="muted" style={{ fontSize: 12, margin: '8px 0 10px', display: 'flex', gap: 6, flexWrap: 'wrap' }}>
                    {[n.gpu, n.cpu, n.mem, n.disk].filter(Boolean).map((m, i) => (
                      <React.Fragment key={i}>{i > 0 && <span style={{ opacity: .5 }}>·</span>}<span>{m}</span></React.Fragment>
                    ))}
                  </div>
                  <div className="flex" style={{ justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 5 }}>
                    <span className="muted" style={{ fontSize: 12 }}>{t('dash.gpuAvail')}</span>
                    <span className="free-n">{n.gpuFree}/{n.gpuTotal}</span>
                  </div>
                  <div className="flex" style={{ gap: 4 }}>
                    {Array.from({ length: n.gpuTotal }).map((_, j) => (
                      <span key={j} style={{ flex: 1, height: 10, borderRadius: 3, background: j < n.gpuFree ? 'var(--free)' : 'var(--danger)' }} />
                    ))}
                  </div>
                </div>
              ))}
            </div>
          ) : (
            avail.byType.map((r, i) => {
              // 전용(통 카드)과 HAMi(분할)를 분리해 표기한다. 전용=정수 카드, HAMi=소수 GPU 단위
              //  (물리 GPU=1, 잔여는 VRAM·코어 비율 → 예 1.3/2). "10슬롯 뻥튀기" 폐기.
              const hasExcl = r.total > 0;
              const hasHami = r.fractionalTotal > 0;
              const hamiFree = r.fractionalFreeUnits || 0;
              const anyFree = (hasExcl && r.free > 0) || (hasHami && hamiFree > 0.05);
              return (
                <div className="avail" key={i} style={{ flexDirection: 'column', alignItems: 'stretch', gap: 10 }}>
                  <div className="flex" style={{ justifyContent: 'space-between', alignItems: 'center' }}>
                    <span className="gpu-name">{r.gpuType}</span>
                    <span className="flex gap" style={{ alignItems: 'center' }}>
                      {hasHami && <Pill variant="gpu">{t('dash.hamiShared', { defaultValue: 'HAMi 분할' })}</Pill>}
                      <Pill variant={anyFree ? 'ok' : 'wait'}>{anyFree ? t('dash.available') : t('dash.unavailable')}</Pill>
                    </span>
                  </div>
                  {hasExcl && (
                    <div>
                      <div className="flex" style={{ justifyContent: 'space-between', fontSize: 12, marginBottom: 4 }}>
                        <span className="muted">{t('dash.exclusiveWhole', { defaultValue: '전용 (통 카드)' })}</span>
                        <span className="free-n">{r.free}/{r.total}</span>
                      </div>
                      <div className="flex" style={{ gap: 4 }}>
                        {Array.from({ length: r.total }).map((_, j) => (
                          <span key={j} style={{ flex: 1, height: 10, borderRadius: 3, background: j < r.free ? 'var(--free)' : 'var(--danger)' }} />
                        ))}
                      </div>
                    </div>
                  )}
                  {hasHami && (
                    <div>
                      <div className="flex" style={{ justifyContent: 'space-between', fontSize: 12, marginBottom: 4 }}>
                        <span className="muted">{t('dash.hamiFractional', { defaultValue: '분할 (VRAM·코어 단위)' })}</span>
                        <span className="free-n">{hamiFree.toFixed(1)}/{r.fractionalTotal} GPU</span>
                      </div>
                      {/* 카드별 소수 채움: 카드 j 에 (hamiFree - j) 만큼 부분 채움 */}
                      <div className="flex" style={{ gap: 4 }}>
                        {Array.from({ length: r.fractionalTotal }).map((_, j) => {
                          const fill = Math.max(0, Math.min(1, hamiFree - j));
                          return (
                            <span key={j} style={{ flex: 1, height: 10, borderRadius: 3, position: 'relative', overflow: 'hidden', background: 'var(--danger)' }}>
                              <span style={{ position: 'absolute', inset: 0, width: `${fill * 100}%`, background: 'var(--gpu)' }} />
                            </span>
                          );
                        })}
                      </div>
                    </div>
                  )}
                  {!hasExcl && !hasHami && <div className="muted" style={{ fontSize: 12 }}>{t('dash.unavailable')}</div>}
                </div>
              );
            })
          )}
        </div>

      <div className="card mb">
        <h3><Boxes size={16} /> {t('dash.mySessions')}</h3>
        <DataTable
          columns={[
            { key: 'name', header: t('dash.colName') },
            { key: 'kind', header: t('dash.colKind'), render: (r) => <Pill variant={r.kind.includes('CPU') ? 'free' : 'gpu'}>{r.kind}</Pill> },
            { key: 'spec', header: t('dash.colSpec'), className: 'mono' },
            ...(creditMode ? [{ key: 'credit', header: t('dash.colCredit'), render: (r) => (r.credit ? `${r.credit} C/h` : t('dash.free')) }] : []),
            { key: 'conn', header: t('dash.colConn'), render: (r) => (
              (r.status === 'running' && (r.conn || []).length)
                ? <span className="flex" style={{ gap: 6, flexWrap: 'wrap' }}>
                    {(r.conn || []).map((c) => {
                      const key = String(c).toLowerCase();
                      const meta = CONN_CH[key] || { icon: KeyRound, label: c };
                      const Icon = meta.icon;
                      return (
                        <button key={key} className="btn sm" onClick={() => openConn(r, key)}>
                          <Icon size={13} /> {meta.label}
                        </button>
                      );
                    })}
                  </span>
                : <span className="muted">—</span>
            ) },
            { key: 'status', header: t('dash.colStatus'), render: (r) => {
              // 모든 상태를 i18n 라벨로 — running 만 번역하고 나머지를 raw enum(영문)으로 찍던 버그 수정.
              const map = { running: ['run', 'running'], provisioning: ['wait', 'provisioning'], queued: ['wait', 'queued'], paused: ['pause', 'paused'], stopped: ['pause', 'paused'], terminated: ['pause', 'terminated'], failed: ['err', 'terminated'] };
              const [v, k] = map[r.status] || ['wait', null];
              return <Pill variant={v} dot>{k ? t('session.' + k, { defaultValue: r.status }) : r.status}</Pill>;
            } },
          ]}
          rows={d.sessions}
          rowKey={(r) => r.id}
        />
      </div>

      <ConnectionModal session={connSession} initialTab={connTab} onClose={() => setConnSession(null)} />
    </div>
  );
}
