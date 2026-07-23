import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { RefreshCcw } from 'lucide-react';
import Toggle from './Toggle';

// RefillCard는 팀/개인 정기 크레딧 리필 설정(양·주기·이월)을 편집한다.
// 주기는 상위(팀은 조직, 개인은 팀) 상한을 넘으면 백엔드가 자동 클램프한다.
// current: { recurringCredit, refillIntervalDays, carryover }. onSave({recurring, interval, carryover}).
export default function RefillCard({ title, current, maxInterval, onSave, ns = 'consoleAdmin' }) {
  const { t } = useTranslation(ns);
  const [v, setV] = useState(String(current?.recurringCredit ?? 0));
  const [iv, setIv] = useState(String(current?.refillIntervalDays || maxInterval || 30));
  const [carry, setCarry] = useState(!!current?.carryover);
  return (
    <div className="card" style={{ margin: 0 }}>
      <h3><RefreshCcw size={15} /> {title || t('refill.title', { defaultValue: '정기 리필' })}</h3>
      <div className="legend mb">{t('refill.note', { defaultValue: '주기마다 이 양으로 리필합니다(0이면 리필 안 함). 주기는 상위보다 길 수 없어 자동 제한됩니다.' })}</div>
      <label className="fld" style={{ marginTop: 0 }}>{t('refill.amount', { defaultValue: '주기당 크레딧' })}</label>
      <input type="number" min={0} value={v} onChange={(e) => setV(e.target.value)} />
      <div className="grid cols-2" style={{ gap: 12 }}>
        <div><label className="fld">{t('refill.interval', { defaultValue: '리필 주기(일)' })}</label>
          <input type="number" min={1} max={maxInterval || undefined} value={iv} onChange={(e) => setIv(e.target.value)} /></div>
        <div><label className="fld">{t('refill.carryover', { defaultValue: '이월' })}</label>
          <div className="flex" style={{ gap: 8, alignItems: 'center', height: 38 }}>
            <Toggle checked={carry} onChange={setCarry} />
            <span style={{ fontSize: 12.5 }}>{carry ? t('refill.carryOn', { defaultValue: '이월' }) : t('refill.carryOff', { defaultValue: '소멸' })}</span>
          </div></div>
      </div>
      {maxInterval > 0 && <div className="legend" style={{ marginTop: 6 }}>{t('refill.maxHint', { n: maxInterval, defaultValue: `상위 주기 상한: ${maxInterval}일 이하` })}</div>}
      <button className="btn primary" style={{ marginTop: 12 }}
        onClick={() => onSave({ recurring: Number(v) || 0, interval: Number(iv) || 0, carryover: carry })}>
        {t('common.save', { defaultValue: '저장' })}
      </button>
    </div>
  );
}
