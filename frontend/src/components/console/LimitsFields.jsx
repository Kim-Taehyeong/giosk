import React from 'react';
import { useTranslation } from 'react-i18next';

// LimitsFields는 하드 리소스 제한(GPU·VRAM·볼륨·동시세션) 4개 입력을 렌더한다.
// 값이 null/빈값이면 "상속"(상위 계층으로 폴백) 의미 — placeholder 로 안내.
// value: { maxGpu, maxVramGb, maxVolumeGib, maxConcurrentSessions } (각 항목 number|null)
export default function LimitsFields({ value, onChange }) {
  const { t } = useTranslation('consoleAdmin');
  const set = (k, raw) => onChange({ ...value, [k]: raw === '' ? null : Math.max(0, Number(raw)) });
  const field = (k, unit) => (
    <div>
      <label className="fld" htmlFor="console-limitsfields-fld-0" style={{ marginTop: 0 }}>{t('limits.' + k)}</label>
      {/* 입력이 칸을 채우게 둔다 — 고정 폭(120px)이면 넓은 모달에서 옆이 텅 비어 답답해 보인다.
          단위는 입력 안쪽 오른쪽에 붙여 한 줄을 아낀다. */}
      <span style={{ position: 'relative', display: 'block' }}>
        <input id="console-limitsfields-fld-0" type="number" min={0} value={value?.[k] ?? ''} placeholder={t('limits.inherit')}
          onChange={(e) => set(k, e.target.value)} style={{ paddingRight: 46 }} />
        <span className="muted" style={{ position: 'absolute', right: 11, top: '50%', transform: 'translateY(-50%)',
          fontSize: 11.5, whiteSpace: 'nowrap', pointerEvents: 'none' }}>{unit}</span>
      </span>
    </div>
  );
  return (
    <div className="grid cols-2" style={{ gap: 12 }}>
      {field('maxGpu', 'GPU')}
      {field('maxVramGb', 'GB')}
      {field('maxVolumeGib', 'GiB')}
      {field('maxConcurrentSessions', t('limits.sessionsUnit'))}
      {field('maxStoppedSessions', t('limits.sessionsUnit'))}
      {field('maxEphemeralGib', 'GiB')}
    </div>
  );
}
