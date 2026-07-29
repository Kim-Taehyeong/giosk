import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useTheme } from '../../context/ThemeContext';
import { c } from '../../lib/credit';
import { clickable } from '../../utils/a11y';

// 월별 달력. 두 가지 모드:
//  1) 기본(표시 전용): 날짜별 소모 크레딧을 색/숫자로 표기. 표시 데이터는
//     byMonth={ '<year>-<month0>': { [day]: amount } } (월별 탐색 지원) 또는
//     byDay={ [day]: amount } (현재 표시 월 단건) 로 공급.
//  2) selectable: 날짜 선택용 date-picker. selected(Date)/onPick(Date)/minDate/maxDate 로 제어.
const WD = ['일', '월', '화', '수', '목', '금', '토'];
const LEVELS = ['transparent', '#e0e7ff', '#a5b4fc', '#818cf8', '#4f46e5'];
const LEVELS_DARK = ['transparent', '#26284a', '#3730a3', '#4f46e5', '#818cf8'];

// 날짜를 자정 기준으로 비교하기 위한 정규화(시간 제거).
const atMidnight = (d) => new Date(d.getFullYear(), d.getMonth(), d.getDate());
const sameDay = (a, b) => a && b && a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();

export default function Calendar({ byDay = {}, byMonth = null, selectable = false, selected = null, onPick, minDate, maxDate }) {
  const { t } = useTranslation('consoleUser');
  const { theme } = useTheme();
  const palette = theme === 'dark' ? LEVELS_DARK : LEVELS;
  const now = new Date();
  const initial = selectable && selected ? selected : now;
  const [ym, setYm] = useState({ y: initial.getFullYear(), m: initial.getMonth() });

  // 표시 월의 일자별 소모. byMonth(월별 탐색)가 있으면 현재 월을 조회, 없으면 byDay.
  const dayMap = byMonth ? (byMonth[`${ym.y}-${ym.m}`] || {}) : byDay;

  const first = new Date(ym.y, ym.m, 1).getDay();
  const days = new Date(ym.y, ym.m + 1, 0).getDate();
  const max = Math.max(1, ...Object.values(dayMap));
  const level = (v) => (!v ? 0 : Math.min(4, Math.ceil((v / max) * 4)));

  const cells = [];
  for (let i = 0; i < first; i++) cells.push(null);
  for (let d = 1; d <= days; d++) cells.push(d);
  const monthTotal = Object.values(dayMap).reduce((a, v) => a + v, 0);

  // selectable 모드에서 이전/다음 달 이동이 [minDate, maxDate] 범위를 벗어나는지.
  const minM = minDate && atMidnight(new Date(minDate.getFullYear(), minDate.getMonth(), 1));
  const maxM = maxDate && atMidnight(new Date(maxDate.getFullYear(), maxDate.getMonth(), 1));
  const curM = new Date(ym.y, ym.m, 1);
  const prevDisabled = selectable && minM && curM <= minM;
  const nextDisabled = selectable && maxM && curM >= maxM;

  const outOfRange = (d) => {
    if (!selectable) return false;
    const date = atMidnight(new Date(ym.y, ym.m, d));
    if (minDate && date < atMidnight(minDate)) return true;
    if (maxDate && date > atMidnight(maxDate)) return true;
    return false;
  };

  const shift = (delta) => setYm(({ y, m }) => {
    const nm = m + delta;
    return { y: y + Math.floor(nm / 12), m: ((nm % 12) + 12) % 12 };
  });

  return (
    <div style={{ width: '100%' }}>
      <div className="flex" style={{ justifyContent: 'space-between', marginBottom: 4 }}>
        <button className="btn sm" onClick={() => shift(-1)} disabled={prevDisabled}>‹</button>
        <strong style={{ fontSize: 13 }}>{ym.y}.{ym.m + 1}</strong>
        <button className="btn sm" onClick={() => shift(1)} disabled={nextDisabled}>›</button>
      </div>
      {!selectable && (
        <div style={{ textAlign: 'center', marginBottom: 6, fontSize: 12 }}>
          <span className="muted">{t('wallet.thisMonth')} </span><strong style={{ color: 'var(--primary)' }}>{c(monthTotal)} C</strong>
        </div>
      )}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(7, 1fr)', gap: 4 }}>
        {WD.map((w) => <div key={w} className="muted" style={{ textAlign: 'center', fontSize: 11.5, fontWeight: 700, paddingBottom: 2 }}>{w}</div>)}
        {cells.map((d, i) => {
          if (!d) return <div key={i} style={{ height: 42 }} />;
          if (selectable) {
            const date = new Date(ym.y, ym.m, d);
            const disabled = outOfRange(d);
            const isSel = sameDay(atMidnight(date), selected && atMidnight(selected));
            return (
              <div key={i} {...clickable(disabled || !onPick ? undefined : () => onPick(atMidnight(date)), { label: `${ym.y}-${ym.m + 1}-${d}` })}
                aria-current={isSel || undefined}
                title={`${ym.m + 1}/${d}`}
                style={{
                  height: 42, borderRadius: 7, display: 'flex', alignItems: 'center', justifyContent: 'center',
                  fontSize: 13, fontWeight: 700,
                  border: '1px solid ' + (isSel ? 'var(--primary)' : 'var(--border)'),
                  background: isSel ? 'var(--primary)' : 'var(--surface)',
                  color: isSel ? '#fff' : disabled ? 'var(--muted)' : 'var(--text)',
                  opacity: disabled ? 0.4 : 1, cursor: disabled ? 'not-allowed' : 'pointer',
                }}>{d}</div>
            );
          }
          const v = dayMap[d] || 0;
          const lv = level(v);
          return (
            <div key={i} title={`${ym.m + 1}/${d} · ${c(v)} C`}
              style={{
                height: 42, borderRadius: 7, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 1,
                border: '1px solid var(--border)',
                background: lv ? palette[lv] : 'var(--surface)',
                color: lv >= 3 ? '#fff' : 'var(--text)', cursor: 'pointer',
              }}>
              <span style={{ fontSize: 13, fontWeight: 700 }}>{d}</span>
              {v > 0 && <span style={{ fontSize: 10, opacity: 0.82, fontWeight: 600 }}>{v}</span>}
            </div>
          );
        })}
      </div>
    </div>
  );
}
