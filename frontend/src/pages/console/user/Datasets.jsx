import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { formatBytes } from '../../../utils/format';
import { Database, Upload, Server, Inbox } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import Pill from '../../../components/console/Pill';
import DataTable from '../../../components/console/DataTable';
import Modal from '../../../components/console/Modal';
import Select from '../../../components/console/Select';
import { useToast } from '../../../components/console/Toast';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { getDatasets, registerDataset } from '../../../api/console/datasets';

const sizeVariant = { Large: 'primary', Medium: 'gpu', Small: 'free' };
const SIZE_OPTS = ['Small', 'Medium', 'Large'];

export default function Datasets() {
  const { t } = useTranslation('consoleUser');
  const { toast } = useToast();
  const { config } = useSystemConfig();
  const canRegister = config.features.datasetRegister;
  const [data, setData] = useState(null);
  const [modal, setModal] = useState(false);
  const [submitting, setSubmitting] = useState(false); // 등록 중(서버가 URL 용량 측정 ~수초)
  const [form, setForm] = useState({ name: '', sizeClass: 'Small', url: '' });

  const load = () => getDatasets().then((d) => setData({ global: d.global, requests: [...(d.requests || [])] }));
  // 5초 폴링으로 적재 상태(다운로드 중에서 완료까지)를 라이브 갱신한다.
  useEffect(() => { load(); const id = setInterval(load, 5000); return () => clearInterval(id); }, []);

  const submit = async () => {
    if (!form.name || !form.url) { toast(t('datasets.needNameUrl')); return; }
    if (submitting) return;
    setSubmitting(true);
    try {
      await registerDataset(form);
      toast(t('datasets.requested'));
    } catch {
      toast(t('datasets.requestFailed'));
      return;
    } finally {
      setSubmitting(false);
    }
    setModal(false);
    setForm({ name: '', sizeClass: 'Small', url: '' });
    load(); // 백엔드 반영분 재조회(pending 신청 표시)
  };

  const statusPill = (s) => (s === 'pending'
    ? <Pill variant="wait" dot>{t('datasets.statusPending')}</Pill>
    : <Pill variant="ok" dot>{t('datasets.statusApproved')}</Pill>);

  return (
    <div>
      <PageHead icon={Database} title={t('datasets.title')} subtitle={t('datasets.subtitle')}
        actions={canRegister
          ? <button className="btn primary" onClick={() => setModal(true)}><Upload size={15} /> {t('datasets.register')}</button>
          : null} />

      {/* 전역 데이터셋 */}
      <div className="card mb">
        <h3><Server size={16} /> {t('datasets.globalTitle')}</h3>
        <div className="legend mb">{t('datasets.globalHint')}</div>
        <DataTable
          rows={data?.global || []}
          rowKey={(r) => r.id}
          columns={[
            { key: 'name', header: t('datasets.name'), render: (r) => (
              <span style={{ fontWeight: 600, display: 'inline-flex', gap: 6, alignItems: 'center' }}>{r.name}
                {r.loadStatus === 'loading' && <Pill variant="wait" dot>{t('datasets.loading')}</Pill>}
                {r.loadStatus === 'failed' && <Pill variant="err">{t('datasets.loadFailed')}</Pill>}
              </span>
            ) },
            { key: 'desc', header: t('datasets.desc'), render: (r) => <span className="muted">{r.desc}</span> },
            { key: 'sizeClass', header: t('datasets.class'), render: (r) => <Pill variant={sizeVariant[r.sizeClass]}>{r.sizeClass}</Pill> },
            { key: 'sizeGb', header: t('datasets.size'), render: (r) => formatBytes(r.sizeBytes) },
            { key: 'cachedNodes', header: t('datasets.cached'), render: (r) => t('datasets.cachedN', { n: (r.nodes || []).length }) },
          ]}
        />
      </div>

      {/* 내 등록 요청 — 데이터셋 등록 기능이 켜진 경우만 */}
      {canRegister && (
        <div className="card">
          <h3><Inbox size={16} /> {t('datasets.myRequests')}</h3>
          <DataTable
            rows={data?.requests || []}
            rowKey={(r) => r.id}
            emptyText={t('datasets.noRequests')}
            columns={[
              { key: 'name', header: t('datasets.name'), render: (r) => <span style={{ fontWeight: 600 }}>{r.name}</span> },
              { key: 'sizeClass', header: t('datasets.class'), render: (r) => <Pill variant={sizeVariant[r.sizeClass]}>{r.sizeClass}</Pill> },
              { key: 'sizeGb', header: t('datasets.size'), render: (r) => (r.sizeBytes ? formatBytes(r.sizeBytes) : <span className="muted">{t('datasets.measuring')}</span>) },
              { key: 'status', header: t('datasets.status'), render: (r) => statusPill(r.status) },
            ]}
          />
        </div>
      )}

      <Modal open={modal} title={t('datasets.registerTitle')} onClose={() => (submitting ? null : setModal(false))} width={460}
        footer={(
          <>
            <button className="btn" onClick={() => setModal(false)} disabled={submitting}>{t('datasets.cancel')}</button>
            <button className="btn primary" onClick={submit} disabled={submitting}>{t('datasets.submit')}</button>
          </>
        )}>
        <label className="fld" htmlFor="user-datasets-fld-0" style={{ marginTop: 0 }}>{t('datasets.fName')}</label>
        <input id="user-datasets-fld-0" type="text" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="my-dataset" />
        <label className="fld" id="user-datasets-fld-1-lbl">{t('datasets.fClass')}</label>
        <Select ariaLabelledBy="user-datasets-fld-1-lbl" value={form.sizeClass} onChange={(v) => setForm({ ...form, sizeClass: v })} options={SIZE_OPTS.map((x) => ({ value: x, label: x }))} />
        <label className="fld" htmlFor="user-datasets-fld-2">{t('datasets.fUrl')}</label>
        <input id="user-datasets-fld-2" type="text" value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} placeholder="https://example.com/dataset.zip" />
        <div className="legend">{t('datasets.fUrlHint')}</div>
        <div className="legend mt">{t('datasets.fGlobalHint')}</div>
      </Modal>
    </div>
  );
}
