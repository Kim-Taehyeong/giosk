import React, { useState } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { useTheme } from '../../context/ThemeContext';

// GitHub 잔디 스타일 히트맵. data: [{ value }] (오래된→최근 순, 하루 1칸).
// 행=요일(일~토)에 정렬되고, 마지막 칸이 오늘. 상단에 월 라벨, hover 시 실제 날짜 툴팁.
const LEVELS = ['#ebedf0', '#9be9a8', '#40c463', '#30a14e', '#216e39'];
const LEVELS_DARK = ['#23272e', '#0e4429', '#006d32', '#26a641', '#39d353'];
const WD_KO = ['일', '월', '화', '수', '목', '금', '토'];
const WD_EN = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
const MON_KO = ['1월', '2월', '3월', '4월', '5월', '6월', '7월', '8월', '9월', '10월', '11월', '12월'];
const MON_EN = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

export default function Heatmap({ data, unit = 'C', rows = 7, cell = 16 }) {
  const { t, i18n } = useTranslation('consoleUser');
  const { theme } = useTheme();
  const [tip, setTip] = useState(null);
  const en = i18n.language?.startsWith('en');
  const WD = en ? WD_EN : WD_KO;
  const MON = en ? MON_EN : MON_KO;
  const palette = theme === 'dark' ? LEVELS_DARK : LEVELS;
  const gap = 3;

  const max = Math.max(1, ...data.map((d) => d.value));
  const level = (v) => (v <= 0 ? 0 : Math.min(4, Math.ceil((v / max) * 4)));

  // 각 칸의 실제 날짜. label('YYYY-MM-DD')이 오면 그걸 쓰고, 없을 때만 "마지막 칸=오늘"로 역산한다.
  // 역산은 데이터가 하루도 빠짐없이 연속일 때만 맞다 — 하루라도 비면 이후 칸의 날짜와 요일 정렬이
  // 통째로 밀린다(값이 0인 날을 빼고 내려주던 시절의 버그). label 우선이면 그 함정이 사라진다.
  // 문자열은 직접 분해해 만든다 — new Date('YYYY-MM-DD')는 UTC 자정으로 파싱돼 시간대에 따라 하루 밀린다.
  const today = new Date(); today.setHours(0, 0, 0, 0);
  const N = data.length;
  const parseDay = (s) => {
    const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(String(s || ''));
    return m ? new Date(+m[1], +m[2] - 1, +m[3]) : null;
  };
  const dateOf = (i) => parseDay(data[i]?.label)
    || (() => { const d = new Date(today); d.setDate(d.getDate() - (N - 1 - i)); return d; })();
  const lead = dateOf(0).getDay(); // 첫 칸의 요일만큼 앞을 비워 요일 정렬
  const cells = [...Array(lead).fill(null), ...data.map((d, i) => ({ v: d.value, i }))];
  const cols = Math.ceil(cells.length / rows);
  const gridWidth = cols * cell + (cols - 1) * gap;

  const fmtDate = (d) => (en
    ? `${MON_EN[d.getMonth()]} ${d.getDate()}, ${d.getFullYear()} (${WD_EN[d.getDay()]})`
    : `${d.getFullYear()}.${d.getMonth() + 1}.${d.getDate()} (${WD_KO[d.getDay()]})`);

  // 월 라벨: 각 열의 첫 데이터 칸의 월이 바뀌는 지점에 표시
  const monthMarks = [];
  let prevMon = -1;
  for (let c = 0; c < cols; c++) {
    let colDate = null;
    for (let r = 0; r < rows; r++) { const it = cells[c * rows + r]; if (it) { colDate = dateOf(it.i); break; } }
    if (colDate && colDate.getMonth() !== prevMon) { monthMarks.push({ col: c, label: MON[colDate.getMonth()] }); prevMon = colDate.getMonth(); }
  }

  return (
    <div style={{ position: 'relative' }}>
      <div style={{ overflowX: 'auto', paddingBottom: 4 }}>
        <div style={{ display: 'flex', gap: 6, width: 'max-content' }}>
          {/* 요일 라벨 (월·수·금만) */}
          <div style={{ display: 'grid', gridTemplateRows: `repeat(${rows}, ${cell}px)`, rowGap: gap, marginTop: 16 }}>
            {WD.map((w, r) => (
              <span key={r} style={{ height: cell, display: 'flex', alignItems: 'center', fontSize: 10, fontWeight: 600, color: 'var(--muted)' }}>
                {r % 2 === 1 ? w : ''}
              </span>
            ))}
          </div>
          <div>
            {/* 월 라벨 */}
            <div style={{ position: 'relative', height: 16, width: gridWidth }}>
              {monthMarks.map((m, idx) => (
                <span key={idx} style={{ position: 'absolute', left: m.col * (cell + gap), fontSize: 10.5, fontWeight: 700, color: 'var(--muted)' }}>{m.label}</span>
              ))}
            </div>
            {/* 그리드 */}
            <div style={{ display: 'grid', gridAutoFlow: 'column', gridTemplateRows: `repeat(${rows}, ${cell}px)`, gridAutoColumns: `${cell}px`, columnGap: gap, rowGap: gap }}>
              {cells.map((it, p) => {
                if (!it) return <div key={p} style={{ width: cell, height: cell }} />;
                const d = dateOf(it.i);
                return (
                  <div key={p}
                    onMouseEnter={(e) => setTip({ x: e.clientX, y: e.clientY, text: `${fmtDate(d)} · ${it.v} ${unit}` })}
                    onMouseMove={(e) => setTip((tp) => (tp ? { ...tp, x: e.clientX, y: e.clientY } : tp))}
                    onMouseLeave={() => setTip(null)}
                    style={{ width: cell, height: cell, borderRadius: 3, background: palette[level(it.v)], cursor: 'pointer', border: '1px solid rgba(27,31,35,.06)' }} />
                );
              })}
            </div>
          </div>
        </div>
      </div>

      <div className="flex gap" style={{ marginTop: 12, justifyContent: 'flex-end', alignItems: 'center' }}>
        <span className="muted" style={{ fontSize: 11.5 }}>{t('wallet.less')}</span>
        {palette.map((bg, i) => (
          <span key={i} style={{ width: 13, height: 13, borderRadius: 3, background: bg, display: 'inline-block' }} />
        ))}
        <span className="muted" style={{ fontSize: 11.5 }}>{t('wallet.more')}</span>
      </div>

      {tip && createPortal(
        <div style={{
          position: 'fixed', left: tip.x + 12, top: tip.y - 36, zIndex: 9999,
          background: 'var(--text)', color: 'var(--bg)', padding: '6px 10px', borderRadius: 8,
          fontSize: 12, fontWeight: 700, pointerEvents: 'none', whiteSpace: 'nowrap', boxShadow: 'var(--shadow)',
        }}>{tip.text}</div>,
        document.body,
      )}
    </div>
  );
}
