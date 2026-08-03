import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { ShieldCheck, LayoutGrid, Coins, Server, Bell } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
import { useConsole } from '../../context/ConsoleContext';
import { useSystemConfig } from '../../context/SystemConfigContext';
import { isManager } from '../../config/consoleRoles';
import { getWallet } from '../../api/console/wallet';
import { getAdminDashboard } from '../../api/console/dashboard';
import { getInboxUnread, getInbox, markInboxRead } from '../../api/console/notify';
import OrgGroupSelector from './OrgGroupSelector';
import ScopeSwitcher from './ScopeSwitcher';
import UserMenu from './UserMenu';
import LanguageSwitcher from '../LanguageSwitcher';
import { notifyText } from '../../utils/notify';

// 콘솔 탑바 (.topbar 그리드 영역).
export default function Topbar({ variant, ns }) {
  const { t } = useTranslation(ns);
  const { user } = useAuth();
  const { summary, setSummary } = useConsole();
  const { config } = useSystemConfig();
  const creditMode = config.billing.mode === 'credit';
  const navigate = useNavigate();
  const [unread, setUnread] = useState(0);
  const [bellOpen, setBellOpen] = useState(false); // 알림 드롭다운(최근만)
  const [recent, setRecent] = useState([]);
  const sevDot = { err: 'var(--danger)', warn: 'var(--warn)', info: 'var(--primary)' };

  const isAdmin = user?.role === 'admin';

  // 벨 클릭 — 최근 알림 드롭다운 토글. 열 때 최신 목록 로드.
  const toggleBell = () => {
    const next = !bellOpen;
    setBellOpen(next);
    if (next) getInbox().then((d) => { setRecent(d.items.slice(0, 8)); setUnread(d.unread); }).catch(() => {});
  };
  const openCenter = () => { setBellOpen(false); navigate('/console/notifications'); };
  const readRecent = async (n) => {
    if (n.read) return;
    await markInboxRead(n.id).catch(() => {});
    setRecent((cur) => cur.map((x) => (x.id === n.id ? { ...x, read: true } : x)));
    setUnread((u) => Math.max(0, u - 1));
  };

  // 인앱 알림 미읽음 수 — 60초 폴링(알림센터가 새 알림을 놓치지 않도록).
  useEffect(() => {
    let alive = true;
    const tick = () => getInboxUnread().then((n) => { if (alive) setUnread(n); }).catch(() => {});
    tick();
    const id = setInterval(tick, 60000);
    return () => { alive = false; clearInterval(id); };
  }, []);

  // 탑바 배지 요약 채우기 — 플랫폼 관리자만 노드·GPU 현황(대시보드는 플랫폼 전용),
  // 조직/그룹 관리자·사용자는 개인 크레딧 잔액(크레딧 모드).
  useEffect(() => {
    if (variant === 'admin' && isAdmin) {
      getAdminDashboard()
        .then((d) => {
          const k = d.kpis || {};
          // 모니터링(DCGM) 이 없으면 GPU 사용률은 0% 가 아니라 미측정 — 0% 로 보이면 "GPU 가 논다"고 오해된다.
          const nt = { up: k.nodesUp ?? 0, total: k.nodesTotal ?? 0 };
          const badge = d.metrics?.dcgm
            ? t('topbar.nodeBadge', { ...nt, gpu: k.gpuUtil ?? 0, defaultValue: 'Nodes {{up}}/{{total}} · GPU {{gpu}}%' })
            : t('topbar.nodeBadgeNoGpu', { ...nt, defaultValue: 'Nodes {{up}}/{{total}} · GPU —' });
          setSummary((s) => ({ ...s, adminBadge: badge }));
        })
        .catch(() => {});
      return;
    }
    if (!creditMode) return;
    getWallet()
      .then((w) => setSummary((s) => ({ ...s, credit: w.balance })))
      .catch(() => {});
  }, [variant, isAdmin, creditMode, setSummary]);
  const manager = isManager(user);

  return (
    <div className="topbar">
      {manager ? (
        <ScopeSwitcher ns={ns} />
      ) : (
        <OrgGroupSelector variant={variant} ns={ns} />
      )}
      <div className="spacer" />

      {/* 정보성 배지: admin=클러스터 요약 / user=크레딧 잔액(크레딧 모드, 클릭 시 지갑) */}
      {variant === 'admin' ? (
        <div className="badge-credit" title="클러스터 현황" style={{ cursor: 'default' }}>
          <Server size={14} /> {summary?.adminBadge || '…'}
        </div>
      ) : creditMode ? (
        <div className="badge-credit" title="크레딧 잔액" onClick={() => navigate('/console/wallet')}>
          <Coins size={14} /> {summary?.credit != null ? `${summary.credit} C` : '… C'}
        </div>
      ) : null}

      {/* 콘솔 전환(단일 콘솔): 관리자(플랫폼/조직/그룹) ↔ 사용자 화면. */}
      {(isAdmin || manager) && (
        variant === 'admin'
          ? <button className="btn sm" onClick={() => navigate('/console')} title={t('topbar.toUser')}><LayoutGrid size={14} /> {t('topbar.toUser')}</button>
          : <button className="btn sm" onClick={() => navigate('/console/admin')} title={t('topbar.toAdmin')}><ShieldCheck size={14} /> {t('topbar.toAdmin')}</button>
      )}

      {/* 인앱 알림 종 — 최근 알림 드롭다운. 전체는 알림센터에서. */}
      <div style={{ position: 'relative', display: 'inline-flex' }}>
        <button className="topbar-bell" title={t('topbar.alerts', { defaultValue: '알림' })} onClick={toggleBell}
          style={{ position: 'relative', background: 'transparent', border: 'none', cursor: 'pointer', color: bellOpen ? 'var(--primary)' : 'var(--muted)', display: 'inline-flex', alignItems: 'center', padding: 6 }}>
          <Bell size={17} />
          {unread > 0 && (
            <span style={{ position: 'absolute', top: 1, right: 1, minWidth: 15, height: 15, padding: '0 4px', borderRadius: 8,
              background: 'var(--danger)', color: '#fff', fontSize: 10, fontWeight: 800, lineHeight: '15px', textAlign: 'center' }}>
              {unread > 99 ? '99+' : unread}
            </span>
          )}
        </button>
        {bellOpen && (<>
          {/* 바깥 클릭 닫기 */}
          <div onClick={() => setBellOpen(false)} style={{ position: 'fixed', inset: 0, zIndex: 60 }} />
          <div style={{ position: 'absolute', top: '100%', right: 0, marginTop: 8, width: 340, maxWidth: '90vw', zIndex: 61,
            background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 12, boxShadow: '0 12px 32px rgba(0,0,0,.18)', overflow: 'hidden' }}>
            <div className="flex" style={{ justifyContent: 'space-between', alignItems: 'center', padding: '11px 14px', borderBottom: '1px solid var(--border)' }}>
              <span style={{ fontWeight: 800, fontSize: 13.5 }}>{t('topbar.alerts', { defaultValue: '알림' })}</span>
              {unread > 0 && <span className="muted" style={{ fontSize: 12 }}>{t('topbar.unreadN', { n: unread, defaultValue: `안 읽음 ${unread}` })}</span>}
            </div>
            <div style={{ maxHeight: 360, overflowY: 'auto' }}>
              {recent.length === 0 ? (
                <div className="muted" style={{ padding: '20px 14px', fontSize: 13, textAlign: 'center' }}>{t('topbar.noAlerts', { defaultValue: '받은 알림이 없습니다.' })}</div>
              ) : recent.map((n, i) => {
                const txt = notifyText(n, t);
                return (
                <div key={n.id} onClick={() => readRecent(n)}
                  style={{ display: 'flex', gap: 9, alignItems: 'flex-start', padding: '11px 14px', borderTop: i ? '1px solid var(--border)' : 'none',
                    cursor: n.read ? 'default' : 'pointer', background: n.read ? 'transparent' : 'var(--primary-soft)' }}>
                  <span style={{ marginTop: 5, flex: '0 0 auto', width: 7, height: 7, borderRadius: 4, background: n.read ? 'var(--border)' : (sevDot[n.severity] || 'var(--primary)') }} />
                  <div style={{ minWidth: 0, flex: 1 }}>
                    <div style={{ fontWeight: 700, fontSize: 13, marginBottom: 2, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{txt.title}</div>
                    <div className="muted" style={{ fontSize: 12, lineHeight: 1.4, display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden' }}>{txt.body}</div>
                  </div>
                </div>
                );
              })}
            </div>
            <button onClick={openCenter}
              style={{ width: '100%', padding: '11px 14px', border: 'none', borderTop: '1px solid var(--border)', background: 'var(--surface-2)',
                color: 'var(--primary)', fontWeight: 700, fontSize: 13, cursor: 'pointer' }}>
              {t('topbar.viewAll', { defaultValue: '알림센터에서 전체 보기' })}
            </button>
          </div>
        </>)}
      </div>

      {/* 언어 선택 — 40개 언어라 탑바 독립 드롭다운. 개인 항목(알림·설정·테마·로그아웃)은 아바타 메뉴로. */}
      <LanguageSwitcher />
      <UserMenu ns={ns} />
    </div>
  );
}
