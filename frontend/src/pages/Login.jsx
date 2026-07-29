import React, { useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { User, Lock } from 'lucide-react';
import { useAuth } from '../context/AuthContext';
import { useSystemConfig } from '../context/SystemConfigContext';
import LanguageSwitcher from '../components/LanguageSwitcher';
import { LogoMark } from '../components/BrandLogo';

const Login = () => {
  const { t } = useTranslation();
  const { loginLocal } = useAuth();
  const { config } = useSystemConfig();
  const navigate = useNavigate();
  const [form, setForm] = useState({ username: '', password: '' });
  const [isLoading, setIsLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState('');

  const handleChange = (e) => {
    const { name, value } = e.target;
    setForm((prev) => ({ ...prev, [name]: value }));
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setIsLoading(true);
    setErrorMessage('');
    try {
      await loginLocal(form.username, form.password);
      navigate('/', { replace: true });
    } catch (err) {
      // pending은 별도 안내 페이지로 이동 (가입 신청 일자 함께 표시)
      if (err.code === 'pending_approval') {
        navigate('/signup-pending', { replace: true, state: { createdAt: err.createdAt } });
        return;
      }
      const codeMap = {
        suspended: 'auth.login.errorSuspended',
        rejected: 'auth.login.errorRejected',
        unauthorized: 'auth.login.errorInvalid',
      };
      const key = codeMap[err.code];
      setErrorMessage(key ? t(key) : (err.message || t('auth.login.errorGeneric')));
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="auth-root">
      <div className="auth-card">
        <div className="auth-brand">
          <LogoMark size={50} style={config.branding?.accent ? { color: config.branding.accent } : undefined} />
          <span className="auth-name">
            <strong>{config.branding?.name?.trim() || 'Giosk'}</strong>
            <small>CONSOLE</small>
          </span>
        </div>

        <form onSubmit={handleSubmit}>
          <div className="auth-field has-icon">
            <User size={17} aria-hidden="true" />
            <input
              id="auth-username"
              type="text"
              name="username"
              value={form.username}
              onChange={handleChange}
              placeholder={t('auth.login.usernameLocal')}
              aria-label={t('auth.login.usernameLocal')}
              autoComplete="username"
              required
            />
          </div>

          <div className="auth-field has-icon">
            <Lock size={17} aria-hidden="true" />
            <input
              id="auth-password"
              type="password"
              name="password"
              value={form.password}
              onChange={handleChange}
              placeholder={t('auth.login.password')}
              aria-label={t('auth.login.password')}
              autoComplete="current-password"
              required
            />
          </div>

          {/* 로그인 실패는 화면이 바뀌지 않아 놓치기 쉽다 — 라이브 리전으로 읽어준다. */}
          {errorMessage && <div className="auth-note err" role="alert">{errorMessage}</div>}

          <button type="submit" className="auth-submit" disabled={isLoading}>
            {isLoading ? t('auth.login.submitting') : t('auth.login.submit')}
          </button>
        </form>

        {config.features.signupRequest && (
          <div style={{ marginTop: 16, textAlign: 'center' }}>
            <Link to="/signup-local" className="auth-link">
              {t('auth.login.signupLink')}
            </Link>
          </div>
        )}

        <div className="auth-foot">
          <LanguageSwitcher />
        </div>
      </div>
    </div>
  );
};

export default Login;
