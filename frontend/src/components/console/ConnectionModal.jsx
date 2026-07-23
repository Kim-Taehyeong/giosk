import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Code2, NotebookPen, TerminalSquare, ExternalLink, Copy, Clock, RefreshCw } from 'lucide-react';
import Modal from './Modal';
import { useToast } from './Toast';
import { getAccess } from '../../api/console/sessions';

const TABS = [
  { key: 'vscode', label: 'VSCode', icon: Code2 },
  { key: 'jupyter', label: 'Jupyter', icon: NotebookPen },
  // 웹터미널(컨테이너 세션) — 브라우저 xterm 으로 바로 접속. "SSH" 로 표기.
  { key: 'terminal', label: 'SSH', icon: TerminalSquare },
  // 네이티브 SSH(물리 세션·sshd 사이드카) — 복붙 명령 + 웹으로 연결하기.
  { key: 'ssh', label: 'SSH', icon: TerminalSquare },
];

// copyText는 클립보드 복사를 시도한다. navigator.clipboard 는 보안 컨텍스트(HTTPS/localhost)에서만
// 동작하므로(HTTP 배포에선 undefined), 실패 시 execCommand 폴백을 사용한다. 성공 여부 반환.
async function copyText(text) {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch { /* 폴백으로 진행 */ }
  try {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.top = '-1000px';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    const ok = document.execCommand('copy');
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}

function CodeRow({ text }) {
  const { t } = useTranslation('consoleUser');
  const { toast } = useToast();
  const onCopy = async () => {
    const ok = await copyText(text);
    toast(ok ? t('conn.copied') : t('conn.copyFail'));
  };
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 4,
      background: 'var(--surface-2)', border: '1px solid var(--border)', borderRadius: 8, padding: '8px 10px' }}>
      {/* 토큰 URL·명령은 매우 길다 — 줄바꿈 대신 한 줄 가로 스크롤(복사는 전체). */}
      <code style={{ flex: 1, minWidth: 0, fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
        fontSize: 12.5, lineHeight: 1.6, whiteSpace: 'nowrap', overflowX: 'auto', color: 'var(--text)' }}>{text}</code>
      <button type="button" className="btn sm" onClick={onCopy} style={{ flexShrink: 0 }}>
        <Copy size={13} /> {t('conn.copy')}
      </button>
    </div>
  );
}

