import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ShieldCheck, Pencil, Trash2, Plus } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import Pill from '../../../components/console/Pill';
import DataTable from '../../../components/console/DataTable';
import Modal from '../../../components/console/Modal';
import Select from '../../../components/console/Select';
import LimitsFields from '../../../components/console/LimitsFields';
import UserPicker from '../../../components/console/UserPicker';
import { useToast } from '../../../components/console/Toast';
import { useConfirm } from '../../../components/console/Confirm';
import { getPolicies, setUserLimits, setGroupLimits, setOrgLimits, setGlobalLimits,
  setUserLimitsScoped, setGroupLimitsScoped, setOrgLimitsScoped, getParentLimits } from '../../../api/console/limits';
import { getOrgs, getGroups } from '../../../api/console/governance';
import { useAuth } from '../../../context/AuthContext';


// 범위별 저장 함수 — platform 은 /admin(무제한), 매니저는 /console(스코프+상위캡 강제).
const SETTER_ADMIN = { user: setUserLimits, group: setGroupLimits, org: setOrgLimits };
const SETTER_SCOPED = { user: setUserLimitsScoped, group: setGroupLimitsScoped, org: setOrgLimitsScoped };
const SCOPE_VARIANT = { org: 'primary', group: 'gpu', user: 'pause' };

// 값 표기 — 미설정(null)은 "상속"이라 대시로. 전역 행은 항상 구체값.
const val = (v, unit = '') => (v === null || v === undefined ? '—' : `${v}${unit}`);

