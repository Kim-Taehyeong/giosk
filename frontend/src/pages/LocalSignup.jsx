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

  const inputClass =
    'w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500';

  // 가입 신청 기능이 꺼져 있으면 접근 차단.
  if (!config.features.signupRequest) return <Navigate to="/login" replace />;

  return (
    <div className="min-h-screen flex flex-col items-center bg-white py-10">
      <div className="w-full max-w-2xl px-8">
        <div className="mb-6 flex items-center justify-between">
          <Link to="/login" className="text-sm text-blue-600 hover:underline inline-flex items-center gap-1">
            <ArrowLeft size={16} /> {t('auth.signupPending.backToLogin')}
          </Link>
          <LanguageSwitcher />
        </div>

        <div className="text-center mb-8">
          <h1 className="text-2xl font-bold text-blue-900">{t('auth.localSignup.title')}</h1>
          <p className="text-gray-500 text-sm mt-2">{t('auth.localSignup.subtitle')}</p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-6">
          <section className="border rounded-md p-4">
            <h2 className="font-medium mb-3">{t('auth.localSignup.section.credentials')}</h2>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-sm text-gray-600 mb-1">{t('auth.localSignup.field.username')}</label>
                <input type="text" name="username" value={form.username} onChange={handleChange} required className={inputClass} />
              </div>
              <div>
                <label className="block text-sm text-gray-600 mb-1">{t('auth.localSignup.field.email')}</label>
                <input type="email" name="email" value={form.email} onChange={handleChange} required className={inputClass} />
              </div>
              <div className="col-span-2">
                <label className="block text-sm text-gray-600 mb-1">{t('auth.localSignup.field.password')}</label>
                <input type="password" name="password" value={form.password} onChange={handleChange} minLength={8} required className={inputClass} autoComplete="new-password" />
              </div>
              <div className="col-span-2">
                <label className="block text-sm text-gray-600 mb-1">{t('auth.localSignup.field.passwordConfirm', { defaultValue: '비밀번호 확인' })}</label>
                <input type="password" value={pwConfirm} onChange={(e) => setPwConfirm(e.target.value)} required autoComplete="new-password"
                  className={inputClass}
                  style={pwConfirm ? { borderColor: form.password === pwConfirm ? '#16a34a' : '#dc2626', boxShadow: `0 0 0 2px ${form.password === pwConfirm ? '#16a34a22' : '#dc262622'}` } : undefined} />
                {pwConfirm && (
                  <div style={{ display: 'flex', alignItems: 'center', gap: 5, marginTop: 5, fontSize: 12.5, fontWeight: 600,
                    color: form.password === pwConfirm ? '#16a34a' : '#dc2626' }}>
                    {form.password === pwConfirm
                      ? <><Check size={14} /> {t('auth.localSignup.pwMatch', { defaultValue: '비밀번호가 일치합니다' })}</>
                      : <><X size={14} /> {t('auth.localSignup.pwMismatch', { defaultValue: '비밀번호가 일치하지 않습니다' })}</>}
                  </div>
                )}
              </div>
            </div>
          </section>

          <section className="border rounded-md p-4">
            <h2 className="font-medium mb-3">{t('auth.signup.section.name')}</h2>
            <div>
              <label className="block text-sm text-gray-600 mb-1">{t('auth.signup.field.name')}</label>
              <input type="text" name="name" value={form.name} onChange={handleChange} required className={inputClass} placeholder={t('auth.signup.field.namePlaceholder')} />
            </div>
          </section>

          <section className="border rounded-md p-4">
            <h2 className="font-medium mb-3">{t('auth.localSignup.section.affiliation')}</h2>

            {/* 조직 / 그룹 — 관리자가 등록한 목록에서 선택 */}
            {orgs.length > 0 && (
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-sm text-gray-600 mb-1">{t('auth.signup.field.org')} <span className="text-red-500">*</span></label>
                  <select value={orgId} onChange={(e) => { setOrgId(e.target.value); setForm((p) => ({ ...p, groupId: '' })); }} required className={inputClass}>
                    <option value="">{t('auth.signup.field.selectPh')}</option>
                    {orgs.map((o) => <option key={o.id} value={o.id}>{o.displayName || o.name}</option>)}
                  </select>
                </div>
                <div>
                  <label className="block text-sm text-gray-600 mb-1">{t('auth.signup.field.group')} <span className="text-red-500">*</span></label>
                  <select name="groupId" value={form.groupId} onChange={handleChange} disabled={!selectedOrg} required className={inputClass}>
                    <option value="">{t('auth.signup.field.selectPh')}</option>
                    {(selectedOrg?.groups || []).map((g) => <option key={g.id} value={g.id}>{g.displayName || g.name}</option>)}
                  </select>
                  {/* 가입 가능한 그룹이 하나도 없는 조직 — 빈 필수 셀렉트로 막히지 않게 이유를 알린다. */}
                  {selectedOrg && (selectedOrg.groups || []).length === 0 && (
                    <p className="mt-1 text-xs text-amber-600">{t('auth.signup.field.noJoinableGroup')}</p>
                  )}
                </div>
              </div>
            )}

            {/* 담당자 — 신청을 책임지는 사람(지도교수·팀장 등). 선택 사항 */}
            <div className="mt-3">
              <label className="block text-sm text-gray-600 mb-1">{t('auth.signup.field.sponsor')}</label>
              <input
                type="text"
                name="sponsor"
                value={form.sponsor}
                onChange={handleChange}
                className={inputClass}
                placeholder={t('auth.signup.field.sponsorPlaceholder')}
              />
            </div>
          </section>

          <section className="border rounded-md p-4">
            <label className="flex items-start gap-2 text-sm">
              <input
                type="checkbox"
                name="termsAccepted"
                checked={form.termsAccepted}
                onChange={handleChange}
                className="mt-1"
              />
              <span>
                {t('auth.signup.terms')}{' '}
                <a href="/terms.html" target="_blank" rel="noreferrer" className="text-blue-600 underline">{t('auth.signup.termsView')}</a>
              </span>
            </label>
          </section>

          {errorMessage && (
            <div className="text-red-500 text-sm text-center font-medium">{errorMessage}</div>
          )}

          <button
            type="submit"
            disabled={isLoading}
            className={`w-full text-white py-2 rounded-md font-bold ${
              isLoading ? 'bg-blue-400 cursor-not-allowed' : 'bg-blue-600 hover:bg-blue-700'
            }`}
          >
            {isLoading ? t('auth.localSignup.submitting') : t('auth.localSignup.submit')}
          </button>
        </form>
      </div>
    </div>
  );
};

export default LocalSignup;
