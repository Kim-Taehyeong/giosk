import React, { createContext, useContext, useState, useCallback, useRef } from 'react';
import Modal from './Modal';

// 파괴적 동작(삭제·종료 등) 확인 다이얼로그. ConfirmProvider 하위에서 useConfirm()으로 호출.
//   const confirm = useConfirm();
//   if (await confirm({ title, message, confirmText, danger })) { ...삭제... }
const ConfirmContext = createContext(null);

export const ConfirmProvider = ({ children }) => {
  const [state, setState] = useState(null); // { title, message, confirmText, cancelText, danger }
  const resolver = useRef(null);

  const confirm = useCallback((opts = {}) => new Promise((resolve) => {
    resolver.current = resolve;
    setState(opts);
  }), []);

  const done = (ok) => {
    setState(null);
    if (resolver.current) { resolver.current(ok); resolver.current = null; }
  };

  return (
    <ConfirmContext.Provider value={{ confirm }}>
      {children}
      <Modal
        open={!!state}
        title={state?.title || '확인'}
        onClose={() => done(false)}
        width={420}
        footer={(
          <>
            <button className="btn" onClick={() => done(false)}>{state?.cancelText || '취소'}</button>
            <button className={`btn ${state?.danger === false ? 'primary' : 'danger'}`} onClick={() => done(true)}>
              {state?.confirmText || '삭제'}
            </button>
          </>
        )}
      >
        <div style={{ fontSize: 13.5, lineHeight: 1.6, whiteSpace: 'pre-line' }}>{state?.message}</div>
      </Modal>
    </ConfirmContext.Provider>
  );
};

// Provider 밖에서도 안전하게 동작(window.confirm 폴백).
export const useConfirm = () => {
  const ctx = useContext(ConfirmContext);
  return ctx ? ctx.confirm : (opts) => Promise.resolve(window.confirm(opts?.message || '계속할까요?'));
};
