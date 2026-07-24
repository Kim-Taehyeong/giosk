import React from 'react';
import { useTranslation } from 'react-i18next';
import { Info } from 'lucide-react';

// MetricsOffNotice는 지표 수집 스택이 없어 GPU 지표를 못 보여준다는 안내(노드·인프라 대시보드 공용).
// 값이 "0" 인 것과 "측정 자체가 없는" 것을 구분해 주는 게 목적이라 문구는 한 곳에서만 관리한다.
//   dcgmOnly: Prometheus 는 붙었지만 DCGM exporter 시리즈만 없는 경우.
export default function MetricsOffNotice({ dcgmOnly = false }) {
  const { t } = useTranslation('consoleAdmin');
  return (
    <div className="card mb" style={{ display: 'flex', alignItems: 'center', gap: 10, borderLeft: '3px solid var(--warn)' }}>
      <Info size={18} style={{ color: 'var(--warn)', flex: '0 0 auto' }} />
      <span style={{ fontSize: 13, color: 'var(--muted)' }}>
        {dcgmOnly ? t('dash.metricsOffDcgm') : t('nodes.metricsOffNotice')}
      </span>
    </div>
  );
}
