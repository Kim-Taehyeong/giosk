import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { formatBytes, formatEta } from '../../../utils/format';
import { Database, HardDrive, Layers, Inbox, ChevronDown, ChevronRight, Server, Upload } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import StatCard from '../../../components/console/StatCard';
import Pill from '../../../components/console/Pill';
import DataTable from '../../../components/console/DataTable';
import Modal from '../../../components/console/Modal';
import { Req } from '../../../components/console/Advanced';
import { useToast } from '../../../components/console/Toast';
import { useConfirm } from '../../../components/console/Confirm';
import { getDatasets, deleteDataset, approveDatasetRequest, rejectDatasetRequest, toggleDatasetCache, chunkedUpload } from '../../../api/console/datasets';
import { getAdminNodes } from '../../../api/console/nodes';

// 사이즈 클래스 → Pill 변형.
export const sizeVariant = { Large: 'primary', Medium: 'gpu', Small: 'free' };

export default function Datasets() {
  const navigate = useNavigate();
  const { t } = useTranslation('consoleAdmin');
  const { toast } = useToast();
  const confirm = useConfirm();
  const [data, setData] = useState(null);
  const [allNodes, setAllNodes] = useState([]);
  const [tab, setTab] = useState('registry');
  const [openId, setOpenId] = useState(null); // 노드 배치 펼친 데이터셋
  const [openUp, setOpenUp] = useState(false); // 파일 업로드 모달(선택만; 업로드는 닫고 백그라운드)
  const [up, setUp] = useState({ file: null, name: '', scope: 'global' });
  const [upActive, setUpActive] = useState(null); // 진행 중 업로드 { name, size, pct } — 리스트 배너 표시
  const [drag, setDrag] = useState(false); // 드래그오버 하이라이트
  // 새로고침 재개: 진행 중이던 업로드 메타(localStorage). 파일 객체는 새로고침에 사라지므로 재선택으로 이어간다.
  const [pending, setPending] = useState(() => { try { return JSON.parse(localStorage.getItem('giosk_pending_upload') || 'null'); } catch { return null; } });
  const clearPending = () => { localStorage.removeItem('giosk_pending_upload'); setPending(null); };

  const load = () => getDatasets().then((d) => { setData({ global: d.global.map((x) => ({ ...x })), requests: [...d.requests] }); });
  // 5초 폴링 — 적재 상태(다운로드중→완료) 라이브 갱신.
  useEffect(() => { load(); const id = setInterval(load, 5000); return () => clearInterval(id); }, []);
  // 캐시 배치 대상 노드 목록(클러스터 노드). GPU 노드 우선 정렬.
  useEffect(() => { getAdminNodes().then(setAllNodes).catch(() => {}); }, []);

  const remove = async (id) => {
    if (!(await confirm({ title: t('datasets.delete'), message: t('confirmDelete'), confirmText: t('datasets.delete') }))) return;
    await deleteDataset(id); setData((d) => ({ ...d, global: d.global.filter((x) => x.id !== id) })); toast(t('datasets.removed'));
  };
  // 노드 로컬 캐시 토글 — 백엔드가 NFS→노드 로컬 복사 Job 기동/해제. 이후 폴링으로 상태 갱신.
  const toggleNode = async (dsId, node) => { await toggleDatasetCache(dsId, node); load(); };
  const approve = async (req) => {
    await approveDatasetRequest(req.id);
    setData((d) => ({
      global: [...d.global, { id: req.id, name: req.name, sizeClass: req.sizeClass, sizeGb: req.sizeGb, sizeBytes: req.sizeBytes, hash: req.hash, status: 'active', owner: req.requester, desc: '—', nodes: [] }],
      requests: d.requests.filter((x) => x.id !== req.id),
    }));
    toast(t('datasets.approved'));
  };
  const reject = async (id) => { await rejectDatasetRequest(id); setData((d) => ({ ...d, requests: d.requests.filter((x) => x.id !== id) })); toast(t('datasets.rejected')); };

  // 업로드는 모달을 닫고 백그라운드 청크 전송 — 리스트 위 배너에 진행률/전송량이 뜨고, 사용자는 다른 작업 가능.
  // 청크(16MB)라 Cloudflare 100MB 리밋을 우회하고, 중단 시 서버 offset 으로 재개된다.
  // runUpload는 파일을 청크 업로드한다(신규 또는 재개). 진행 메타를 localStorage 에 저장해 새로고침 후 재개 가능.
  const runUpload = (file, name, scope) => {
    localStorage.setItem('giosk_pending_upload', JSON.stringify({ name, filename: file.name, size: file.size, scope }));
    setPending(null);
    setUpActive({ name, size: file.size, pct: 0, uploaded: 0 });
    chunkedUpload({ file, name, scope }, (pct, uploaded) => setUpActive((a) => (a ? { ...a, pct, uploaded } : a)))
      .then(() => { toast(t('datasets.upStarted', { defaultValue: '업로드 완료 — 서버에서 압축을 해제 중입니다.' })); setUpActive(null); clearPending(); load(); })
      .catch((e) => { toast(e?.code === 'name_taken' ? t('datasets.upTaken', { defaultValue: '같은 이름의 데이터셋이 있습니다.' }) : (e?.message || t('datasets.upFail', { defaultValue: '업로드 실패' }))); setUpActive(null); });
  };
  const startUpload = () => {
    if (!up.file || !up.name.trim()) { toast(t('datasets.upNeed', { defaultValue: '파일과 이름을 입력하세요.' })); return; }
    setOpenUp(false); const file = up.file; const name = up.name.trim(); const scope = up.scope;
    setUp({ file: null, name: '', scope: 'global' });
    runUpload(file, name, scope);
  };
  // 재개: 저장된 pending 과 이름·크기가 일치하는 파일을 다시 고르면 서버 offset 부터 이어 업로드.
  const resumeUpload = (file) => {
    if (!pending) return;
    if (file.name !== pending.filename || file.size !== pending.size) { toast(t('datasets.upResumeMismatch', { defaultValue: '같은 파일을 선택하세요(이름·크기 일치).' })); return; }
    runUpload(file, pending.name, pending.scope);
  };

  const global = data?.global || [];
  const requests = data?.requests || [];
  const totalGb = global.reduce((a, x) => a + x.sizeGb, 0);

  return (
    <div>
      <PageHead icon={Database} title={t('datasets.title')} subtitle={t('datasets.subtitle')}
        actions={<button className="btn primary" onClick={() => setOpenUp(true)}><Upload size={15} /> {t('datasets.upload', { defaultValue: '파일 업로드' })}</button>} />

      <div className="grid cols-4 mb">
        <StatCard icon={Database} tone="gpu" label={t('datasets.count')} value={String(global.length)} />
        <StatCard icon={HardDrive} tone="primary" label={t('datasets.totalSize')} value={`${totalGb}`} unit="GB" />
        <StatCard icon={Layers} tone="free" label={t('datasets.classes')} value="L · M · S" />
        <StatCard icon={Inbox} tone="warn" label={t('datasets.pending')} value={String(requests.length)} />
      </div>

      <div className="subtabs">
        <span className={`st${tab === 'registry' ? ' active' : ''}`} onClick={() => setTab('registry')}>{t('datasets.tabRegistry')}</span>
        <span className={`st${tab === 'requests' ? ' active' : ''}`} onClick={() => setTab('requests')}>{t('datasets.tabRequests')} {requests.length > 0 && <Pill variant="warn">{requests.length}</Pill>}</span>
      </div>

      {/* 새로고침 재개 배너 — 이전에 진행 중이던 업로드가 있으면 같은 파일 재선택으로 이어올린다. */}
      {pending && !upActive && (
        <div className="card mb" style={{ display: 'flex', alignItems: 'center', gap: 14, padding: '14px 18px', border: '1px solid var(--primary)' }}>
          <Upload size={18} style={{ color: 'var(--primary)', flex: '0 0 auto' }} />
          <div style={{ flex: 1, minWidth: 0, fontSize: 13 }}>
            <b>{pending.name}</b> <span className="muted">· {formatBytes(pending.size)}</span>
            <div className="muted" style={{ fontSize: 12 }}>{t('datasets.upResumeHint', { defaultValue: '중단된 업로드가 있습니다. 같은 파일을 다시 선택하면 이어서 올립니다.' })}</div>
          </div>
          <label className="btn sm primary" style={{ flex: '0 0 auto', cursor: 'pointer' }}>
            {t('datasets.upResume', { defaultValue: '이어올리기' })}
            <input type="file" accept=".zip,.tar,.gz,.tgz" style={{ display: 'none' }} onChange={(e) => { const f = e.target.files?.[0]; if (f) resumeUpload(f); }} />
          </label>
          <button className="btn sm" style={{ flex: '0 0 auto' }} onClick={clearPending}>{t('common.cancel', { defaultValue: '취소' })}</button>
        </div>
      )}

      {/* 진행 중 업로드 배너 — 모달을 닫아도 여기서 계속 진행률이 갱신된다(백그라운드). */}
      {upActive && (
        <div className="card mb" style={{ display: 'flex', alignItems: 'center', gap: 14, padding: '14px 18px' }}>
          <Upload size={18} style={{ color: 'var(--primary)', flex: '0 0 auto' }} />
          <div style={{ flex: 1, minWidth: 0 }}>
            <div className="flex" style={{ justifyContent: 'space-between', fontSize: 13, marginBottom: 6 }}>
              <span style={{ fontWeight: 700, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                {upActive.name} <span className="muted" style={{ fontWeight: 500 }}>· {formatBytes(upActive.size)}</span>
              </span>
              <span className="muted" style={{ flex: '0 0 auto', marginLeft: 12 }}>
                {upActive.pct < 100 ? t('datasets.upProgress', { defaultValue: '업로드 중' }) : t('datasets.upExtract', { defaultValue: '서버에서 압축 해제 중' })} <b style={{ color: 'var(--primary)' }}>{upActive.pct}%</b>
                {upActive.size ? <span> · {formatBytes(Math.round(upActive.size * upActive.pct / 100))}</span> : null}
              </span>
            </div>
            <div style={{ height: 8, borderRadius: 6, background: 'var(--surface-2)', overflow: 'hidden' }}>
              <div style={{ height: '100%', width: `${upActive.pct}%`, background: 'var(--primary)', transition: 'width .2s', borderRadius: 6 }} />
            </div>
          </div>
        </div>
      )}

      <div className="card">
        {tab === 'registry' ? (
          <table>
            <thead>
              <tr>
                <th>{t('datasets.name')}</th><th>{t('datasets.class')}</th><th>{t('datasets.size')}</th>
                <th>{t('datasets.hash')}</th><th>{t('datasets.cached')}</th><th>{t('datasets.owner')}</th><th>{t('datasets.action')}</th>
              </tr>
            </thead>
            <tbody>
              {global.length === 0 && <tr><td colSpan={7} style={{ textAlign: 'center', color: 'var(--muted)' }}>{t('datasets.none')}</td></tr>}
              {global.map((r) => (
                <React.Fragment key={r.id}>
                  <tr className="row-link" style={{ cursor: 'pointer' }}
                    onClick={(e) => { if (e.target.closest('button, a, input, select, label')) return; navigate(`/console/admin/datasets/${r.id}`); }}>
                    <td style={{ fontWeight: 600 }}>
                      <span style={{ display: 'inline-flex', gap: 6, alignItems: 'center' }}>{r.name}
                        {r.loadStatus === 'loading' && <Pill variant="wait" dot>{t('datasets.loading')}</Pill>}
                        {r.loadStatus === 'failed' && <Pill variant="err">{t('datasets.loadFailed')}</Pill>}
                      </span>
                    </td>
                    <td><Pill variant={sizeVariant[r.sizeClass]}>{r.sizeClass}</Pill></td>
                    <td>{formatBytes(r.sizeBytes)}</td>
                    <td><span className="mono" style={{ fontSize: 12 }}>{r.hash}</span></td>
                    <td>{t('datasets.cachedN', { n: r.nodes.length })}</td>
                    <td>{r.owner}</td>
                    <td><ChevronRight size={15} style={{ color: 'var(--muted)' }} /></td>
                  </tr>
                  {openId === r.id && (
                    <tr>
                      <td colSpan={7} style={{ background: 'var(--surface)' }}>
                        <div style={{ padding: '10px 0 12px 18px', marginLeft: 8, borderLeft: '3px solid var(--primary)' }}>
                          {/* 다운로드 중: NFS 적재 진행률(%) — 완료 후 노드 배치 UI 로 전환 */}
                          {r.loadStatus === 'loading' ? (
                            <div>
                              <div className="flex" style={{ gap: 6, marginBottom: 8, fontWeight: 700 }}><HardDrive size={15} /> {t('datasets.dlTitle')}</div>
                              <div className="flex" style={{ justifyContent: 'space-between', fontSize: 12.5, marginBottom: 6 }}>
                                <span className="muted">{formatBytes(r.downloaded)} / {formatBytes(r.sizeBytes)}{r.etaSec > 0 && <> · ETA {formatEta(r.etaSec)}</>}</span>
                                <span style={{ fontWeight: 700 }}>{r.progress || 0}%</span>
                              </div>
                              <div style={{ height: 10, borderRadius: 6, background: 'var(--surface-2)', overflow: 'hidden' }}>
                                <div style={{ height: '100%', width: `${r.progress || 0}%`, background: 'var(--primary)', transition: 'width .5s' }} />
                              </div>
                              <div className="legend mt">{t('datasets.dlHint')}</div>
                            </div>
                          ) : r.loadStatus === 'failed' ? (
                            <div style={{ color: 'var(--danger)', fontSize: 13, fontWeight: 600 }}>{t('datasets.loadFailed')}</div>
                          ) : (
                          <>
                          <div className="flex" style={{ gap: 6, marginBottom: 4, fontWeight: 700 }}><Server size={15} /> {t('datasets.assignTitle')}
                            <span className="muted" style={{ fontWeight: 500, fontSize: 12 }}>· {t('datasets.cachedN', { n: (r.nodes || []).length })}</span>
                          </div>
                          <div className="legend mb">{t('datasets.assignHint')}</div>
                          <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                            {allNodes.map((n) => {
                              const cache = (r.caches || []).find((c) => c.node === n.node);
                              const on = !!cache; // 캐시 행 존재(caching/cached/failed) = 토글 on
                              const st = cache?.status;
                              const badge = st === 'cached' ? <Pill variant="ok">{t('datasets.cacheDone')}</Pill>
                                : st === 'caching' ? <Pill variant="wait" dot>{t('datasets.caching')} {cache.progress ? `${cache.progress}%` : '…'}</Pill>
                                  : st === 'failed' ? <Pill variant="err">{t('datasets.cacheFailed')}</Pill>
                                    : <span className="muted" style={{ fontSize: 12 }}>{t('datasets.notCached')}</span>;
                              return (
                                <label key={n.node} onClick={() => toggleNode(r.id, n.node)}
                                  style={{ display: 'flex', alignItems: 'center', gap: 16, padding: '16px 20px', borderRadius: 12, cursor: 'pointer',
                                    border: '1.5px solid ' + (on ? 'var(--primary)' : 'var(--border)'),
                                    background: on ? 'var(--primary-soft)' : 'var(--surface-2)' }}>
                                  <input type="checkbox" checked={on} readOnly style={{ width: 20, height: 20, flex: '0 0 auto', accentColor: 'var(--primary)' }} />
                                  {/* 노드명 + 사양 */}
                                  <div style={{ minWidth: 0, flex: '1 1 auto' }}>
                                    <div style={{ fontWeight: 800, fontSize: 15, color: on ? 'var(--primary)' : 'var(--text)', marginBottom: 3 }}>{n.node}</div>
                                    <div className="muted" style={{ fontSize: 12.5, display: 'flex', gap: 10, flexWrap: 'wrap' }}>
                                      {[n.gpu, n.cpu, n.mem].filter(Boolean).map((m, j) => (
                                        <React.Fragment key={j}>{j > 0 && <span style={{ opacity: 0.4 }}>·</span>}<span>{m}</span></React.Fragment>
                                      ))}
                                    </div>
                                  </div>
                                  {/* 노드 상태 */}
                                  <Pill variant={n.status === 'ready' ? 'ok' : 'pause'} dot>{n.status === 'ready' ? t('datasets.nodeReady') : n.status}</Pill>
                                  {/* 캐시 상태 */}
                                  <div style={{ width: 92, textAlign: 'center', flex: '0 0 auto' }}>{badge}</div>
                                  {/* 액션 */}
                                  <span style={{ flex: '0 0 auto', width: 60, textAlign: 'right', color: on ? 'var(--danger)' : 'var(--primary)', fontWeight: 700, fontSize: 13 }}>
                                    {on ? t('datasets.uncache') : t('datasets.doCache')}
                                  </span>
                                </label>
                              );
                            })}
                            {allNodes.length === 0 && <div className="muted" style={{ fontSize: 13 }}>{t('datasets.noNodes')}</div>}
                          </div>
                          </>
                          )}
                        </div>
                      </td>
                    </tr>
                  )}
                </React.Fragment>
              ))}
            </tbody>
          </table>
        ) : (
          <DataTable
            rows={requests}
            rowKey={(r) => r.id}
            emptyText={t('datasets.noRequests')}
            columns={[
              { key: 'name', header: t('datasets.name'), render: (r) => <span style={{ fontWeight: 600 }}>{r.name}</span> },
              { key: 'sizeClass', header: t('datasets.class'), render: (r) => <Pill variant={sizeVariant[r.sizeClass]}>{r.sizeClass}</Pill> },
              { key: 'sizeGb', header: t('datasets.size'), render: (r) => (r.sizeBytes ? formatBytes(r.sizeBytes) : <span className="muted">{t('datasets.measuring')}</span>) },
              { key: 'requester', header: t('datasets.requester') },
              { key: 'createdAt', header: t('datasets.requestedAt') },
              { key: 'act', header: '', className: 'flex', render: (r) => (
                <>
                  <button className="btn sm ok" onClick={() => approve(r)}>{t('datasets.approve')}</button>
                  <button className="btn sm danger" onClick={() => reject(r.id)}>{t('datasets.reject')}</button>
                </>
              ) },
            ]}
          />
        )}
      </div>

      <Modal open={openUp} title={t('datasets.upTitle', { defaultValue: '데이터셋 파일 업로드' })} onClose={() => setOpenUp(false)} width={560}
        footer={<button className="btn primary" disabled={!up.file || !up.name.trim()} onClick={startUpload}>{t('datasets.upload', { defaultValue: '파일 업로드' })}</button>}>
        <p className="muted" style={{ marginTop: 0, fontSize: 13 }}>
          {t('datasets.upHint', { defaultValue: 'zip · tar · tar.gz 아카이브를 올리면 서버가 압축을 풀어 전역 데이터셋으로 등록합니다. 단일 파일도 그대로 등록됩니다.' })}
        </p>
        <label className="fld" style={{ marginTop: 0 }}>{t('datasets.name')}<Req /></label>
        <input type="text" value={up.name} onChange={(e) => setUp({ ...up, name: e.target.value })} placeholder="imagenet-mini" />
        <label className="fld">{t('datasets.upFile', { defaultValue: '파일' })}<Req /></label>
        {/* 드래그앤드롭 영역 + 클릭 파일 선택 */}
        <label
          onDragOver={(e) => { e.preventDefault(); setDrag(true); }}
          onDragLeave={() => setDrag(false)}
          onDrop={(e) => { e.preventDefault(); setDrag(false); const f = e.dataTransfer.files?.[0]; if (f) setUp((u) => ({ ...u, file: f, name: u.name || f.name.replace(/\.(zip|tar\.gz|tgz|tar)$/i, '') })); }}
          style={{ display: 'block', cursor: 'pointer', border: `2px dashed ${drag ? 'var(--primary)' : 'var(--border)'}`, background: drag ? 'var(--primary-soft)' : 'var(--surface-2)', borderRadius: 12, padding: '22px 16px', textAlign: 'center', transition: 'all .12s' }}>
          <input type="file" accept=".zip,.tar,.gz,.tgz" style={{ display: 'none' }}
            onChange={(e) => { const f = e.target.files?.[0] || null; setUp((u) => ({ ...u, file: f, name: u.name || (f?.name || '').replace(/\.(zip|tar\.gz|tgz|tar)$/i, '') })); }} />
          <Upload size={22} style={{ color: 'var(--muted)', marginBottom: 6 }} />
          {up.file
            ? <div style={{ fontWeight: 700, fontSize: 13.5 }}>{up.file.name} <span className="muted" style={{ fontWeight: 500 }}>· {formatBytes(up.file.size)}</span></div>
            : <div className="muted" style={{ fontSize: 13 }}>{t('datasets.upDrop', { defaultValue: '파일을 여기로 끌어다 놓거나 클릭해 선택' })}</div>}
        </label>
        <label className="fld">{t('datasets.upScope', { defaultValue: '공개 범위' })}</label>
        <select value={up.scope} onChange={(e) => setUp({ ...up, scope: e.target.value })}>
          <option value="global">{t('datasets.scopeGlobal', { defaultValue: '전역(모든 사용자)' })}</option>
          <option value="personal">{t('datasets.scopePersonal', { defaultValue: '개인(나만)' })}</option>
        </select>
      </Modal>
    </div>
  );
}
