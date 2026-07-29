import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Megaphone, Plus, Pencil, Trash2, Info, AlertTriangle } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import Pill from '../../../components/console/Pill';
import Toggle from '../../../components/console/Toggle';
import Select from '../../../components/console/Select';
import Modal from '../../../components/console/Modal';
import { useToast } from '../../../components/console/Toast';
import { useConfirm } from '../../../components/console/Confirm';
import { useAuth } from '../../../context/AuthContext';
import { activeLevelOf } from '../../../config/consoleRoles';
import { getOrgs, getGroups } from '../../../api/console/governance';
import {
  getAllAnnouncements, createAnnouncement, updateAnnouncement, deleteAnnouncement, toggleAnnouncement,
} from '../../../api/console/announcements';

const LEVEL_PILL = { info: 'primary', warning: 'cordon', critical: 'err' };
const LEVEL_ICON = { info: Info, warning: AlertTriangle, critical: Megaphone };
const blank = { level: 'info', title: '', body: '', active: true, pinned: false, targetOrgId: null, targetGroupId: null };

// 타겟 표기 모드 — 저장은 targetOrgId/targetGroupId 로만. 둘 다 없으면 전역.
const modeOf = (r) => (r.targetGroupId ? 'group' : r.targetOrgId ? 'org' : 'global');

