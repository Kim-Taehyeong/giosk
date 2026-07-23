import React from 'react';

// 마법사 단계 헤더. steps: string[], current: index
export default function Stepper({ steps, current }) {
  return (
    <div className="steps">
      {steps.map((s, i) => (
        <div key={i} className={`s${i === current ? ' active' : i < current ? ' done' : ''}`}>
          {i + 1}. {s}
        </div>
      ))}
    </div>
  );
}
