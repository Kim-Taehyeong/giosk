import React, { useState, useEffect } from 'react';
import { useNavigate, Link, Navigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { ArrowLeft, Check, X } from 'lucide-react';

import { useAuth } from '../context/AuthContext';
import { useSystemConfig } from '../context/SystemConfigContext';
import LanguageSwitcher from '../components/LanguageSwitcher';
import { apiGet } from '../api/client';

const LocalSignup = () => {
  const { t } = useTranslation();
  const { localSignup } = useAuth();
  const { config } = useSystemConfig();
  const navigate = useNavigate();

  const [form, setForm] = useState({
    username: '',
    password: '',
    email: '',
    name: '', // 단일 이름(성+이름 분리 입력 제거)
    sponsor: '',
    groupId: '',
    termsAccepted: false,
  });
  const [isLoading, setIsLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState('');
  const [pwConfirm, setPwConfirm] = useState(''); // 비밀번호 확인

  // 관리자가 등록한 조직/그룹 공개 카탈로그(실 DB: GET /public/orgs).
  const [orgs, setOrgs] = useState([]);
  const [orgId, setOrgId] = useState('');
  useEffect(() => { apiGet('/public/orgs').then((d) => setOrgs(d.items || [])).catch(() => setOrgs([])); }, []);
  const selectedOrg = orgs.find((o) => String(o.id) === String(orgId));

  const handleChange = (e) => {
    const { name, value, type, checked } = e.target;
    setForm((prev) => ({ ...prev, [name]: type === 'checkbox' ? checked : value }));
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setErrorMessage('');

    if (!form.username || !form.password || !form.email || !form.name) {
      setErrorMessage(t('auth.localSignup.errorRequired'));
      return;
    }
    if (form.password.length < 8) {
      setErrorMessage(t('auth.localSignup.errorPasswordShort'));
      return;
    }
    if (form.password !== pwConfirm) {
      setErrorMessage(t('auth.localSignup.errorPasswordMismatch', { defaultValue: '비밀번호가 일치하지 않습니다.' }));
      return;
    }
    if (!form.termsAccepted) {
      setErrorMessage(t('auth.localSignup.errorTerms'));
      return;
    }
    // 그룹 가입 필수 — 조직/그룹 목록이 있으면 반드시 그룹을 선택해야 함
    if (orgs.length > 0 && !form.groupId) {
      setErrorMessage(t('auth.localSignup.errorGroup'));
      return;
    }

    setIsLoading(true);
    try {
      // 단일 이름은 lastName 에 담아 전송(표시는 CONCAT(last_name, first_name) → 동일하게 노출).
      const { name, ...rest } = form;
      await localSignup({ ...rest, lastName: name, firstName: '', groupId: form.groupId ? Number(form.groupId) : null });
      navigate('/signup-pending', { replace: true });
    } catch (err) {
      setErrorMessage(err.message || t('auth.localSignup.errorGeneric'));
    } finally {
      setIsLoading(false);
    }
  };

  // 가입 신청 기능이 꺼져 있으면 접근 차단.
  if (!config.features.signupRequest) return <Navigate to="/login" replace />;

  const pwOk = form.password === pwConfirm;
  const card = { border: '1px solid var(--border)', borderRadius: 12, padding: 18, background: 'var(--surface)', marginBottom: 16 };
  const two = { display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 14 };

  return (
    <div className="auth-root" style={{ alignItems: 'start', paddingTop: 40 }}>
      <div className="auth-card" style={{ maxWidth: 640 }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 22, gap: 12 }}>
          <Link to="/login" className="auth-link" style={{ display: 'inline-flex', alignItems: 'center', gap: 5 }}>
            <ArrowLeft size={15} /> {t('auth.signupPending.backToLogin')}
          </Link>
          <LanguageSwitcher />
        </div>

        <div style={{ textAlign: 'center', marginBottom: 26 }}>
          <h1 className="auth-title" style={{ fontSize: 23 }}>{t('auth.localSignup.title')}</h1>
          <p className="auth-sub" style={{ marginBottom: 0 }}>{t('auth.localSignup.subtitle')}</p>
        </div>

        <form onSubmit={handleSubmit}>
          <section style={card}>
            <h2 style={{ fontSize: 15.5, fontWeight: 700, marginBottom: 4 }}>{t('auth.localSignup.section.credentials')}</h2>
            <div style={two}>
              <div>
                <label htmlFor="signup-username">{t('auth.localSignup.field.username')}</label>
                <input id="signup-username" type="text" name="username" value={form.username} onChange={handleChange} required />
              </div>
              <div>
                <label htmlFor="signup-email">{t('auth.localSignup.field.email')}</label>
                <input id="signup-email" type="email" name="email" value={form.email} onChange={handleChange} required />
              </div>
            </div>
            <label htmlFor="signup-password">{t('auth.localSignup.field.password')}</label>
            <input id="signup-password" type="password" name="password" value={form.password} onChange={handleChange} minLength={8} required autoComplete="new-password" />

            <label htmlFor="signup-password2">{t('auth.localSignup.field.passwordConfirm', { defaultValue: '비밀번호 확인' })}</label>
            <input id="signup-password2" type="password" value={pwConfirm} onChange={(e) => setPwConfirm(e.target.value)} required autoComplete="new-password"
              aria-invalid={pwConfirm ? !pwOk : undefined}
              aria-describedby={pwConfirm ? 'signup-password2-note' : undefined}
              style={pwConfirm ? {
                borderColor: pwOk ? 'var(--free)' : 'var(--danger)',
                boxShadow: `0 0 0 3px ${pwOk ? 'var(--free-soft)' : 'var(--danger-soft)'}`,
              } : undefined} />
            {/* 일치 여부는 색만이 아니라 아이콘과 문구로도 말한다. */}
            {pwConfirm && (
              <div id="signup-password2-note" role="status" style={{
                display: 'flex', alignItems: 'center', gap: 5, marginTop: 6, fontSize: 12.5, fontWeight: 600,
                color: pwOk ? 'var(--free)' : 'var(--danger)',
              }}>
                {pwOk
                  ? <><Check size={14} /> {t('auth.localSignup.pwMatch', { defaultValue: '비밀번호가 일치합니다' })}</>
                  : <><X size={14} /> {t('auth.localSignup.pwMismatch', { defaultValue: '비밀번호가 일치하지 않습니다' })}</>}
              </div>
            )}
          </section>

          <section style={card}>
            <h2 style={{ fontSize: 15.5, fontWeight: 700, marginBottom: 4 }}>{t('auth.signup.section.name')}</h2>
            <label htmlFor="signup-name">{t('auth.signup.field.name')}</label>
            <input id="signup-name" type="text" name="name" value={form.name} onChange={handleChange} required placeholder={t('auth.signup.field.namePlaceholder')} />
          </section>

          <section style={card}>
            <h2 style={{ fontSize: 15.5, fontWeight: 700, marginBottom: 4 }}>{t('auth.localSignup.section.affiliation')}</h2>

            {/* 조직 / 그룹 — 관리자가 등록한 목록에서 선택 */}
            {orgs.length > 0 && (
              <div style={two}>
                <div>
                  <label htmlFor="signup-org">{t('auth.signup.field.org')} <span style={{ color: 'var(--danger)' }}>*</span></label>
                  <select id="signup-org" value={orgId} onChange={(e) => { setOrgId(e.target.value); setForm((p) => ({ ...p, groupId: '' })); }} required>
                    <option value="">{t('auth.signup.field.selectPh')}</option>
                    {orgs.map((o) => <option key={o.id} value={o.id}>{o.displayName || o.name}</option>)}
                  </select>
                </div>
                <div>
                  <label htmlFor="signup-group">{t('auth.signup.field.group')} <span style={{ color: 'var(--danger)' }}>*</span></label>
                  <select id="signup-group" name="groupId" value={form.groupId} onChange={handleChange} disabled={!selectedOrg} required>
                    <option value="">{t('auth.signup.field.selectPh')}</option>
                    {(selectedOrg?.groups || []).map((g) => <option key={g.id} value={g.id}>{g.displayName || g.name}</option>)}
                  </select>
                  {/* 가입 가능한 그룹이 하나도 없는 조직 — 빈 필수 셀렉트로 막히지 않게 이유를 알린다. */}
                  {selectedOrg && (selectedOrg.groups || []).length === 0 && (
                    <p style={{ marginTop: 5, fontSize: 12, color: 'var(--warn)', fontWeight: 600 }}>{t('auth.signup.field.noJoinableGroup')}</p>
                  )}
                </div>
              </div>
            )}

            {/* 담당자 — 신청을 책임지는 사람(지도교수·팀장 등). 선택 사항 */}
            <label htmlFor="signup-sponsor">{t('auth.signup.field.sponsor')}</label>
            <input id="signup-sponsor" type="text" name="sponsor" value={form.sponsor} onChange={handleChange}
              placeholder={t('auth.signup.field.sponsorPlaceholder')} />
          </section>

          <section style={card}>
            <label style={{ display: 'flex', alignItems: 'flex-start', gap: 9, margin: 0, fontSize: 13.5, fontWeight: 500 }}>
              <input type="checkbox" name="termsAccepted" checked={form.termsAccepted} onChange={handleChange}
                style={{ width: 'auto', marginTop: 3, accentColor: 'var(--primary)' }} />
              <span>
                {t('auth.signup.terms')}{' '}
                <a href="/terms.html" target="_blank" rel="noreferrer" className="auth-link">{t('auth.signup.termsView')}</a>
              </span>
            </label>
          </section>

          {errorMessage && <div className="auth-note err" role="alert">{errorMessage}</div>}

          <button type="submit" className="auth-submit" disabled={isLoading}>
            {isLoading ? t('auth.localSignup.submitting') : t('auth.localSignup.submit')}
          </button>
        </form>
      </div>
    </div>
  );
};

export default LocalSignup;
