import React, { useEffect, useState } from 'react';
import { useParams, useNavigate, Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { ArrowLeft, Users, UserPlus, Trash2, Coins, Building2, ShieldCheck, ChevronRight, Pencil } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import StatCard from '../../../components/console/StatCard';
import CreditGrantCard from '../../../components/console/CreditGrantCard';
import Pill from '../../../components/console/Pill';
import DataTable from '../../../components/console/DataTable';
import Modal from '../../../components/console/Modal';
import Select from '../../../components/console/Select';
import UserPicker from '../../../components/console/UserPicker';
import { useToast } from '../../../components/console/Toast';
import { useConfirm } from '../../../components/console/Confirm';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { useAuth } from '../../../context/AuthContext';
import { getGroups, getMembers, addMember, updateMember, grantGroupCredit, updateGroup, deleteGroup, setGroupRefill } from '../../../api/console/governance';
import RefillCard from '../../../components/console/RefillCard';
import { c } from '../../../lib/credit';

// 멤버십 역할 — 조직/팀 권한은 전부 여기서 정해진다(플랫폼 role=member|admin 과 별개).
const ROLES = ['org_admin', 'project_admin', 'member'];

export default function GroupDetail() {
  const { id } = useParams();
  const gid = Number(id);
  const navigate = useNavigate();
  const { t } = useTranslation('consoleAdmin');
  const { config } = useSystemConfig();
  const { user } = useAuth();
  const isPlatform = user?.role === 'admin'; // 부여 회계: platform=예외(상위 백필) / org=조직 풀 차감
  const creditMode = config.billing.mode === 'credit';
  const { toast } = useToast();
  const confirm = useConfirm();

  const [group, setGroup] = useState(null);
  const [members, setMembers] = useState([]);
  const [add, setAdd] = useState(null);     // { account, role }
  const [edit, setEdit] = useState(null);   // { displayName, acceptsJoin }

  const loadMembers = () => getMembers(gid).then((d) => setMembers(d.items));
  const loadGroup = () => getGroups().then((d) => {
    setGroup((d.items || []).find((g) => g.id === gid) || null);
  });
  useEffect(() => { loadGroup(); loadMembers(); }, [gid]);

  const submitAdd = async () => {
    if (!add.account.trim()) { toast(t('gdetail.needAccount')); return; }
    try { await addMember(gid, { account: add.account, role: add.role }); } catch (e) {
      toast(e?.status === 404 ? t('gdetail.userUnknown') : (e?.message || t('gdetail.addFail')));
      return; // 실패 시 모달 유지(원인 표시)
    }
    setAdd(null); toast(t('gdetail.added', { name: add.account })); loadMembers(); loadGroup();
  };
  const setRole = async (m, role) => {
    await updateMember(gid, m.userId, { role });
    toast(t('gdetail.roleSet', { name: m.name, role })); loadMembers();
  };
  const submitGrant = async (amount, reason) => {
    try {
      await grantGroupCredit(gid, { amount, reason });
    } catch (e) {
      toast(e?.code === 'insufficient_org_pool' ? t('groups.insufficientOrgPool') : (e?.message || t('groups.grantFail')));
      return;
    }
    toast(t('groups.granted', { name: group?.displayName, n: amount }));
    loadGroup();
  };
  // 팀 정기 리필 설정 — 주기는 조직 상한으로 백엔드가 클램프.
  const submitGroupRefill = async (spec) => {
    try { await setGroupRefill(gid, spec); toast(t('gdetail.refillSaved', { defaultValue: '정기 리필을 설정했습니다.' })); }
    catch { toast(t('gdetail.refillFail', { defaultValue: '설정 실패' })); }
    loadGroup();
  };

  // 그룹 설정(표시명·가입 수락) — 목록에 있던 걸 상세로 옮겼다(관리는 상세 한 곳에서).
  const saveEdit = async () => {
    await updateGroup(gid, { displayName: edit.displayName, acceptsJoin: edit.acceptsJoin });
    setEdit(null); toast(t('groups.updated')); loadGroup();
  };
  // 그룹 삭제 — 리스트가 아니라 여기서만(실수 방지). 성공 시 그룹 목록으로.
  const removeGroup = async () => {
    if (!(await confirm({ title: t('groups.deleteTitle'), message: t('groups.deleteConfirm', { name: group?.displayName }), confirmText: t('common.delete') }))) return;
    await deleteGroup(gid); toast(t('groups.deleted', { name: group?.displayName })); navigate('/console/admin/groups');
  };

  const memberIds = new Set(members.map((m) => m.userId));

  return (
    <div>
      <PageHead
        crumb={<Link to="/console/admin/groups" className="flex" style={{ gap: 5, alignItems: 'center', color: 'inherit' }}>
          <ArrowLeft size={12} /> {t('groups.title')}</Link>}
        title={group?.displayName || t('gdetail.title')}
        subtitle={group?.orgName ? t('gdetail.subtitleOrg', { org: group.orgName, name: group?.name }) : t('gdetail.subtitle')}
        actions={<span className="flex gap">
          <button className="btn" onClick={() => setEdit({ displayName: group?.displayName || '', acceptsJoin: !!group?.acceptsJoin })}>
            <Pencil size={14} /> {t('common.edit')}</button>
          <button className="btn danger" onClick={removeGroup}><Trash2 size={14} /> {t('common.delete')}</button>
        </span>} />

      <div className={`grid ${creditMode ? 'cols-3' : 'cols-2'} mb`}>
        <StatCard icon={Users} tone="primary" label={t('groups.colMembers')} value={`${members.length}`} />
        <StatCard icon={Building2} tone="free" label={t('groups.colOrg')} value={group?.orgName || '—'} />
        {creditMode && <StatCard icon={Coins} tone="gpu" label={t('groups.colWallet')} value={c(group?.balance)} unit="C" />}
      </div>

      {creditMode && (
        <div className="grid cols-2 mb" style={{ gap: 16, alignItems: 'start' }}>
          <CreditGrantCard balance={group?.balance} onGrant={submitGrant} hint={isPlatform ? t('groups.grantHintSuper') : t('groups.grantHintOrg')} />
          <RefillCard key={group?.id} title={t('gdetail.refillTitle', { defaultValue: '팀 정기 리필' })}
            current={{ recurringCredit: group?.recurringCredit, refillIntervalDays: group?.refillIntervalDays, carryover: group?.carryover }}
            onSave={submitGroupRefill} />
        </div>
      )}

      <div className="card">
        <div className="flex mb" style={{ justifyContent: 'space-between', alignItems: 'center' }}>
          <h3 style={{ margin: 0 }}><Users size={16} /> {t('gdetail.members')}</h3>
          <button className="btn primary" onClick={() => setAdd({ account: '', role: 'member' })}>
            <UserPlus size={14} /> {t('gdetail.addMember')}</button>
        </div>
        <div className="legend mb flex" style={{ alignItems: 'flex-start', gap: 6 }}><ShieldCheck size={14} style={{ flex: '0 0 auto', marginTop: 1 }} /> <span>{t('gdetail.roleHint')}</span></div>
        <DataTable
          rows={members}
          rowKey={(r) => r.userId}
          onRowClick={(r) => navigate(`/console/admin/users/${r.userId}`)}
          columns={[
            { key: 'name', header: t('groups.colMember'), render: (r) => {
              // 멀티롤 배지 — 이 사용자가 '다른' 그룹/조직에서 갖는 관리 역할(이 그룹 역할은 아래 Select).
              const others = (r.roles || []).filter((b) => b.groupId !== gid);
              return (
                <div>
                  <span style={{ fontWeight: 600 }}>{r.name}</span>
                  {others.length > 0 && (
                    <div className="flex gap" style={{ gap: 4, marginTop: 3, flexWrap: 'wrap' }}>
                      {others.map((b) => (
                        <Pill key={`${b.role}-${b.groupId}`} variant={b.role === 'org_admin' ? 'gpu' : 'primary'}>
                          {t(`roles.${b.role}`)} · {b.role === 'org_admin' ? b.orgName : b.groupName}
                        </Pill>
                      ))}
                    </div>
                  )}
                </div>
              );
            } },
            { key: 'username', header: t('groups.colAccount'), className: 'mono' },
            { key: 'role', header: t('groups.colRole'), render: (r) => (
              <Select size="sm" value={r.role} onChange={(v) => setRole(r, v)}
                options={ROLES.map((x) => ({ value: x, label: t(`roles.${x}`) }))} />) },
            { key: 'status', header: t('groups.colStatus'), render: (r) => <Pill variant={r.status === 'active' ? 'ok' : 'wait'} dot>{r.status}</Pill> },
            { key: 'act', header: '', render: () => <ChevronRight size={15} style={{ color: 'var(--muted)' }} /> },
          ]}
        />
      </div>

      <Modal open={!!add} title={t('gdetail.addTitle')} onClose={() => setAdd(null)} width={600}
        footer={<>
          <button className="btn" onClick={() => setAdd(null)}>{t('common.cancel')}</button>
          <button className="btn primary" onClick={submitAdd}>{t('gdetail.addMember')}</button>
        </>}>
        {add && (<>
          <label className="fld" style={{ marginTop: 0 }}>{t('gdetail.account')}</label>
          {/* 이미 멤버인 사용자는 후보에서 제외 — 중복 추가 시도를 원천 차단. */}
          <UserPicker value={add.account} placeholder={t('gdetail.searchUser')}
            filter={(u) => !memberIds.has(u.id)}
            onChange={(v) => setAdd((a) => ({ ...a, account: v }))} />
          <label className="fld">{t('groups.colRole')}</label>
          <Select value={add.role} onChange={(v) => setAdd({ ...add, role: v })}
            options={ROLES.map((x) => ({ value: x, label: t(`roles.${x}`) }))} />
          <div className="legend mt">{t('gdetail.roleHint')}</div>
        </>)}
      </Modal>

      <Modal open={!!edit} title={t('groups.editTitle')} onClose={() => setEdit(null)} width={520}
        footer={<>
          <button className="btn" onClick={() => setEdit(null)}>{t('common.cancel')}</button>
          <button className="btn primary" onClick={saveEdit}>{t('common.save')}</button>
        </>}>
        {edit && (<>
          <label className="fld" style={{ marginTop: 0 }}>{t('groups.display')}</label>
          <input type="text" value={edit.displayName} onChange={(e) => setEdit({ ...edit, displayName: e.target.value })} />
          <label className="fld">{t('groups.acceptsJoin')}</label>
          <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13.5, cursor: 'pointer' }}>
            <input type="checkbox" checked={edit.acceptsJoin} onChange={(e) => setEdit({ ...edit, acceptsJoin: e.target.checked })} />
            {t('groups.acceptsJoinHint')}
          </label>
        </>)}
      </Modal>

    </div>
  );
}
