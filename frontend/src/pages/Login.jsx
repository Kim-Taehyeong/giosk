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
    <div className="min-h-screen flex flex-col items-center justify-center bg-white">
      <div className="w-full max-w-md px-8">
        <div className="flex justify-center mb-10">
          <div className="flex items-center gap-3">
            <LogoMark size={52} style={{ color: config.branding?.accent || '#2563eb' }} />
            <div className="flex flex-col leading-none">
              <span className="text-blue-900 font-black text-4xl tracking-tight">{config.branding?.name?.trim() || 'Giosk'}</span>
              <span className="text-blue-700 text-sm font-semibold tracking-[0.3em] mt-1">CONSOLE</span>
            </div>
          </div>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4 mt-2">
          <div className="relative">
            <User className="absolute left-3 top-3 text-gray-400 w-5 h-5" />
            <input
              type="text"
              name="username"
              value={form.username}
              onChange={handleChange}
              placeholder={t('auth.login.usernameLocal')}
              autoComplete="username"
              required
              className="w-full pl-10 pr-4 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </div>

          <div className="relative">
            <Lock className="absolute left-3 top-3 text-gray-400 w-5 h-5" />
            <input
              type="password"
              name="password"
              value={form.password}
              onChange={handleChange}
              placeholder={t('auth.login.password')}
              autoComplete="current-password"
              required
              className="w-full pl-10 pr-4 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </div>

          {errorMessage && (
            <div className="text-red-500 text-sm text-center font-medium">{errorMessage}</div>
          )}

          <button
            type="submit"
            disabled={isLoading}
            className={`w-full text-white py-2 rounded-md transition duration-200 font-bold ${
              isLoading ? 'bg-blue-400 cursor-not-allowed' : 'bg-blue-600 hover:bg-blue-700'
            }`}
          >
            {isLoading ? t('auth.login.submitting') : t('auth.login.submit')}
          </button>
        </form>

        {config.features.signupRequest && (
          <div className="mt-4 text-center">
            <Link to="/signup-local" className="text-sm text-blue-600 hover:underline">
              {t('auth.login.signupLink')}
            </Link>
          </div>
        )}

        <div className="mt-6 flex justify-center">
          <LanguageSwitcher />
        </div>
      </div>
    </div>
  );
};

export default Login;
