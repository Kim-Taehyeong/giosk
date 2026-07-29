import React, { useEffect, useState } from 'react';
import { useParams, useNavigate, Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { ArrowLeft, Users, Coins, Layers, ChevronRight, Pencil, Trash2 } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import StatCard from '../../../components/console/StatCard';
import CreditGrantCard from '../../../components/console/CreditGrantCard';
import Toggle from '../../../components/console/Toggle';
import Pill from '../../../components/console/Pill';
import PagedTable from '../../../components/console/PagedTable';
import Modal from '../../../components/console/Modal';
import UserPicker from '../../../components/console/UserPicker';
import Advanced, { Req } from '../../../components/console/Advanced';
import { useToast } from '../../../components/console/Toast';
import { useConfirm } from '../../../components/console/Confirm';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { getOrgs, getGroups, createGroup, deleteGroup, grantOrgCredit, updateOrg, deleteOrg } from '../../../api/console/governance';
import { getUsers } from '../../../api/console/misc';
import { c, cU } from '../../../lib/credit';

const roleVariant = { org_admin: 'gpu', project_admin: 'primary', member: 'pause' };

// 정기 크레딧 카드 — 주기마다 조직 풀을 이 양으로 리필. 주기는 플랫폼 상한(maxInterval)을 넘을 수 없다.
function RecurringCreditCard({ org, maxInterval, onSave, t }) {
  const [v, setV] = useState(String(org?.recurringCredit ?? 0));
  const [iv, setIv] = useState(String(org?.refillIntervalDays || maxInterval || 30));
  const [carry, setCarry] = useState(!!org?.carryover);
  const clampIv = () => { const n = Number(iv) || 0; if (maxInterval && n > maxInterval) setIv(String(maxInterval)); };
  return (
    <div className="card" style={{ margin: 0 }}>
      <h3><Coins size={15} /> {t('odetail.recurringTitle', { defaultValue: '정기 크레딧' })}</h3>
      <div className="legend mb">{t('odetail.recurringNote', { defaultValue: '설정한 주기마다 이 양으로 조직 풀을 리필합니다(0이면 리필 안 함).' })}</div>
      <label className="fld" htmlFor="admin-orgdetail-fld-0" style={{ marginTop: 0 }}>{t('odetail.recurringAmount', { defaultValue: '주기당 크레딧' })}</label>
      <input id="admin-orgdetail-fld-0" type="number" min={0} value={v} onChange={(e) => setV(e.target.value)} />
      <div className="grid cols-2" style={{ gap: 12 }}>
        <div><label className="fld" htmlFor="admin-orgdetail-fld-1">{t('odetail.recurringInterval', { defaultValue: '리필 주기(일)' })}</label>
          <input id="admin-orgdetail-fld-1" type="number" min={1} max={maxInterval || undefined} value={iv} onChange={(e) => setIv(e.target.value)} onBlur={clampIv} /></div>
        <div><label className="fld">{t('odetail.recurringCarryover', { defaultValue: '이월' })}</label>
          <div className="flex" style={{ gap: 8, alignItems: 'center', height: 38 }}>
            <Toggle checked={carry} onChange={setCarry} />
            <span style={{ fontSize: 12.5 }}>{carry ? t('odetail.carryOn', { defaultValue: '이월' }) : t('odetail.carryOff', { defaultValue: '소멸' })}</span>
          </div></div>
      </div>
      {maxInterval > 0 && <div className="legend" style={{ marginTop: 6 }}>{t('odetail.maxIntervalHint', { n: maxInterval, defaultValue: `플랫폼 상한: ${maxInterval}일 이하` })}</div>}
      <button className="btn primary" style={{ marginTop: 12 }} onClick={() => onSave({ recurring: Number(v) || 0, interval: Number(iv) || 0, carryover: carry })}>{t('common.save', { defaultValue: '저장' })}</button>
    </div>
  );
}

export default function OrgDetail() {
  const { id } = useParams();
  const oid = Number(id);
  const navigate = useNavigate();
  const { t } = useTranslation('consoleAdmin');
  const { config } = useSystemConfig();
  const creditMode = config.billing.mode === 'credit';
  const { toast } = useToast();
  const confirm = useConfirm();

  const [org, setOrg] = useState(null);
  const [groups, setGroups] = useState([]);
  const [users, setUsers] = useState([]);
  const [userTotal, setUserTotal] = useState(0);
  const [newGroup, setNewGroup] = useState(null); // { name, displayName, adminAccount }
  const [edit, setEdit] = useState(null);    // { displayName, defaultGroupBudget }

  // 조직·그룹·사용자를 한 번에 로드 — 사용자 필터가 조직 표시명에 의존하므로 같은 스냅샷에서 뽑아야
  // 이름 수정 직후에도 어긋나지 않는다.
  const load = async () => {
    const [o, g] = await Promise.all([getOrgs(), getGroups()]);
    const cur = (o.items || []).find((x) => x.id === oid) || null;
    setOrg(cur);
    setGroups((g.items || []).filter((x) => x.orgId === oid));
    // 사용자는 서버에서 이 조직으로 걸러 받는다(전체를 내려받아 거르지 않는다).
    if (!cur) { setUsers([]); return; }
    const u = await getUsers({ org: cur.displayName, size: 200 });
    setUsers(u.items); setUserTotal(u.total);
  };
  useEffect(() => { load(); }, [oid]);

  const submitGrant = async (amount, reason) => {
    await grantOrgCredit(oid, { amount, reason });
    toast(t('orgs.granted', { name: org?.displayName, n: amount }));
    load();
  };
  // 정기 크레딧 설정 — 주기마다 이 값으로 조직 풀을 리필. 주기/이월도 함께(주기는 플랫폼 상한).
  const submitRecurring = async ({ recurring, interval, carryover }) => {
    await grantOrgCredit(oid, { amount: 0, recurring, interval, carryover });
    toast(t('odetail.recurringSaved', { defaultValue: '정기 크레딧을 설정했습니다.' }));
    load();
  };
  const submitGroup = async () => {
    try { await createGroup({ ...newGroup, orgId: oid }); } catch (e) {
      toast(e?.code === 'admin_unknown' ? t('groups.adminUnknown') : (e?.message || t('groups.createFail')));
      return;
    }
    setNewGroup(null); toast(t('groups.created')); load();
  };
  const removeGroup = async (g) => {
    if (!(await confirm({ title: t('groups.deleteTitle'), message: t('groups.deleteConfirm', { name: g.displayName }), confirmText: t('common.delete') }))) return;
    try { await deleteGroup(g.id); } catch (e) {
      toast(e?.code === 'default_group' ? t('groups.cantDeleteDefault', { defaultValue: "'일반' 팀은 삭제할 수 없습니다." })
        : e?.code === 'group_has_members' ? t('groups.hasMembers', { defaultValue: '팀에 멤버가 있어 삭제할 수 없습니다. 멤버를 옮기거나 제거하세요.' })
          : (e?.message || t('groups.deleteFail', { defaultValue: '삭제 실패' })));
      return;
    }
    toast(t('groups.deleted', { name: g.displayName })); load();
  };
  const saveEdit = async () => {
    await updateOrg(oid, { displayName: edit.displayName, defaultGroupBudget: Number(edit.defaultGroupBudget) || 0 });
    setEdit(null); toast(t('orgs.updated')); load();
  };
  // 조직 삭제 — 리스트가 아니라 여기서만(실수 방지). 성공 시 조직 목록으로.
  const removeOrg = async () => {
    if (!(await confirm({ title: t('orgs.deleteTitle'), message: t('orgs.deleteConfirm', { name: org?.displayName }), confirmText: t('common.delete') }))) return;
    try { await deleteOrg(oid); } catch (e) {
      toast(e?.code === 'org_has_users' ? t('orgs.hasUsers', { defaultValue: '조직에 사용자가 있어 삭제할 수 없습니다. 모든 사용자를 다른 조직으로 옮기거나 삭제하세요.' })
        : (e?.message || t('orgs.deleteFail', { defaultValue: '삭제 실패' })));
      return;
    }
    toast(t('orgs.deleted', { name: org?.displayName })); navigate('/console/admin/orgs');
  };

  return (
    <div>
      <PageHead
        crumb={<Link to="/console/admin/orgs" className="flex" style={{ gap: 5, alignItems: 'center', color: 'inherit' }}>
          <ArrowLeft size={12} /> {t('orgs.title')}</Link>}
        title={org?.displayName || t('odetail.title')}
        subtitle={t('odetail.subtitle', { name: org?.name || '' })}
        actions={<span className="flex gap">
          <button className="btn" onClick={() => setEdit({ displayName: org?.displayName || '', defaultGroupBudget: org?.defaultGroupBudget || 0 })}>
            <Pencil size={14} /> {t('common.edit')}</button>
          <button className="btn danger" onClick={removeOrg}><Trash2 size={14} /> {t('common.delete')}</button>
        </span>} />

      <div className={`grid ${creditMode ? 'cols-3' : 'cols-2'} mb`}>
        <StatCard icon={Layers} tone="primary" label={t('orgs.colGroups')} value={`${groups.length}`} />
        <StatCard icon={Users} tone="free" label={t('orgs.colUsers')} value={`${org?.userCount ?? users.length}`} />
        {creditMode && <StatCard icon={Coins} tone="gpu" label={t('orgs.colPool')} value={c(org?.creditPool)} unit="C" />}
      </div>

      {creditMode && (
        <div className="grid cols-2 mb" style={{ gap: 16, alignItems: 'start' }}>
          <CreditGrantCard balance={org?.creditPool} onGrant={submitGrant} hint={t('orgs.grantHint')} />
          <RecurringCreditCard key={org?.id} org={org} maxInterval={config.billing?.credit?.recharge?.intervalDays || 0} onSave={submitRecurring} t={t} />
        </div>
      )}

      {/* 조직 산하 그룹 — 그룹을 눌러 멤버 관리로 들어간다. */}
      <div className="card mb">
        <div className="flex mb" style={{ justifyContent: 'space-between', alignItems: 'center' }}>
          <h3 style={{ margin: 0 }}><Layers size={16} /> {t('odetail.groups')}</h3>
          <button className="btn primary" onClick={() => setNewGroup({ name: '', displayName: '', adminAccount: '' })}>
            {t('groups.newGroup')}</button>
        </div>
        <PagedTable pageSize={12}
          rows={groups}
          rowKey={(r) => r.id}
          columns={[
            { key: 'displayName', header: t('groups.colGroup'), render: (r) => (
              <span>{r.displayName} <span className="muted mono" style={{ fontSize: 11.5 }}>({r.name})</span></span>) },
            { key: 'memberCount', header: t('groups.colMembers'), render: (r) => t('groups.memberN', { n: r.memberCount }) },
            ...(creditMode ? [{ key: 'balance', header: t('groups.colWallet'), render: (r) => cU(r.balance) }] : []),
            { key: 'status', header: t('groups.colStatus'), render: (r) => <Pill variant="ok" dot>{r.status}</Pill> },
            { key: 'act', header: t('groups.colAct'), className: 'flex', render: (r) => (
              <span className="flex gap">
                <button className="btn sm" onClick={() => navigate(`/console/admin/groups/${r.id}`)}>
                  {t('odetail.manage')} <ChevronRight size={13} /></button>
                <button className="btn sm danger" onClick={() => removeGroup(r)}><Trash2 size={13} /> {t('common.delete')}</button>
              </span>) },
          ]}
        />
      </div>

      {/* 조직 소속 사용자 — 역할은 멤버십 역할(팀장/조직관리자)을 보여준다. */}
      <div className="card">
        <h3><Users size={16} /> {t('odetail.users')}</h3>
        <div className="legend mb">{t('odetail.usersHint')}{userTotal > users.length ? ` · ${t('odetail.usersCapped', { shown: users.length, total: userTotal })}` : ''}</div>
        <PagedTable pageSize={12}
          rows={users}
          rowKey={(r) => r.id}
          columns={[
            { key: 'name', header: t('users.colUser'), render: (r) => <span style={{ fontWeight: 600 }}>{r.name}</span> },
            { key: 'username', header: t('groups.colAccount'), className: 'mono' },
            { key: 'group', header: t('users.colGroup'), render: (r) => <Pill variant="primary">{r.group}</Pill> },
            { key: 'memberRole', header: t('odetail.memberRole'), render: (r) => {
              // 멀티롤 — 이 사용자가 가진 모든 관리 역할을 배지로. 관리 역할이 없으면 소속 역할/대시.
              const badges = r.roles || [];
              if (badges.length === 0) {
                return r.memberRole ? <Pill variant={roleVariant[r.memberRole] || 'pause'}>{t(`roles.${r.memberRole}`)}</Pill> : <span className="muted">—</span>;
              }
              return (
                <span className="flex gap" style={{ gap: 4, flexWrap: 'wrap' }}>
                  {badges.map((b) => (
                    <Pill key={`${b.role}-${b.groupId}`} variant={roleVariant[b.role] || 'pause'}>
                      {t(`roles.${b.role}`)} · {b.role === 'org_admin' ? b.orgName : b.groupName}
                    </Pill>
                  ))}
                </span>
              );
            } },
            { key: 'status', header: t('users.colStatus'), render: (r) => <Pill variant={r.status === 'approved' ? 'ok' : 'wait'} dot>{r.status}</Pill> },
            { key: 'act', header: t('users.colAct'), render: (r) => (
              r.groupId ? <button className="btn sm" onClick={() => navigate(`/console/admin/groups/${r.groupId}`)}>{t('odetail.inGroup')}</button> : null) },
          ]}
        />
      </div>

      <Modal open={!!newGroup} title={t('groups.createTitle')} onClose={() => setNewGroup(null)} width={600}
        footer={<>
          <button className="btn" onClick={() => setNewGroup(null)}>{t('common.cancel')}</button>
          <button className="btn primary" onClick={submitGroup}>{t('common.create')}</button>
        </>}>
        {newGroup && (<>
          <div className="grid cols-2" style={{ gap: 14 }}>
            <div>
              <label className="fld" htmlFor="admin-orgdetail-fld-2" style={{ marginTop: 0 }}>{t('groups.ident')}<Req /></label>
              <input id="admin-orgdetail-fld-2" type="text" value={newGroup.name} onChange={(e) => setNewGroup({ ...newGroup, name: e.target.value })} placeholder="ml-lab" />
            </div>
            <div>
              <label className="fld" htmlFor="admin-orgdetail-fld-3" style={{ marginTop: 0 }}>{t('groups.display')}</label>
              <input id="admin-orgdetail-fld-3" type="text" value={newGroup.displayName} onChange={(e) => setNewGroup({ ...newGroup, displayName: e.target.value })} />
            </div>
          </div>
          <Advanced title={t('groups.admin')} hint={t('groups.adminHintOpt')} defaultOpen={!!newGroup.adminAccount}>
            <UserPicker value={newGroup.adminAccount} placeholder={t('groups.adminSearchPh')}
              onChange={(v) => setNewGroup((g) => ({ ...g, adminAccount: v }))} />
          </Advanced>
        </>)}
      </Modal>

      <Modal open={!!edit} title={t('orgs.editTitle')} onClose={() => setEdit(null)} width={520}
        footer={<button className="btn primary" onClick={saveEdit}>{t('common.save')}</button>}>
        {edit && (<>
          <label className="fld" htmlFor="admin-orgdetail-fld-4" style={{ marginTop: 0 }}>{t('orgs.display')}</label>
          <input id="admin-orgdetail-fld-4" type="text" value={edit.displayName} onChange={(e) => setEdit({ ...edit, displayName: e.target.value })} />
          {creditMode && (<>
            <label className="fld" htmlFor="admin-orgdetail-fld-5">{t('orgs.defaultGroupBudget')}</label>
            <input id="admin-orgdetail-fld-5" type="number" value={edit.defaultGroupBudget} onChange={(e) => setEdit({ ...edit, defaultGroupBudget: e.target.value })} />
          </>)}
        </>)}
      </Modal>

    </div>
  );
}
