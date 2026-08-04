import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Container, ShieldCheck, ShieldAlert, Hammer, Bug, BadgeCheck, ChevronRight } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import StatCard from '../../../components/console/StatCard';
import Pill from '../../../components/console/Pill';
import DataTable from '../../../components/console/DataTable';
import Modal from '../../../components/console/Modal';
import { useToast } from '../../../components/console/Toast';
import { useConfirm } from '../../../components/console/Confirm';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { getAdminImages, buildImage, registerExternalImage, publishImage, rebuildImage, deleteImage, getImageCache, cacheImageOnNode, uncacheImageOnNode, getImageLogs } from '../../../api/console/resources';
import { getAdminNodes } from '../../../api/console/nodes';

export default function Images() {
  const { t } = useTranslation('consoleAdmin');
  const navigate = useNavigate();
  const { config } = useSystemConfig();
  const buildEnabled = !!config.features?.imageBuild; // 설치시 고정(레지스트리 유무)
  const [rows, setRows] = useState([]);
  const [open, setOpen] = useState(false);
  const blankForm = { name: '', desc: '', base: 'codercom/code-server:latest', apt: '', pip: '', channels: { VSCode: true, Jupyter: false, SSH: true } };
  const [form, setForm] = useState(blankForm);
  // 외부 이미지 등록(빌드 없음) 모달.
  const blankExt = { ref: 'codercom/code-server:latest', desc: '', webPort: '', webName: '', channels: { VSCode: true, Jupyter: false, SSH: true } };
  const [ext, setExt] = useState(null);
  const { toast } = useToast();
  const confirm = useConfirm();

  const load = () => getAdminImages().then(setRows);
  useEffect(() => { load(); }, []);

  // wrap 모델: 선택(=감지)된 채널로 등록. SSH(공개키)는 표준 런처가 항상 주입하므로 강제 포함.
  const build = async () => {
    await buildImage(form);
    setOpen(false); setForm(blankForm); toast(t('images.buildStarted')); load();
  };
  const submitExt = async () => {
    if (!ext.ref.trim()) { toast(t('images.needRef')); return; }
    try { await registerExternalImage(ext); } catch (e) { toast(e?.message || t('images.extFail')); return; }
    setExt(null); toast(t('images.extRegistered')); load();
  };
  const rebuild = async (id) => { await rebuildImage(id); toast(t('images.rebuildStarted')); load(); };
  const publish = async (id) => { await publishImage(id); toast(t('images.published')); load(); };
  const remove = async (id) => { if (!(await confirm({ title: t('images.delete'), message: t('confirmDelete'), confirmText: t('images.delete') }))) return; await deleteImage(id); toast(t('images.deleted')); load(); };

  // 노드별 캐시(프리페치)는 모달이 아니라 해당 이미지 행 아래 인라인 패널로 펼친다.
  const [cacheFor, setCacheFor] = useState(null);
  const [cacheRows, setCacheRows] = useState([]);
  const [nodes, setNodes] = useState([]);
  useEffect(() => { getAdminNodes().then(setNodes).catch(() => {}); }, []);
  // 같은 이미지를 다시 누르면 닫는다(토글). 열려 있는 동안 3초마다 폴링해 진행상황을 갱신한다.
  const openCache = async (img) => {
    if (cacheFor?.id === img.id) { setCacheFor(null); return; }
    setCacheFor(img); setCacheRows(await getImageCache(img.id));
  };
  useEffect(() => {
    if (!cacheFor) return undefined;
    const id = setInterval(() => getImageCache(cacheFor.id).then(setCacheRows).catch(() => {}), 3000);
    return () => clearInterval(id);
  }, [cacheFor]);
  const toggleCache = async (node, on) => {
    if (on) await cacheImageOnNode(cacheFor.id, node); else await uncacheImageOnNode(cacheFor.id, node);
    setCacheRows(await getImageCache(cacheFor.id));
  };
  const cacheStatusOf = (node) => cacheRows.find((c) => c.node === node)?.status;

  // 캐시 패널이다. 선택한 이미지 행 바로 아래에 아코디언으로 노드 체크리스트와 진행상황을 보여준다.
  const cachePanel = () => (
    <div style={{ padding: '12px 4px 14px' }}>
      <div className="legend mb">{t('images.cacheHint')}</div>
      {nodes.length === 0 && <div className="muted" style={{ fontSize: 13 }}>{t('images.noNodes')}</div>}
      <div className="grid" style={{ gap: 8, gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))' }}>
        {nodes.filter((n) => n.gpu && n.gpu !== '— (CPU 풀)').concat(nodes.filter((n) => !n.gpu || n.gpu === '— (CPU 풀)')).map((n) => {
          const st = cacheStatusOf(n.node);
          const on = !!st;
          return (
            <label key={n.node} className="flex" style={{ justifyContent: 'space-between', alignItems: 'center', gap: 8, padding: '10px 12px', borderRadius: 10, border: '1px solid var(--border)', background: 'var(--surface-2)', cursor: 'pointer' }}>
              <span className="flex" style={{ gap: 8, alignItems: 'center', fontSize: 13.5, minWidth: 0 }}>
                <input type="checkbox" checked={on} onChange={(e) => toggleCache(n.node, e.target.checked)} />
                <span style={{ fontWeight: 600 }}>{n.node}</span>
              </span>
              {st === 'cached' && <Pill variant="ok"><BadgeCheck size={12} /> {t('images.cached')}</Pill>}
              {st === 'pulling' && <Pill variant="wait">{t('images.pulling')}…</Pill>}
              {st === 'failed' && <Pill variant="err">{t('images.cacheFailed')}</Pill>}
            </label>
          );
        })}
      </div>
    </div>
  );

  // 빌드/스캔 로그 모달.
  const [logsFor, setLogsFor] = useState(null);
  const [logsText, setLogsText] = useState('');
  const openLogs = async (img) => { setLogsFor(img); setLogsText('…'); setLogsText((await getImageLogs(img.id)) || t('images.noLogs')); };

  return (
    <div>
      <PageHead title={t('images.title')} subtitle={t('images.subtitle')}
        actions={<span className="flex gap">
          <button className="btn primary" onClick={() => setExt(blankExt)}>{t('images.addExternal')}</button>
          {buildEnabled && <button className="btn" onClick={() => setOpen(true)}>{t('images.build')}</button>}
        </span>} />

      <div className="grid cols-4 mb">
        <StatCard icon={Container} tone="primary" label={t('images.registered')} value={`${rows.length}`} unit={`(active ${rows.filter((r) => r.status === 'active').length})`} />
        <StatCard icon={Hammer} tone="warn" label={t('images.building')} value={`${rows.filter((r) => r.status === 'building').length}`} unit="(Kaniko)" />
        <StatCard icon={Bug} tone="warn" label={t('images.vulns')} value={`${rows.filter((r) => r.scan === 'high').length}`} unit="(trivy)" />
        <StatCard icon={BadgeCheck} tone="free" label={t('images.signed')} value={`${rows.filter((r) => r.sign).length}`} unit={`/ ${rows.length}`} />
      </div>

      <div className="card">
        <h3><Container size={16} /> {t('images.registry')}</h3>
        <DataTable
          rows={rows}
          rowKey={(r) => r.id}
          onRowClick={(r) => navigate(`/console/admin/images/${r.id}`)}
          columns={[
            { key: 'name', header: t('images.image'), render: (r) => (
              <span className="flex" style={{ gap: 6, alignItems: 'center', fontWeight: 600 }}>{r.name}:{r.tag}
                {r.external && <Pill variant="pause">{t('images.external')}</Pill>}
              </span>) },
            { key: 'base', header: t('images.base'), className: 'mono' },
            { key: 'channels', header: t('images.channels'), render: (r) => r.channels.join('·') },
            { key: 'scan', header: t('images.scan'), render: (r) => (
              r.scan === 'pass' ? <Pill variant="ok"><ShieldCheck size={12} /> {t('images.pass')}</Pill>
                : r.scan === 'high' ? <Pill variant="err"><ShieldAlert size={12} /> High</Pill>
                  : <Pill variant="wait">{t('images.buildingP')}</Pill>
            ) },
            { key: 'sign', header: t('images.sign'), render: (r) => (r.sign ? <Pill variant="ok">{t('images.signed')}</Pill> : <Pill variant="pause">{t('images.unsigned')}</Pill>) },
            { key: 'status', header: t('images.status'), render: (r) => <Pill variant={r.status === 'active' ? 'ok' : r.status === 'building' ? 'wait' : 'pause'} dot>{r.status}</Pill> },
            { key: 'act', header: '', render: () => <ChevronRight size={15} style={{ color: 'var(--muted)' }} /> },
          ]}
        />
      </div>

      <Modal open={open} title={t('images.buildTitle')} onClose={() => setOpen(false)} width={560}
        footer={<button className="btn primary" onClick={build}>{t('images.buildBtn')}</button>}>
        <label className="fld" htmlFor="admin-images-fld-0" style={{ marginTop: 0 }}>{t('images.imgName')}</label>
        <input id="admin-images-fld-0" type="text" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="my-train" />
        <label className="fld" htmlFor="admin-images-fld-1">{t('images.imgDesc')}</label>
        <input id="admin-images-fld-1" type="text" value={form.desc} onChange={(e) => setForm({ ...form, desc: e.target.value })} placeholder={t('images.imgDescPh')} />
        <label className="fld" htmlFor="admin-images-fld-2">{t('images.baseImg')}</label>
        <input id="admin-images-fld-2" type="text" value={form.base} onChange={(e) => setForm({ ...form, base: e.target.value })} placeholder="codercom/code-server:latest" />
        <div className="legend">{t('images.baseHint')}</div>
        <label className="fld" htmlFor="admin-images-fld-3">{t('images.aptPkgs')}</label>
        <input id="admin-images-fld-3" type="text" value={form.apt} onChange={(e) => setForm({ ...form, apt: e.target.value })} placeholder="git curl build-essential" />
        <label className="fld" htmlFor="admin-images-fld-4">{t('images.pipPkgs')}</label>
        <input id="admin-images-fld-4" type="text" value={form.pip} onChange={(e) => setForm({ ...form, pip: e.target.value })} placeholder="torch pandas scikit-learn" />
        <div className="legend">{t('images.pkgHint')}</div>

        {/* 제공 채널 — Dockerfile LABEL에서 자동 감지, 관리자가 확인·조정. SSH는 표준 런처가 항상 주입. */}
        <label className="fld" htmlFor="admin-images-fld-5">{t('images.selectChannels')}</label>
        <div className="flex gap wrap" style={{ gap: 16, marginTop: 4 }}>
          {['VSCode', 'Jupyter', 'SSH'].map((c) => {
            const locked = c === 'SSH';
            return (
              <label key={c} style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13.5, cursor: locked ? 'default' : 'pointer', opacity: locked ? 0.7 : 1 }}>
                <input id="admin-images-fld-5" type="checkbox" checked={locked ? true : !!form.channels[c]} disabled={locked}
                  onChange={(e) => setForm({ ...form, channels: { ...form.channels, [c]: e.target.checked } })} />
                {c}
              </label>
            );
          })}
        </div>
        <div className="legend">{t('images.chDetected')} · {t('images.chSshAlways')}</div>
        <div className="legend" style={{ marginTop: 6, color: 'var(--primary)' }}>{t('images.wrapNote')}</div>
      </Modal>

      {/* 외부 이미지 등록(빌드 없음) — 기본 경로. Docker Hub/외부 레지스트리 ref 를 그대로 등록. */}
      <Modal open={!!ext} title={t('images.extTitle')} onClose={() => setExt(null)} width={560}
        footer={<>
          <button className="btn" onClick={() => setExt(null)}>{t('common.cancel')}</button>
          <button className="btn primary" onClick={submitExt}>{t('images.addExternal')}</button>
        </>}>
        {ext && (<>
          <label className="fld" htmlFor="admin-images-fld-6" style={{ marginTop: 0 }}>{t('images.imgRef')}</label>
          <input id="admin-images-fld-6" type="text" value={ext.ref} onChange={(e) => setExt({ ...ext, ref: e.target.value })} placeholder="codercom/code-server:latest" />
          <div className="legend">{t('images.imgRefHint')}</div>
          <label className="fld" htmlFor="admin-images-fld-7">{t('images.imgDesc')}</label>
          <input id="admin-images-fld-7" type="text" value={ext.desc} onChange={(e) => setExt({ ...ext, desc: e.target.value })} placeholder={t('images.imgDescPh')} />

          <label className="fld" htmlFor="admin-images-fld-8">{t('images.selectChannels')}</label>
          <div className="flex gap wrap" style={{ gap: 16, marginTop: 4 }}>
            {['VSCode', 'Jupyter', 'SSH'].map((c) => {
              const locked = c === 'SSH';
              return (
                <label key={c} style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13.5, cursor: locked ? 'default' : 'pointer', opacity: locked ? 0.7 : 1 }}>
                  <input id="admin-images-fld-8" type="checkbox" checked={locked ? true : !!ext.channels[c]} disabled={locked}
                    onChange={(e) => setExt({ ...ext, channels: { ...ext.channels, [c]: e.target.checked } })} />
                  {c}
                </label>
              );
            })}
          </div>
          {/* 표준(8080/8888) 외 웹앱 — 관리자가 포트 입력 시 포트포워딩 채널로 노출. */}
          <div className="grid cols-2" style={{ gap: 12, marginTop: 10 }}>
            <div>
              <label className="fld" htmlFor="admin-images-fld-9" style={{ marginTop: 0 }}>{t('images.webPort')}</label>
              <input id="admin-images-fld-9" type="number" value={ext.webPort} onChange={(e) => setExt({ ...ext, webPort: e.target.value })} placeholder="8501" />
            </div>
            <div>
              <label className="fld" htmlFor="admin-images-fld-10" style={{ marginTop: 0 }}>{t('images.webName')}</label>
              <input id="admin-images-fld-10" type="text" value={ext.webName} onChange={(e) => setExt({ ...ext, webName: e.target.value })} placeholder="app" />
            </div>
          </div>
          <div className="legend mt">{t('images.extSshOnly')}</div>
        </>)}
      </Modal>

      <Modal open={!!logsFor} title={t('images.logsTitle', { name: logsFor?.name || '' })} onClose={() => setLogsFor(null)} width={680}>
        <pre style={{ maxHeight: 420, overflow: 'auto', background: 'var(--surface-2)', border: '1px solid var(--border)', borderRadius: 8, padding: 12, fontSize: 12, lineHeight: 1.5, whiteSpace: 'pre-wrap' }}>{logsText}</pre>
      </Modal>
    </div>
  );
}
