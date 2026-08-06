import React from 'react';
import Bar from './Bar';

// 세션 실사용량 미터. measureRows(sessionUsage)가 만든 행을 그린다.
// 사용률을 크게 읽히게 두고, 절대값(2.0/12G)은 라벨 옆에 보조로 붙인다.
// GPU 행은 보조값이 사용률과 같은 값이라 중복이므로 표시하지 않는다.
export default function UsageMeters({ rows }) {
  return (
    <div className="usage-meters">
      {rows.map((x) => {
        const sub = x.txt === `${Math.round(x.pct)}%` ? '' : x.txt;
        return (
          <div className="usage-meter" key={x.label}>
            <div className="usage-meter-head">
              <span className="usage-meter-label">{x.label}</span>
              {sub && <span className="usage-meter-sub mono">{sub}</span>}
              <b className="usage-meter-val">{Math.round(x.pct)}<i>%</i></b>
            </div>
            <Bar value={x.pct} max={100} variant={x.variant} className="framed" />
          </div>
        );
      })}
    </div>
  );
}
