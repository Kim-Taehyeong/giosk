import React, { useEffect, useState, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Code2, NotebookPen, TerminalSquare, Power, RotateCcw, Trash2, Clock } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import Pill from '../../../components/console/Pill';
import Bar from '../../../components/console/Bar';
import ConnectionModal from '../../../components/console/ConnectionModal';
import RowMenu from '../../../components/console/RowMenu';
import { useToast } from '../../../components/console/Toast';
import { useConfirm } from '../../../components/console/Confirm';
import { getMySessionsWithUsage, stopSession, startSession, deleteSession, extendSession } from '../../../api/console/sessions';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { measureRows, gpuUnmeasurable } from '../../../utils/sessionUsage';
import { reclaimInfo, RECLAIM_VARIANT } from '../../../utils/reclaim';
import { cU } from '../../../lib/credit';

const CONN_ICON = { vscode: Code2, jupyter: NotebookPen, ssh: TerminalSquare, terminal: TerminalSquare };
const CONN_LABEL = { vscode: 'VSCode', jupyter: 'Jupyter', ssh: 'SSH', terminal: 'SSH' };

// connChips는 세션 채널 목록을 버튼용으로 정규화한다. 웹터미널(terminal)과 SSH(ssh)는 접속 모달의
// 통합 SSH 탭 하나로 다루므로 terminal 을 ssh 로 접어 중복을 없앤다(SSH 칩이 두 개로 뜨는 걸 막는다).
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
  // 상세로 가는 화살표 열은 없앴다. 행 전체가 클릭이고 키보드로도 열리므로 열을 하나 더 쓸 이유가 없다.
  const colCount = 9 + ((creditMode || dynamicMode) ? 1 : 0);
  const [rows, setRows] = useState(null);
  const [conn, setConn] = useState(null);
  const [connTab, setConnTab] = useState(null); // 클릭한 채널(모달 초기 탭). 안 넘기면 항상 VSCode 로 열린다
  const navigate = useNavigate();
  const { toast } = useToast();
  const confirm = useConfirm();

  // 임대와 연장은 선착순(Dynamic) 모드 전용 개념이라 크레딧 모드에선 노출하지 않는다.
  const lease = config.lease || { extensionHours: 0, maxExtensions: 0 };
  // 4초 폴링이지만 데이터가 실제로 바뀐 경우에만 setRows 한다. 매 틱마다 새 배열로 갱신하면
  // 테이블 전체가 리렌더되며 "폴링처럼 깜박"인다(usePoll 과 같은 시그니처 비교로 방지).
  const lastSig = useRef('');
  const [now, setNow] = useState(() => Date.now());
  const load = () => getMySessionsWithUsage().then((d) => {
    setNow(Date.now()); // 남은 임대시간 기준시각. 렌더 중에 Date.now() 를 부르면 순수하지 않다
    const sig = (() => { try { return JSON.stringify(d); } catch { return null; } })();
    if (sig === null || sig !== lastSig.current) { lastSig.current = sig; setRows(d); }
  });
  // 마운트할 때 한 번 부르고 4초마다 폴링한다(프로비저닝에서 실행으로 넘어가는 상태를 자동 갱신).
  useEffect(() => {
    load();
    const id = setInterval(load, 4000);
    return () => clearInterval(id);
  }, []);
  // 정지, 재시작, 삭제 공통이다. 실패도 반드시 말해준다. 가용성 관문이 재시작을 거절할 수 있는데
  // (자리 없으면 대기 없이 거절) 조용히 삼키면 사용자에겐 "버튼이 안 먹는다"로 보인다.
  // 진행 중인 행 동작(연타 방지). 서버도 CAS 로 중복 시작을 막지만, 버튼이 눌리는 족족 요청이
  // 나가면 두 번째는 "이미 시작 중"이라는 실패 토스트로 돌아와 사용자에겐 오작동처럼 보인다.
  const [pending, setPending] = useState({});
  const act = async (fn, id, msg) => {
    if (pending[id]) return;
    setPending((p) => ({ ...p, [id]: true }));
    try {
      await fn(id);
      toast(msg);
    } catch (e) {
      toast(e?.message || t('session.actionFailed', { defaultValue: '요청을 처리하지 못했습니다' }));
    }
    setPending((p) => { const n = { ...p }; delete n[id]; return n; });
    load();
  };
  // 임대 연장. 백엔드가 정책(maxExtensions) 안에서 연장 횟수를 하나 올린다. 성공하면 재조회한다.
  // 연장 시간은 사용자가 고르는 값이 아니라 정책값(lease.extensionHours)이다. 백엔드 Extend 는
  // 시간 인자를 받지 않고 "maxLeaseHours + 연장횟수 × extensionHours" 로 만료를 계산한다.
  // (예전엔 시간 선택 모달이 있었지만 고른 값이 어디에도 쓰이지 않아 사용자를 오해시켰다.)
  const doExtend = async (r) => {
    try {
      await extendSession(r.id);
      toast(t('session.extended', { h: lease.extensionHours || 1 }));
    } catch {
      toast(t('session.extendFailed'));
    }
    load();
  };

  // 선착순(dynamic) 남은 임대 시간 = maxLeaseHours + 연장×extensionHours − 경과시간.
  const leaseLeftText = (r) => {
    if (r.status !== 'running' || !r.startedAt) return '—';
    const totalH = config.billing.dynamic.maxLeaseHours + (r.extensionsUsed || 0) * (lease.extensionHours || 0);
    const leftH = totalH - (now - new Date(r.startedAt).getTime()) / 3600000;
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
  // 중단 세션 홈 회수 안내는 상태 pill 아래에 작게 둔다. 실제 삭제는 노드 디스크가 찰 때만
  // 일어나므로 "삭제 예정"이 아니라 "회수 대상"으로 적는다(title 에 조건 전문).
  const reclaimPill = (r) => {
    const { state, days } = reclaimInfo(r, config);
    if (state === 'off' || state === 'ok') return null;
    const label = state === 'exempt' ? t('session.reclaimKept')
      : state === 'due' ? t('session.reclaimDue')
        : t('session.reclaimIn', { n: days });
    return (
      <div style={{ marginTop: 4 }} title={state === 'exempt' ? t('session.reclaimKeptHint') : t('session.reclaimHint', { n: config.reclaim.stoppedTtlDays })}>
        <Pill variant={RECLAIM_VARIANT[state]}>{label}</Pill>
      </div>
    );
  };

  // 행 작업. 예전엔 이 열에 접속 칩만 있어서 중단된 세션은 목록에서 아무것도 할 수 없었다
  // (상세로 들어가야만 재시작·삭제 가능). 중단 세션은 홈 디스크를 계속 물고 있으므로,
  // 정리하는 길이 목록에 바로 있어야 한다. 그 결정은 유지한다.
  //
  // 다만 노출 무게는 다르게 준다. 행마다 "지금 할 일" 하나만 고스트 버튼으로 내놓고
  // (실행 중=정지, 중단=재시작), 되돌릴 수 없는 삭제는 케밥 메뉴 뒤로 보낸다.
  // 그래야 행마다 버튼 폭이 같아 열이 세로로 정렬되고, 빨간 아이콘이 상태 배지보다
  // 시끄러워지는 일도 없다.
  const rowActions = (r) => {
    const stop = () => act(stopSession, r.id, t('session.stopped'));
    const start = () => act(startSession, r.id, t('session.restarted'));
    const del = async () => {
      // 상세 페이지와 같은 문구에 같은 확인 절차를 쓴다. 삭제는 홈 데이터까지 지운다(중단은 보존한다).
      if (!(await confirm({ title: t('session.delete'), message: t('confirmDelete'), confirmText: t('session.delete') }))) return;
      act(deleteSession, r.id, t('session.deleted'));
    };
    if (r.status === 'running') {
      const canExtend = dynamicMode && r.mode !== 'cpu' && (r.extensionsUsed || 0) < lease.maxExtensions;
      return (<>
        <button className="btn sm ghost" onClick={stop} disabled={!!pending[r.id]} title={t('session.stop')}><Power size={13} /> {t('session.stop')}</button>
        {/* 연장은 동적 임대 모드에서만, 그것도 가끔 쓰는 행위 → 메뉴로 */}
        <RowMenu label={r.name} items={[
          canExtend && { key: 'extend', label: t('session.extend'), icon: Clock, onSelect: () => doExtend(r) },
        ].filter(Boolean)} />
      </>);
    }
    if (r.status === 'stopped') {
      return (<>
        <button className="btn sm ghost" onClick={start} disabled={!!pending[r.id]} title={t('session.restart')}><RotateCcw size={13} /> {t('session.restart')}</button>
        {/* 자원 변경은 상세 페이지에만 둔다. 사양을 바꾸려면 지금 사양과 남은 자리를 보고 판단해야 하는데
            목록에는 그 정보가 없다. 목록에서는 재시작과 삭제까지만 한다. */}
        <RowMenu label={r.name} items={[
          { key: 'delete', label: t('session.delete'), icon: Trash2, tone: 'danger', onSelect: del },
        ]} />
      </>);
    }
    return null; // provisioning/queued 등 전이 중에는 조작을 막는다(중복 요청 방지)
  };

  const modePill = (m) => {
    if (m === 'ssh') return <Pill variant="primary">{t('session.physical')}</Pill>;
    if (m === 'exclusive') return <Pill variant="primary">{t('session.exclusive')}</Pill>;
    if (m === 'cpu') return <Pill variant="free">{t('session.cpu')}</Pill>;
    return <Pill variant="gpu">{t('session.shared')}</Pill>;
  };

  // 사용률은 자세히 보기를 열지 않아도 목록에서 바로 보이게 한다.
  // CPU/RAM 은 cgroup 기반이라 항상 잴 수 있고, GPU 는 공유 방식에 따라 못 재는 경우가 있다
  // (measureRows 참조) 못 재는 건 0% 막대가 아니라 사유를 적는다.
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
                tabIndex={0} aria-label={r.name}
                onClick={(e) => { if (e.target.closest('button, a, input, select, textarea')) return; navigate(`/console/sessions/${r.id}`); }}
                onKeyDown={(e) => {
                  if (e.key !== 'Enter' && e.key !== ' ') return;
                  if (e.target.closest('button, a, input, select, textarea')) return; // 행 안 컨트롤의 키 입력은 그쪽 것
                  e.preventDefault();
                  navigate(`/console/sessions/${r.id}`);
                }}>
                <td>{statusPill(r.status)}{reclaimPill(r)}</td>
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
                <td className="flex">{rowActions(r) || <span className="muted">—</span>}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <ConnectionModal session={conn} initialTab={connTab} onClose={() => setConn(null)} />
      {/* 세션별 key + 조건부 렌더 — 선택 상태가 그 세션의 현재 사양에서 시작하도록 새로 마운트한다. */}
          </div>
  );
}
