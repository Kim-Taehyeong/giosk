import React, { createContext, useState, useContext, useEffect } from 'react';
import { apiGet, apiPost, setSessionKey, getSessionKey, getScopeForRoute, setScopeForRoute, setConsoleScope, setUserScope } from '../api/client';

const AuthContext = createContext(null);

const STORAGE_KEY = 'giosk_user';

export const AuthProvider = ({ children }) => {
  const [user, setUser] = useState(() => {
    const saved = localStorage.getItem(STORAGE_KEY);
    return saved ? JSON.parse(saved) : null;
  });

  // 활성 스코프("org:10"|"group:2"|null). 현재 콘솔(관리자/사용자)에 맞는 키에서 초기화한다
  // — 관리자 콘솔과 사용자 콘솔이 스코프를 공유하지 않도록(콘솔 오염 방지). null=기본(백엔드 scopes[0]).
  const [activeScope, setActiveScopeState] = useState(() => getScopeForRoute());

  // 가입 신청 완료 후 SignupPending 페이지에 표시할 메시지.
  const [pendingMessage, setPendingMessage] = useState(null);

  const persist = (u) => {
    setUser(u);
    if (u) localStorage.setItem(STORAGE_KEY, JSON.stringify(u));
    else localStorage.removeItem(STORAGE_KEY);
  };

  // 활성 스코프 전환 — 현재 콘솔에 맞는 localStorage 키(요청 헤더용) + 상태(리렌더용) 동기화.
  const setActiveScope = (s) => {
    setScopeForRoute(s);
    setActiveScopeState(s || null);
  };
  // 콘솔 간 이동 시(사용자↔관리자) 그 콘솔 전용 키에서 스코프를 다시 로드 — 상태가 다른 콘솔 값을
  // 물고 넘어오는 것(SPA 이동 누수) 방지. ConsoleLayout 이 variant 변경마다 호출한다.
  const reloadScopeForConsole = () => setActiveScopeState(getScopeForRoute() || null);

  // 세션키가 있으면 /me 로 사용자 정보를 갱신(새로고침 후 컨텍스트 복원).
  useEffect(() => {
    if (!getSessionKey()) return;
    apiGet('/auth/me').then(persist).catch(() => {
      // 세션 만료 → 로컬 상태 정리
      setSessionKey(null);
      persist(null);
    });
  }, []);

  // /me 재조회 — 프로필 변경(SSH 공개키 등록 등) 직후 컨텍스트를 최신화한다. 갱신된 사용자 반환.
  const refreshUser = async () => {
    if (!getSessionKey()) return null;
    const me = await apiGet('/auth/me');
    persist(me);
    return me;
  };

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
    setConsoleScope(null); setUserScope(null); // 두 콘솔 스코프 모두 초기화(다음 로그인 사용자와 섞이지 않도록)
    setActiveScopeState(null);
    persist(null);
    setPendingMessage(null);
  };

  return (
    <AuthContext.Provider
      value={{
        user,
        activeScope,
        setActiveScope,
        reloadScopeForConsole,
        refreshUser,
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
