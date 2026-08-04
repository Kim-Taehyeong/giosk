import React, { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { ArrowLeft, Container, ShieldCheck, ShieldAlert, BadgeCheck, Server, RefreshCw, Terminal } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import Pill from '../../../components/console/Pill';
import Spinner from '../../../components/console/Spinner';
import { useToast } from '../../../components/console/Toast';
import { useConfirm } from '../../../components/console/Confirm';
import { getAdminImages, publishImage, rebuildImage, deleteImage, getImageCache, cacheImageOnNode, uncacheImageOnNode, getImageLogs } from '../../../api/console/resources';
import { getAdminNodes } from '../../../api/console/nodes';

export default function ImageDetail() {
  const { id } = useParams();
  const iid = Number(id);
  const navigate = useNavigate();
  const { t } = useTranslation('consoleAdmin');
  const { toast } = useToast();
  const confirm = useConfirm();

  const [img, setImg] = useState(null);
  const [notFound, setNotFound] = useState(false);
  const [nodes, setNodes] = useState([]);
  const [cachePage, setCachePage] = useState(0);
  const [cacheRows, setCacheRows] = useState([]);
  const [logs, setLogs] = useState(null);
  const [logsBusy, setLogsBusy] = useState(false);

  const load = () => getAdminImages().then((rows) => { const f = rows.find((r) => r.id === iid); if (!f) setNotFound(true); else setImg(f); });
  const loadCache = () => getImageCache(iid).then(setCacheRows).catch(() => {});
  useEffect(() => { load(); getAdminNodes().then(setNodes).catch(() => {}); loadCache(); /* eslint-disable-next-line */ }, [iid]);
  // 캐시 진행상황을 3초마다 폴링한다(pulling 에서 cached 로).
  useEffect(() => { const t = setInterval(loadCache, 3000); return () => clearInterval(t); /* eslint-disable-next-line */ }, [iid]);

  if (notFound) return <div className="card">{t('images.notFound', { defaultValue: '이미지를 찾을 수 없습니다.' })}</div>;
  if (!img) return <Spinner pad label={t('images.loading', { defaultValue: '…' })} />;

  const cacheStatusOf = (node) => cacheRows.find((c) => c.node === node)?.status;
  const toggleCache = async (node, on) => {
    if (on) await cacheImageOnNode(iid, node); else await uncacheImageOnNode(iid, node);
    loadCache();
  };
  const loadLogs = () => { setLogsBusy(true); getImageLogs(iid).then((x) => setLogs(x || '')).catch(() => setLogs('')).finally(() => setLogsBusy(false)); };
  const doRebuild = async () => { await rebuildImage(iid); toast(t('images.rebuildStarted')); load(); };
  const doPublish = async () => { await publishImage(iid); toast(t('images.published')); load(); };
  const doDelete = async () => { if (!(await confirm({ title: t('images.delete'), message: t('confirmDelete'), confirmText: t('images.delete'), danger: true }))) return; await deleteImage(iid); toast(t('images.deleted')); navigate('/console/admin/images'); };

  // GPU 노드를 먼저, CPU 풀을 뒤에 둔 라인 리스트다(노드가 많아도 세로로 스크롤한다).
  const orderedNodes = nodes.filter((n) => n.gpu && n.gpu !== '— (CPU 풀)').concat(nodes.filter((n) => !n.gpu || n.gpu === '— (CPU 풀)'));
  const cachedCount = cacheRows.filter((c) => c.status === 'cached').length;
  const CACHE_PER = 12; // 노드 많아질 수 있어 페이지네이션
  const cachePages = Math.max(1, Math.ceil(orderedNodes.length / CACHE_PER));
  const pageNodes = orderedNodes.slice(cachePage * CACHE_PER, cachePage * CACHE_PER + CACHE_PER);

  return (
    <div>
      <button className="btn sm" style={{ marginBottom: 12 }} onClick={() => navigate('/console/admin/images')}>
        <ArrowLeft size={13} /> {t('images.title')}
      </button>
      <PageHead
        icon={Container}
        title={`${img.name}:${img.tag}`}
        subtitle={<span className="flex" style={{ gap: 10, alignItems: 'center', flexWrap: 'wrap' }}>
          {img.external && <Pill variant="pause">{t('images.external')}</Pill>}
          <Pill variant={img.status === 'active' ? 'ok' : img.status === 'building' ? 'wait' : 'pause'} dot>{img.status}</Pill>
          {img.channels?.length ? <span className="muted">{img.channels.join(' · ')}</span> : null}
        </span>}
        actions={
          <span className="flex" style={{ gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
            {img.dockerfile && <button className="btn" onClick={doRebuild}><RefreshCw size={14} /> {t('images.rebuild')}</button>}
            {img.status === 'draft' && <button className="btn ok" onClick={doPublish}>{t('images.publish')}</button>}
            <button className="btn danger" onClick={doDelete}>{t('images.delete')}</button>
          </span>
        } />

      {/* 1열 — 상단 이미지 정보+로그 */}
      <div className="card mb">
        <h3 style={{ marginTop: 0 }}><Container size={16} /> {t('images.info', { defaultValue: '이미지 정보' })}</h3>
        <div className="grid cols-2" style={{ gap: '2px 24px' }}>
          {[['Base', img.base, true], [t('images.channels'), (img.channels || []).join(' · '), false],
            [t('images.scan'), null, false], [t('images.sign'), null, false], ['CUDA', img.cudaVersion || '—', false]].map(([k, v, mono], i) => (
            <div key={i} className="flex" style={{ justifyContent: 'space-between', gap: 12, padding: '8px 0', fontSize: 13 }}>
              <span className="muted">{k}</span>
              {k === t('images.scan')
                ? (img.scan === 'pass' ? <Pill variant="ok"><ShieldCheck size={12} /> {t('images.pass')}</Pill> : img.scan === 'high' ? <Pill variant="err"><ShieldAlert size={12} /> High</Pill> : <Pill variant="wait">{t('images.buildingP')}</Pill>)
                : k === t('images.sign')
                  ? (img.sign ? <Pill variant="ok">{t('images.signed')}</Pill> : <Pill variant="pause">{t('images.unsigned')}</Pill>)
                  : <span className={mono ? 'mono' : ''} style={{ fontWeight: 600, textAlign: 'right' }}>{v}</span>}
            </div>
          ))}
        </div>
        {img.desc && <div className="legend mt">{img.desc}</div>}
        {/* 로그 */}
        <div style={{ marginTop: 14, paddingTop: 12, borderTop: '1px solid var(--border)' }}>
          <div className="flex" style={{ justifyContent: 'space-between', alignItems: 'center', marginBottom: 6 }}>
            <span style={{ fontWeight: 700, fontSize: 13 }}><Terminal size={14} /> {t('images.logs')}</span>
            <button className="btn sm" onClick={loadLogs} disabled={logsBusy}>{logs == null ? t('images.logs') : t('sdetail.reloadLogs', { defaultValue: '새로고침' })}</button>
          </div>
          {logsBusy && logs == null ? <Spinner pad label="…" />
            : logs == null ? <div className="legend">{t('images.logsHint', { defaultValue: '빌드/스캔 로그를 불러옵니다.' })}</div>
            : logs.trim() === '' ? <div className="legend">{t('images.noLogs')}</div>
            : <pre style={{ maxHeight: 260, overflow: 'auto', background: 'var(--surface-2)', border: '1px solid var(--border)', borderRadius: 8, padding: 12, fontSize: 12, lineHeight: 1.5, whiteSpace: 'pre-wrap' }}>{logs}</pre>}
        </div>
      </div>

      {/* 노드별 캐시 — 박스형 선택 */}
      <div className="card">
        <h3 style={{ marginTop: 0 }}><Server size={16} /> {t('images.cache')} <span className="muted" style={{ fontSize: 12.5, fontWeight: 600 }}>({cachedCount}/{orderedNodes.length})</span></h3>
        <div className="legend mb">{t('images.cacheHint')}</div>
        {orderedNodes.length === 0 ? <div className="muted" style={{ fontSize: 13 }}>{t('images.noNodes')}</div> : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            {pageNodes.map((n) => {
              const st = cacheStatusOf(n.node);
              const on = !!st;
              return (
                <label key={n.node} onClick={() => toggleCache(n.node, !on)}
                  style={{ display: 'flex', alignItems: 'center', gap: 14, padding: '16px 18px', borderRadius: 12, cursor: 'pointer', width: '100%',
                    border: '1.5px solid ' + (on ? 'var(--primary)' : 'var(--border)'), background: on ? 'var(--primary-soft)' : 'var(--surface-2)' }}>
                  <input type="checkbox" checked={on} readOnly style={{ width: 19, height: 19, flex: '0 0 auto', accentColor: 'var(--primary)' }} />
                  <div style={{ minWidth: 0, flex: 1 }}>
                    <div style={{ fontWeight: 800, fontSize: 15, color: on ? 'var(--primary)' : 'var(--text)' }}>{n.node}</div>
                    {n.gpu && n.gpu !== '— (CPU 풀)' && <div className="muted mono" style={{ fontSize: 11.5, marginTop: 2 }}>{n.gpu.split(' ')[0].replace(/^NVIDIA-/, '')}</div>}
                  </div>
                  {st === 'cached' && <Pill variant="ok"><BadgeCheck size={12} /> {t('images.cached')}</Pill>}
                  {st === 'pulling' && <Pill variant="wait">{t('images.pulling')}…</Pill>}
                  {st === 'failed' && <Pill variant="err">{t('images.cacheFailed')}</Pill>}
                  {!st && <span className="muted" style={{ fontSize: 12 }}>{t('images.notCached', { defaultValue: '미캐시' })}</span>}
                </label>
              );
            })}
            {cachePages > 1 && (
              <div className="flex" style={{ justifyContent: 'center', alignItems: 'center', gap: 10, marginTop: 4 }}>
                <button className="btn sm" disabled={cachePage === 0} onClick={() => setCachePage((p) => Math.max(0, p - 1))}>{t('common.prev', { defaultValue: '이전' })}</button>
                <span className="muted" style={{ fontSize: 12.5 }}>{cachePage + 1} / {cachePages}</span>
                <button className="btn sm" disabled={cachePage >= cachePages - 1} onClick={() => setCachePage((p) => Math.min(cachePages - 1, p + 1))}>{t('common.next', { defaultValue: '다음' })}</button>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
