import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import PageHead from '../../../components/console/PageHead';
import Pill from '../../../components/console/Pill';
import PagedTable from '../../../components/console/PagedTable';
import { getAudit } from '../../../api/console/misc';

const resultVariant = { applied: 'ok', success: 'ok', allowed: 'ok', denied: 'err', mfa: 'wait' };

export default function Audit() {
  const { t } = useTranslation('consoleAdmin');
  const [data, setData] = useState(null);
  const [q, setQ] = useState('');
  useEffect(() => { getAudit().then(setData); }, []);
  const rows = (data?.items || []).filter((r) => !q || r.action.includes(q) || (r.actorUsername || '').includes(q));

  return (
    <div>
      <PageHead title={t('audit.title')} subtitle={t('audit.subtitle')}
        actions={<input type="text" placeholder={t('audit.searchPh')} value={q} onChange={(e) => setQ(e.target.value)} style={{ width: 200 }} />} />
      <div className="card">
        <PagedTable
          rows={rows}
          pageSize={15}
          className="fixed-cols"
          rowKey={(r, i) => i}
          columns={[
            { key: 'createdAt', header: t('audit.time'), className: 'mono' },
            { key: 'actorUsername', header: t('audit.actor') },
            { key: 'action', header: t('audit.action'), className: 'mono' },
            { key: 'targetUsername', header: t('audit.target'), className: 'mono' },
            { key: 'result', header: t('audit.result'), render: (r) => <Pill variant={resultVariant[r.result] || 'pause'} dot>{t(`audit.result_${r.result}`, { defaultValue: r.result })}</Pill> },
          ]}
        />
      </div>
    </div>
  );
}
