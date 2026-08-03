import React, { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { ArrowLeft, Coins, Timer, Server, HardDrive, Database, UserCheck, Mail, Building2, Users, ArrowRightLeft, Plus, Trash2 } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import StatCard from '../../../components/console/StatCard';
import DataTable from '../../../components/console/DataTable';
import PagedTable from '../../../components/console/PagedTable';
import Pill from '../../../components/console/Pill';
import Select from '../../../components/console/Select';
import Modal from '../../../components/console/Modal';
import Spinner from '../../../components/console/Spinner';
import { useToast } from '../../../components/console/Toast';
import { useConfirm } from '../../../components/console/Confirm';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { useAuth } from '../../../context/AuthContext';
import { activeLevelOf } from '../../../config/consoleRoles';
import { getUserDetail, grantUserCredit, updateUserStatus } from '../../../api/console/misc';
import { getGroups, getOrgs, addMember, updateMember, moveMember, removeMember } from '../../../api/console/governance';
import { c, cU } from '../../../lib/credit';

const MROLES = ['org_admin', 'project_admin', 'member'];

const roleVariant = { member: 'pause', admin: 'err' };
const memberRoleVariant = { org_admin: 'gpu', project_admin: 'primary', member: 'pause' };
const statusVariant = { approved: 'ok', pending: 'wait', suspended: 'err', rejected: 'err', running: 'run', stopped: 'pause', terminated: 'pause', failed: 'err', ready: 'ok', loading: 'wait' };
const sessVariant = { running: 'run', provisioning: 'wait', stopped: 'pause', terminated: 'pause', failed: 'err' };
// 세션 상태 → monitor.st* i18n 키(원문 영어 노출 방지).
const SESS_ST = { running: 'stRunning', provisioning: 'stProvisioning', paused: 'stPaused', stopped: 'stStopped', terminated: 'stTerminated', failed: 'stFailed', queued: 'stProvisioning' };

const fmtDate = (s) => (s ? new Date(s).toLocaleString() : '—');

// 섹션 카드 — 제목 + 개수 배지 + 테이블(비면 안내).
function Section({ icon: Icon, title, count, empty, children }) {
  return (
    <div className="card mb">
      <h3 className="flex" style={{ alignItems: 'center', gap: 8 }}>
        <Icon size={16} /> {title}
        <span className="muted" style={{ fontSize: 12.5, fontWeight: 600 }}>({count})</span>
      </h3>
      {count === 0 ? <div className="legend">{empty}</div> : children}
    </div>
  );
}

export default function UserDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { t } = useTranslation('consoleAdmin');
  const { config } = useSystemConfig();
  const creditMode = config.billing.mode === 'credit';

  const { user, activeScope } = useAuth();
  const isPlatform = activeLevelOf(user, activeScope) === 'platform'; // 크레딧 부여·정지/복구는 플랫폼 전용
  const { toast } = useToast();
  const confirm = useConfirm();
  const [grant, setGrant] = useState(null); // 크레딧 부여 모달 { amount }
  const [d, setD] = useState(null);
  const [err, setErr] = useState(false);
  const [allGroups, setAllGroups] = useState([]);
  const [orgs, setOrgs] = useState([]);
  const [move, setMove] = useState(null); // { fromGroupId, toGroupId }
  const [addMem, setAddMem] = useState(null); // 소속 추가 모달 { orgId, groupId, role }

  const load = () => getUserDetail(id).then(setD).catch(() => setErr(true));
  useEffect(() => { setD(null); setErr(false); load(); /* eslint-disable-next-line */ }, [id]);
  useEffect(() => { getGroups().then((x) => setAllGroups(x.items || [])).catch(() => {}); getOrgs().then((x) => setOrgs(x.items || [])).catch(() => {}); }, []);

  if (err) return <div className="card">{t('userDetail.notFound')}</div>;
  if (!d) return <Spinner pad label={t('userDetail.loading', { defaultValue: '…' })} />;

  const u = d.user || {};
  // 전체 소속(다중 조직/팀) — 백엔드 memberships. 각 소속은 동일한 형식으로 나열한다.
  const memberships = d.memberships || [];
  const setRole = async (groupId, role) => { await updateMember(groupId, u.id, { role }); toast(t('userDetail.roleSet')); load(); };
  const doMove = async () => {
    if (!move.toGroupId || !move.fromGroupId) return;
    try { await moveMember(move.fromGroupId, u.id, { toGroupId: Number(move.toGroupId) }); } catch (e) { toast(e?.message || t('userDetail.moveFail')); return; }
    const to = allGroups.find((g) => g.id === Number(move.toGroupId));
    setMove(null); toast(t('userDetail.moved', { group: to?.displayName || '' })); load();
  };
  const doRemove = async (groupId, groupName) => {
    if (!(await confirm({ title: t('userDetail.removeTitle'), message: t('userDetail.removeConfirm', { group: groupName }), confirmText: t('userDetail.remove'), danger: true }))) return;
    await removeMember(groupId, u.id); toast(t('userDetail.removed')); load();
  };
  // 소속 추가 — 다른 조직/팀에도 참여시킨다(다중 소속). 조직 선택 시 그 조직의 팀만 후보.
  const submitAdd = async () => {
    if (!addMem.groupId) { toast(t('userDetail.pickGroup', { defaultValue: '팀을 선택하세요' })); return; }
    try { await addMember(Number(addMem.groupId), { account: u.username, role: addMem.role || 'member' }); toast(t('userDetail.memberAdded', { defaultValue: '소속을 추가했습니다.' })); }
    catch (e) { toast(e?.message || t('userDetail.addFail', { defaultValue: '추가 실패' })); return; }
    setAddMem(null); load();
  };
  // 크레딧 부여(플랫폼) — 잔액에 즉시 반영, 감사 로그 기록.
  const submitGrant = async () => {
    const amt = Number(grant.amount);
    if (!amt) { setGrant(null); return; }
    try { await grantUserCredit(u.id, { amount: amt }); toast(t('userDetail.granted', { n: amt, defaultValue: `${c(amt)} C 부여됨` })); }
    catch { toast(t('userDetail.grantFail', { defaultValue: '부여 실패' })); }
    setGrant(null); load();
  };
  const setStatus = async (status, msg) => {
    if (!(await confirm({ title: msg, message: t('userDetail.statusConfirm', { defaultValue: '이 사용자의 상태를 변경합니다.' }), confirmText: msg, danger: status !== 'approved' }))) return;
    await updateUserStatus(u.id, status); toast(msg); load();
  };

  const wallet = d.wallet || {};
  const usage = d.usage || {};
  const volumes = (d.volumes?.owned) || [];
  const sessions = d.sessions || [];
  // 데이터셋 "요청"만 — mine 은 승인된 소유 데이터셋과 대기중 요청을 함께 담는다(요청=pending).
  const datasetReqs = (d.datasets?.mine || []).filter((x) => x.status === 'pending');
  const joinReqs = d.joinRequests || [];

  return (
    <div>
      <button className="btn sm" style={{ marginBottom: 12 }} onClick={() => navigate('/console/admin/users')}>
        <ArrowLeft size={13} /> {t('userDetail.back')}
      </button>
      <PageHead
        title={u.name || u.username}
        subtitle={
          <span className="flex" style={{ gap: 10, alignItems: 'center', flexWrap: 'wrap' }}>
            <span className="mono">{u.username}</span>
            {u.email && <span className="flex muted" style={{ gap: 4, alignItems: 'center' }}><Mail size={12} /> {u.email}</span>}
            {u.org && <span className="muted">· {u.org}</span>}
            {u.group && <span className="muted">/ {u.group}</span>}
          </span>
        }
        actions={
          <span className="flex" style={{ gap: 6, alignItems: 'center', flexWrap: 'wrap' }}>
            {/* 계정 역할 + 상태만 — 소속별 역할은 아래 "소속 & 권한"에서 일괄 표시 */}
            <Pill variant={roleVariant[u.role] || 'pause'}>{u.role}</Pill>
            <Pill variant={statusVariant[u.status] || 'pause'} dot>{u.status}</Pill>
            {isPlatform && creditMode && <button className="btn sm" onClick={() => setGrant({ amount: 100 })}><Coins size={13} /> {t('userDetail.grant', { defaultValue: '크레딧 부여' })}</button>}
            {isPlatform && u.status === 'approved' && <button className="btn sm warn" onClick={() => setStatus('suspended', t('userDetail.suspend', { defaultValue: '정지' }))}>{t('userDetail.suspend', { defaultValue: '정지' })}</button>}
            {isPlatform && u.status === 'suspended' && <button className="btn sm ok" onClick={() => setStatus('approved', t('userDetail.restore', { defaultValue: '복구' }))}>{t('userDetail.restore', { defaultValue: '복구' })}</button>}
          </span>
        } />

      {/* KPI — 과금 모드에 맞게(크레딧 위젯은 credit 모드만) */}
      <div className="grid cols-4 mb">
        {creditMode && <StatCard icon={Coins} tone="gpu" label={t('userDetail.balance')} value={c(wallet.balance)} unit="C" />}
        <StatCard icon={Timer} tone="primary" label={t('userDetail.gpuHours')} value={`${usage.gpuHours ?? 0}`} unit="h" />
        {creditMode && <StatCard icon={Coins} tone="warn" label={t('userDetail.consumed')} value={c(usage.consumed)} unit="C" />}
        <StatCard icon={Server} tone="free" label={t('userDetail.sessions')} value={`${sessions.length}`} unit={t('userDetail.total')} />
        <StatCard icon={HardDrive} tone="gpu" label={t('userDetail.volumes')} value={`${volumes.length}`} unit={t('userDetail.total')} />
      </div>

      {/* 소속 & 권한 — 다중 조직/팀을 동일한 형식으로 평면 나열(각 소속마다 역할·이동·제거) */}
      <div className="card mb">
        <h3 className="flex" style={{ justifyContent: 'space-between' }}>
          <span className="flex gap"><Building2 size={16} /> {t('userDetail.membership')}</span>
          <button className="btn sm" onClick={() => setAddMem({ orgId: orgs[0]?.id || 0, groupId: 0, role: 'member' })}><Plus size={13} /> {t('userDetail.addMembership', { defaultValue: '소속 추가' })}</button>
        </h3>
        {memberships.length === 0 ? (
          <div className="legend">{t('userDetail.noMembership')}</div>
        ) : (
          <div>
            {memberships.map((m, i) => (
              <div key={`${m.groupId}-${i}`} className="flex" style={{ gap: 16, alignItems: 'center', flexWrap: 'wrap', padding: '12px 2px', borderTop: i ? '1px solid var(--border)' : 'none' }}>
                <div className="flex" style={{ gap: 6, alignItems: 'center', minWidth: 150 }}><Building2 size={14} className="muted" /> <span style={{ fontWeight: 700 }}>{m.orgName || '—'}</span></div>
                <div className="flex" style={{ gap: 6, alignItems: 'center', minWidth: 130 }}><Users size={14} className="muted" /> <span style={{ fontWeight: 600 }}>{m.groupName}</span></div>
                <div className="flex" style={{ gap: 6, alignItems: 'center' }}>
                  <span className="muted" style={{ fontSize: 12.5 }}>{t('userDetail.role')}</span>
                  <Select size="sm" value={m.role || 'member'} onChange={(v) => setRole(m.groupId, v)}
                    options={MROLES.map((x) => ({ value: x, label: t(`roles.${x}`, { defaultValue: x }) }))} />
                </div>
                <div className="flex" style={{ gap: 6, marginLeft: 'auto' }}>
                  <button className="btn sm" onClick={() => setMove({ fromGroupId: m.groupId, toGroupId: 0 })}><ArrowRightLeft size={13} /> {t('userDetail.move')}</button>
                  <button className="btn sm danger" onClick={() => doRemove(m.groupId, m.groupName)}><Trash2 size={13} /> {t('userDetail.remove')}</button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* 개인 정기 리필은 폐지 — 크레딧/리필은 팀 단위로 관리한다(팀 상세의 '팀 정기 리필' +
          팀→멤버 배분). 개인은 세션이 뜨는 팀 컨텍스트의 크레딧을 사용한다. */}

      <Section icon={Server} title={t('userDetail.sessionsTitle')} count={sessions.length} empty={t('userDetail.noSessions')}>
        <DataTable rows={sessions} rowKey={(r) => r.id}
          onRowClick={() => navigate('/console/admin/sessions')}
          columns={[
            { key: 'status', header: t('userDetail.status'), render: (r) => <Pill variant={sessVariant[r.status] || 'wait'} dot>{SESS_ST[r.status] ? t(`monitor.${SESS_ST[r.status]}`) : r.status}</Pill> },
            { key: 'name', header: t('userDetail.name') },
            { key: 'gpuType', header: 'GPU', className: 'mono', render: (r) => r.gpuType ? `${r.gpuType}${r.gpuCount > 1 ? ` ×${r.gpuCount}` : ''}` : '—' },
            { key: 'node', header: t('userDetail.node'), className: 'mono', render: (r) => r.node || '—' },
            ...(creditMode ? [{ key: 'billedCredits', header: t('userDetail.billed'), render: (r) => cU(r.billedCredits) }] : []),
            { key: 'createdAt', header: t('userDetail.created'), className: 'mono', render: (r) => fmtDate(r.createdAt) },
          ]} />
      </Section>

      <Section icon={HardDrive} title={t('userDetail.volumesTitle')} count={volumes.length} empty={t('userDetail.noVolumes')}>
        <DataTable rows={volumes} rowKey={(r) => r.id}
          columns={[
            { key: 'name', header: t('userDetail.name') },
            { key: 'kind', header: t('userDetail.kind'), render: (r) => r.kind || '—' },
            { key: 'cap', header: t('userDetail.capacity'), render: (r) => `${r.usedGb ?? 0} / ${r.capGb ?? 0} GB` },
            { key: 'accessMode', header: t('userDetail.access'), className: 'mono', render: (r) => r.accessMode || '—' },
            { key: 'status', header: t('userDetail.status'), render: (r) => <Pill variant={statusVariant[r.status] || 'pause'} dot>{r.status}</Pill> },
          ]} />
      </Section>

      <Section icon={Database} title={t('userDetail.datasetReqTitle')} count={datasetReqs.length} empty={t('userDetail.noDatasetReq')}>
        <PagedTable rows={datasetReqs} pageSize={10} rowKey={(r) => r.id}
          columns={[
            { key: 'name', header: t('userDetail.name') },
            { key: 'sizeClass', header: t('userDetail.size'), render: (r) => `${r.sizeClass || ''} ${r.sizeGb ? `(${r.sizeGb}GB)` : ''}`.trim() || '—' },
            { key: 'scope', header: t('userDetail.scope'), className: 'mono', render: (r) => r.scope || '—' },
            { key: 'status', header: t('userDetail.status'), render: (r) => <Pill variant={statusVariant[r.status] || 'wait'} dot>{r.status}</Pill> },
          ]} />
      </Section>

      <Section icon={UserCheck} title={t('userDetail.joinReqTitle')} count={joinReqs.length} empty={t('userDetail.noJoinReq')}>
        <DataTable rows={joinReqs} rowKey={(r) => r.id}
          columns={[
            { key: 'groupName', header: t('userDetail.group') },
            { key: 'orgName', header: t('userDetail.org'), render: (r) => r.orgName || '—' },
            { key: 'status', header: t('userDetail.status'), render: (r) => <Pill variant={statusVariant[r.status] || 'wait'} dot>{r.status}</Pill> },
            { key: 'requestedAt', header: t('userDetail.requested'), className: 'mono', render: (r) => fmtDate(r.requestedAt) },
          ]} />
      </Section>

      {creditMode && (wallet.history || []).length > 0 && (
        <Section icon={Coins} title={t('userDetail.walletTitle')} count={(wallet.history || []).length} empty="">
          <PagedTable rows={wallet.history || []} pageSize={10} rowKey={(r, i) => i}
            columns={[
              { key: 'createdAt', header: t('userDetail.time'), className: 'mono', render: (r) => fmtDate(r.createdAt) },
              { key: 'type', header: t('userDetail.type'), render: (r) => <Pill variant={r.amount >= 0 ? 'ok' : 'pause'}>{r.type}</Pill> },
              { key: 'amount', header: t('userDetail.amount'), render: (r) => <span style={{ fontWeight: 700, color: r.amount >= 0 ? 'var(--free)' : 'var(--warn)' }}>{r.amount >= 0 ? '+' : ''}{c(r.amount)} C</span> },
              { key: 'balance', header: t('userDetail.balanceAfter'), render: (r) => cU(r.balance) },
              { key: 'desc', header: t('userDetail.memo'), render: (r) => r.desc || '—' },
            ]} />
        </Section>
      )}

      <Modal open={!!move} title={t('userDetail.moveTitle', { name: u.name || u.username })} onClose={() => setMove(null)} width={480}
        footer={<>
          <button className="btn" onClick={() => setMove(null)}>{t('userDetail.cancel')}</button>
          <button className="btn primary" onClick={doMove}>{t('userDetail.move')}</button>
        </>}>
        {move && (
          <>
            <label className="fld" id="admin-userdetail-fld-0-lbl" style={{ marginTop: 0 }}>{t('userDetail.moveTo')}</label>
            <Select ariaLabelledBy="admin-userdetail-fld-0-lbl" value={move.toGroupId} placeholder={t('userDetail.pickGroup')}
              onChange={(v) => setMove({ ...move, toGroupId: Number(v) })}
              options={allGroups.filter((g) => g.id !== move.fromGroupId).map((g) => ({ value: g.id, label: `${g.displayName} (${g.orgName})` }))} />
          </>
        )}
      </Modal>

      {/* 소속 추가 — 조직을 고르면 그 조직의 팀만 후보로 나온다(다중 조직/팀 참여) */}
      <Modal open={!!addMem} title={t('userDetail.addMembership', { defaultValue: '소속 추가' })} onClose={() => setAddMem(null)} width={480}
        footer={<>
          <button className="btn" onClick={() => setAddMem(null)}>{t('userDetail.cancel')}</button>
          <button className="btn primary" onClick={submitAdd}>{t('userDetail.addMembership', { defaultValue: '소속 추가' })}</button>
        </>}>
        {addMem && (
          <>
            <label className="fld" id="admin-userdetail-fld-1-lbl" style={{ marginTop: 0 }}>{t('userDetail.org', { defaultValue: '조직' })}</label>
            <Select ariaLabelledBy="admin-userdetail-fld-1-lbl" value={addMem.orgId} placeholder={t('userDetail.pickOrg', { defaultValue: '조직 선택' })}
              onChange={(v) => setAddMem({ ...addMem, orgId: Number(v), groupId: 0 })}
              options={orgs.map((o) => ({ value: o.id, label: o.displayName || o.name }))} />
            <label className="fld" id="admin-userdetail-fld-2-lbl">{t('userDetail.group', { defaultValue: '팀' })}</label>
            <Select ariaLabelledBy="admin-userdetail-fld-2-lbl" value={addMem.groupId} placeholder={t('userDetail.pickGroup', { defaultValue: '팀 선택' })}
              onChange={(v) => setAddMem({ ...addMem, groupId: Number(v) })}
              options={allGroups.filter((g) => g.orgId === addMem.orgId).map((g) => ({ value: g.id, label: g.displayName }))} />
            <label className="fld" id="admin-userdetail-fld-3-lbl">{t('userDetail.role')}</label>
            <Select ariaLabelledBy="admin-userdetail-fld-3-lbl" value={addMem.role} onChange={(v) => setAddMem({ ...addMem, role: v })}
              options={MROLES.map((x) => ({ value: x, label: t(`roles.${x}`, { defaultValue: x }) }))} />
            {allGroups.filter((g) => g.orgId === addMem.orgId).length === 0 && <div className="legend mt">{t('userDetail.noTeamsInOrg', { defaultValue: '이 조직에 팀이 없습니다.' })}</div>}
          </>
        )}
      </Modal>

      <Modal open={!!grant} title={t('userDetail.grantTitle', { name: u.name || u.username, defaultValue: `크레딧 부여 — ${u.name || u.username}` })} onClose={() => setGrant(null)} width={440}
        footer={<>
          <button className="btn" onClick={() => setGrant(null)}>{t('userDetail.cancel')}</button>
          <button className="btn primary" onClick={submitGrant}>{t('userDetail.grant', { defaultValue: '크레딧 부여' })}</button>
        </>}>
        {grant && (
          <>
            <label className="fld" htmlFor="admin-userdetail-fld-4" style={{ marginTop: 0 }}>{t('userDetail.grantAmount', { defaultValue: '부여 크레딧 (C, 음수 차감)' })}</label>
            <input id="admin-userdetail-fld-4" type="number" value={grant.amount} onChange={(e) => setGrant({ amount: e.target.value })} />
            <div className="legend mt">{t('userDetail.grantHint', { defaultValue: '개인 크레딧 잔액에 즉시 반영되며 감사 로그에 기록됩니다(일회성 조정).' })}</div>
          </>
        )}
      </Modal>
    </div>
  );
}
