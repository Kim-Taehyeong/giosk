import React, { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Activity } from 'lucide-react';
import UtilLineChart from './UtilLineChart';
import Spinner from './Spinner';

// 세션 사용률 이력은 24시간 고정이다 — 세션 수명은 짧고, DB 미저장(Prometheus Range)이라
// 24시간 이전 데이터는 애초에 조회하지 않는다(Prometheus 보존정책이 알아서 폐기). 그래서 구간 탭이 없다.
const HOURS = 24;

// 시각 라벨 — 24h 이내라 시:분.
function fmtTime(t) {
  const d = new Date(t * 1000);
  const p = (n) => String(n).padStart(2, '0');
  return `${p(d.getHours())}:${p(d.getMinutes())}`;
}

// GPU 측정 불가 사유 → 안내 문구 키.
const REASON = {
  cpu: 'histReasonCpu', physical: 'histReasonPhysical', timeslice: 'histReasonTimeslice',
  unavailable: 'histReasonUnavailable',
};

// SessionHistoryChart는 세션 사용률 이력(24h 고정)을 라인차트로 렌더한다.
// fetch(id, hours) → { points[], insights }. 좁은 열에 들어가도록 두 차트를 세로로 쌓는다.
export default function SessionHistoryChart({ id, fetch }) {
  const { t } = useTranslation('consoleUser');
  const [data, setData] = useState(null);

  useEffect(() => { setData(null); fetch(id, HOURS).then(setData).catch(() => setData({ points: [], insights: {} })); }, [id]); // eslint-disable-line

  const ins = data?.insights || {};
  const points = data?.points || [];
  const labels = useMemo(() => points.map((p) => fmtTime(p.t)), [points]);
  const series = (key) => points.map((p) => p[key]);
  const has = points.length > 0;
  const reasonKey = ins.gpuSource === 'none' && ins.gpuReason ? REASON[ins.gpuReason] : null;

  return (
    <div className="card">
      <h3 style={{ marginTop: 0 }}>
        <Activity size={16} /> {t('sdetail.history', { defaultValue: '사용률 이력' })}
        <span className="muted" style={{ fontSize: 12, fontWeight: 600, marginLeft: 6 }}>· 24h</span>
      </h3>

      {/* 구간 요약 인사이트 */}
      {has && (
        <div className="flex" style={{ gap: 16, flexWrap: 'wrap', marginBottom: 12, fontSize: 12.5 }}>
          {ins.gpuSource !== 'none' && <span className="muted">{t('sdetail.avgGpu', { defaultValue: '평균 GPU' })} <b style={{ color: 'var(--text)' }}>{ins.avgGpu ?? 0}%</b> · {t('sdetail.peakGpu', { defaultValue: '최대' })} {ins.peakGpu ?? 0}%</span>}
          <span className="muted">{t('sdetail.avgCpu', { defaultValue: '평균 CPU' })} <b style={{ color: 'var(--text)' }}>{ins.avgCpu ?? 0}%</b></span>
          <span className="muted">{t('sdetail.avgMem', { defaultValue: '평균 RAM' })} <b style={{ color: 'var(--text)' }}>{ins.avgMem ?? 0}%</b></span>
        </div>
      )}

      {!data ? (
        <Spinner pad label={t('sdetail.loadingWindow', { defaultValue: '메트릭 불러오는 중…' })} />
      ) : !has ? (
        <div className="legend" style={{ padding: 30, textAlign: 'center' }}>
          {t('sdetail.noMetrics', { defaultValue: '이 구간의 메트릭이 없습니다.' })}
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 18 }}>
          <div>
            <div className="legend" style={{ marginBottom: 6, fontWeight: 700 }}>{t('sdetail.chartGpu', { defaultValue: 'GPU · VRAM' })}</div>
            {reasonKey ? (
              <div className="legend" style={{ padding: 24, textAlign: 'center', minHeight: 100, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                {t(`sdetail.${reasonKey}`, { defaultValue: 'GPU 사용률을 측정할 수 없습니다.' })}
              </div>
            ) : (
              <UtilLineChart labels={labels} values={series('gpuUtil')} label={t('sdetail.gpu', { defaultValue: 'GPU' })}
                extra={{ label: 'VRAM', values: series('vramPct') }} height={190} />
            )}
          </div>
          <div>
            <div className="legend" style={{ marginBottom: 6, fontWeight: 700 }}>{t('sdetail.chartCpuMem', { defaultValue: 'CPU · RAM' })}</div>
            <UtilLineChart labels={labels} values={series('cpu')} label="CPU"
              extra={{ label: 'RAM', values: series('mem') }} height={190} />
          </div>
        </div>
      )}
    </div>
  );
}
