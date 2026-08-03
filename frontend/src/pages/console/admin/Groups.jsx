import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { ChevronRight } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import Pill from '../../../components/console/Pill';
import DataTable from '../../../components/console/DataTable';
import Modal from '../../../components/console/Modal';
import UserPicker from '../../../components/console/UserPicker';
import Advanced, { Req } from '../../../components/console/Advanced';
import BulkImport from '../../../components/console/BulkImport';
import { useToast } from '../../../components/console/Toast';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { getGroups, getOrgs, createGroup } from '../../../api/console/governance';


export default function Groups() {
  const { t } = useTranslation('consoleAdmin');
  const { config } = useSystemConfig();
  const creditMode = config.billing.mode === 'credit';
  const navigate = useNavigate();
  const [rows, setRows] = useState(null);
  const [orgs, setOrgs] = useState([]);
  const [openCreate, setOpenCreate] = useState(false);
  const [form, setForm] = useState({ orgId: 0, name: '', displayName: '', adminAccount: '', initialCredit: 0, recurring: 0, interval: 30, carryover: false });
  const { toast } = useToast();

  const [orgFilter, setOrgFilter] = useState('all');
  const load = () => getGroups().then((d) => setRows(d.items));
  useEffect(() => { load(); getOrgs().then((d) => setOrgs(d.items)); }, []);
  const orgNames = [...new Set((rows || []).map((g) => g.orgName).filter(Boolean))];
  const filteredRows = (rows || []).filter((g) => orgFilter === 'all' || g.orgName === orgFilter);

  const submitCreate = async () => {
    try { await createGroup(form); } catch (e) { toast(e?.code === 'admin_unknown' ? t('groups.adminUnknown') : (e?.message || t('groups.createFail'))); return; }
    setOpenCreate(false); setForm({ orgId: 0, name: '', displayName: '', adminAccount: '', initialCredit: 0, recurring: 0, interval: 30, carryover: false }); toast(t('groups.created')); load();
  };

  const TPL_HEADER = ['그룹 식별자', '표시 이름', '조직'];
  const TPL_SAMPLE = [['ml-lab', 'ML 연구실', 'AI센터'], ['vision-lab', '비전 연구실', 'AI센터']];
  const onBulkRows = (data) => {
    const parsed = data.filter((r) => r[0]).map((r, i) => ({ id: Date.now() + i, name: r[0], displayName: r[1] || r[0], orgName: r[2] || '', memberCount: 0, balance: 0, budgetCap: 0, status: 'active' }));
    if (parsed.length === 0) { toast(t('bulk.empty')); return; }
    setRows((p) => [...parsed, ...(p || [])]);
    toast(t('groups.bulkDone', { n: parsed.length }));
  };

  return (
    <div>
      <PageHead title={t('groups.title')} subtitle={t('groups.subtitle')}
        actions={<span className="flex gap">
          <select value={orgFilter} onChange={(e) => setOrgFilter(e.target.value)} style={{ width: 'auto' }}>
            <option value="all">{t('groups.allOrgs')}</option>
            {orgNames.map((o) => <option key={o} value={o}>{o}</option>)}
          </select>
          <BulkImport filename="group-template.csv" header={TPL_HEADER} samples={TPL_SAMPLE} onRows={onBulkRows} />
          <button className="btn primary" onClick={() => setOpenCreate(true)}>{t('groups.newGroup')}</button>
        </span>} />
      <div className="card mb">
        <DataTable
          rows={filteredRows}
          rowKey={(r) => r.id}
          onRowClick={(r) => navigate(`/console/admin/groups/${r.id}`)}
          columns={[
            { key: 'displayName', header: t('groups.colGroup'), render: (r) => (<span>{r.displayName} <span className="muted mono" style={{ fontSize: 11.5 }}>({r.name})</span></span>) },
            { key: 'orgName', header: t('groups.colOrg') },
            { key: 'memberCount', header: t('groups.colMembers'), render: (r) => t('groups.memberN', { n: r.memberCount }) },
            ...(creditMode ? [
              { key: 'wallet', header: t('groups.colWallet', { defaultValue: '현재 잔여' }), render: (r) => <b>{r.balance || 0} C</b> },
              { key: 'refill', header: t('groups.colRefill', { defaultValue: '정기 리필' }), render: (r) => (r.recurringCredit ? <span>{r.recurringCredit} C <span className="muted" style={{ fontSize: 12 }}>/ {r.refillIntervalDays || t('groups.refillDefault', { defaultValue: '기본' })}{r.refillIntervalDays ? '일' : ''}</span></span> : <span className="muted">—</span>) },
            ] : []),
            { key: 'status', header: t('groups.colStatus'), render: (r) => <Pill variant="ok" dot>{r.status}</Pill> },
            { key: 'act', header: t('groups.colAct'), className: 'flex', render: () => (
              // 관리·삭제 모두 행을 눌러 상세에서 — 리스트엔 위험 작업을 두지 않는다.
              <ChevronRight size={15} style={{ color: 'var(--muted)', alignSelf: 'center' }} />
            ) },
          ]}
        />
      </div>


      <Modal open={openCreate} title={t('groups.createTitle')} onClose={() => setOpenCreate(false)} width={600}
        footer={<button className="btn primary" onClick={submitCreate}>{t('common.create')}</button>}>
        <label className="fld" style={{ marginTop: 0 }}>{t('groups.org')}<Req /></label>
        <select value={form.orgId} onChange={(e) => setForm({ ...form, orgId: Number(e.target.value) })}>
          <option value={0}>{t('groups.select')}</option>
          {orgs.map((o) => <option key={o.id} value={o.id}>{o.displayName}</option>)}
        </select>
        <div className="grid cols-2" style={{ gap: 14 }}>
          <div>
            <label className="fld">{t('groups.ident')}<Req /></label>
            <input type="text" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="ml-lab" />
          </div>
          <div>
            <label className="fld">{t('groups.display')}</label>
            <input type="text" value={form.displayName} onChange={(e) => setForm({ ...form, displayName: e.target.value })} />
          </div>
        </div>
        <Advanced title={t('groups.admin')} hint={t('groups.adminHintOpt')} defaultOpen={!!form.adminAccount}>
          <UserPicker value={form.adminAccount} onChange={(v) => setForm({ ...form, adminAccount: v })} placeholder={t('groups.adminSearchPh')} />
        </Advanced>
        {creditMode && (
          <Advanced title={t('groups.initCredit', { defaultValue: '초기 크레딧 · 정기 리필' })} hint={t('groups.initCreditHint', { defaultValue: '팀 생성과 함께 조직 풀에서 배분·리필 설정(선택). 나중에 팀 상세에서도 가능.' })} defaultOpen={!!form.initialCredit || !!form.recurring}>
            <div className="grid cols-2" style={{ gap: 14 }}>
              <div>
                <label className="fld" style={{ marginTop: 0 }}>{t('groups.initCreditAmt', { defaultValue: '초기 크레딧' })}</label>
                <input type="number" min={0} value={form.initialCredit} onChange={(e) => setForm({ ...form, initialCredit: Math.max(0, Number(e.target.value)) })} placeholder="0" />
              </div>
              <div>
                <label className="fld" style={{ marginTop: 0 }}>{t('groups.refillAmt', { defaultValue: '정기 리필(회당)' })}</label>
                <input type="number" min={0} value={form.recurring} onChange={(e) => setForm({ ...form, recurring: Math.max(0, Number(e.target.value)) })} placeholder="0" />
              </div>
              <div>
                <label className="fld" style={{ marginTop: 0 }}>{t('groups.refillInterval', { defaultValue: '리필 주기(일)' })}</label>
                <input type="number" min={1} value={form.interval} onChange={(e) => setForm({ ...form, interval: Math.max(1, Number(e.target.value)) })} />
              </div>
              <label className="flex gap" style={{ alignItems: 'center', gap: 8, marginTop: 24 }}>
                <input type="checkbox" checked={form.carryover} onChange={(e) => setForm({ ...form, carryover: e.target.checked })} />
                {t('groups.carryover', { defaultValue: '미사용분 이월' })}
              </label>
            </div>
          </Advanced>
        )}
      </Modal>

    </div>
  );
}
