import React, { createContext, useState, useContext, useEffect } from 'react';
import { apiGet, apiPost, setSessionKey, getSessionKey, getConsoleScope, setConsoleScope } from '../api/client';

const AuthContext = createContext(null);

const STORAGE_KEY = 'giosk_user';

export const AuthProvider = ({ children }) => {
  const [user, setUser] = useState(() => {
    const saved = localStorage.getItem(STORAGE_KEY);
    return saved ? JSON.parse(saved) : null;
  });

  // 멀티롤 활성 스코프("org:10"|"group:2"|null). null=기본(최우선 스코프, 백엔드가 scopes[0] 사용).
  const [activeScope, setActiveScopeState] = useState(() => getConsoleScope());

  // 가입 신청 완료 후 SignupPending 페이지에 표시할 메시지.
  const [pendingMessage, setPendingMessage] = useState(null);

  const persist = (u) => {
    setUser(u);
    if (u) localStorage.setItem(STORAGE_KEY, JSON.stringify(u));
    else localStorage.removeItem(STORAGE_KEY);
  };

  // 활성 스코프 전환 — localStorage(요청 헤더용) + 상태(리렌더용) 동기화.
  const setActiveScope = (s) => {
    setConsoleScope(s);
    setActiveScopeState(s || null);
  };

  // 세션키가 있으면 /me 로 사용자 정보를 갱신(새로고침 후 컨텍스트 복원).
  useEffect(() => {
    if (!getSessionKey()) return;
    apiGet('/auth/me').then(persist).catch(() => {
      // 세션 만료 → 로컬 상태 정리
      setSessionKey(null);
      persist(null);
    });
  }, []);

  // 자체 로그인 — 실 백엔드. 성공 시 세션키 보관 + 사용자 컨텍스트 저장.
  const loginLocal = async (username, password) => {
    const res = await apiPost('/auth/login', { username, password });
    setSessionKey(res.sessionkey);
    persist(res.user);
    return res.user;
  };

  // 자체 가입 신청 — 실 백엔드. 자동 로그인 없이 신청만 접수.
  const localSignup = async (form) => {
    await apiPost('/auth/signup', form);
    setPendingMessage('submitted');
    return { ok: true, message: 'submitted' };
  };

  const clearPendingMessage = () => setPendingMessage(null);

  const logout = async () => {
    try { await apiGet('/auth/logout'); } catch { /* 무시 */ }
    setSessionKey(null);
    setActiveScope(null); // 전환기 스코프도 초기화(다음 로그인 사용자와 섞이지 않도록)
    persist(null);
    setPendingMessage(null);
  };

  return (
    <AuthContext.Provider
      value={{
        user,
        activeScope,
        setActiveScope,
        pendingMessage,
        loginLocal,
        localSignup,
        clearPendingMessage,
        logout,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
};

export const useAuth = () => useContext(AuthContext);
