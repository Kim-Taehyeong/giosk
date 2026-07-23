import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { formatBytes, formatEta } from '../../../utils/format';
import { Database, HardDrive, Layers, Inbox, ChevronDown, ChevronRight, Server } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import StatCard from '../../../components/console/StatCard';
import Pill from '../../../components/console/Pill';
import DataTable from '../../../components/console/DataTable';
import { useToast } from '../../../components/console/Toast';
import { useConfirm } from '../../../components/console/Confirm';
import { getDatasets, deleteDataset, approveDatasetRequest, rejectDatasetRequest, toggleDatasetCache } from '../../../api/console/datasets';
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

  const global = data?.global || [];
  const requests = data?.requests || [];
  const totalGb = global.reduce((a, x) => a + x.sizeGb, 0);

  return (
    <div>
      <PageHead icon={Database} title={t('datasets.title')} subtitle={t('datasets.subtitle')} />

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
                                : st === 'caching' ? <Pill variant="wait" dot>{t('datasets.caching')}…</Pill>
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
    </div>
  );
}
