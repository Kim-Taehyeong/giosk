import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { formatBytes, formatEta } from '../../../utils/format';
import { Database, HardDrive, Layers, Inbox, ChevronDown, ChevronRight, Server, Plus, RefreshCw, FolderInput, Link2 } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import StatCard from '../../../components/console/StatCard';
import Pill from '../../../components/console/Pill';
import DataTable from '../../../components/console/DataTable';
import Modal from '../../../components/console/Modal';
import { Req } from '../../../components/console/Advanced';
import { useToast } from '../../../components/console/Toast';
import { useConfirm } from '../../../components/console/Confirm';
import { getDatasets, deleteDataset, approveDatasetRequest, rejectDatasetRequest, toggleDatasetCache, getDatasetInbox, registerDatasetNFS, registerDatasetURL } from '../../../api/console/datasets';
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
  // 데이터셋 등록 모달 — 방식 2가지: ① NFS 인박스(SCP 복사 후 선택) ② URL(wget).
  const [openReg, setOpenReg] = useState(false);
  const [regTab, setRegTab] = useState('nfs');
  const [reg, setReg] = useState({ name: '', scope: 'global', filename: '', url: '' });
  const [inbox, setInbox] = useState({ scpTarget: '', files: [] });
  const [regBusy, setRegBusy] = useState(false);
  const loadInbox = () => getDatasetInbox().then((d) => setInbox({ scpTarget: d.scpTarget || '', files: d.files || [] })).catch(() => {});

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

  const openRegister = () => { setReg({ name: '', scope: 'global', filename: '', url: '' }); setRegTab('nfs'); setOpenReg(true); loadInbox(); };
  // 인박스 파일 선택 시 이름 기본값 채우기(확장자 제거).
  const pickInboxFile = (fn) => setReg((r) => ({ ...r, filename: fn, name: r.name || fn.replace(/\.(zip|tar\.gz|tgz|tar)$/i, '') }));
  const submitRegister = async () => {
    const name = reg.name.trim();
    if (!name) { toast(t('datasets.regNeedName', { defaultValue: '데이터셋 이름을 입력하세요.' })); return; }
    setRegBusy(true);
    try {
      if (regTab === 'nfs') {
        if (!reg.filename) { toast(t('datasets.regNeedFile', { defaultValue: '인박스에서 파일을 선택하세요.' })); setRegBusy(false); return; }
        await registerDatasetNFS({ name, filename: reg.filename, scope: reg.scope });
      } else {
        if (!reg.url.trim()) { toast(t('datasets.regNeedUrl', { defaultValue: 'URL 을 입력하세요.' })); setRegBusy(false); return; }
        await registerDatasetURL({ name, url: reg.url.trim(), scope: reg.scope });
      }
      setOpenReg(false);
      toast(t('datasets.regStarted', { defaultValue: '등록 시작 — 목록에서 진행률을 확인하세요.' }));
      load();
    } catch (e) {
      toast(e?.code === 'name_taken' ? t('datasets.upTaken', { defaultValue: '같은 이름의 데이터셋이 있습니다.' }) : (e?.message || t('datasets.regFail', { defaultValue: '등록 실패' })));
    } finally { setRegBusy(false); }
  };

  const global = data?.global || [];
  const requests = data?.requests || [];
  const totalGb = global.reduce((a, x) => a + x.sizeGb, 0);
  // 적재/캐시 단계 라벨(다운로드중/복사중/압축해제중).
  const phaseLabel = (phase) => phase === 'extract' ? t('datasets.phaseExtract', { defaultValue: '압축 해제중' })
    : phase === 'copy' ? t('datasets.phaseCopy', { defaultValue: '복사중' })
      : t('datasets.phaseDownload', { defaultValue: '다운로드중' });

  return (
    <div>
      <PageHead icon={Database} title={t('datasets.title')} subtitle={t('datasets.subtitle')}
        actions={<button className="btn primary" onClick={openRegister}><Plus size={15} /> {t('datasets.register', { defaultValue: '데이터셋 등록' })}</button>} />

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
                        {r.loadStatus === 'loading' && <Pill variant="wait" dot>{phaseLabel(r.phase)} {r.progress || 0}%</Pill>}
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
                              <div className="flex" style={{ gap: 6, marginBottom: 8, fontWeight: 700 }}><HardDrive size={15} /> {phaseLabel(r.phase)}</div>
                              <div className="flex" style={{ justifyContent: 'space-between', fontSize: 12.5, marginBottom: 6 }}>
                                {/* 다운로드 단계만 받은용량/ETA 표시, 해제 단계는 % 만 */}
                                <span className="muted">{r.phase === 'extract' ? t('datasets.extracting', { defaultValue: '압축 해제 중…' }) : <>{formatBytes(r.downloaded)} / {formatBytes(r.sizeBytes)}{r.etaSec > 0 && <> · ETA {formatEta(r.etaSec)}</>}</>}</span>
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
                                : st === 'caching' ? (
                                  <div style={{ width: 120 }}>
                                    <div className="flex" style={{ justifyContent: 'space-between', fontSize: 11, marginBottom: 3 }}>
                                      <span className="muted">{phaseLabel(cache.phase)}</span>
                                      <span style={{ fontWeight: 700, color: 'var(--primary)' }}>{cache.progress || 0}%</span>
                                    </div>
                                    <div style={{ height: 6, borderRadius: 4, background: 'var(--surface-2)', overflow: 'hidden' }}>
                                      <div style={{ height: '100%', width: `${cache.progress || 0}%`, background: 'var(--primary)', transition: 'width .5s' }} />
                                    </div>
                                  </div>
                                ) : st === 'failed' ? <Pill variant="err">{t('datasets.cacheFailed')}</Pill>
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
                                  <div style={{ width: 130, textAlign: 'center', flex: '0 0 auto' }}>{badge}</div>
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

      <Modal open={openReg} title={t('datasets.regTitle', { defaultValue: '데이터셋 등록' })} onClose={() => !regBusy && setOpenReg(false)} width={600}
        footer={<button className="btn primary" disabled={regBusy} onClick={submitRegister}>{regBusy ? t('datasets.regBusy', { defaultValue: '등록 중…' }) : t('datasets.register', { defaultValue: '데이터셋 등록' })}</button>}>
        {/* 등록 방식 2가지 */}
        <div className="subtabs" style={{ marginTop: 0 }}>
          <span className={`st${regTab === 'nfs' ? ' active' : ''}`} onClick={() => setRegTab('nfs')}><FolderInput size={13} /> {t('datasets.regNfs', { defaultValue: 'NFS 복사(SCP)' })}</span>
          <span className={`st${regTab === 'url' ? ' active' : ''}`} onClick={() => setRegTab('url')}><Link2 size={13} /> {t('datasets.regUrl', { defaultValue: 'URL(wget)' })}</span>
        </div>

        <label className="fld">{t('datasets.name')}<Req /></label>
        <input type="text" value={reg.name} onChange={(e) => setReg({ ...reg, name: e.target.value })} placeholder="imagenet-mini" />

        {regTab === 'nfs' ? (
          <>
            {/* SCP 안내 + 인박스 파일 목록 */}
            <div className="legend" style={{ marginTop: 12 }}>{t('datasets.regNfsHint', { defaultValue: '아래 경로로 아카이브(zip·tar·tar.gz)를 복사한 뒤 파일을 선택해 등록하세요. 등록하면 인박스에서 이동되고 자동으로 해제됩니다.' })}</div>
            <div className="flex" style={{ gap: 8, alignItems: 'center', margin: '8px 0' }}>
              <code style={{ flex: 1, background: 'var(--surface-2)', padding: '9px 12px', borderRadius: 8, fontSize: 12.5, overflowX: 'auto', whiteSpace: 'nowrap' }}>
                scp &lt;파일&gt; {inbox.scpTarget || '<NFS>:/export/dataset-inbox/'}
              </code>
              <button className="btn sm" onClick={loadInbox} title={t('datasets.regRefresh', { defaultValue: '새로고침' })}><RefreshCw size={13} /></button>
            </div>
            <label className="fld">{t('datasets.regInboxFile', { defaultValue: '인박스 파일' })}<Req /></label>
            <div style={{ maxHeight: 220, overflowY: 'auto', border: '1px solid var(--border)', borderRadius: 10 }}>
              {inbox.files.length === 0 && <div className="muted" style={{ padding: '16px', fontSize: 13, textAlign: 'center' }}>{t('datasets.regInboxEmpty', { defaultValue: '인박스가 비어 있습니다. 위 경로로 복사 후 새로고침하세요.' })}</div>}
              {inbox.files.map((f) => (
                <label key={f.name} className="flex" style={{ gap: 10, alignItems: 'center', padding: '10px 12px', borderTop: '1px solid var(--border)', cursor: 'pointer', background: reg.filename === f.name ? 'var(--primary-soft)' : 'transparent' }}>
                  <input type="radio" name="inboxfile" checked={reg.filename === f.name} onChange={() => pickInboxFile(f.name)} style={{ accentColor: 'var(--primary)' }} />
                  <span style={{ flex: 1, fontWeight: 600, fontSize: 13, wordBreak: 'break-all' }}>{f.name}</span>
                  <span className="muted" style={{ fontSize: 12.5 }}>{formatBytes(f.bytes)}</span>
                </label>
              ))}
            </div>
          </>
        ) : (
          <>
            <label className="fld">{t('datasets.regUrlField', { defaultValue: '다운로드 URL' })}<Req /></label>
            <input type="text" value={reg.url} onChange={(e) => setReg({ ...reg, url: e.target.value })} placeholder="https://example.com/dataset.tar.gz" />
            <div className="legend" style={{ marginTop: 8 }}>{t('datasets.regUrlHint', { defaultValue: '서버가 이 URL 을 직접 다운로드해 등록합니다(브라우저 업로드 없음). 진행률은 목록에서 확인하세요.' })}</div>
          </>
        )}

        <label className="fld">{t('datasets.upScope', { defaultValue: '공개 범위' })}</label>
        <select value={reg.scope} onChange={(e) => setReg({ ...reg, scope: e.target.value })}>
          <option value="global">{t('datasets.scopeGlobal', { defaultValue: '전역(모든 사용자)' })}</option>
          <option value="personal">{t('datasets.scopePersonal', { defaultValue: '개인(나만)' })}</option>
        </select>
      </Modal>
    </div>
  );
}
