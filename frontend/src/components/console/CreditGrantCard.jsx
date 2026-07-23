import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Coins } from 'lucide-react';

// CreditGrantCard — 조직/그룹 상세에서 크레딧을 바로 부여하는 카드.
//
// 모달로 띄우면 (1) 버튼이 헤더에 묻혀 안 보이고 (2) 잔액을 보면서 넣지 못한다.
// 상세 화면 안에 잔액과 입력을 나란히 두어 "여기서 하는 것"이 보이게 한다.
//  balance: 현재 잔여(C). onGrant(amount, reason): 부여 실행(성공 시 입력 초기화).
//  hint: 회계 규칙 안내(예: 상위 풀 자동 가산).
export default function CreditGrantCard({ balance, onGrant, hint, title }) {
  const { t } = useTranslation('consoleAdmin');
  const [amount, setAmount] = useState('');
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);

  const n = Number(amount);
  const valid = amount !== '' && Number.isFinite(n) && n !== 0;

  const submit = async () => {
    if (!valid || busy) return;
    setBusy(true);
    try {
      await onGrant(n, reason);
      setAmount(''); setReason('');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="card">
      <h3 style={{ marginTop: 0 }}><Coins size={16} /> {title || t('credit.title')}</h3>
      <div className="flex" style={{ alignItems: 'baseline', gap: 8, marginBottom: 14 }}>
        <span style={{ fontSize: 26, fontWeight: 800 }}>{(balance ?? 0).toLocaleString()}</span>
        <span className="muted" style={{ fontWeight: 700 }}>C</span>
        <span className="muted" style={{ fontSize: 12.5 }}>{t('credit.balance')}</span>
      </div>
      <div className="flex gap wrap" style={{ alignItems: 'flex-end' }}>
        <div style={{ width: 160 }}>
          <label className="fld" style={{ marginTop: 0 }}>{t('credit.amount')}</label>
          <input type="number" value={amount} placeholder="100"
            onChange={(e) => setAmount(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && submit()} />
        </div>
        <div style={{ flex: 1, minWidth: 200 }}>
          <label className="fld" style={{ marginTop: 0 }}>{t('credit.reason')}</label>
          <input type="text" value={reason} placeholder={t('credit.reasonPh')}
            onChange={(e) => setReason(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && submit()} />
        </div>
        <button className="btn primary" disabled={!valid || busy} onClick={submit}>
          <Coins size={14} /> {t('credit.grant')}
        </button>
      </div>
      {hint && <div className="legend mt">{hint}</div>}
    </div>
  );
}