// ExpiryHint는 접속 링크 유효시간을 초 단위로 실시간 카운트다운한다(발급 후 짧게만 유효).
// onRegenerate: 있으면 옆에 "재생성" 버튼 — 만료됐거나 곧 만료될 때 새 토큰/링크를 발급받는다.
function ExpiryHint({ expiresAt, t, onRegenerate, regenerating }) {
  const target = expiresAt ? new Date(expiresAt).getTime() : 0;
  const [left, setLeft] = useState(() => Math.max(0, Math.round((target - Date.now()) / 1000)));
  useEffect(() => {
    if (!target) return undefined;
    const tick = () => setLeft(Math.max(0, Math.round((target - Date.now()) / 1000)));
    tick();
    const id = setInterval(tick, 1000);
    return () => clearInterval(id);
  }, [target]);
  if (!expiresAt) return null;
  const mm = Math.floor(left / 60);
  const ss = String(left % 60).padStart(2, '0');
  const expired = left <= 0;
  // 남은 시간에 따라 색을 바꿔 눈에 띄게: 만료/30초 이하=빨강, 2분 이하=주황, 그 외=초록.
  const tone = expired || left <= 30 ? 'var(--danger, #dc2626)' : left <= 120 ? 'var(--warn, #f59e0b)' : 'var(--free, #16a34a)';
  const urgent = !expired && left <= 30;
  return (
    <div className="flex mt" style={{ alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
      <div className="flex" style={{
        alignItems: 'center', gap: 7, padding: '6px 11px', borderRadius: 999, width: 'fit-content',
        fontSize: 12.5, fontWeight: 700, color: tone, background: `color-mix(in srgb, ${tone} 12%, transparent)`,
        border: `1px solid color-mix(in srgb, ${tone} 35%, transparent)`,
        animation: urgent ? 'giosk-pulse 1s ease-in-out infinite' : undefined,
      }}>
        <Clock size={13} />
        {expired ? t('conn.accessExpired') : t('conn.accessExpiryLive', { time: `${mm}:${ss}` })}
      </div>
      {onRegenerate && (
        <button type="button" className="btn sm" onClick={onRegenerate} disabled={regenerating}>
          <RefreshCw size={13} /> {t('conn.regenerate', { defaultValue: '재생성' })}
        </button>
      )}
    </div>
  );
}

// WebPane은 웹 채널(VSCode/Jupyter) 접속 패널 — 게이트웨이 토큰 URL(원본 비밀 미노출) + 열기 버튼.
// 게이트웨이 비활성 배포에선 URL 이 직접접속(NodePort/LB)이고 비밀번호가 따라온다 → 그때만 비밀번호를 보여준다.
function WebPane({ info, urlLabel, openLabel, expiresAt, t, onRegenerate, regenerating }) {
  const tokenized = !!info.url && info.url.includes('access=');
  return (
    <div className="tabpane active">
      <label className="fld" style={{ marginTop: 0 }}>{urlLabel}</label>
      <CodeRow text={info.url} />
      <div className="mt">
        <a className="btn primary" href={info.url} target="_blank" rel="noreferrer"><ExternalLink size={14} /> {openLabel}</a>
      </div>
      {tokenized ? <ExpiryHint expiresAt={expiresAt} t={t} onRegenerate={onRegenerate} regenerating={regenerating} /> : null}
      {info.password && (<>
        <label className="fld">{t('conn.password')}</label>
        <CodeRow text={info.password} />
        <div className="legend">{t('conn.passwordHint')}</div>
      </>)}
    </div>
  );
}

// SSHPane은 SSH 접속 패널 — 두 가지 경로: (1) 프록시(게이트웨이) 한 점 접속(어디서든),
// (2) 사내망 직접 접속(192 대역, 사무실 네트워크 안일 때 게이트웨이 우회). 물리 세션만 직접 경로 제공.
function SSHPane({ info, expiresAt, t, onRegenerate, regenerating, onWebConnect }) {
  // 웹으로 연결하기 — 브라우저 터미널(키/명령 불요). 복붙 SSH 위에 우선 제공.
  const webBtn = onWebConnect ? (
    <div className="mt">
      <button type="button" className="btn primary" onClick={onWebConnect}>
        <TerminalSquare size={14} /> {t('conn.webConnect', { defaultValue: '웹으로 연결하기' })}
      </button>
      <div className="legend mt">{t('conn.webConnectHint', { defaultValue: '브라우저에서 바로 터미널로 접속합니다(SSH 키·명령 불필요).' })}</div>
    </div>
  ) : null;
  // 게이트웨이 off 폴백(direct=true)은 사용자 키로 노드에 바로 붙는 단일 경로.
  if (info.direct) {
    return (
      <div className="tabpane active">
        {webBtn}
        <label className="fld">{t('conn.sshDirect', { defaultValue: '직접 접속 (사내망)' })}</label>
        <CodeRow text={info.cmd} />
        <div className="legend mt">{t('conn.sshDirectHint')}</div>
      </div>
    );
  }
  return (
    <div className="tabpane active">
      {webBtn}
      {/* 1) 프록시(게이트웨이) — 최초 인증 후 접속이 유지된다(주기 재검사 없음). */}
      <label className="fld" style={{ marginTop: 0 }}>{t('conn.sshProxy', { defaultValue: '프록시 (게이트웨이)' })}</label>
      <CodeRow text={info.cmd} />
      <div className="legend mt">{t('conn.sshProxyHint', { defaultValue: '어디서든 게이트웨이 한 점으로 접속합니다. 최초 인증 후에는 접속이 끊기지 않고 유지됩니다.' })}</div>
      <ExpiryHint expiresAt={expiresAt} t={t} onRegenerate={onRegenerate} regenerating={regenerating} />

      {/* 2) 사내망 직접(192 대역) — 사무실 네트워크 안이면 게이트웨이 없이 바로. */}
      {info.directCmd && (<>
        <label className="fld">{t('conn.sshDirect', { defaultValue: '직접 접속 (사내망)' })}</label>
        <CodeRow text={info.directCmd} />
        <div className="legend">{t('conn.sshDirectInHint', { defaultValue: '사무실 네트워크(사내망) 안에서 노드에 바로 접속합니다. 게이트웨이를 거치지 않습니다.' })}</div>
      </>)}
    </div>
  );
}

// 웹터미널 실행 패널 — 새 창(전체화면)으로 브라우저 터미널을 연다(SSH 키·명령 불요).
function TerminalLaunchPane({ onOpen, t }) {
  return (
    <div className="tabpane active">
      <button type="button" className="btn primary" onClick={onOpen}>
        <ExternalLink size={14} /> {t('conn.webConnect', { defaultValue: '웹으로 연결하기' })}
      </button>
      <div className="legend mt">{t('conn.webConnectNewWindow', { defaultValue: '새 창에서 브라우저 터미널이 열립니다 (SSH 키·명령 불필요).' })}</div>
    </div>
  );
}

// 세션 접속 모달 — 게이트웨이 단기 토큰 URL(VSCode/Jupyter)·복붙 SSH 제공.
// 접속 링크는 발급 시점부터 짧게만 유효(게이트웨이가 원본 비밀을 대신 주입 → 사용자에게 비밀 미노출).
export default function ConnectionModal({ session, onClose, initialTab }) {
  const { t } = useTranslation('consoleUser');
  const { toast } = useToast();
  const [conn, setConn] = useState(null);
  const [err, setErr] = useState('');
  const [tab, setTab] = useState('vscode');
  const [regenerating, setRegenerating] = useState(false);

  // 웹 터미널은 새 창(전체화면)으로 연다 — 모달 안 임베드보다 넓고, 세션 접속 옵션(복붙 SSH)과 공존.
  const openTerminal = () => window.open(`/terminal/${session.id}`, `giosk-term-${session.id}`, 'width=940,height=640');

  useEffect(() => {
    if (!session) return;
    // 세션이 제공하는 연결 방식 중 첫 탭 선택(호출부가 initialTab 을 주면 그 채널을 우선).
    const avail = session.conn?.length ? session.conn.map((c) => c.toLowerCase()) : ['vscode', 'jupyter', 'ssh'];
    const want = initialTab && avail.includes(initialTab) ? initialTab : null;
    setTab(want || (avail.includes('vscode') ? 'vscode' : avail[0]));
    setConn(null);
    setErr('');
    let stale = false; // 모달을 빠르게 다시 열면 이전 요청 응답이 늦게 와 덮어쓸 수 있다.
    getAccess(session.id)
      .then((r) => { if (!stale) setConn(r); })
      .catch((e) => { if (!stale) setErr(e?.message || t('conn.loadFail')); }); // 실패 시 로딩 문구가 영영 남지 않게
    return () => { stale = true; };
  }, [session, initialTab, t]);

  // 재생성 — 새 단기 토큰/링크를 발급받는다(만료됐거나 다시 붙을 때).
  const regenerate = async () => {
    if (!session) return;
    setRegenerating(true);
    try { setConn(await getAccess(session.id)); toast(t('conn.regenerated', { defaultValue: '새 접속 링크를 발급했습니다.' })); }
    catch (e) { toast(e?.message || t('conn.loadFail')); }
    setRegenerating(false);
  };

  if (!session) return null;
  const avail = (session.conn || []).map((c) => c.toLowerCase());
  const tabs = TABS.filter((t) => avail.length === 0 || avail.includes(t.key));

  return (
    <Modal open={!!session} title={t('conn.title', { name: session.name })} onClose={onClose} width={560}>
      <div style={{ display: 'flex', gap: 4, marginBottom: 16, borderBottom: '1px solid var(--border)' }}>
        {tabs.map((tb) => {
          const Icon = tb.icon;
          const on = tab === tb.key;
          return (
            <button key={tb.key} type="button" onClick={() => setTab(tb.key)}
              style={{ display: 'inline-flex', alignItems: 'center', gap: 7, padding: '9px 16px',
                fontSize: 13.5, fontWeight: 700, background: 'none', border: 'none', cursor: 'pointer',
                color: on ? 'var(--primary)' : 'var(--muted)',
                borderBottom: on ? '2px solid var(--primary)' : '2px solid transparent', marginBottom: -1 }}>
              <Icon size={15} /> {tb.label}
            </button>
          );
        })}
      </div>

      {err ? <div className="legend" style={{ color: 'var(--danger)' }}>{err}</div>
        : !conn ? <div className="muted">{t('conn.loading')}</div> : (
        <>
          {tab === 'vscode' && (conn.vscode
            ? <WebPane info={conn.vscode} urlLabel={t('conn.vscodeUrl')} openLabel={t('conn.openVscode')} expiresAt={conn.expiresAt} t={t} onRegenerate={regenerate} regenerating={regenerating} />
            : <div className="muted">{t('conn.unavailable')}</div>)}
          {tab === 'jupyter' && (conn.jupyter
            ? <WebPane info={conn.jupyter} urlLabel={t('conn.jupyterUrl')} openLabel={t('conn.openJupyter')} expiresAt={conn.expiresAt} t={t} onRegenerate={regenerate} regenerating={regenerating} />
            : <div className="muted">{t('conn.unavailable')}</div>)}
          {/* 웹터미널(컨테이너 세션) — 새 창으로 브라우저 xterm 열기 */}
          {tab === 'terminal' && <TerminalLaunchPane onOpen={openTerminal} t={t} />}
          {/* 네이티브 SSH(물리 세션) — Proxy/직접 복붙 명령 + 웹으로 연결하기(새 창) */}
          {tab === 'ssh' && (conn.ssh
            ? <SSHPane info={conn.ssh} expiresAt={conn.expiresAt} t={t} onRegenerate={regenerate} regenerating={regenerating} onWebConnect={openTerminal} />
            : <TerminalLaunchPane onOpen={openTerminal} t={t} />)}
        </>
      )}
    </Modal>
  );
}
