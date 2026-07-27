import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Coins, CheckCheck, UserPlus } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import Pill from '../../../components/console/Pill';
import { useToast } from '../../../components/console/Toast';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { getTopupRequests, approveTopup, rejectTopup, getUsers, updateUserStatus } from '../../../api/console/misc';
import { c } from '../../../lib/credit';

export default function Approvals() {
  const { t } = useTranslation('consoleAdmin');
  const { toast } = useToast();
  const { config } = useSystemConfig();
  const creditMode = config.billing.mode === 'credit';
  const signupOn = config.features.signupRequest;
  const topupOn = creditMode && config.features.creditRequest;
  const [topups, setTopups] = useState([]);
  const [signups, setSignups] = useState([]);
  const [signupTotal, setSignupTotal] = useState(0);
  const load = () => {
    getTopupRequests().then((r) => setTopups((r.items || []).filter((x) => x.status === 'pending')));
    getUsers({ status: 'pending', size: 200 }).then((r) => { setSignups(r.items); setSignupTotal(r.total); });
  };
  useEffect(() => { load(); }, []);

  const onApprove = async (r) => { await approveTopup(r.id); setTopups((p) => p.filter((x) => x.id !== r.id)); toast(t('dash.topupApproved', { name: r.targetName, n: r.amount })); };
  const onReject = async (r) => { await rejectTopup(r.id); setTopups((p) => p.filter((x) => x.id !== r.id)); toast(t('dash.topupRejected', { name: r.targetName })); };
  const onApproveUser = async (u) => { await updateUserStatus(u.id, 'approved'); setSignups((p) => p.filter((x) => x.id !== u.id)); toast(t('approvals.userApproved', { name: u.name })); };
  const onRejectUser = async (u) => { await updateUserStatus(u.id, 'rejected'); setSignups((p) => p.filter((x) => x.id !== u.id)); toast(t('approvals.userRejected', { name: u.name })); };

  return (
    <div>
      <PageHead crumb={t('badge')} title={t('approvals.title')} subtitle={t('approvals.subtitle')} />

      {/* 사용자 가입 신청 — 가입 신청 기능이 켜진 경우만 노출 */}
      {signupOn && (
      <div className="card mb">
        <h3><UserPlus size={16} /> {t('approvals.signupReq')}
          {signupTotal > 0 && <span className="muted right" style={{ fontSize: 12 }}>{t('dash.pendingN', { n: signupTotal })}</span>}</h3>
        {signups.length === 0 && (
          <div className="muted" style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '24px 0', justifyContent: 'center' }}>
            <CheckCheck size={18} /> {t('approvals.noSignup')}
          </div>
        )}
        {signups.map((u) => (
          <div key={u.id} className="avail">
            <span className="flex gap" style={{ flexWrap: 'wrap' }}>
              <strong>{u.name}</strong>
              <span className="muted">{u.email || u.username}</span>
              <Pill variant="pause">{u.role}</Pill>
              <span className="muted">· {u.org} / {u.group}</span>
              {u.advisor && <span className="muted">· {t('approvals.advisor', { name: u.advisor })}</span>}
              {u.createdAt && <span className="muted">· {u.createdAt}</span>}
            </span>
            <span className="flex gap">
              <button className="btn sm ok" onClick={() => onApproveUser(u)}>{t('approvals.approve')}</button>
              <button className="btn sm danger" onClick={() => onRejectUser(u)}>{t('approvals.reject')}</button>
            </span>
          </div>
        ))}
      </div>
      )}

      {/* 조직 예산 충전 요청 (조직→플랫폼) — 크레딧 모드 + 충전 요청 허용 시 */}
      {topupOn && (
      <div className="card">
        <h3><Coins size={16} /> {t('approvals.topupReq')}
          {topups.length > 0 && <span className="muted right" style={{ fontSize: 12 }}>{t('dash.pendingN', { n: topups.length })}</span>}</h3>
        {topups.length === 0 && (
          <div className="muted" style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '24px 0', justifyContent: 'center' }}>
            <CheckCheck size={18} /> {t('approvals.noPending')}
          </div>
        )}
        {topups.map((r) => (
          <div key={r.id} style={{ display: 'flex', gap: 16, alignItems: 'flex-start', padding: '16px 0', borderTop: '1px solid var(--border)' }}>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div className="flex gap" style={{ alignItems: 'center', flexWrap: 'wrap' }}>
                <Pill variant="primary">{r.targetType === 'org' ? t('approvals.tOrg') : r.targetType === 'group' ? t('dash.tGroup') : t('dash.tUser')}</Pill>
                <strong style={{ fontSize: 14.5 }}>{r.targetName}</strong>
                {r.orgName && <span className="muted" style={{ fontSize: 12.5 }}>· {r.orgName}</span>}
                <span style={{ fontWeight: 800, color: 'var(--primary)', fontSize: 15 }}>+{c(r.amount)} C</span>
                {r.requesterName && <span className="muted" style={{ fontSize: 12.5 }}>{t('approvals.by', { name: r.requesterName })}{r.requesterRole ? ` · ${r.requesterRole}` : ''}</span>}
                {r.createdAt && <span className="muted" style={{ fontSize: 12.5 }}>· {r.createdAt}</span>}
              </div>
              <div style={{ marginTop: 8, padding: '10px 12px', borderRadius: 10, background: 'var(--surface-2)', border: '1px solid var(--border)' }}>
                <div className="muted" style={{ fontSize: 11.5, fontWeight: 700, marginBottom: 3 }}>{t('approvals.reason')}</div>
                <div style={{ fontSize: 13.5, lineHeight: 1.55, whiteSpace: 'pre-wrap' }}>{r.reason || t('approvals.noReason')}</div>
              </div>
            </div>
            <span className="flex gap" style={{ flex: '0 0 auto', paddingTop: 2 }}>
              <button className="btn sm ok" onClick={() => onApprove(r)}>{t('dash.approve')}</button>
              <button className="btn sm danger" onClick={() => onReject(r)}>{t('dash.reject')}</button>
            </span>
          </div>
        ))}
      </div>
      )}
    </div>
  );
}
