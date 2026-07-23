import { apiGet, apiPost } from '../client';

const TREND_DAYS = 210; // 잔디 고정 구간(30주) — 빈 날 포함 항상 동일한 격자

// 'YYYY-MM-DD'(또는 'YYYY-MM-DDTHH:...' 타임스탬프) → 로컬 자정 Date.
// 앞 10자만 취해 new Date(str)의 UTC 파싱 TZ 밀림을 피한다.
function parseDay(s) {
  const [y, m, d] = String(s).slice(0, 10).split('-').map(Number);
  if (!y || !m || !d) return null;
  return new Date(y, m - 1, d);
}

// 일자별 소비 Map(key=로컬 자정 ms → 소비 합계). 소스는 백엔드 일자별 집계
// (d.trend = [{day:'YYYY-MM-DD', amount}], 소비 있는 날만)뿐 — 폴백 없음.
function dailyMap(d) {
  const byDay = new Map();
  for (const p of d.trend || []) {
    const day = parseDay(p.day);
    if (day) byDay.set(day.getTime(), p.amount);
  }
  return byDay;
}

// 잔디 데이터: 오늘부터 과거 210일 고정 창(빈 날은 0). 데이터 유무와 무관하게 항상 동일한 격자.
function buildTrend(byDay) {
  const today = new Date(); today.setHours(0, 0, 0, 0);
  const out = [];
  for (let i = TREND_DAYS - 1; i >= 0; i--) {
    const d = new Date(today); d.setDate(d.getDate() - i);
    out.push({ day: `${d.getMonth() + 1}/${d.getDate()}`, amount: byDay.get(d.getTime()) || 0 });
  }
  return out;
}

// Calendar 월별 탐색용: { '2026-5': { 20: 30 }, ... } (key=YYYY-M, m=0~11).
function buildByMonth(byDay) {
  const byMonth = {};
  for (const [key, amount] of byDay.entries()) {
    const d = new Date(key);
    (byMonth[`${d.getFullYear()}-${d.getMonth()}`] ||= {})[d.getDate()] = amount;
  }
  return byMonth;
}

// 개인 지갑 — 실 백엔드. 라이브 지표(burn/eta/bySession)는 아직 미산출 → 기본값 보정.
// trend(잔디)·byMonth(달력)는 백엔드 일자별 집계(d.trend), 없으면 history에서 파생.
export const getWallet = () =>
  apiGet('/me/wallet').then((d) => {
    const byDay = dailyMap(d);
    return {
      balance: d.balance || 0,
      reserved: d.reserved || 0,
      cap: d.cap || 0,
      burn: 0,
      etaDays: 0,
      monthUsed: 0,
      trend: buildTrend(byDay),
      byMonth: buildByMonth(byDay),
      bySession: (d.bySession || []).map((s) => ({ name: s.name || s.ref, credit: s.credit || 0 })),
      history: d.history || [],
    };
  });

// 개인 충전 요청 → 본인(user) 대상. 그룹 관리자가 승인.
export const requestTopup = (form) =>
  apiPost('/me/topup-requests', { targetType: 'user', targetId: 0, amount: Number(form.amount), reason: form.reason });
