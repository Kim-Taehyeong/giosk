import React, { createContext, useContext, useState, useCallback } from 'react';
import { setUserScope } from '../api/client';

// 콘솔 전역 선택 상태:
//  - admin: 선택 클러스터 / user: 선택 그룹(org/group)
//  - 크레딧/노드 요약(탑바 배지용)
const ConsoleContext = createContext(null);

const GROUP_KEY = 'giosk_active_group';

export const ConsoleProvider = ({ children }) => {
  // 선택된 그룹(사용자 콘솔의 도메인 컨텍스트). { id, name, displayName, orgName }
  const [activeGroup, setActiveGroupState] = useState(() => {
    const saved = localStorage.getItem(GROUP_KEY);
    return saved ? JSON.parse(saved) : null;
  });

  // 선택된 클러스터(관리자 콘솔). { id, name }
  const [activeCluster, setActiveCluster] = useState(null);

  // 탑바 배지에 표시할 요약 (user: 크레딧 / admin: 노드·GPU%)
  const [summary, setSummary] = useState(null);

  const setActiveGroup = useCallback((g) => {
    setActiveGroupState(g);
    // 사용자 뷰의 활성 팀 = 사용자 콘솔 스코프(X-Console-Scope group:N). D 지갑/세션이 이 팀에 귀속된다.
    // 사용자 콘솔 전용 키에 저장해 관리자 콘솔 스코프를 오염시키지 않는다.
    if (g) { localStorage.setItem(GROUP_KEY, JSON.stringify(g)); setUserScope(`group:${g.id}`); }
    else { localStorage.removeItem(GROUP_KEY); setUserScope(null); }
  }, []);

  return (
    <ConsoleContext.Provider
      value={{ activeGroup, setActiveGroup, activeCluster, setActiveCluster, summary, setSummary }}
    >
      {children}
    </ConsoleContext.Provider>
  );
};

export const useConsole = () => useContext(ConsoleContext);
