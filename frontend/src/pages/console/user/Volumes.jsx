import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ChevronDown, ChevronRight, Trash2, HardDrive, Share2 } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import Bar from '../../../components/console/Bar';
import Pill from '../../../components/console/Pill';
import DataTable from '../../../components/console/DataTable';
import Modal from '../../../components/console/Modal';
import Select from '../../../components/console/Select';
import { useToast } from '../../../components/console/Toast';
import { useConfirm } from '../../../components/console/Confirm';
import { getVolumes, createVolume, shareVolume, deleteVolume, changeVolumeTeam } from '../../../api/console/volumes';
import { getShareTargets } from '../../../api/console/membership';

// 용량은 할당량이다. 사용량은 표시하지 않는다. 볼륨이 NFS 익스포트의 하위 디렉터리라
// kubelet 통계가 그 볼륨이 아니라 익스포트 전체 사용량을 돌려주기 때문(10GB 볼륨이 "50/10 GB"로 보였다).
const usageCell = (r) => (
  <div style={{ minWidth: 90, fontWeight: 600 }}>{r.capGb} GB</div>
);

export default function Volumes() {
  const { t } = useTranslation('consoleUser');
  const [data, setData] = useState({ owned: [], shared: [], quota: { allocatedGb: 0, totalGb: 0 } });
  const [openCreate, setOpenCreate] = useState(false);
  const [openShare, setOpenShare] = useState(null);
  const [detailId, setDetailId] = useState(null);
  const [form, setForm] = useState({ name: '', sizeGib: 30 });
  const [share, setShare] = useState({ target: 'user', value: '', permission: 'ro' });
  const [shareTargets, setShareTargets] = useState({ users: [], groups: [] });
  const [teamEdit, setTeamEdit] = useState(null); // 귀속 팀 변경 모달 { id, name, groupId }
  const { toast } = useToast();
  const confirm = useConfirm();

  const saveTeam = async () => {
    try { await changeVolumeTeam(teamEdit.id, Number(teamEdit.groupId) || 0); }
    catch (e) { toast(e?.code === 'quota_exceeded' ? t('volume.teamQuota', { defaultValue: '대상 팀 볼륨 쿼터를 초과합니다' }) : (e?.message || t('volume.teamFail', { defaultValue: '팀 변경 실패' }))); return; }
    toast(t('volume.teamChanged', { defaultValue: '귀속 팀을 변경했습니다' })); setTeamEdit(null); load();
  };

  const load = () => getVolumes().then(setData);
  useEffect(() => { load(); }, []);
  useEffect(() => { getShareTargets().then(setShareTargets); }, []);

  const quota = data.quota || { allocatedGb: 0, totalGb: 0 };
  const remainGb = Math.max(0, quota.totalGb - quota.allocatedGb);
  const overQuota = form.sizeGib > remainGb;
  // 생성 후 예상이다. 이 볼륨을 만들면 얼마가 쓰이고 얼마가 남는지 미리 보여준다.
  const wantGb = Math.max(0, form.sizeGib || 0);
  const afterAlloc = quota.allocatedGb + wantGb;
  const afterRemain = Math.max(0, remainGb - wantGb);
  const submitCreate = async () => {
    if (overQuota) { toast(t('volume.overQuota', { remain: remainGb })); return; }
    await createVolume(form); setOpenCreate(false); toast(t('volume.created')); load();
  };
  const remove = async (id) => { if (!(await confirm({ title: t('volume.delete'), message: t('confirmDelete'), confirmText: t('volume.delete') }))) return; await deleteVolume(id); toast(t('volume.deleted')); load(); };

  const submitShare = async () => {
    if (!share.value) { toast(t('volume.selectPh')); return; } // 대상을 안 골랐을 때 조용히 아무 일도 안 하는 걸 막는다
    // 백엔드 ShareReq 는 {target, value, permission} 이다(value=username|groupId). 예전엔
    // {username|groupId, permission} 로 보내 매핑이 안 돼 공유가 조용히 실패했다.
    try {
      await shareVolume(openShare.id, { target: share.target, value: share.value, permission: share.permission });
    } catch (e) {
      toast(e?.message || '공유에 실패했습니다'); return;
    }
    setData((d) => ({ ...d, owned: d.owned.map((v) => (v.id === openShare.id
      ? { ...v, sharedWith: [...(v.sharedWith || []), { type: share.target, name: share.value, perm: share.permission }] } : v)) }));
    setOpenShare(null); toast(t('volume.shareSet'));
  };
  const changeSharePerm = (volId, idx, perm) => setData((d) => ({
    ...d, owned: d.owned.map((v) => (v.id === volId ? { ...v, sharedWith: v.sharedWith.map((s, i) => (i === idx ? { ...s, perm } : s)) } : v)) }));
  const removeShare = (volId, idx) => {
    setData((d) => ({ ...d, owned: d.owned.map((v) => (v.id === volId ? { ...v, sharedWith: v.sharedWith.filter((_, i) => i !== idx) } : v)) }));
    toast(t('volume.shareRemoved'));
  };

  return (
    <div>
      <PageHead title={t('volume.title')} subtitle={t('volume.subtitle')}
        actions={<button className="btn primary" onClick={() => setOpenCreate(true)}>{t('volume.newVol')}</button>} />

      {/* 내 볼륨 제한량 — 사용자 단위(그룹 무관) 저장 한도 */}
      <div className="card mb">
        <h3><HardDrive size={16} /> {t('volume.quota')}</h3>
        <div className="flex" style={{ justifyContent: 'space-between', fontSize: 13.5, fontWeight: 700, marginBottom: 6 }}>
          <span>{quota.allocatedGb} / {quota.totalGb} GB</span>
          <span className="muted">{t('volume.remain', { n: remainGb })}</span>
        </div>
        <Bar value={quota.allocatedGb} max={quota.totalGb} variant={remainGb < quota.totalGb * 0.15 ? 'warn' : 'gpu'} />
        <div className="legend mt">{t('volume.quotaHint')}</div>
      </div>

      <div className="card mb">
        <h3><HardDrive size={16} /> {t('volume.mine')}</h3>
        <table>
          <thead>
            <tr><th>{t('volume.name')}</th><th>{t('volume.usage')}</th><th>{t('volume.shareCol')}</th><th>{t('volume.action')}</th></tr>
          </thead>
          <tbody>
            {data.owned.length === 0 && <tr><td colSpan={4} style={{ textAlign: 'center', color: 'var(--muted)' }}>{t('volume.noMine')}</td></tr>}
            {data.owned.map((r) => (
              <React.Fragment key={r.id}>
                <tr>
                  <td style={{ fontWeight: 600 }}>
                    {r.name}
                    <div className="muted" style={{ fontSize: 11.5, fontWeight: 500, marginTop: 2 }}>
                      {t('volume.teamHome', { defaultValue: '귀속 팀' })}: {r.teamName || t('volume.personal', { defaultValue: '개인' })}
                    </div>
                  </td>
                  <td>{usageCell(r)}</td>
                  <td>{r.sharedWith?.length ? <Pill variant="primary">{t('volume.sharedN', { n: r.sharedWith.length })}</Pill> : <span className="muted">{t('volume.private')}</span>}</td>
                  <td className="flex">
                    <button className="btn sm" onClick={() => setDetailId(detailId === r.id ? null : r.id)}>
                      {detailId === r.id ? <ChevronDown size={13} /> : <ChevronRight size={13} />} {t('volume.details')}
                    </button>
                    <button className="btn sm" onClick={() => { setShare({ target: 'user', value: '', permission: 'ro' }); setOpenShare(r); }}>{t('volume.share')}</button>
                    <button className="btn sm" onClick={() => setTeamEdit({ id: r.id, name: r.name, groupId: r.groupId || '' })}>{t('volume.changeTeam', { defaultValue: '팀 변경' })}</button>
                    <button className="btn sm danger" onClick={() => remove(r.id)}>{t('volume.delete')}</button>
                  </td>
                </tr>
                {detailId === r.id && (
                  <tr>
                    <td colSpan={4} style={{ background: 'var(--surface)' }}>
                      <div style={{ padding: '8px 0 12px 18px', marginLeft: 8, borderInlineStart: '1px solid var(--border)' }}>
                        <div className="flex" style={{ justifyContent: 'space-between', marginBottom: 8 }}>
                          <strong>{r.name} · {t('volume.shareStatus')}</strong>
                          <button className="btn sm primary" onClick={() => { setShare({ target: 'user', value: '', permission: 'ro' }); setOpenShare(r); }}>{t('volume.addShare')}</button>
                        </div>
                        {(!r.sharedWith || r.sharedWith.length === 0) && <div className="muted">{t('volume.noShares')}</div>}
                        {r.sharedWith?.map((s, i) => (
                          <div className="flex gap" key={i} style={{ padding: '9px 0', borderTop: i ? '1px solid var(--border)' : 'none' }}>
                            <Pill variant={s.type === 'group' ? 'primary' : 'pause'}>{s.type === 'group' ? t('volume.group') : t('volume.user')}</Pill>
                            <span style={{ fontWeight: 600, flex: 1 }}>{s.name}</span>
                            <Select size="sm" value={s.perm} onChange={(v) => changeSharePerm(r.id, i, v)}
                              options={[{ value: 'ro', label: t('volume.ro') }, { value: 'rw', label: t('volume.rw') }]} />
                            <button className="btn sm danger" onClick={() => removeShare(r.id, i)}><Trash2 size={13} /></button>
                          </div>
                        ))}
                      </div>
                    </td>
                  </tr>
                )}
              </React.Fragment>
            ))}
          </tbody>
        </table>
      </div>

      <div className="card">
        <h3><Share2 size={16} /> {t('volume.shared')}</h3>
        <DataTable
          rows={data.shared}
          rowKey={(r) => r.id}
          emptyText={t('volume.noShared')}
          columns={[
            { key: 'name', header: t('volume.name') },
            { key: 'sharedBy', header: t('volume.sharedBy', { defaultValue: '공유자' }), render: (r) => (r.sharedBy ? <span style={{ fontWeight: 600 }}>{r.sharedBy}</span> : <span className="muted">—</span>) },
            { key: 'perm', header: t('volume.perm'), render: (r) => <Pill variant={r.perm === 'rw' ? 'ok' : 'pause'}>{r.perm}</Pill> },
            { key: 'usage', header: t('volume.usage'), render: usageCell },
          ]}
        />
      </div>

      {(data.localHomes?.length > 0) && (
        <div className="card mt">
          <h3><HardDrive size={16} /> {t('volume.localHome')}</h3>
          <div className="legend mb">{t('volume.localHomeHint')}</div>
          <DataTable
            rows={data.localHomes}
            rowKey={(r) => r.node}
            columns={[
              { key: 'name', header: t('volume.name') },
              { key: 'node', header: t('volume.node'), render: (r) => <span className="mono">{r.node}</span> },
              { key: 'kind', header: t('volume.perm'), render: () => <Pill variant="warn">hostPath · {t('volume.nodePinned')}</Pill> },
            ]}
          />
        </div>
      )}

      <Modal open={openCreate} title={t('volume.createTitle')} onClose={() => setOpenCreate(false)} width={460}
        footer={<button className="btn primary" onClick={submitCreate} disabled={overQuota || form.sizeGib < 1}>{t('volume.create')}</button>}>
        {/* 사용자별 볼륨 용량 한도 — 남은 용량 내에서만 생성 가능 */}
        <div className="flex" style={{ justifyContent: 'space-between', fontSize: 13, fontWeight: 600, marginBottom: 6 }}>
          <span>{t('volume.quota')}</span>
          <span>{t('volume.remain', { n: remainGb })}</span>
        </div>
        <Bar value={quota.allocatedGb} max={quota.totalGb} variant={remainGb < quota.totalGb * 0.15 ? 'warn' : 'gpu'} />

        <label className="fld" htmlFor="user-volumes-fld-0">{t('volume.name')}</label>
        <input id="user-volumes-fld-0" type="text" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
        <label className="fld" htmlFor="user-volumes-fld-1">{t('volume.capacity')}</label>
        <input id="user-volumes-fld-1" type="number" min={1} max={remainGb} value={form.sizeGib} onChange={(e) => setForm({ ...form, sizeGib: Number(e.target.value) })} />

        {/* 생성 후 예상 — 이 볼륨이 남은 한도에서 얼마를 쓰고 얼마가 남는지. 한도 초과분은 붉게 표시. */}
        <div className="cost-box mt">
          <div className="row"><span>{t('volume.thisVol')}</span><span style={{ fontWeight: 700 }}>{wantGb} GB</span></div>
          <div className="row"><span>{t('volume.afterUse')}</span>
            <span style={{ color: overQuota ? 'var(--danger)' : 'inherit', fontWeight: overQuota ? 700 : 400 }}>{afterAlloc} / {quota.totalGb} GB</span></div>
          <div className="row"><span>{t('volume.afterRemain')}</span><span>{afterRemain} GB</span></div>
          <Bar value={Math.min(afterAlloc, quota.totalGb)} max={quota.totalGb} variant={overQuota ? 'err' : afterRemain < quota.totalGb * 0.15 ? 'warn' : 'gpu'} />
        </div>
        {overQuota
          ? <div className="legend" style={{ color: 'var(--danger)', fontWeight: 600 }}>{t('volume.overQuota', { remain: remainGb })}</div>
          : <div className="legend mt">{t('volume.nfsHint')}</div>}
      </Modal>

      <Modal open={!!openShare} title={t('volume.shareTitle', { name: openShare?.name || '' })} onClose={() => setOpenShare(null)} width={460}
        footer={<button className="btn primary" onClick={submitShare}>{t('volume.share')}</button>}>
        <label className="fld" id="user-volumes-fld-2-lbl" style={{ marginTop: 0 }}>{t('volume.shareTarget')}</label>
        <Select ariaLabelledBy="user-volumes-fld-2-lbl" value={share.target} onChange={(v) => setShare({ ...share, target: v, value: '' })}
          options={[{ value: 'user', label: t('volume.targetUser') }, { value: 'group', label: t('volume.targetGroup') }]} />
        <label className="fld" id="user-volumes-fld-3-lbl">{share.target === 'user' ? t('volume.selUser') : t('volume.selGroup')}</label>
        <Select ariaLabelledBy="user-volumes-fld-3-lbl" value={share.value} onChange={(v) => setShare({ ...share, value: v })} placeholder={t('volume.selectPh')} searchable
          options={share.target === 'user'
            ? shareTargets.users.map((u) => ({ value: u.username, label: `${u.name || u.username} (${u.username})` }))
            : shareTargets.groups.map((g) => ({ value: String(g.id), label: g.displayName || g.name }))} />
        <label className="fld" id="user-volumes-fld-4-lbl">{t('volume.perm')}</label>
        <Select ariaLabelledBy="user-volumes-fld-4-lbl" value={share.permission} onChange={(v) => setShare({ ...share, permission: v })}
          options={[{ value: 'ro', label: t('volume.ro') }, { value: 'rw', label: t('volume.rw') }]} />
        <div className="legend mt">{share.target === 'user' ? t('volume.hintUser') : t('volume.hintGroup')}</div>
      </Modal>

      <Modal open={!!teamEdit} title={t('volume.changeTeamTitle', { name: teamEdit?.name, defaultValue: `귀속 팀 변경 — ${teamEdit?.name || ''}` })} onClose={() => setTeamEdit(null)} width={460}
        footer={<>
          <button className="btn" onClick={() => setTeamEdit(null)}>{t('common.cancel')}</button>
          <button className="btn primary" onClick={saveTeam}>{t('common.save')}</button>
        </>}>
        {teamEdit && (<>
          <label className="fld" id="user-volumes-fld-5-lbl" style={{ marginTop: 0 }}>{t('volume.teamHome', { defaultValue: '귀속 팀' })}</label>
          <Select ariaLabelledBy="user-volumes-fld-5-lbl" value={String(teamEdit.groupId || '')} onChange={(v) => setTeamEdit({ ...teamEdit, groupId: v })}
            options={[{ value: '', label: t('volume.personal', { defaultValue: '개인(팀 없음)' }) }, ...(shareTargets.groups || []).map((g) => ({ value: String(g.id), label: g.displayName || g.name }))]} />
          <div className="legend mt">{t('volume.teamHint', { defaultValue: '이 볼륨의 용량 쿼터와 스토리지 크레딧이 선택한 팀에서 나갑니다. 소유는 그대로 본인입니다.' })}</div>
        </>)}
      </Modal>
    </div>
  );
}
