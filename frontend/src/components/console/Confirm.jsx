import React, { createContext, useContext, useState, useCallback, useRef } from 'react';
import { Trash2, Check } from 'lucide-react';
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
        <div style={{ fontSize: 14.5, lineHeight: 1.55, whiteSpace: 'pre-line' }}>{state?.message}</div>
        {/* 되돌릴 수 없는 작업은 무엇을 잃고 무엇이 남는지가 한 줄로 읽혀야 한다.
            문장 안에 섞어 두면 사용자는 확인 버튼을 누를 때까지 그걸 못 찾는다. */}
        {(state?.lost || state?.kept) && (
          <dl style={{
            margin: '14px 0 0', display: 'grid', gridTemplateColumns: 'auto 1fr',
            gap: '8px 12px', alignItems: 'baseline',
            borderTop: '1px solid var(--border)', paddingTop: 14,
          }}>
            {state.lost && (<>
              <dt style={{ display: 'flex', alignItems: 'center', gap: 6, color: 'var(--danger)', fontSize: 12, fontWeight: 700, letterSpacing: '.02em' }}>
                <Trash2 size={13} aria-hidden="true" /> {state.lostLabel || '삭제'}
              </dt>
              <dd style={{ margin: 0, fontSize: 14, color: 'var(--text)' }}>{state.lost}</dd>
            </>)}
            {state.kept && (<>
              <dt style={{ display: 'flex', alignItems: 'center', gap: 6, color: 'var(--free)', fontSize: 12, fontWeight: 700, letterSpacing: '.02em' }}>
                <Check size={13} aria-hidden="true" /> {state.keptLabel || '유지'}
              </dt>
              <dd style={{ margin: 0, fontSize: 14, color: 'var(--muted)' }}>{state.kept}</dd>
            </>)}
          </dl>
        )}
        {state?.note && (
          <p style={{ margin: '14px 0 0', fontSize: 13, fontWeight: 700, color: 'var(--danger)' }}>{state.note}</p>
        )}
      </Modal>
    </ConfirmContext.Provider>
  );
};

// Provider 밖에서도 안전하게 동작(window.confirm 폴백).
export const useConfirm = () => {
  const ctx = useContext(ConfirmContext);
  return ctx ? ctx.confirm : (opts) => Promise.resolve(window.confirm(opts?.message || '계속할까요?'));
};
