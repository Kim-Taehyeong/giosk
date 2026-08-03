import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { HardDrive, Database, Server, User, Download, Upload } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import StatCard from '../../../components/console/StatCard';
import Pill from '../../../components/console/Pill';
import Bar from '../../../components/console/Bar';
import Modal from '../../../components/console/Modal';
import { Req } from '../../../components/console/Advanced';
import PagedTable from '../../../components/console/PagedTable';
import UserPicker from '../../../components/console/UserPicker';
import { useToast } from '../../../components/console/Toast';
import { useAuth } from '../../../context/AuthContext';
import { activeLevelOf } from '../../../config/consoleRoles';
import { getAdminVolumes, getScopedVolumes, getAdminStorage, importNfsVolume } from '../../../api/console/nodes';

const statusVariant = { bound: 'ok', pending: 'wait', failed: 'err' };

export default function VolumesAdmin() {
  const { t } = useTranslation('consoleAdmin');
  const { user, activeScope } = useAuth();
  const isPlatform = activeLevelOf(user, activeScope) === 'platform';
  const navigate = useNavigate();
  const { toast } = useToast();

  const [vols, setVols] = useState([]);
  const [storage, setStorage] = useState(null);
  const [imp, setImp] = useState(null);
  const [impBusy, setImpBusy] = useState(false);

  const load = () => (isPlatform ? getAdminVolumes() : getScopedVolumes()).then((d) => { setVols(d.items); if (d.storage) setStorage(d.storage); });
  useEffect(() => { load(); if (isPlatform) getAdminStorage().then(setStorage).catch(() => {}); /* eslint-disable-next-line */ }, []);

  const totalCap = vols.reduce((a, v) => a + (v.capGb || 0), 0);
  const nfsPct = storage?.nfsTotalGb ? Math.round((storage.nfsUsedGb / storage.nfsTotalGb) * 100) : 0;

  // 소유자는 목록에서 고른 계정(ownerUser)만 인정한다. 예전엔 입력한 문자열로 검색해 첫 결과에
  // 배정했는데, 동명이인·부분일치로 엉뚱한 사람 소유가 될 수 있었다.
  const submitImport = async () => {
    if (!imp?.name || !imp?.nfsServer || !imp?.nfsPath) { toast(t('nodes.impMissing')); return; }
    if (!imp?.ownerUser) { toast(t('nodes.impNoUser')); return; }
    setImpBusy(true);
    try {
      await importNfsVolume({ name: imp.name, ownerUserId: imp.ownerUser.id, nfsServer: imp.nfsServer, nfsPath: imp.nfsPath, sizeGiB: Number(imp.sizeGiB) || 1 });
      toast(t('nodes.impDone', { name: imp.name, user: imp.ownerUser.username }));
      setImp(null); load();
    } catch { toast(t('nodes.impFail')); }
    setImpBusy(false);
  };

  return (
    <div>
      <PageHead icon={HardDrive} title={t('volumes.title')} subtitle={t('volumes.subtitle')}
        actions={isPlatform && <button className="btn primary" onClick={() => setImp({ name: '', owner: '', ownerUser: null, nfsServer: storage?.nfsServer || '', nfsPath: '', sizeGiB: 10 })}><Upload size={14} /> {t('nodes.importNfs')}</button>} />

      {/* KPI */}
      <div className="grid cols-4 mb">
        <StatCard icon={Database} tone="gpu" label={t('volumes.count')} value={`${vols.length}`} />
        <StatCard icon={HardDrive} tone="warn" label={t('volumes.totalAlloc')} value={`${totalCap}`} unit="GB" />
        {isPlatform && storage && <StatCard icon={Server} tone="free" label={t('volumes.nfsUsed')} value={`${storage.nfsUsedGb ?? 0}`} unit={`/ ${storage.nfsTotalGb ?? 0} GB`} sub={storage.nfsTotalGb ? `${nfsPct}%` : t('nodes.nfsNoMetrics')} bar={storage.nfsTotalGb ? { value: storage.nfsUsedGb, max: storage.nfsTotalGb, variant: nfsPct > 85 ? 'warn' : 'gpu' } : undefined} />}
        {isPlatform && storage && <StatCard icon={User} tone="primary" label={t('volumes.users')} value={`${(storage.users || []).length}`} sub={t('volumes.withVol')} />}
      </div>

      {/* 볼륨 목록 */}
      <div className="card mb">
        <h3><Database size={16} /> {t('volumes.listTitle')} <span className="muted" style={{ fontSize: 12.5, fontWeight: 600 }}>({vols.length})</span></h3>
        <PagedTable rows={vols} pageSize={15} rowKey={(r) => r.id}
          onRowClick={(r) => r.ownerUserId && navigate(`/console/admin/users/${r.ownerUserId}`)}
          columns={[
            { key: 'name', header: t('volumes.name'), render: (r) => <span style={{ fontWeight: 600 }}>{r.name}</span> },
            { key: 'owner', header: t('volumes.owner'), render: (r) => r.owner || (r.kind === 'group' ? t('volumes.groupVol') : '—') },
            { key: 'org', header: t('volumes.org'), render: (r) => r.org || '—' },
            { key: 'group', header: t('volumes.group'), render: (r) => r.group || '—' },
            { key: 'cap', header: t('volumes.capacity'), render: (r) => `${r.usedGb ?? 0} / ${r.capGb ?? 0} GB` },
            { key: 'accessMode', header: t('volumes.access'), className: 'mono' },
            { key: 'nfs', header: 'NFS', className: 'mono', render: (r) => r.nfsServer ? `${r.nfsServer}:${r.nfsPath}` : '—' },
            { key: 'status', header: t('volumes.status'), render: (r) => <Pill variant={statusVariant[r.status] || 'pause'} dot>{r.status}</Pill> },
          ]} />
      </div>

      {/* 사용자별 할당(플랫폼) */}
      {isPlatform && storage && (storage.users || []).length > 0 && (
        <div className="card">
          <h3><User size={16} /> {t('volumes.byUser')}</h3>
          <PagedTable rows={[...(storage.users || [])].sort((a, b) => b.allocatedGb - a.allocatedGb)} pageSize={10} rowKey={(u) => u.userId}
            onRowClick={(u) => navigate(`/console/admin/users/${u.userId}`)}
            columns={[
              { key: 'name', header: t('volumes.user'), render: (u) => <span className="flex" style={{ gap: 6, alignItems: 'center' }}><User size={13} /> {u.name}</span> },
              { key: 'allocatedGb', header: t('volumes.allocated'), render: (u) => `${u.allocatedGb} GB` },
            ]} />
        </div>
      )}

      {/* NFS 임포트(플랫폼) */}
      <Modal open={!!imp} title={t('nodes.importTitle')} onClose={() => setImp(null)} width={560}
        footer={<><button className="btn" onClick={() => setImp(null)}>{t('nodes.impCancel')}</button>
          <button className="btn primary" disabled={impBusy} onClick={submitImport}>{impBusy ? '…' : t('nodes.importNfs')}</button></>}>
        {imp && (
          <div className="grid" style={{ gap: 12 }}>
            <div className="legend">{t('nodes.importHint')}</div>
            <div><label className="fld" htmlFor="admin-volumesadmin-fld-0" style={{ marginTop: 0 }}>{t('nodes.impName')}<Req /></label>
              <input id="admin-volumesadmin-fld-0" type="text" value={imp.name} onChange={(e) => setImp({ ...imp, name: e.target.value })} placeholder="team-shared" /></div>
            {/* 소유자 — 조직·그룹으로 좁히고 목록에서 계정을 직접 고른다(동명이인 오배정 방지). */}
            <div><label className="fld" style={{ marginTop: 0 }}>{t('nodes.impOwner')}<Req /></label>
              <UserPicker value={imp.owner}
                onChange={(v) => setImp((cur) => ({ ...cur, owner: v }))}
                onPick={(u) => setImp((cur) => ({ ...cur, ownerUser: u, owner: u ? u.username : cur.owner }))} />
              <div className="legend" style={{ marginTop: 4 }}>
                {imp.ownerUser
                  ? t('nodes.impOwnerPicked', { name: imp.ownerUser.name || imp.ownerUser.username, id: imp.ownerUser.username, defaultValue: '선택됨: {{name}} ({{id}})' })
                  : t('nodes.impOwnerHint', { defaultValue: '목록에서 계정을 선택해야 임포트할 수 있습니다.' })}
              </div>
            </div>
            <div className="grid cols-2" style={{ gap: 12 }}>
              <div><label className="fld" htmlFor="admin-volumesadmin-fld-1" style={{ marginTop: 0 }}>{t('nodes.impServer')}<Req /></label>
                <input id="admin-volumesadmin-fld-1" type="text" value={imp.nfsServer} onChange={(e) => setImp({ ...imp, nfsServer: e.target.value })} placeholder="192.168.0.10" /></div>
              <div><label className="fld" htmlFor="admin-volumesadmin-fld-2" style={{ marginTop: 0 }}>{t('nodes.impSize')}</label>
                <input id="admin-volumesadmin-fld-2" type="number" min={1} value={imp.sizeGiB} onChange={(e) => setImp({ ...imp, sizeGiB: e.target.value })} /></div>
            </div>
            <div><label className="fld" htmlFor="admin-volumesadmin-fld-3" style={{ marginTop: 0 }}>{t('nodes.impPath')}<Req /></label>
              <input id="admin-volumesadmin-fld-3" type="text" value={imp.nfsPath} onChange={(e) => setImp({ ...imp, nfsPath: e.target.value })} placeholder="/srv/nfs/legacy/team-data" /></div>
          </div>
        )}
      </Modal>
    </div>
  );
}
