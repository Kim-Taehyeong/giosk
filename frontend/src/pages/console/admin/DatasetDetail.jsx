import React, { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { ArrowLeft, Database, Server, BadgeCheck } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import Pill from '../../../components/console/Pill';
import Spinner from '../../../components/console/Spinner';
import { formatBytes } from '../../../utils/format';
import { useToast } from '../../../components/console/Toast';
import { useConfirm } from '../../../components/console/Confirm';
import { getDatasets, deleteDataset, toggleDatasetCache, updateDatasetDescription, updateDatasetMeta } from '../../../api/console/datasets';
import { getAdminNodes } from '../../../api/console/nodes';

const sizeVariant = { Large: 'primary', Medium: 'gpu', Small: 'free' };

export default function DatasetDetail() {
  const { id } = useParams();
  const did = Number(id);
  const navigate = useNavigate();
  const { t } = useTranslation('consoleAdmin');
  const { toast } = useToast();
  const confirm = useConfirm();

  const [ds, setDs] = useState(null);
  const [notFound, setNotFound] = useState(false);
  const [nodes, setNodes] = useState([]);
  const [cachePage, setCachePage] = useState(0);
  const [descDraft, setDescDraft] = useState(null); // null=원본(ds.desc) 사용, 문자열=편집중

  const load = () => getDatasets().then((d) => { const f = (d.global || []).find((x) => x.id === did); if (!f) setNotFound(true); else setDs(f); });
  useEffect(() => { load(); getAdminNodes().then(setNodes).catch(() => {}); /* eslint-disable-next-line */ }, [did]);
  // 캐시 진행상황 3초 폴링.
  // 진행 중(로딩/캐싱)이면 촘촘히(1.2s), 아니면 느슨히(4s) 폴링 — 진행률이 부드럽게 갱신되게.
  const busy = ds && (ds.loadStatus === 'loading' || (ds.caches || []).some((c) => c.status === 'caching'));
  useEffect(() => { const t = setInterval(load, busy ? 1200 : 4000); return () => clearInterval(t); /* eslint-disable-next-line */ }, [did, busy]);

  if (notFound) return <div className="card">{t('datasets.notFound', { defaultValue: '데이터셋을 찾을 수 없습니다.' })}</div>;
  if (!ds) return <Spinner pad label={t('datasets.loading', { defaultValue: '…' })} />;

  // 낙관적 토글 — 클릭 즉시 UI 반영(caching pill/해제) 후 백그라운드로 요청. job 생성 대기로 클릭이 굼떠 보이지 않게.
  const toggleNode = (node) => {
    const already = cacheObjOf(node);
    setDs((d) => {
      const caches = (d.caches || []).filter((c) => c.node !== node);
      if (!already) caches.push({ node, status: 'caching', progress: 0, phase: 'copy' });
      return { ...d, caches };
    });
    toggleDatasetCache(did, node).then(load).catch(() => { toast(t('datasets.cacheFail', { defaultValue: '캐시 요청 실패' })); load(); });
  };
  const saveDesc = async () => { try { await updateDatasetDescription(did, descDraft ?? ''); setDescDraft(null); load(); toast(t('datasets.descSaved', { defaultValue: '설명을 저장했습니다.' })); } catch { toast(t('datasets.descFail', { defaultValue: '설명 저장 실패' })); } };
  const cacheObjOf = (node) => (ds.caches || []).find((c) => c.node === node);
  const cacheOf = (node) => cacheObjOf(node)?.status || ((ds.nodes || []).includes(node) ? 'cached' : undefined);
  const phaseLabel = (phase) => phase === 'extract' ? t('datasets.phaseExtract', { defaultValue: '압축 해제중' })
    : phase === 'copy' ? t('datasets.phaseCopy', { defaultValue: '복사중' })
      : t('datasets.phaseDownload', { defaultValue: '다운로드중' });
  const doDelete = async () => { if (!(await confirm({ title: t('datasets.delete'), message: t('confirmDelete'), confirmText: t('datasets.delete'), danger: true }))) return; await deleteDataset(did); toast(t('datasets.removed')); navigate('/console/admin/datasets'); };

  const orderedNodes = nodes.filter((n) => n.gpu && n.gpu !== '— (CPU 풀)').concat(nodes.filter((n) => !n.gpu || n.gpu === '— (CPU 풀)'));
  const cachedCount = orderedNodes.filter((n) => cacheOf(n.node) === 'cached').length;
  const CACHE_PER = 12; // 노드 많아질 수 있어 페이지네이션
  const cachePages = Math.max(1, Math.ceil(orderedNodes.length / CACHE_PER));
  const pageNodes = orderedNodes.slice(cachePage * CACHE_PER, cachePage * CACHE_PER + CACHE_PER);

  return (
    <div>
      <button className="btn sm" style={{ marginBottom: 12 }} onClick={() => navigate('/console/admin/datasets')}>
        <ArrowLeft size={13} /> {t('datasets.title')}
      </button>
      <PageHead
        icon={Database}
        title={ds.name}
        subtitle={<span className="flex" style={{ gap: 10, alignItems: 'center', flexWrap: 'wrap' }}>
          {ds.sizeClass && <Pill variant={sizeVariant[ds.sizeClass] || 'pause'}>{ds.sizeClass}</Pill>}
          <span className="muted">{formatBytes(ds.sizeBytes || (ds.sizeGb || 0) * 1e9)}</span>
          {ds.owner && <span className="muted">· {ds.owner}</span>}
        </span>}
        actions={<button className="btn danger" onClick={doDelete}>{t('datasets.delete')}</button>} />

      {/* 1열 — 상단 데이터셋 정보, 하단 노드 캐시(박스형 선택) */}
      <div className="card mb">
        <h3 style={{ marginTop: 0 }}><Database size={16} /> {t('datasets.info', { defaultValue: '데이터셋 정보' })}</h3>
        <div className="grid cols-2" style={{ gap: '2px 24px' }}>
          {[['Size', ds.sizeGb ? `${ds.sizeGb} GB` : formatBytes(ds.sizeBytes || 0), false], ['Scope', ds.scope || '—', true],
            ['Hash', ds.hash || '—', true], ['Status', ds.status || '—', false]].map(([k, v, mono], i) => (
            <div key={i} className="flex" style={{ justifyContent: 'space-between', gap: 12, padding: '8px 0', fontSize: 13 }}>
              <span className="muted">{k}</span>
              <span className={mono ? 'mono' : ''} style={{ fontWeight: 600, textAlign: 'right', wordBreak: 'break-all' }}>{v}</span>
            </div>
          ))}
          {/* 크기 클래스 — 편집 가능(즉시 저장) */}
          <div className="flex" style={{ justifyContent: 'space-between', gap: 12, padding: '8px 0', fontSize: 13, alignItems: 'center' }}>
            <span className="muted">{t('datasets.sizeClass', { defaultValue: '크기 클래스' })}</span>
            <select value={ds.sizeClass || 'Medium'} style={{ width: 'auto' }}
              onChange={async (e) => { try { await updateDatasetMeta(did, { description: ds.desc && ds.desc !== '—' ? ds.desc : '', sizeClass: e.target.value }); load(); toast(t('datasets.sizeClassSaved', { defaultValue: '크기 클래스를 저장했습니다.' })); } catch { toast(t('datasets.descFail', { defaultValue: '저장 실패' })); } }}>
              <option value="Small">Small</option>
              <option value="Medium">Medium</option>
              <option value="Large">Large</option>
            </select>
          </div>
        </div>
        <div className="mt">
          <label className="fld" htmlFor="admin-datasetdetail-fld-0" style={{ marginTop: 6 }}>{t('datasets.description', { defaultValue: '설명' })}</label>
          <textarea id="admin-datasetdetail-fld-0" rows={2} value={descDraft ?? (ds.desc && ds.desc !== '—' ? ds.desc : '')}
            onChange={(e) => setDescDraft(e.target.value)}
            placeholder={t('datasets.descPh', { defaultValue: '데이터셋 설명을 입력하세요' })}
            style={{ width: '100%', resize: 'vertical' }} />
          {descDraft !== null && (
            <button className="btn primary sm" style={{ marginTop: 8 }} onClick={saveDesc}>{t('common.save', { defaultValue: '저장' })}</button>
          )}
        </div>
      </div>

      {/* 노드별 캐시 — 박스형 선택(체크로 노드에 프리페치/해제) */}
      <div className="card">
        <h3 style={{ marginTop: 0 }}><Server size={16} /> {t('datasets.cache', { defaultValue: '노드 캐시' })} <span className="muted" style={{ fontSize: 12.5, fontWeight: 600 }}>({cachedCount}/{orderedNodes.length})</span></h3>
        <div className="legend mb">{t('datasets.cacheHint', { defaultValue: '데이터셋을 노드에 프리페치하면 세션이 그 노드에서 바로 마운트합니다.' })}</div>
        {orderedNodes.length === 0 ? <div className="muted" style={{ fontSize: 13 }}>{t('images.noNodes')}</div> : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            {pageNodes.map((n) => {
              const st = cacheOf(n.node);
              const on = !!st;
              return (
                <label key={n.node} onClick={() => toggleNode(n.node)}
                  style={{ display: 'flex', alignItems: 'center', gap: 14, padding: '16px 18px', borderRadius: 12, cursor: 'pointer', width: '100%',
                    border: '1.5px solid ' + (on ? 'var(--primary)' : 'var(--border)'), background: on ? 'var(--primary-soft)' : 'var(--surface-2)' }}>
                  <input type="checkbox" checked={on} readOnly style={{ width: 19, height: 19, flex: '0 0 auto', accentColor: 'var(--primary)' }} />
                  <div style={{ minWidth: 0, flex: 1 }}>
                    <div style={{ fontWeight: 800, fontSize: 15, color: on ? 'var(--primary)' : 'var(--text)' }}>{n.node}</div>
                    {n.gpu && n.gpu !== '— (CPU 풀)' && <div className="muted mono" style={{ fontSize: 11.5, marginTop: 2 }}>{n.gpu.split(' ')[0].replace(/^NVIDIA-/, '')}</div>}
                  </div>
                  {st === 'cached' && <Pill variant="ok"><BadgeCheck size={12} /> {t('images.cached')}</Pill>}
                  {st === 'caching' && (() => { const c = cacheObjOf(n.node); return (
                    <div style={{ width: 150, flex: '0 0 auto' }} onClick={(e) => e.preventDefault()}>
                      <div className="flex" style={{ justifyContent: 'space-between', fontSize: 11.5, marginBottom: 3 }}>
                        <span className="muted">{phaseLabel(c?.phase)}</span>
                        <span style={{ fontWeight: 700, color: 'var(--primary)' }}>{c?.progress || 0}%</span>
                      </div>
                      <div style={{ height: 7, borderRadius: 4, background: 'var(--surface)', overflow: 'hidden' }}>
                        <div className="progress-fill" style={{ '--p': (c?.progress || 0) / 100 }} />
                      </div>
                    </div>
                  ); })()}
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
