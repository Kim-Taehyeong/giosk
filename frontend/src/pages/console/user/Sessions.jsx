import React, { useEffect, useState, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Code2, NotebookPen, TerminalSquare, ChevronDown, ChevronRight, Power, ScrollText, Clock, Activity } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import Pill from '../../../components/console/Pill';
import Bar from '../../../components/console/Bar';
import ConnectionModal from '../../../components/console/ConnectionModal';
import Modal from '../../../components/console/Modal';
import Select from '../../../components/console/Select';
import { useToast } from '../../../components/console/Toast';
import { useConfirm } from '../../../components/console/Confirm';
import { getMySessionsWithUsage, stopSession, startSession, deleteSession, extendSession, getSessionAudit } from '../../../api/console/sessions';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { measureRows, gpuUnmeasurable } from '../../../utils/sessionUsage';
import { cU } from '../../../lib/credit';

const CONN_ICON = { vscode: Code2, jupyter: NotebookPen, ssh: TerminalSquare, terminal: TerminalSquare };
const CONN_LABEL = { vscode: 'VSCode', jupyter: 'Jupyter', ssh: 'SSH', terminal: 'SSH' };

// connChips는 세션 채널 목록을 버튼용으로 정규화한다 — 웹터미널(terminal)과 SSH(ssh)는 접속 모달의
// 통합 SSH 탭 하나로 다루므로 terminal→ssh 로 접고 중복을 없앤다(SSH 칩이 두 개로 뜨는 것 방지).
function connChips(conn) {
  return [...new Set((conn || []).map((c) => { const k = c.toLowerCase(); return k === 'terminal' ? 'ssh' : k; }))];
}

