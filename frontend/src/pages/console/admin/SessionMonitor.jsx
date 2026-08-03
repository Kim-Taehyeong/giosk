import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { ChevronRight } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import Pill from '../../../components/console/Pill';
import DataTable from '../../../components/console/DataTable';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import Bar from '../../../components/console/Bar';
import { getAllSessionsWithUsage } from '../../../api/console/sessions';
import { measureRows, gpuUnmeasurable } from '../../../utils/sessionUsage';
import { cU } from '../../../lib/credit';

// 상태 → (Pill 변형, i18n 키). 미정의 상태는 원문 표시(폴백).
const ST = {
  running: ['run', 'stRunning'], provisioning: ['wait', 'stProvisioning'],
  paused: ['pause', 'stPaused'], stopped: ['pause', 'stStopped'],
  terminated: ['pause', 'stTerminated'], failed: ['err', 'stFailed'], queued: ['wait', 'stProvisioning'],
};
const sp = (s, t) => {
  const [variant, key] = ST[s] || ['wait', null];
  return <Pill variant={variant} dot>{key ? t('monitor.' + key) : s}</Pill>;
};

// UsageCell — 세션별 실사용(GPU/VRAM, 못 재면 CPU/RAM). 사용자 화면과 같은 규칙을 쓴다:
// 못 재는 지표는 0% 막대가 아니라 "측정 불가"로 적는다(놀고 있는 세션과 구분).
function UsageCell({ r, t }) {
  if (r.status !== 'running') return <span className="muted">—</span>;
  const rows = measureRows(r);
  if (!rows.length && !gpuUnmeasurable(r)) return <span className="muted">—</span>;
  // 컴팩트: 지표당 한 줄(라벨 · 바 · 값)로 붙여 행 높이를 줄인다.
  return (
    <div style={{ minWidth: 150 }}>
      {rows.map((x) => (
        <div key={x.label} className="flex" style={{ alignItems: 'center', gap: 6, lineHeight: 1.2, marginBottom: 1 }}>
          <span style={{ width: 34, fontSize: 10, color: 'var(--muted)', fontWeight: 700 }}>{x.label}</span>
          <div style={{ flex: 1, minWidth: 40 }}><Bar value={x.pct} max={100} variant={x.variant} /></div>
          <span style={{ width: 52, textAlign: 'right', fontSize: 10, color: 'var(--muted)', fontWeight: 600, whiteSpace: 'nowrap' }}>{x.txt}</span>
        </div>
      ))}
      {gpuUnmeasurable(r) && (
        <div className="muted" style={{ fontSize: 9.5, marginTop: 1 }} title={t(`monitor.gpuReason.${r.gpuReason}`)}>
          GPU {t('monitor.notMeasurable')}
        </div>
      )}
    </div>
  );
}

export default function SessionMonitor() {
  const { t } = useTranslation('consoleAdmin');
  const navigate = useNavigate();
  const { config } = useSystemConfig();
  const creditMode = config.billing.mode === 'credit';
  const [rows, setRows] = useState(null);
  const [q, setQ] = useState('');

  const load = () => getAllSessionsWithUsage().then(setRows);
  // 5초 폴링 — 실행시간/실시간 누적 크레딧 갱신.
  useEffect(() => { load(); const id = setInterval(load, 5000); return () => clearInterval(id); }, []);

  const filtered = (rows || []).filter((r) => !q || r.name.includes(q) || r.owner.includes(q) || r.group.includes(q) || (r.org || '').includes(q));

  return (
    <div>
      <PageHead title={t('monitor.title')} subtitle={t('monitor.subtitle')}
        actions={<input type="text" placeholder={t('monitor.searchPh')} value={q} onChange={(e) => setQ(e.target.value)} style={{ width: 220 }} />} />
      <div className="card">
        <DataTable
          rows={filtered}
          rowKey={(r) => r.id}
          onRowClick={(r) => navigate(`/console/admin/sessions/${r.id}`)}
          columns={[
            { key: 'status', header: t('monitor.status'), render: (r) => sp(r.status, t) },
            { key: 'name', header: t('monitor.session') },
            { key: 'owner', header: t('monitor.owner') },
            { key: 'org', header: t('monitor.org'), render: (r) => r.org || '—' },
            { key: 'group', header: t('monitor.group') },
            { key: 'offering', header: t('monitor.offering'), className: 'mono' },
            { key: 'gpu', header: t('monitor.gpu'), className: 'mono' },
            { key: 'usage', header: t('monitor.usage'), render: (r) => <UsageCell r={r} t={t} /> },
            { key: 'runtime', header: t('monitor.runtime'), render: (r) => (
              <span>{r.runtime}{r.idle > 0 && <span className="muted"> ({t('monitor.idle', { n: r.idle })})</span>}</span>
            ) },
            ...(creditMode ? [{ key: 'credit', header: t('monitor.credit'), render: (r) => cU(r.credit) }] : []),
            { key: 'act', header: '', render: () => <ChevronRight size={15} style={{ color: 'var(--muted)' }} /> },
          ]}
        />
      </div>
    </div>
  );
}
