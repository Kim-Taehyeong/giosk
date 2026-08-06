import React, { useEffect } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Clock, CalendarDays } from 'lucide-react';

import { useAuth } from '../context/AuthContext';
import LanguageSwitcher from '../components/LanguageSwitcher';

const formatDate = (iso, lang) => {
  if (!iso) return null;
  try {
    const d = new Date(iso);
    return d.toLocaleString(lang || undefined, {
      year: 'numeric', month: 'long', day: 'numeric',
      hour: '2-digit', minute: '2-digit',
    });
  } catch {
    return iso;
  }
};

const SignupPending = () => {
  const { t, i18n } = useTranslation();
  const { clearPendingMessage } = useAuth();
  const location = useLocation();
  const createdAt = location.state?.createdAt;

  useEffect(() => () => clearPendingMessage(), [clearPendingMessage]);

  return (
    <div className="auth-root">
      <div className="auth-card" style={{ textAlign: 'center' }}>
        {/* 대기 상태 = 앰버. 색만으로 알리지 않도록 아이콘과 문구가 함께 말한다. */}
        <div style={{
          display: 'inline-grid', placeItems: 'center', width: 62, height: 62, borderRadius: '50%',
          background: 'var(--warn-soft)', color: 'var(--warn)', marginBottom: 18,
        }}>
          <Clock size={30} />
        </div>
        <h1 className="auth-title" style={{ fontSize: 22 }}>{t('auth.signupPending.title')}</h1>
        <p className="auth-sub">{t('auth.signupPending.body')}</p>

        {createdAt && (
          <div style={{
            display: 'inline-flex', alignItems: 'center', gap: 8, marginBottom: 26,
            padding: '9px 14px', background: 'var(--surface-2)', border: '1px solid var(--border)',
            borderRadius: 10, fontSize: 13,
          }}>
            <CalendarDays size={15} style={{ color: 'var(--muted)', flex: '0 0 auto' }} />
            <span className="muted" style={{ color: 'var(--muted)' }}>{t('auth.signupPending.submittedAt')}</span>
            <span style={{ fontWeight: 700 }}>{formatDate(createdAt, i18n.language)}</span>
          </div>
        )}

        <div>
          <Link to="/login" className="auth-submit" style={{ display: 'inline-block', width: 'auto', textDecoration: 'none' }}>
            {t('auth.signupPending.backToLogin')}
          </Link>
        </div>

        <div className="auth-foot">
          <LanguageSwitcher />
        </div>
      </div>
    </div>
  );
};

export default SignupPending;
