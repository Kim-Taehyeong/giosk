import React from 'react';

// 상태/역할 pill. variant: run|wait|pause|err|ok|gpu|free|cordon|primary|danger|warn
export default function Pill({ variant = '', dot = false, children }) {
  return (
    <span className={`pill ${variant}`.trim()}>
      {dot && <span className="dot-s" />}
      {children}
    </span>
  );
}