export default function Policies() {
  const { t } = useTranslation('consoleAdmin');
  const [rows, setRows] = useState([]);
  const [global, setGlobal] = useState({});
  const [edit, setEdit] = useState(null); // { scope, id, name, limits } | 'new'
  const [targets, setTargets] = useState({ org: [], group: [] });
  const { toast } = useToast();
  const confirm = useConfirm();
  const { user } = useAuth();
  const isPlatform = user?.role === 'admin'; // platform=전역 편집·무제한 / 매니저=자기 스코프 편집(상위캡 강제)
  const SETTER = isPlatform ? SETTER_ADMIN : SETTER_SCOPED;
  const [caps, setCaps] = useState(null); // 매니저 편집 시 설정 가능한 최대치(상위 유효 상한)

  // 대상이 정해지면(매니저) 상위 상한을 불러와 "최대 N" 힌트로 보여준다.
  useEffect(() => {
    setCaps(null);
    if (isPlatform || !edit || edit.scope === 'global' || !edit.scope || !edit.id) return;
    getParentLimits(edit.scope, edit.id).then(setCaps).catch(() => setCaps(null));
  }, [isPlatform, edit?.scope, edit?.id]);

  const load = () => getPolicies().then((d) => { setRows(d.items); setGlobal(d.global); });
  useEffect(() => {
    load();
    // 정책 대상 후보 — /console 은 스코프로 필터되므로 매니저는 자기 범위만 받는다.
    Promise.all([getOrgs(), getGroups()]).then(([o, g]) => setTargets({
      org: (o.items || []).map((x) => ({ value: x.id, label: x.displayName })),
      group: (g.items || []).map((x) => ({ value: x.id, label: x.displayName })),
    })).catch(() => {});
  }, []);

  // 백엔드 스코프 거부(상위캡 초과·범위 밖)를 사용자 메시지로.
  const errMsg = (e) => {
    if (e?.code === 'exceeds_parent') return e?.error || t('policies.exceedsParent');
    if (e?.code === 'out_of_scope') return t('policies.outOfScope');
    return e?.message || t('policies.saveFail');
  };

  const save = async () => {
    // 전역은 대상이 없다(모든 사용자) — 폴백이라 빈 값을 허용하지 않는다. platform 전용.
    if (edit.scope === 'global') {
      const l = edit.limits || {};
      if ([l.maxGpu, l.maxVramGb, l.maxVolumeGib, l.maxConcurrentSessions].some((v) => !v || v < 1)) {
        toast(t('policies.globalNeedsValue')); return;
      }
      try { await setGlobalLimits(l); } catch (e) { toast(errMsg(e)); return; }
      setEdit(null); toast(t('policies.saved')); load(); return;
    }
    if (!edit.scope || !edit.id) { toast(t('policies.needTarget')); return; }
    try { await SETTER[edit.scope](edit.id, edit.limits || {}); } catch (e) { toast(errMsg(e)); return; }
    setEdit(null); toast(t('policies.saved')); load();
  };
  // 삭제 = 그 범위의 상한을 전부 null 로(= 상위로 상속). 별도 delete API 없이 의미가 정확하다.
  const remove = async (r) => {
    if (!(await confirm({ title: t('policies.removeTitle'), message: t('policies.removeConfirm', { name: r.name }), confirmText: t('common.delete') }))) return;
    try { await SETTER[r.scope](r.id, { maxGpu: null, maxVramGb: null, maxVolumeGib: null, maxConcurrentSessions: null }); } catch (e) { toast(errMsg(e)); return; }
    toast(t('policies.removed', { name: r.name })); load();
  };

  // 전역 행(설치 고정)을 목록 맨 위에 합쳐 "무엇이 폴백인지"를 같은 표에서 보게 한다.
  const all = [
    { scope: 'global', id: 0, name: t('policies.everyone'), ...global, _fixed: true },
    ...rows,
  ];

  return (
    <div>
      <PageHead title={t('policies.title')} subtitle={t('policies.subtitle')}
        actions={<button className="btn primary" onClick={() => setEdit({ scope: isPlatform ? 'user' : 'group', id: 0, account: '', limits: {} })}>
          <Plus size={15} /> {t('policies.add')}</button>} />
      <div className="card">
        <div className="legend mb flex" style={{ alignItems: 'flex-start', gap: 6 }}>
          <ShieldCheck size={14} style={{ flex: '0 0 auto', marginTop: 1 }} />
          <span>{isPlatform ? t('policies.rule') : t('policies.ruleScoped')}</span>
        </div>
        <DataTable
          rows={all}
          rowKey={(r) => `${r.scope}-${r.id}`}
          columns={[
            { key: 'scope', header: t('policies.scope'), render: (r) => (
              <Pill variant={r._fixed ? 'free' : SCOPE_VARIANT[r.scope]}>{t(`policies.scope_${r.scope}`)}</Pill>) },
            { key: 'name', header: t('policies.target'), render: (r) => <span style={{ fontWeight: 600 }}>{r.name}</span> },
            { key: 'maxGpu', header: t('limits.maxGpu'), render: (r) => val(r.maxGpu) },
            { key: 'maxVramGb', header: t('limits.maxVramGb'), render: (r) => val(r.maxVramGb, ' GB') },
            { key: 'maxVolumeGib', header: t('limits.maxVolumeGib'), render: (r) => val(r.maxVolumeGib, ' GiB') },
            { key: 'maxConcurrentSessions', header: t('limits.maxConcurrentSessions'), render: (r) => val(r.maxConcurrentSessions) },
            { key: 'act', header: t('policies.act'), className: 'flex', render: (r) => (r._fixed
              ? (isPlatform && <button className="btn sm" onClick={() => setEdit({ scope: 'global', id: 0, name: r.name, limits: {
                  maxGpu: r.maxGpu, maxVramGb: r.maxVramGb, maxVolumeGib: r.maxVolumeGib, maxConcurrentSessions: r.maxConcurrentSessions } })}>
                  <Pencil size={13} /> {t('common.edit')}</button>)
              : <span className="flex gap">
                  <button className="btn sm" onClick={() => setEdit({ scope: r.scope, id: r.id, name: r.name, limits: {
                    maxGpu: r.maxGpu, maxVramGb: r.maxVramGb, maxVolumeGib: r.maxVolumeGib, maxConcurrentSessions: r.maxConcurrentSessions } })}>
                    <Pencil size={13} /> {t('common.edit')}</button>
                  <button className="btn sm danger" onClick={() => remove(r)}><Trash2 size={13} /> {t('common.delete')}</button>
                </span>) },
          ]}
        />
      </div>

      <Modal open={!!edit} title={edit?.name ? t('policies.editTitle', { name: edit.name }) : t('policies.addTitle')}
        onClose={() => setEdit(null)} width={620}
        footer={<>
          <button className="btn" onClick={() => setEdit(null)}>{t('common.cancel')}</button>
          <button className="btn primary" onClick={save}>{t('common.save')}</button>
        </>}>
        {edit && (<>
          {/* 새로 추가할 때만 범위·대상을 고른다(기존 행은 범위가 정해져 있음). */}
          {!edit.name && (
            <>
              <div style={{ marginBottom: 12 }}><label className="fld" style={{ marginTop: 0 }}>{t('policies.scope')}</label>
                <Select value={edit.scope} onChange={(v) => setEdit({ ...edit, scope: v, id: 0, account: '' })}
                  width="100%"
                  options={[
                    { value: 'org', label: t('policies.scope_org') },
                    { value: 'group', label: t('policies.scope_group') },
                    { value: 'user', label: t('policies.scope_user') },
                  ]} /></div>
              <div><label className="fld" style={{ marginTop: 0 }}>{t('policies.target')}</label>
                {/* 사용자는 수백 명이 될 수 있어 드롭다운 대신 검색(UserPicker). 조직/그룹은 수가 적어 드롭다운 유지. */}
                {edit.scope === 'user'
                  ? <UserPicker value={edit.account || ''} placeholder={t('policies.searchUser')}
                      onChange={(v) => setEdit((e) => ({ ...e, account: v }))}
                      onPick={(u) => setEdit((e) => ({ ...e, id: u ? u.id : 0, account: u ? u.username : e.account }))} />
                  : <Select value={edit.id} placeholder={t('policies.pickTarget')} width="100%"
                      onChange={(v) => setEdit({ ...edit, id: Number(v) })} options={targets[edit.scope] || []} />}</div>
            </>
          )}
          <div className="legend mt mb">{edit.scope === 'global' ? t('policies.globalHint') : t('limits.hint')}</div>
          {caps && (
            <div className="legend mb" style={{ color: 'var(--primary)', fontWeight: 600 }}>
              {t('policies.capHint', {
                gpu: caps.maxGpu, vram: caps.maxVramGb, vol: caps.maxVolumeGib, sess: caps.maxConcurrentSessions,
                defaultValue: `설정 가능한 최대 — GPU ≤ ${caps.maxGpu} · VRAM ≤ ${caps.maxVramGb}GB · 볼륨 ≤ ${caps.maxVolumeGib}GiB · 동시세션 ≤ ${caps.maxConcurrentSessions}`,
              })}
            </div>
          )}
          <LimitsFields value={edit.limits} onChange={(v) => setEdit({ ...edit, limits: v })} />
        </>)}
      </Modal>
    </div>
  );
}
