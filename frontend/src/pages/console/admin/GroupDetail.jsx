import React, { useEffect, useState } from 'react';
import { useParams, useNavigate, Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { ArrowLeft, Users, UserPlus, Trash2, Coins, Building2, ShieldCheck, ChevronRight, Pencil, RefreshCw, Clock } from 'lucide-react';
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
import { activeLevelOf } from '../../../config/consoleRoles';
import { getGroups, getMembers, addMember, updateMember, grantGroupCredit, updateGroup, deleteGroup, setGroupRefill, refillGroupNow, getGroupWallet, setMemberRefill, refillMemberNow } from '../../../api/console/governance';
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
  const { user, activeScope } = useAuth();
  const isPlatform = activeLevelOf(user, activeScope) === 'platform'; // 부여 회계: platform=예외(상위 백필) / org=조직 풀 차감
  const creditMode = config.billing.mode === 'credit';
  const { toast } = useToast();
  const confirm = useConfirm();

  const [group, setGroup] = useState(null);
  const [wallet, setWallet] = useState(null); // 팀 지갑(다음 리필일)
  const [members, setMembers] = useState([]);
  const [add, setAdd] = useState(null);     // { account, role }
  const [edit, setEdit] = useState(null);   // { displayName, acceptsJoin }
  const [refill, setRefill] = useState(null); // 멤버 리필 모달 { userId, name, recurring, interval, carryover }

  const loadMembers = () => getMembers(gid).then((d) => setMembers(d.items));
  const loadGroup = () => getGroups().then((d) => {
    setGroup((d.items || []).find((g) => g.id === gid) || null);
  });
  const loadWallet = () => { if (creditMode) getGroupWallet(gid).then(setWallet).catch(() => {}); };
  useEffect(() => { loadGroup(); loadMembers(); loadWallet(); }, [gid]); // eslint-disable-line

  // 다음 리필일 표시(백엔드 nextRefillAt ISO). 없으면 —.
  const fmtNext = (iso) => (iso ? new Date(iso).toLocaleDateString() : '—');
  // 팀 즉시 리필.
  const doGroupRefillNow = async () => {
    try { const r = await refillGroupNow(gid); toast(t('gdetail.refilledNow', { n: r?.amount || 0, defaultValue: `즉시 리필 +${r?.amount || 0} C` })); }
    catch (e) { toast(e?.code === 'no_recurring' ? t('gdetail.noRecurring', { defaultValue: '정기 리필 금액이 설정되지 않았습니다' }) : t('gdetail.refillFail', { defaultValue: '리필 실패' })); return; }
    loadGroup(); loadWallet();
  };
  // 멤버 리필 금액 저장.
  const saveMemberRefill = async () => {
    try { await setMemberRefill(gid, refill.userId, { recurring: Number(refill.recurring) || 0, interval: Number(refill.interval) || 0, carryover: !!refill.carryover }); }
    catch { toast(t('gdetail.refillFail', { defaultValue: '설정 실패' })); return; }
    toast(t('gdetail.memberRefillSaved', { name: refill.name, defaultValue: '정기 리필을 설정했습니다' })); setRefill(null);
  };
  // 멤버 즉시 리필.
  const doMemberRefillNow = async (m) => {
    try { const r = await refillMemberNow(gid, m.userId); toast(t('gdetail.refilledNow', { n: r?.amount || 0, defaultValue: `즉시 리필 +${r?.amount || 0} C` })); }
    catch (e) { toast(e?.code === 'no_recurring' ? t('gdetail.noRecurring', { defaultValue: '정기 리필 금액이 설정되지 않았습니다' }) : t('gdetail.refillFail', { defaultValue: '리필 실패' })); }
  };

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
    loadGroup(); loadWallet();
  };

  // 그룹 설정(표시명·가입 수락) — 목록에 있던 걸 상세로 옮겼다(관리는 상세 한 곳에서).
  const saveEdit = async () => {
    await updateGroup(gid, { displayName: edit.displayName, acceptsJoin: edit.acceptsJoin });
    setEdit(null); toast(t('groups.updated')); loadGroup();
  };
  // 그룹 삭제 — 리스트가 아니라 여기서만(실수 방지). 성공 시 그룹 목록으로.
  const removeGroup = async () => {
    if (!(await confirm({ title: t('groups.deleteTitle'), message: t('groups.deleteConfirm', { name: group?.displayName }), confirmText: t('common.delete') }))) return;
    try { await deleteGroup(gid); } catch (e) {
      toast(e?.code === 'default_group' ? t('groups.cantDeleteDefault', { defaultValue: "'일반' 팀은 삭제할 수 없습니다." })
        : e?.code === 'group_has_members' ? t('groups.hasMembers', { defaultValue: '팀에 멤버가 있어 삭제할 수 없습니다. 멤버를 옮기거나 제거하세요.' })
          : (e?.message || t('groups.deleteFail', { defaultValue: '삭제 실패' })));
      return;
    }
    toast(t('groups.deleted', { name: group?.displayName })); navigate('/console/admin/groups');
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
          <div>
            <RefillCard key={group?.id} title={t('gdetail.refillTitle', { defaultValue: '팀 정기 리필' })}
              current={{ recurringCredit: group?.recurringCredit, refillIntervalDays: group?.refillIntervalDays, carryover: group?.carryover }}
              onSave={submitGroupRefill} />
            <div className="flex" style={{ gap: 10, alignItems: 'center', marginTop: 10, flexWrap: 'wrap' }}>
              <span className="muted flex" style={{ gap: 5, alignItems: 'center', fontSize: 12.5 }}>
                <Clock size={13} /> {t('gdetail.nextRefill', { defaultValue: '다음 리필' })}: <b>{wallet?.recurringCredit ? fmtNext(wallet?.nextRefillAt) : t('gdetail.noRecurringShort', { defaultValue: '정기 리필 미설정' })}</b></span>
              <button className="btn sm" onClick={doGroupRefillNow}>
                <RefreshCw size={13} /> {t('gdetail.refillNow', { defaultValue: '지금 리필' })}</button>
            </div>
          </div>
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
            ...(creditMode ? [{ key: 'recurring', header: t('gdetail.refillCol', { defaultValue: '정기 리필' }), render: (r) => (r.recurringCredit ? <span style={{ fontWeight: 600 }}>{c(r.recurringCredit)} C</span> : <span className="muted">—</span>) }] : []),
            { key: 'act', header: '', className: 'flex', render: (r) => (
              <span className="flex gap" style={{ gap: 6, alignItems: 'center' }}>
                {creditMode && (<>
                  <button className="btn sm" title={t('gdetail.memberRefillSet', { defaultValue: '정기 리필 설정' })}
                    onClick={(e) => { e.stopPropagation(); setRefill({ userId: r.userId, name: r.name, recurring: r.recurringCredit || '', interval: '', carryover: false }); }}>
                    <Coins size={13} /> {t('gdetail.refillMenu', { defaultValue: '리필' })}</button>
                  <button className="btn sm" title={t('gdetail.refillNow', { defaultValue: '지금 리필' })}
                    onClick={(e) => { e.stopPropagation(); doMemberRefillNow(r); }}><RefreshCw size={13} /></button>
                </>)}
                <ChevronRight size={15} style={{ color: 'var(--muted)' }} />
              </span>) },
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

      <Modal open={!!refill} title={t('gdetail.memberRefillTitle', { name: refill?.name, defaultValue: `정기 리필 — ${refill?.name || ''}` })} onClose={() => setRefill(null)} width={480}
        footer={<>
          <button className="btn" onClick={() => setRefill(null)}>{t('common.cancel')}</button>
          <button className="btn primary" onClick={saveMemberRefill}>{t('common.save')}</button>
        </>}>
        {refill && (<>
          <label className="fld" style={{ marginTop: 0 }}>{t('gdetail.refillAmount', { defaultValue: '정기 리필 금액 (C)' })}</label>
          <input type="number" min={0} value={refill.recurring} onChange={(e) => setRefill({ ...refill, recurring: e.target.value })} placeholder="0" />
          <label className="fld">{t('gdetail.refillInterval', { defaultValue: '주기 (일) — 비우면 팀 기본' })}</label>
          <input type="number" min={0} value={refill.interval} onChange={(e) => setRefill({ ...refill, interval: e.target.value })} placeholder={t('gdetail.refillInheritPh', { defaultValue: '상속' })} />
          <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13.5, cursor: 'pointer', marginTop: 12 }}>
            <input type="checkbox" checked={refill.carryover} onChange={(e) => setRefill({ ...refill, carryover: e.target.checked })} />
            {t('gdetail.refillCarryover', { defaultValue: '미사용분 이월(carryover)' })}
          </label>
          <div className="legend mt">{t('gdetail.memberRefillHint', { defaultValue: '설정한 금액이 주기마다 이 멤버 지갑에 지급됩니다. "지금 리필"로 즉시 지급도 가능합니다.' })}</div>
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
