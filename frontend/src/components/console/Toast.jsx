import React, { createContext, useContext, useState, useCallback, useRef } from 'react';

// 목업의 toast() 대체. ConsoleProvider 하위에서 useToast()로 호출.
// 여러 번 호출하면 아래에서 위로 쌓이고(스택), 각자 시간이 지나면 개별로 사라진다.
const ToastContext = createContext(null);

const TOAST_MS = 2600; // 개별 토스트 표시 시간

export const ToastProvider = ({ children }) => {
  const [toasts, setToasts] = useState([]); // [{id, msg}]
  const seq = useRef(0);

  const toast = useCallback((msg) => {
    const id = ++seq.current;
    setToasts((cur) => [...cur, { id, msg }]);
    setTimeout(() => setToasts((cur) => cur.filter((t) => t.id !== id)), TOAST_MS);
  }, []);

  return (
    <ToastContext.Provider value={{ toast }}>
      {children}
      <div className="console-toast-stack">
        {toasts.map((t) => (
          <div key={t.id} className="console-toast show">{t.msg}</div>
        ))}
      </div>
    </ToastContext.Provider>
  );
};

export const useToast = () => {
  const ctx = useContext(ToastContext);
  // Provider 밖에서도 안전하게 no-op 반환
  return ctx || { toast: () => {} };
};