export default function Sessions() {
  const { t } = useTranslation('consoleUser');
  const { config } = useSystemConfig();
  const creditMode = config.billing.mode === 'credit';
  const dynamicMode = config.billing.mode === 'dynamic';
  // 기본 9열(상태·이름·종류·오퍼링·노드·사용률·실행시간·접속·작업) + 모드별 1열
  // (credit=크레딧 / dynamic=남은시간 / free=없음).
  const colCount = 9 + ((creditMode || dynamicMode) ? 1 : 0);
  const [rows, setRows] = useState(null);
  const [conn, setConn] = useState(null);
  const [connTab, setConnTab] = useState(null); // 클릭한 채널(모달 초기 탭) — 안 넘기면 항상 VSCode 로 열린다
  const [openId, setOpenId] = useState(null);
  const navigate = useNavigate();
  const { toast } = useToast();
  const confirm = useConfirm();

  // 임대/연장은 선착순(Dynamic) 모드 전용 개념 — 크레딧 모드에선 노출하지 않음.
  const lease = config.lease || { extensionHours: 0, maxExtensions: 0 };
  const [extendFor, setExtendFor] = useState(null);
  const [extHours, setExtHours] = useState(1);
  const hourOpts = Array.from({ length: Math.max(1, lease.extensionHours) }, (_, i) => ({ value: i + 1, label: t('session.hoursN', { h: i + 1 }) }));
  // 4초 폴링 — 데이터가 실제로 바뀐 경우에만 setRows 한다. 매 틱마다 새 배열로 갱신하면
  // 테이블 전체가 리렌더되며 "폴링처럼 깜박"인다(usePoll 과 같은 시그니처 비교로 방지).
  const lastSig = useRef('');
  const load = () => getMySessionsWithUsage().then((d) => {
    const sig = (() => { try { return JSON.stringify(d); } catch { return null; } })();
    if (sig === null || sig !== lastSig.current) { lastSig.current = sig; setRows(d); }
  });
  // 마운트 시 1회 + 4초마다 폴링(프로비저닝→실행 등 상태 자동 갱신).
  useEffect(() => {
    load();
    const id = setInterval(load, 4000);
    return () => clearInterval(id);
  }, []); // eslint-disable-line react-hooks/exhaustive-deps
  const act = async (fn, id, msg) => { await fn(id); toast(msg); load(); };
  // 임대 연장 — 백엔드가 정책(maxExtensions) 내에서 연장 횟수를 +1(영속). 성공 시 재조회.
  const doExtend = async () => {
    const r = extendFor;
    try {
      await extendSession(r.id);
      toast(t('session.extended', { h: extHours }));
    } catch {
      toast(t('session.extendFailed'));
    }
    setExtendFor(null);
    load();
  };

  // 선착순(dynamic) 남은 임대 시간 = maxLeaseHours + 연장×extensionHours − 경과시간.
  const leaseLeftText = (r) => {
    if (r.status !== 'running' || !r.startedAt) return '—';
    const totalH = config.billing.dynamic.maxLeaseHours + (r.extensionsUsed || 0) * (lease.extensionHours || 0);
    const leftH = totalH - (Date.now() - new Date(r.startedAt).getTime()) / 3600000;
    if (leftH <= 0) return t('session.leaseExpired');
    const h = Math.floor(leftH);
    const m = Math.floor((leftH - h) * 60);
    return h > 0 ? `${h}h ${m}m` : `${m}m`;
  };

  const statusPill = (s) => {
    if (s === 'running') return <Pill variant="run" dot>{t('session.running')}</Pill>;
    if (s === 'provisioning') return <Pill variant="wait" dot>{t('session.provisioning')}</Pill>;
    if (s === 'queued') return <Pill variant="wait" dot>{t('session.queued')}</Pill>;
    if (s === 'paused') return <Pill variant="pause" dot>{t('session.paused')}</Pill>;
    if (s === 'stopped') return <Pill variant="pause" dot>{t('session.paused')}</Pill>;
    if (s === 'terminated') return <Pill variant="pause" dot>{t('session.terminated')}</Pill>;
    return <Pill variant="pause" dot>{s}</Pill>;
  };
  const modePill = (m) => {
    if (m === 'ssh') return <Pill variant="primary">{t('session.physical')}</Pill>;
    if (m === 'exclusive') return <Pill variant="primary">{t('session.exclusive')}</Pill>;
    if (m === 'cpu') return <Pill variant="free">{t('session.cpu')}</Pill>;
    return <Pill variant="gpu">{t('session.shared')}</Pill>;
  };

  // 사용률 — 자세히 보기를 열지 않아도 목록에서 바로 보이게 한다.
  // CPU/RAM 은 cgroup 기반이라 항상 잴 수 있고, GPU 는 공유 방식에 따라 못 재는 경우가 있다
  // (measureRows 참조) → 못 재는 건 0% 막대가 아니라 사유를 적는다.
  const usageCell = (r) => {
    if (r.status !== 'running') return <span className="muted">—</span>;
    const rows2 = measureRows(r);
    if (!rows2.length) return <span className="muted">—</span>;
    // 컴팩트: 지표당 한 줄(라벨 · 바 · 값)로 붙여 행 높이를 줄인다.
    return (
      <div style={{ minWidth: 170 }}>
        {rows2.map((x) => (
          <div key={x.label} className="flex" style={{ alignItems: 'center', gap: 6, lineHeight: 1.2, marginBottom: 1 }}>
            <span style={{ width: 36, fontSize: 10, color: 'var(--muted)', fontWeight: 700 }}>{x.label}</span>
            <div style={{ flex: 1, minWidth: 44 }}><Bar value={x.pct} max={100} variant={x.variant} /></div>
            <span style={{ width: 58, textAlign: 'right', fontSize: 10, color: 'var(--muted)', fontWeight: 600, whiteSpace: 'nowrap' }}>{x.txt}</span>
          </div>
        ))}
        {gpuUnmeasurable(r) && (
          <div className="muted" style={{ fontSize: 9.5, marginTop: 1 }} title={t(`session.gpuReason.${r.gpuReason}`)}>
            GPU {t('session.notMeasurable')}
          </div>
        )}
      </div>
    );
  };

  const list = rows || [];

  return (
    <div>
      <PageHead title={t('session.title')} subtitle={t('session.subtitle')}
        actions={<button className="btn primary" onClick={() => navigate('/console/sessions/new')}>{t('session.new')}</button>} />
      <div className="legend mb">{t('session.idlePolicy', { n: config.idle.timeoutMin })}</div>
      <div className="card">
        <table>
          <thead>
            <tr>
              <th>{t('session.status')}</th><th>{t('session.name')}</th><th>{t('session.kind')}</th>
              <th>{t('session.offering')}</th><th>{t('session.detNode')}</th><th>{t('session.usage')}</th><th>{t('session.runtime')}</th>
              {creditMode && <th>{t('session.credit')}</th>}
              {dynamicMode && <th>{t('session.leaseLeftCol')}</th>}
              <th>{t('session.conn')}</th><th>{t('session.action')}</th>
            </tr>
          </thead>
          <tbody>
            {list.length === 0 && <tr><td colSpan={colCount} style={{ textAlign: 'center', color: 'var(--muted)' }}>{t('common.empty')}</td></tr>}
            {list.map((r) => (
              <tr key={r.id} className="row-link" style={{ cursor: 'pointer' }}
                onClick={(e) => { if (e.target.closest('button, a, input, select, textarea')) return; navigate(`/console/sessions/${r.id}`); }}>
                <td>{statusPill(r.status)}</td>
                <td style={{ fontWeight: 600 }}>{r.name}</td>
                <td>{modePill(r.mode)}</td>
                <td className="mono">{r.offering}</td>
                <td className="mono" style={{ fontSize: 12 }}>{r.node || '—'}{r.gpuId ? <span className="muted"> · {r.gpuId}</span> : null}</td>
                <td>{usageCell(r)}</td>
                <td>{r.runtime}{r.idleMin > 0 && <span className="muted"> ({t('session.idle', { n: r.idleMin })})</span>}</td>
                {creditMode && <td>{r.pricePerHour > 0 ? cU(r.credit) : t('session.free')}</td>}
                {dynamicMode && <td>{r.mode === 'cpu' ? '—' : leaseLeftText(r)}</td>}
                <td className="flex">
                  {r.status === 'running' && r.conn.length
                    ? connChips(r.conn).map((k) => { const Icon = CONN_ICON[k] || TerminalSquare; return <button key={k} className="btn sm" onClick={() => { setConnTab(k); setConn(r); }} title={CONN_LABEL[k] || k}><Icon size={13} /> {CONN_LABEL[k] || k}</button>; })
                    : <span className="muted">—</span>}
                </td>
                <td><ChevronRight size={15} style={{ color: 'var(--muted)' }} /></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <ConnectionModal session={conn} initialTab={connTab} onClose={() => setConn(null)} />
    </div>
  );
}
