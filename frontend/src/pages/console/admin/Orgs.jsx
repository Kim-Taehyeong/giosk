import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Building2, Coins, Users, ChevronRight } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import StatCard from '../../../components/console/StatCard';
import Pill from '../../../components/console/Pill';
import DataTable from '../../../components/console/DataTable';
import Modal from '../../../components/console/Modal';
import UserPicker from '../../../components/console/UserPicker';
import Advanced, { Req } from '../../../components/console/Advanced';
import BulkImport from '../../../components/console/BulkImport';
import { c, cU } from '../../../lib/credit';
import { useToast } from '../../../components/console/Toast';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { useAuth } from '../../../context/AuthContext';
import { activeLevelOf } from '../../../config/consoleRoles';
import { getOrgs, createOrg } from '../../../api/console/governance';

export default function Orgs() {
  const { t } = useTranslation('consoleAdmin');
  const { config } = useSystemConfig();
  const { user, activeScope } = useAuth();
  const isPlatform = activeLevelOf(user, activeScope) === 'platform'; // 조직 생성은 최고관리자 전용. 매니저(또는 스코프 채택)는 자기 조직만 열람.
  const creditMode = config.billing.mode === 'credit';
  const navigate = useNavigate();
  const [rows, setRows] = useState(null);
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ name: '', displayName: '', creditPool: 1000, adminAccount: '' });
  const { toast } = useToast();

  const load = () => getOrgs().then((d) => setRows(d.items));
  useEffect(() => { load(); }, []);

  const submit = async () => {
    try { await createOrg(form); } catch (e) {
      toast(e?.code === 'admin_unknown' ? t('orgs.adminUnknown')
        : e?.code === 'admin_required' ? t('orgs.adminRequired')
          : (e?.message || t('orgs.createFail'))); return;
    }
    setOpen(false); setForm({ name: '', displayName: '', creditPool: 1000, adminAccount: '' });
    toast(t('orgs.created', { n: form.creditPool })); load();
  };


  const TPL_HEADER = ['조직 식별자', '표시 이름', '크레딧 풀'];
  const TPL_SAMPLE = [['ai-center', 'AI Center', '1000'], ['eng-college', '공과대학', '500']];
  const onBulkRows = (data) => {
    const parsed = data.filter((r) => r[0]).map((r, i) => ({ id: Date.now() + i, name: r[0], displayName: r[1] || r[0], creditPool: Number(r[2]) || 0, consumed: 0, groupCount: 0, userCount: 0, status: 'active' }));
    if (parsed.length === 0) { toast(t('bulk.empty')); return; }
    setRows((p) => [...parsed, ...(p || [])]);
    toast(t('orgs.bulkDone', { n: parsed.length }));
  };

  const totalPool = (rows || []).reduce((a, o) => a + o.creditPool, 0);
  const totalUsers = (rows || []).reduce((a, o) => a + o.userCount, 0);

  return (
    <div>
      <PageHead title={t('orgs.title')} subtitle={t('orgs.subtitle')}
        actions={isPlatform && <span className="flex gap">
          <BulkImport filename="org-template.csv" header={TPL_HEADER} samples={TPL_SAMPLE} onRows={onBulkRows} />
          <button className="btn primary" onClick={() => setOpen(true)}>{t('orgs.newOrg')}</button>
        </span>} />
      <div className={`grid ${creditMode ? 'cols-3' : 'cols-2'} mb`}>
        <StatCard icon={Building2} tone="primary" label={t('orgs.count')} value={`${(rows || []).length}`} />
        {creditMode && <StatCard icon={Coins} tone="gpu" label={t('orgs.totalPool')} value={c(totalPool)} unit="C" />}
        <StatCard icon={Users} tone="free" label={t('orgs.totalUsers')} value={`${totalUsers}`} />
      </div>
      <div className="card">
        <DataTable
          rows={rows || []}
          rowKey={(r) => r.id}
          onRowClick={(r) => navigate(`/console/admin/orgs/${r.id}`)}
          columns={[
            { key: 'displayName', header: t('orgs.colOrg'), render: (r) => (<span>{r.displayName} <span className="muted mono" style={{ fontSize: 11.5 }}>({r.name})</span></span>) },
            { key: 'groupCount', header: t('orgs.colGroups') },
            { key: 'userCount', header: t('orgs.colUsers') },
            ...(creditMode ? [
              { key: 'creditPool', header: t('orgs.colPool', { defaultValue: '현재 잔여' }), render: (r) => <b>{cU(r.creditPool)}</b> },
              { key: 'refill', header: t('orgs.colRefill', { defaultValue: '정기 리필' }), render: (r) => (r.recurringCredit ? <span>{c(r.recurringCredit)} C <span className="muted" style={{ fontSize: 12 }}>/ {r.refillIntervalDays || t('orgs.refillDefault', { defaultValue: '기본' })}{r.refillIntervalDays ? '일' : ''}</span></span> : <span className="muted">—</span>) },
              { key: 'consumed', header: t('orgs.colConsumed'), render: (r) => cU(r.consumed) },
            ] : []),
            { key: 'status', header: t('orgs.colStatus'), render: (r) => <Pill variant="ok" dot>{r.status}</Pill> },
            { key: 'act', header: t('orgs.colAct'), className: 'flex', render: () => (
              // 관리·삭제 모두 행을 눌러 상세에서 — 리스트에선 실수 방지를 위해 위험 작업을 두지 않는다.
              <ChevronRight size={15} style={{ color: 'var(--muted)', alignSelf: 'center' }} />
            ) },
          ]}
        />
      </div>

      <Modal open={open} title={t('orgs.createTitle')} onClose={() => setOpen(false)} width={600}
        footer={<button className="btn primary" onClick={submit}>{t('common.create')}</button>}>
        {/* 필수는 식별자 하나뿐 — 나머지는 접어둔다(조직부터 만들고 사람은 나중에). */}
        <div className="grid cols-2" style={{ gap: 14 }}>
          <div>
            <label className="fld" htmlFor="admin-orgs-fld-0" style={{ marginTop: 0 }}>{t('orgs.ident')}<Req /></label>
            <input id="admin-orgs-fld-0" type="text" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="ai-center" />
          </div>
          <div>
            <label className="fld" htmlFor="admin-orgs-fld-1" style={{ marginTop: 0 }}>{t('orgs.display')}</label>
            <input id="admin-orgs-fld-1" type="text" value={form.displayName} onChange={(e) => setForm({ ...form, displayName: e.target.value })} placeholder="AI Center" />
          </div>
        </div>
        {creditMode && (
          <>
            <label className="fld" htmlFor="admin-orgs-fld-2">{t('orgs.pool')}</label>
            <input id="admin-orgs-fld-2" type="number" value={form.creditPool} onChange={(e) => setForm({ ...form, creditPool: Number(e.target.value) })} />
            <div className="legend mt">{t('orgs.poolHint')}</div>
          </>
        )}
        <Advanced title={t('orgs.admin')} hint={t('orgs.adminHintOpt')} defaultOpen={!!form.adminAccount}>
          <UserPicker value={form.adminAccount} onChange={(v) => setForm({ ...form, adminAccount: v })} placeholder={t('orgs.adminSearchPh')} />
        </Advanced>
      </Modal>


    </div>
  );
}
