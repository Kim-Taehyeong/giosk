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
  } catch (_) {
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
    <div className="min-h-screen flex items-center justify-center bg-white px-4">
      <div className="max-w-md w-full text-center">
        <div className="inline-flex items-center justify-center w-16 h-16 rounded-full bg-yellow-100 text-yellow-600 mb-5">
          <Clock size={32} />
        </div>
        <h1 className="text-2xl font-bold text-gray-900 mb-3">{t('auth.signupPending.title')}</h1>
        <p className="text-gray-600 mb-6 leading-relaxed">{t('auth.signupPending.body')}</p>

        {createdAt && (
          <div className="inline-flex items-center gap-2 mb-8 px-4 py-2 bg-gray-50 border border-gray-200 rounded-lg text-sm text-gray-700">
            <CalendarDays size={16} className="text-gray-400" />
            <span className="text-gray-500">{t('auth.signupPending.submittedAt')}:</span>
            <span className="font-medium text-gray-900">{formatDate(createdAt, i18n.language)}</span>
          </div>
        )}

        <div>
          <Link
            to="/login"
            className="inline-block px-6 py-2 bg-blue-600 text-white rounded-md font-medium hover:bg-blue-700"
          >
            {t('auth.signupPending.backToLogin')}
          </Link>
        </div>

        <div className="mt-8 flex justify-center">
          <LanguageSwitcher />
        </div>
      </div>
    </div>
  );
};

export default SignupPending;