export default function Announcements() {
  const { t } = useTranslation('consoleAdmin');
  const { toast } = useToast();
  const confirm = useConfirm();
  const { user, activeScope } = useAuth();
  const lvl = activeLevelOf(user, activeScope); // platform | org | group
  const [rows, setRows] = useState([]);
  const [edit, setEdit] = useState(null); // { id?, ...fields }
  const [orgs, setOrgs] = useState([]);
  const [groups, setGroups] = useState([]);

  useEffect(() => { getAllAnnouncements().then((r) => setRows(r.items || [])); }, []);
  useEffect(() => {
    // 타겟 선택 후보 — group 관리자는 자기 그룹 고정이라 목록 불필요.
    if (lvl === 'platform') getOrgs().then((r) => setOrgs(r.items || []));
    if (lvl === 'platform' || lvl === 'org') getGroups().then((r) => setGroups(r.items || []));
  }, [lvl]);

  const levelOpts = [
    { value: 'info', label: t('announce.lvInfo') },
    { value: 'warning', label: t('announce.lvWarning') },
    { value: 'critical', label: t('announce.lvCritical') },
  ];

  // 레벨별 타겟 모드 옵션 — platform=전역/조직/그룹, org=조직 전체/특정 그룹, group=자기 그룹 고정.
  const defaultMode = lvl === 'platform' ? 'global' : lvl === 'org' ? 'org' : 'group';
  const modeOpts = lvl === 'platform'
    ? [{ value: 'global', label: t('announce.tgGlobal') }, { value: 'org', label: t('announce.tgOrg') }, { value: 'group', label: t('announce.tgGroup') }]
    : lvl === 'org'
      ? [{ value: 'org', label: t('announce.tgOrgAll') }, { value: 'group', label: t('announce.tgGroup') }]
      : [];

  const openNew = () => setEdit({ ...blank, _mode: defaultMode });
  const openEdit = (r) => setEdit({ ...r, _mode: modeOf(r) });

  // 목록 표기 — 저장된 targetOrgId/targetGroupId 로부터 사람이 읽을 라벨.
  const targetLabel = (r) => {
    const m = modeOf(r);
    if (m === 'global') return t('announce.tgGlobal');
    if (m === 'group') { const g = groups.find((x) => x.id === r.targetGroupId); return `${t('announce.tgGroup')}${g ? `: ${g.displayName}` : ''}`; }
    const o = orgs.find((x) => x.id === r.targetOrgId); return `${t('announce.tgOrg')}${o ? `: ${o.displayName}` : ''}`;
  };

  const save = async () => {
    if (!edit.title.trim()) { toast(t('announce.needTitle')); return; }
    const m = edit._mode || defaultMode;
    if (lvl === 'platform' && m === 'org' && !edit.targetOrgId) { toast(t('announce.needTarget')); return; }
    if (m === 'group' && !edit.targetGroupId) { toast(t('announce.needTarget')); return; }
    const payload = {
      level: edit.level, title: edit.title, body: edit.body, active: edit.active, pinned: edit.pinned,
      targetOrgId: m === 'org' ? (edit.targetOrgId || null) : null,
      targetGroupId: m === 'group' ? (edit.targetGroupId || null) : null,
    };
    if (edit.id) {
      await updateAnnouncement(edit.id, payload);
      toast(t('announce.updated'));
    } else {
      await createAnnouncement(payload);
      toast(t('announce.created2'));
    }
    setEdit(null);
    getAllAnnouncements().then((r) => setRows(r.items || [])); // 타겟은 백엔드가 강제하므로 서버 값으로 재조회.
  };
  const toggle = async (r) => { await toggleAnnouncement(r.id); setRows((p) => p.map((x) => (x.id === r.id ? { ...x, active: !x.active } : x))); };
  const remove = async (r) => { if (!(await confirm({ title: t('announce.delete') || '삭제', message: t('confirmDelete') }))) return; await deleteAnnouncement(r.id); setRows((p) => p.filter((x) => x.id !== r.id)); toast(t('announce.removed')); };

  return (
    <div>
      <PageHead icon={Megaphone} title={t('announce.title')} subtitle={t('announce.subtitle')}
        actions={<button className="btn primary" onClick={openNew}><Plus size={15} /> {t('announce.add')}</button>} />

      <div className="card">
        <div className="legend mb">{t('announce.hint')}</div>
        <table>
          <thead>
            <tr>
              <th>{t('announce.level')}</th><th>{t('announce.content')}</th><th>{t('announce.target')}</th><th>{t('announce.created')}</th>
              <th>{t('announce.pinned')}</th><th>{t('announce.active')}</th><th>{t('announce.action')}</th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 && <tr><td colSpan={7} style={{ textAlign: 'center', color: 'var(--muted)' }}>{t('announce.none')}</td></tr>}
            {rows.map((r) => {
              const Icon = LEVEL_ICON[r.level] || Info;
              return (
                <tr key={r.id} style={{ opacity: r.active ? 1 : 0.5 }}>
                  <td><Pill variant={LEVEL_PILL[r.level] || 'primary'}><Icon size={12} /> {t(`announce.lv${r.level[0].toUpperCase()}${r.level.slice(1)}`)}</Pill></td>
                  <td style={{ maxWidth: 460 }}>
                    <div style={{ fontWeight: 700 }}>{r.title}</div>
                    {r.body && <div className="muted" style={{ fontSize: 12.5, marginTop: 2, whiteSpace: 'normal' }}>{r.body}</div>}
                  </td>
                  <td>{modeOf(r) === 'global'
                    ? <span className="muted">{t('announce.tgGlobal')}</span>
                    : <Pill variant={modeOf(r) === 'group' ? 'gpu' : 'primary'}>{targetLabel(r)}</Pill>}</td>
                  <td className="muted">{r.createdAt}</td>
                  <td>{r.pinned ? <Pill variant="primary">{t('announce.pinnedYes')}</Pill> : <span className="muted">—</span>}</td>
                  <td><Toggle checked={r.active} onChange={() => toggle(r)} /></td>
                  <td className="flex">
                    <button className="btn sm" onClick={() => openEdit(r)}><Pencil size={13} /> {t('announce.edit')}</button>
                    <button className="btn sm danger" onClick={() => remove(r)}><Trash2 size={13} /></button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <Modal open={!!edit} title={edit?.id ? t('announce.editTitle') : t('announce.addTitle')} onClose={() => setEdit(null)} width={520}
        footer={(
          <>
            <button className="btn" onClick={() => setEdit(null)}>{t('announce.cancel')}</button>
            <button className="btn primary" onClick={save}>{t('announce.save')}</button>
          </>
        )}>
        {edit && (
          <>
            <label className="fld" id="admin-announcements-fld-0-lbl" style={{ marginTop: 0 }}>{t('announce.level')}</label>
            <Select ariaLabelledBy="admin-announcements-fld-0-lbl" value={edit.level} onChange={(v) => setEdit({ ...edit, level: v })} options={levelOpts} />
            <label className="fld" htmlFor="admin-announcements-fld-1">{t('announce.fTitle')}</label>
            <input id="admin-announcements-fld-1" type="text" value={edit.title} onChange={(e) => setEdit({ ...edit, title: e.target.value })} placeholder={t('announce.titlePh')} />
            <label className="fld" htmlFor="admin-announcements-fld-2">{t('announce.fBody')}</label>
            <textarea id="admin-announcements-fld-2" value={edit.body} onChange={(e) => setEdit({ ...edit, body: e.target.value })} placeholder={t('announce.bodyPh')} style={{ minHeight: 90 }} />

            {/* 타겟 — group 관리자는 자기 그룹 고정(선택 UI 없음). */}
            {lvl === 'group' ? (
              <>
                <label className="fld" id="admin-announcements-fld-3-lbl">{t('announce.target')}</label>
                <div className="legend">{t('announce.tgGroupFixed')}</div>
              </>
            ) : (
              <>
                <label className="fld">{t('announce.target')}</label>
                <Select ariaLabelledBy="admin-announcements-fld-3-lbl" value={edit._mode || defaultMode}
                  onChange={(v) => setEdit({ ...edit, _mode: v, targetOrgId: null, targetGroupId: null })}
                  options={modeOpts} />
                {(edit._mode || defaultMode) === 'org' && lvl === 'platform' && (
                  <Select value={edit.targetOrgId || ''} placeholder={t('announce.pickOrg')}
                    onChange={(v) => setEdit({ ...edit, targetOrgId: Number(v) })}
                    options={orgs.map((o) => ({ value: o.id, label: o.displayName }))} />
                )}
                {(edit._mode || defaultMode) === 'group' && (
                  <Select value={edit.targetGroupId || ''} placeholder={t('announce.pickGroup')}
                    onChange={(v) => setEdit({ ...edit, targetGroupId: Number(v) })}
                    options={groups.map((g) => ({ value: g.id, label: g.displayName }))} />
                )}
              </>
            )}
            <div className="flex" style={{ gap: 24, marginTop: 14, flexWrap: 'wrap' }}>
              <label className="flex gap" style={{ alignItems: 'center' }}><Toggle checked={edit.active} onChange={(v) => setEdit({ ...edit, active: v })} /> {t('announce.activeNow')}</label>
              <label className="flex gap" style={{ alignItems: 'center' }}><Toggle checked={edit.pinned} onChange={(v) => setEdit({ ...edit, pinned: v })} /> {t('announce.pinTop')}</label>
            </div>
            <div className="legend mt">{t('announce.modalHint')}</div>
          </>
        )}
      </Modal>
    </div>
  );
}
