import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { KeyRound, Palette, User as UserIcon, Building2, Lock } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import Pill from '../../../components/console/Pill';
import { useAuth } from '../../../context/AuthContext';
import { useTheme } from '../../../context/ThemeContext';
import { useConsole } from '../../../context/ConsoleContext';
import { useToast } from '../../../components/console/Toast';
import { apiPut } from '../../../api/client';

const ROLE_LABEL = { org_admin: 'Org Admin', project_admin: 'Group Admin', member: 'Member' };

export default function Account() {
  const { t } = useTranslation('consoleUser');
  const { user } = useAuth();
  const { theme, setTheme } = useTheme();
  const { activeGroup } = useConsole();
  const { toast } = useToast();
  const [sshKey, setSshKey] = useState(() => localStorage.getItem('giosk_sshkey') || '');
  const orgName = activeGroup?.orgName || user?.orgName || '—';
  const groupName = activeGroup?.displayName || activeGroup?.name || user?.groupName;
  const memberRole = activeGroup?.role || user?.membershipRole || 'member';

  const saveSsh = () => { localStorage.setItem('giosk_sshkey', sshKey); toast(t('account.sshSaved')); };

  const [pw, setPw] = useState({ current: '', next: '', confirm: '' });
  const changePw = async () => {
    if (pw.next.length < 8) { toast(t('account.pwShort')); return; }
    if (pw.next !== pw.confirm) { toast(t('account.pwMismatch')); return; }
    try {
      await apiPut('/auth/me/password', { current: pw.current, new: pw.next });
      setPw({ current: '', next: '', confirm: '' });
      toast(t('account.pwChanged'));
    } catch (e) {
      toast(e?.code === 'wrong_password' ? t('account.pwWrong') : (e?.message || t('account.pwFail')));
    }
  };

  return (
    <div>
      <PageHead title={t('account.title')} subtitle={t('account.subtitle')} />

      <div className="grid cols-2 mb">
        <div className="card">
          <h3><UserIcon size={16} /> {t('account.profile')}</h3>
          <div className="kv">
            <span className="k">{t('account.name')}</span><span>{user?.firstName || user?.username}</span>
            <span className="k">{t('account.account')}</span><span>{user?.username}</span>
            <span className="k">{t('account.email')}</span><span>{user?.email || '—'}</span>
            <span className="k">{t('account.role')}</span><span>{user?.role || 'member'}</span>
            <span className="k">{t('account.provider')}</span><span>{user?.provider || 'local'}</span>
          </div>
        </div>

        <div className="card">
          <h3><Building2 size={16} /> {t('account.membership')}</h3>
          {(user?.scopes || []).length > 0 ? (
            <div>
              {user.scopes.map((s, i) => {
                const isOrg = s.level === 'org';
                const isActive = activeGroup && (isOrg
                  ? (activeGroup.level === 'org' && activeGroup.orgId === s.orgId)
                  : (activeGroup.groupId === s.groupId));
                return (
                  <div key={i} className="flex" style={{ justifyContent: 'space-between', alignItems: 'center', gap: 8, padding: '10px 0', borderTop: i ? '1px solid var(--border)' : 'none' }}>
                    <span style={{ minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      <span style={{ fontWeight: 600 }}>{isOrg ? s.orgName : s.groupName}</span>
                      {!isOrg && s.orgName && <span className="muted" style={{ fontSize: 12 }}> · {s.orgName}</span>}
                    </span>
                    <span className="flex" style={{ gap: 6, alignItems: 'center', flex: '0 0 auto' }}>
                      {isActive && <Pill variant="free">{t('account.activeScope', { defaultValue: '활성' })}</Pill>}
                      <Pill variant="primary">{ROLE_LABEL[s.role] || (isOrg ? t('topbar.scopeOrg') : t('account.memberRole'))}</Pill>
                    </span>
                  </div>
                );
              })}
            </div>
          ) : (
            <div className="kv">
              <span className="k">{t('account.org')}</span><span>{orgName}</span>
              <span className="k">{t('account.group')}</span>
              <span>{groupName ? groupName : <span className="muted">{t('account.noGroup')}</span>}</span>
              <span className="k">{t('account.memberRole')}</span>
              <span><Pill variant="primary">{ROLE_LABEL[memberRole] || memberRole}</Pill></span>
            </div>
          )}
        </div>
      </div>

      <div className="card mb">
        <h3><Lock size={16} /> {t('account.pwTitle')}</h3>
        <div className="grid cols-3" style={{ gap: 12 }}>
          <div>
            <label className="fld" style={{ marginTop: 0 }}>{t('account.pwCurrent')}</label>
            <input type="password" autoComplete="current-password" value={pw.current} onChange={(e) => setPw({ ...pw, current: e.target.value })} />
          </div>
          <div>
            <label className="fld" style={{ marginTop: 0 }}>{t('account.pwNew')}</label>
            <input type="password" autoComplete="new-password" value={pw.next} onChange={(e) => setPw({ ...pw, next: e.target.value })} />
          </div>
          <div>
            <label className="fld" style={{ marginTop: 0 }}>{t('account.pwConfirm')}</label>
            <input type="password" autoComplete="new-password" value={pw.confirm} onChange={(e) => setPw({ ...pw, confirm: e.target.value })} />
          </div>
        </div>
        <div className="legend">{t('account.pwHint')}</div>
        <div className="mt"><button className="btn primary" disabled={!pw.current || !pw.next} onClick={changePw}>{t('account.pwChange')}</button></div>
      </div>

      <div className="card mb">
        <h3><KeyRound size={16} /> {t('account.ssh')}</h3>
        <textarea value={sshKey} onChange={(e) => setSshKey(e.target.value)} placeholder="ssh-ed25519 AAAA..." />
        <div className="legend">{t('account.sshHint')}</div>
        <div className="mt"><button className="btn primary" onClick={saveSsh}>{t('account.save')}</button></div>
      </div>

      {/* 물리노드 로컬 Home 은 '데이터·볼륨 > 로컬 Home' 으로 이동(노드별 hostPath 특수 볼륨). 내정보에서는 제거. */}

      <div className="card">
        <h3><Palette size={16} /> {t('account.theme')}</h3>
        <div className="flex gap">
          <button className={`btn${theme === 'light' ? ' primary' : ''}`} onClick={() => setTheme('light')}>☀ {t('account.light')}</button>
          <button className={`btn${theme === 'dark' ? ' primary' : ''}`} onClick={() => setTheme('dark')}>🌙 {t('account.dark')}</button>
        </div>
      </div>
    </div>
  );
}
