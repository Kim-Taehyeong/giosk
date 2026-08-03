import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { UserPlus } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import Pill from '../../../components/console/Pill';
import Select from '../../../components/console/Select';
import DataTable from '../../../components/console/DataTable';
import Modal from '../../../components/console/Modal';
import BulkImport from '../../../components/console/BulkImport';
import Advanced, { Req } from '../../../components/console/Advanced';
import { useToast } from '../../../components/console/Toast';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { useAuth } from '../../../context/AuthContext';
import { activeLevelOf } from '../../../config/consoleRoles';
import { getUsers, grantUserCredit, updateUserStatus, createUser, bulkCreateUsers } from '../../../api/console/misc';
import { getOrgs, getGroups } from '../../../api/console/governance';
import { c, cU } from '../../../lib/credit';

const roleVariant = { member: 'pause', admin: 'err' };
// 멤버십(조직/팀) 역할 — 플랫폼 역할과 색을 구분해 둘을 헷갈리지 않게.
const memberRoleVariant = { org_admin: 'gpu', project_admin: 'primary', member: 'pause' };
const statusVariant = { approved: 'ok', pending: 'wait', suspended: 'err', rejected: 'err' };
const ROLES = ['member', 'admin'];
const PAGE_SIZE = 50;

export default function Users() {
  const { t } = useTranslation('consoleAdmin');
  const { config } = useSystemConfig();
  const { user, activeScope } = useAuth();
  const isPlatform = activeLevelOf(user, activeScope) === 'platform'; // 생성/정지/부여는 플랫폼 전용(백엔드도 /admin 게이트)
  const creditMode = config.billing.mode === 'credit';
  const [data, setData] = useState(null);
  const [q, setQ] = useState('');
  const [dq, setDq] = useState('');   // 디바운스된 검색어(실제 요청에 쓰는 값)
  const [group, setGroup] = useState('all');
  const [status, setStatus] = useState('all');
  const [page, setPage] = useState(1);
  const [pendingCount, setPendingCount] = useState(0);
  const [grant, setGrant] = useState(null);
  const [add, setAdd] = useState(null); // 단건 추가 폼
  const [orgs, setOrgs] = useState([]);
  const [allGroups, setAllGroups] = useState([]);
  const { toast } = useToast();
  const navigate = useNavigate();

  // 검색/필터/페이징은 전부 서버가 한다(수천 명이어도 한 페이지만 내려온다).
  const load = () => getUsers({ q: dq, status, group, page, size: PAGE_SIZE }).then(setData);
  useEffect(() => { load(); }, [dq, status, group, page]);
  useEffect(() => {
    getOrgs().then((d) => setOrgs(d.items));
    getGroups().then((d) => setAllGroups(d.items));
    // 대기 인원은 개수만 필요 → 1건만 받아 total 을 쓴다.
    getUsers({ status: 'pending', size: 1 }).then((r) => setPendingCount(r.total));
  }, []);
  // 입력마다 요청하지 않도록 300ms 디바운스.
  useEffect(() => {
    const id = setTimeout(() => { setDq(q); setPage(1); }, 300);
    return () => clearTimeout(id);
  }, [q]);

  const rows = data?.items || [];
  const total = data?.total || 0;
  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE));
  // 그룹 필터 후보는 그룹 목록에서(사용자 목록이 한 페이지라 거기서 못 뽑는다).
  // 표시명은 조직 간 중복될 수 있으므로 유니크화(중복 옵션·중복 React key 방지).
  const groups = [...new Set(allGroups.map((g) => g.displayName).filter(Boolean))];

  const submitGrant = async () => {
    // 백엔드 GrantReq({amount})를 기대 — 원시 숫자를 보내면 400. (그룹/조직 부여와 동일한 객체 형식)
    await grantUserCredit(grant.user.id, { amount: Number(grant.delta) });
    setGrant(null); toast(t('users.granted', { name: grant.user.name, n: grant.delta })); load();
  };
  const setUserStatus = async (u, st, msg) => { await updateUserStatus(u.id, st); toast(`${u.name} ${msg}`); load(); };

  const submitAdd = async () => {
    if (!add.name.trim() || !add.username.trim()) { toast(t('users.needFields')); return; }
    try {
      await createUser({ ...add, firstName: add.name, groupId: add.groupId || undefined });
    } catch (e) {
      toast(e?.code === 'duplicate_user' ? t('users.duplicate') : (e?.message || t('users.createFail')));
      return; // 실패 시 모달 유지(원인 표시)
    }
    setAdd(null); toast(t('users.userAdded', { name: add.name }));
    load(); // 백엔드 반영분을 재조회(실데이터)
  };

  // 엑셀(CSV) 양식 헤더/예시.
  const TPL_HEADER = ['이름', '계정(아이디)', '이메일', '역할(member/admin)', '조직', '그룹', '담당자'];
  const TPL_SAMPLE = [
    ['홍길동', 'hong', 'hong@giosk.io', 'member', 'AI센터', 'ml-lab', '김담당'],
    ['김철수', 'kim', 'kim@giosk.io', 'member', 'AI센터', 'vision-lab', '이담당'],
  ];

  // 업로드된 행 → 일괄 추가.
  const onBulkRows = async (data) => {
    const parsed = data
      .filter((r) => r[0] && r[1])
      .map((r, i) => ({ id: Date.now() + i, name: r[0], username: r[1], email: r[2] || '', role: ROLES.includes(r[3]) ? r[3] : 'member', org: r[4] || '', group: r[5] || '', advisor: r[6] || '', status: 'approved' }));
    if (parsed.length === 0) { toast(t('bulk.empty')); return; }
    await bulkCreateUsers(parsed);
    toast(t('users.bulkDone', { n: parsed.length }));
    load(); // 백엔드 반영분을 재조회(실데이터)
  };

  return (
    <div>
      <PageHead title={t('users.title')} subtitle={t('users.subtitle', { n: pendingCount })}
        actions={isPlatform && <span className="flex gap">
          <BulkImport filename="user-template.csv" header={TPL_HEADER} samples={TPL_SAMPLE} onRows={onBulkRows} />
          <button className="btn primary" onClick={() => setAdd({ name: '', username: '', email: '', password: '', role: 'member', orgId: 0, groupId: 0, advisor: '' })}><UserPlus size={15} /> {t('users.addUser')}</button>
        </span>} />

      <div className="card">
        <div className="flex gap wrap mb" style={{ justifyContent: 'flex-end' }}>
          <select value={status} onChange={(e) => { setStatus(e.target.value); setPage(1); }} style={{ width: 'auto' }}>
            <option value="all">{t('users.allStatus')}</option>
            <option value="pending">{t('users.pending')}</option>
            <option value="approved">{t('users.approved')}</option>
            <option value="suspended">{t('users.suspended')}</option>
          </select>
          <select value={group} onChange={(e) => { setGroup(e.target.value); setPage(1); }} style={{ width: 'auto' }}>
            <option value="all">{t('users.allGroups')}</option>
            {groups.map((g) => <option key={g} value={g}>{g}</option>)}
          </select>
          <input type="text" placeholder={t('users.searchPh')} value={q} onChange={(e) => setQ(e.target.value)} style={{ width: 180 }} />
        </div>
        <DataTable
          rows={rows}
          rowKey={(r) => r.id}
          onRowClick={(r) => navigate(`/console/admin/users/${r.id}`)}
          columns={[
            { key: 'name', header: t('users.colUser') },
            { key: 'group', header: t('users.colGroup'), render: (r) => (r.groupId
              ? <button className="btn sm" onClick={() => navigate(`/console/admin/groups/${r.groupId}`)}>{r.group}</button>
              : <span className="muted">—</span>) },
            { key: 'email', header: t('users.colEmail'), className: 'mono' },
            { key: 'role', header: t('users.colRole'), render: (r) => <Pill variant={roleVariant[r.role] || 'pause'}>{r.role}</Pill> },
            // 조직/팀 역할은 플랫폼 역할과 별개(멤버십 역할) — 팀장·조직관리자는 여기에만 나타난다.
            { key: 'memberRole', header: t('users.colMemberRole'), render: (r) => (r.memberRole
              ? <Pill variant={memberRoleVariant[r.memberRole] || 'pause'}>{t(`roles.${r.memberRole}`)}</Pill>
              : <span className="muted">—</span>) },
            ...(creditMode ? [
              { key: 'balance', header: t('users.colBalance', { defaultValue: '현재 잔여' }), render: (r) => <b>{cU(r.balance)}</b> },
              { key: 'refill', header: t('users.colRefill', { defaultValue: '정기 리필' }), render: (r) => (r.recurringCredit ? <span>{c(r.recurringCredit)} C <span className="muted" style={{ fontSize: 12 }}>/ {r.refillIntervalDays || t('users.refillDefault', { defaultValue: '기본' })}{r.refillIntervalDays ? '일' : ''}</span></span> : <span className="muted">—</span>) },
            ] : []),
            { key: 'status', header: t('users.colStatus'), render: (r) => <Pill variant={statusVariant[r.status] || 'pause'} dot>{r.status}</Pill> },
            { key: 'act', header: t('users.colAct'), className: 'flex', render: (r) => (
              <>
                {r.status === 'pending' && <span className="muted" style={{ fontSize: 12 }}>{t('users.approveInApprovals')}</span>}
                {isPlatform && r.status === 'approved' && <button className="btn sm warn" onClick={() => setUserStatus(r, 'suspended', t('users.suspended2'))}>{t('users.suspend')}</button>}
                {isPlatform && r.status === 'suspended' && <button className="btn sm ok" onClick={() => setUserStatus(r, 'approved', t('users.restored'))}>{t('users.restore')}</button>}
                {isPlatform && creditMode && <button className="btn sm" onClick={() => setGrant({ user: r, delta: 100 })}>{t('users.credit')}</button>}
              </>
            ) },
          ]}
        />
        {/* 서버 페이징 — 총 건수와 현재 페이지를 보여준다. */}
        <div className="flex" style={{ justifyContent: 'space-between', alignItems: 'center', marginTop: 12 }}>
          <span className="muted" style={{ fontSize: 12.5 }}>{t('users.totalN', { n: total })}</span>
          <span className="flex gap" style={{ alignItems: 'center' }}>
            <button className="btn sm" disabled={page <= 1} onClick={() => setPage((v) => Math.max(1, v - 1))}>{t('users.prev')}</button>
            <span style={{ fontSize: 12.5, fontWeight: 600 }}>{page} / {pageCount}</span>
            <button className="btn sm" disabled={page >= pageCount} onClick={() => setPage((v) => Math.min(pageCount, v + 1))}>{t('users.next')}</button>
          </span>
        </div>
      </div>

      {/* 단건 사용자 추가 */}
      <Modal open={!!add} title={t('users.addTitle')} onClose={() => setAdd(null)} width={680}
        footer={<>
          <button className="btn" onClick={() => setAdd(null)}>{t('users.cancel')}</button>
          <button className="btn primary" onClick={submitAdd}>{t('users.add')}</button>
        </>}>
        {add && (<>
          {/* 필수는 이름·계정뿐. 소속·역할·비번은 나중에도 바꿀 수 있으니 접어둔다. */}
          <div className="grid cols-2" style={{ gap: 14 }}>
            <div><label className="fld" style={{ marginTop: 0 }}>{t('users.fName')}<Req /></label>
              <input type="text" value={add.name} onChange={(e) => setAdd({ ...add, name: e.target.value })} placeholder="홍길동" /></div>
            <div><label className="fld" style={{ marginTop: 0 }}>{t('users.fUsername')}<Req /></label>
              <input type="text" value={add.username} onChange={(e) => setAdd({ ...add, username: e.target.value })} placeholder="hong" /></div>
          </div>
          <label className="fld">{t('users.fEmail')}</label>
          <input type="email" value={add.email} onChange={(e) => setAdd({ ...add, email: e.target.value })} placeholder="hong@giosk.io" />
          <div className="legend mt">{t('users.addHint')}</div>

          <Advanced title={t('users.advBelong')} defaultOpen={!!(add.orgId || add.groupId || add.advisor)}>
            <div className="grid cols-2" style={{ gap: 14 }}>
              <div><label className="fld" style={{ marginTop: 0 }}>{t('users.fOrg')}</label>
                <Select value={add.orgId} placeholder={t('users.orgPh')}
                  onChange={(v) => setAdd({ ...add, orgId: Number(v), groupId: 0 })}
                  options={orgs.map((o) => ({ value: o.id, label: o.displayName }))} /></div>
              <div><label className="fld" style={{ marginTop: 0 }}>{t('users.fGroup')}</label>
                <Select value={add.groupId} placeholder={add.orgId ? t('users.groupPh') : t('users.groupPickOrgFirst')}
                  disabled={!add.orgId}
                  onChange={(v) => setAdd({ ...add, groupId: Number(v) })}
                  options={allGroups.filter((g) => g.orgId === add.orgId).map((g) => ({ value: g.id, label: g.displayName }))} /></div>
              <div><label className="fld">{t('users.fRole')}</label>
                <Select value={add.role} onChange={(v) => setAdd({ ...add, role: v })} options={ROLES.map((x) => ({ value: x, label: x }))} /></div>
              <div><label className="fld">{t('users.fAdvisor')}</label>
                <input type="text" value={add.advisor} onChange={(e) => setAdd({ ...add, advisor: e.target.value })} placeholder="김담당" /></div>
            </div>
            <label className="fld">{t('users.fPassword')}</label>
            <input type="text" value={add.password} onChange={(e) => setAdd({ ...add, password: e.target.value })} placeholder={t('users.fPasswordPh')} />
            <div className="legend">{t('users.fPasswordHint')}</div>
          </Advanced>
        </>)}
      </Modal>

      <Modal open={!!grant} title={t('users.grantTitle', { name: grant?.user?.name || '' })} onClose={() => setGrant(null)} width={520}
        footer={<button className="btn primary" onClick={submitGrant}>{t('users.grant')}</button>}>
        {grant && (<>
          <label className="fld" style={{ marginTop: 0 }}>{t('users.grantLabel')}</label>
          <input type="number" value={grant.delta} onChange={(e) => setGrant({ ...grant, delta: e.target.value })} />
          <div className="legend mt">{t('users.grantHint')}</div>
        </>)}
      </Modal>

    </div>
  );
}
