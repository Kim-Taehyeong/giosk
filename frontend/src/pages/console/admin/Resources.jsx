import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Boxes, Cpu, HardDrive, Layers, Wand2, ChevronRight, Trash2 } from 'lucide-react';
import { putSystemConfig } from '../../../api/console/systemconfig';
import PageHead from '../../../components/console/PageHead';
import Pill from '../../../components/console/Pill';
import Modal from '../../../components/console/Modal';
import Select from '../../../components/console/Select';
import { useToast } from '../../../components/console/Toast';
import { useConfirm } from '../../../components/console/Confirm';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import {
  getOfferings, saveOffering, deleteOffering, getGpuTypes,
  getAdminGpuTypes, getGpuPricing, setGpuPricing,
} from '../../../api/console/resources';
import { c } from '../../../lib/credit';
import { tabbable } from '../../../utils/a11y';

function Field({ label, children, w = 150 }) {
  return (
    <div style={{ width: w }}>
      <label className="fld" style={{ marginTop: 0, marginBottom: 5 }}>{label}</label>
      {children}
    </div>
  );
}

/* ---------------- 오퍼링 ---------------- */
function OfferingsTab({ creditMode, onGotoPricing, title }) {
  const { t } = useTranslation('consoleAdmin');
  const [rows, setRows] = useState([]);
  const [gpuTypes, setGpuTypes] = useState([]);
  const [edit, setEdit] = useState(null); // 모달 폼(추가/수정)
  const { toast } = useToast();
  const confirm = useConfirm();
  const load = () => getOfferings().then((d) => setRows(d.items));
  useEffect(() => { load(); getGpuTypes().then((d) => setGpuTypes(d.items)); }, []);

  // 오퍼링은 공유(HAMi 분할) 전용이라 mode 가 항상 fractional 이다.
  const blank = { name: '', gpuType: '', vramMb: 8192, corePercent: 50, mode: 'fractional', pricePerHour: 120, isActive: true };
  const save = async () => {
    if (!edit.name || !edit.gpuType) { toast(t('res.needNameGpu', { defaultValue: '이름과 GPU 모델을 입력하세요.' })); return; }
    await saveOffering(edit); setEdit(null); toast(t('res.offeringSaved')); load();
  };
  const remove = async (r) => { if (!(await confirm({ title: t('res.delete'), message: t('confirmDelete'), confirmText: t('res.delete'), danger: true }))) return; await deleteOffering(r.id); setEdit(null); toast(t('res.offeringOff')); load(); };

  return (
    <div className="card">
      {/* 제목 줄에 추가 버튼을 함께 둔다 — 버튼만 있는 빈 줄이 생기지 않게. */}
      <div className="flex" style={{ justifyContent: 'space-between', alignItems: 'center', gap: 12, marginBottom: 12 }}>
        <h3 style={{ margin: 0 }}><Boxes size={16} /> {title}</h3>
        <button className="btn primary" onClick={() => setEdit({ ...blank })}>{t('res.addOffering')}</button>
      </div>
      <table>
        <thead><tr>
          <th>{t('res.name')}</th><th>{t('res.model')}</th><th>{t('res.spec')}</th>
          {creditMode && <th>{t('res.priceHour')}</th>}<th>{t('res.status')}</th><th></th>
        </tr></thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.id} className="row-link" style={{ cursor: 'pointer' }}
              onClick={(e) => { if (e.target.closest('button, a, input, select')) return; setEdit({ ...r }); }}>
              <td style={{ fontWeight: 600 }}>{r.name}</td>
              <td>{r.gpuType || '— CPU'}</td>
              <td>{r.vramMb ? `${(r.vramMb / 1024).toFixed(0)}GB·${r.corePercent}%` : '—'}</td>
              {creditMode && (
                <td>
                  {/* 단가는 단가 탭에서 관리 — 여기선 표시만, 클릭하면 단가 페이지로 */}
                  <button className="btn sm" onClick={() => onGotoPricing()} title={t('res.editInPricing', { defaultValue: '단가 탭에서 편집' })}>
                    {c(r.pricePerHour)} C/h →
                  </button>
                </td>
              )}
              <td><Pill variant={r.isActive ? 'ok' : 'pause'} dot>{r.isActive ? t('res.active') : t('res.draft')}</Pill></td>
              {/* 수정/삭제는 행 클릭으로 열리는 모달 안에 있다 — 인라인 액션 제거 */}
              <td><ChevronRight size={15} style={{ color: 'var(--muted)' }} /></td>
            </tr>
          ))}
        </tbody>
      </table>

      {/* 오퍼링 추가/수정 — 모달(수정·삭제를 모두 여기서). 삭제는 기존 오퍼링일 때만 왼쪽에. */}
      <Modal open={!!edit} title={edit?.id ? t('res.editOffering', { defaultValue: '오퍼링 수정' }) : t('res.addOffering')} onClose={() => setEdit(null)} width={560}
        footer={<>
          <span>{edit?.id && <button className="btn danger" onClick={() => remove(edit)}><Trash2 size={14} /> {t('res.delete')}</button>}</span>
          <span className="flex" style={{ gap: 8 }}>
            <button className="btn" onClick={() => setEdit(null)}>{t('res.cancel')}</button>
            <button className="btn primary" onClick={save}>{t('res.save')}</button>
          </span>
        </>}>
        {edit && (
          <div className="grid" style={{ gap: 12 }}>
            <div><label className="fld" htmlFor="admin-resources-fld-0" style={{ marginTop: 0 }}>{t('res.fName')}</label>
              <input id="admin-resources-fld-0" type="text" value={edit.name} onChange={(e) => setEdit({ ...edit, name: e.target.value })} placeholder="A100 Medium" /></div>
            <div><label className="fld" id="admin-resources-fld-1-lbl" style={{ marginTop: 0 }}>{t('res.fGpuModel')}</label>
              <Select ariaLabelledBy="admin-resources-fld-1-lbl" value={edit.gpuType} placeholder={t('res.selectGpu')} width="100%"
                onChange={(v) => setEdit({ ...edit, gpuType: v })}
                options={gpuTypes.map((g) => ({ value: g.name, label: g.name }))} /></div>
            <div className="grid cols-2" style={{ gap: 12 }}>
              <div><label className="fld" htmlFor="admin-resources-fld-2" style={{ marginTop: 0 }}>{t('res.fVram')}</label>
                <input id="admin-resources-fld-2" type="number" value={edit.vramMb} onChange={(e) => setEdit({ ...edit, vramMb: Number(e.target.value) })} /></div>
              <div><label className="fld" htmlFor="admin-resources-fld-3" style={{ marginTop: 0 }}>{t('res.fCore')}</label>
                <input id="admin-resources-fld-3" type="number" value={edit.corePercent} onChange={(e) => setEdit({ ...edit, corePercent: Number(e.target.value) })} /></div>
            </div>
            {creditMode && <div className="legend">{t('res.priceInPricingHint', { defaultValue: '단가는 저장 후 “단가” 탭에서 설정합니다(또는 GPU 단가로 자동 채우기).' })}</div>}
          </div>
        )}
      </Modal>
    </div>
  );
}

/* ---------------- 단가(과금 모델) ---------------- */
function PricingTab() {
  const { t } = useTranslation('consoleAdmin');
  const { config, refresh } = useSystemConfig();
  const [gpus, setGpus] = useState([]);
  const [offs, setOffs] = useState([]);
  const [cpu, setCpu] = useState(null);
  const { toast } = useToast();
  const storagePrice = config.storage?.pricePerGiBMonth || 0;

  const load = () => Promise.all([getAdminGpuTypes(), getGpuPricing(), getOfferings()]).then(([types, prices, od]) => {
    const pm = {}; prices.forEach((p) => { pm[p.gpuType] = p; });
    setGpus(types.map((g) => {
      const p = pm[g.name] || {};
      return { name: g.name, nodes: g.nodes,
        hour: String(p.pricePerHour || 0), gb: String(p.pricePerGb || 0), core: String(p.pricePerCore || 0),
        s_hour: p.pricePerHour || 0, s_gb: p.pricePerGb || 0, s_core: p.pricePerCore || 0 };
    }));
    const c = pm.cpu || {};
    setCpu({ hour: String(c.pricePerHour || 0), s_hour: c.pricePerHour || 0 });
    setOffs((od.items || []).map((o) => ({ id: o.id, name: o.name, mode: o.mode, gpuType: o.gpuType, corePercent: o.corePercent, full: o, price: o.pricePerHour || 0, draft: String(o.pricePerHour || 0) })));
  });
  useEffect(() => { load(); }, []);

  const setG = (name, k, v) => setGpus((cur) => cur.map((x) => (x.name === name ? { ...x, [k]: v } : x)));
  const gpuDirty = (g) => !(String(g.s_hour) === g.hour && String(g.s_gb) === g.gb && String(g.s_core) === g.core);
  const cpuDirty = () => cpu && String(cpu.s_hour) !== cpu.hour;

  const [stDraft, setStDraft] = useState(String(storagePrice));
  useEffect(() => { setStDraft(String(storagePrice)); }, [storagePrice]);
  const stDirty = String(storagePrice) !== stDraft;

  const autosave = async (label, apiCall, commit) => {
    try { await apiCall(); commit(); toast(t('res.priceSavedName', { name: label })); }
    catch { toast(t('res.priceSaveFail')); }
  };
  const commitGpu = (g) => {
    if (!gpuDirty(g)) return;
    autosave(g.name,
      () => setGpuPricing({ gpuType: g.name, pricePerHour: num(g.hour), pricePerGb: num(g.gb), pricePerCore: num(g.core) }),
      () => setGpus((cur) => cur.map((x) => (x.name === g.name ? norm3(x) : x))));
  };
  const commitCpu = () => {
    if (!cpuDirty()) return;
    autosave(t('res.cpuPricing'),
      () => setGpuPricing({ gpuType: 'cpu', pricePerHour: num(cpu.hour), pricePerGb: 0, pricePerCore: 0 }),
      () => setCpu((c) => ({ ...c, hour: String(num(c.hour)), s_hour: num(c.hour) })));
  };
  const commitStorage = () => {
    if (!stDirty) return;
    autosave(t('res.storagePricing'),
      async () => { await putSystemConfig({ storage: { pricePerGiBMonth: num(stDraft) } }); await refresh(); },
      () => setStDraft(String(num(stDraft))));
  };
  const commitOff = (o) => {
    if (String(o.price) === o.draft) return;
    autosave(o.name,
      () => saveOffering({ ...o.full, pricePerHour: num(o.draft) }),
      () => setOffs((cur) => cur.map((x) => (x.id === o.id ? { ...x, price: num(x.draft), draft: String(num(x.draft)) } : x))));
  };

  // GPU 단가로 오퍼링 단가를 자동으로 채운다. 오퍼링 단가는 그 GPU 전용단가에 코어비율(%)을 곱한 값이며 저장까지 한다.
  const autofillOfferings = async () => {
    const gm = {}; gpus.forEach((g) => { gm[g.name] = num(g.hour); });
    let n = 0;
    for (const o of offs) {
      const base = gm[o.gpuType] || 0;
      if (!base) continue;
      const price = Math.max(1, Math.round(base * (o.corePercent || 100) / 100));
      try { await saveOffering({ ...o.full, pricePerHour: price }); n++; } catch { /* skip */ }
    }
    toast(t('res.autofillDone', { n, defaultValue: `오퍼링 ${n}개 단가를 GPU 단가로 채웠습니다.` }));
    load();
  };

  const box = { padding: '16px 18px', marginBottom: 16 };
  return (
    <>
      <div className="legend mb">{t('res.pricingNote')}</div>

      {/* GPU 단가 */}
      <div className="card" style={box}>
        <h3 style={{ marginTop: 0 }}><Layers size={15} /> {t('res.gpuTypePricing')}</h3>
        <table>
          <thead><tr><th>{t('res.model')}</th><th>{t('res.nodes')}</th><th>{t('res.priceExclusive')}</th><th>{t('res.priceGb')}</th><th>{t('res.priceCore')}</th></tr></thead>
          <tbody>
            {gpus.length === 0 && <tr><td colSpan={5} className="muted" style={{ padding: 14 }}>{t('res.noGpuTypes')}</td></tr>}
            {gpus.map((g) => (
              <tr key={g.name}>
                <td style={{ fontWeight: 600 }}><Dirty on={gpuDirty(g)} />{g.name}</td>
                <td>{g.nodes}</td>
                <td><PriceInput value={g.hour} onChange={(v) => setG(g.name, 'hour', v)} onCommit={() => commitGpu(g)} /></td>
                <td><PriceInput value={g.gb} onChange={(v) => setG(g.name, 'gb', v)} onCommit={() => commitGpu(g)} /></td>
                <td><PriceInput value={g.core} onChange={(v) => setG(g.name, 'core', v)} onCommit={() => commitGpu(g)} /></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* CPU 단일 단가 */}
      <div className="card" style={box}>
        <h3 style={{ marginTop: 0 }}><Cpu size={15} /> <Dirty on={cpuDirty()} />{t('res.cpuPricing')}</h3>
        <div className="legend mb">{t('res.cpuPricingHint')}</div>
        {cpu && (
          <div><label className="fld" style={{ marginTop: 0 }}>{t('res.cpuFlat')}</label>
            <PriceInput value={cpu.hour} unit="C/h" onChange={(v) => setCpu({ ...cpu, hour: v })} onCommit={commitCpu} /></div>
        )}
      </div>

      {/* 스토리지 단가 */}
      <div className="card" style={box}>
        <h3 style={{ marginTop: 0 }}><HardDrive size={15} /> <Dirty on={stDirty} />{t('res.storagePricing')}</h3>
        <div className="legend mb">{t('res.storagePricingHint')}</div>
        <div><label className="fld" style={{ marginTop: 0 }}>{t('res.storagePrice')}</label>
          <PriceInput value={stDraft} unit="C/GiB·월" onChange={setStDraft} onCommit={commitStorage} /></div>
        <div className="legend" style={{ marginTop: 6 }}>{storagePrice > 0 ? t('res.storagePriceSetHint', { n: storagePrice }) : t('res.storagePriceFree')}</div>
      </div>

      {/* 오퍼링 단가 */}
      <div className="card" style={box}>
        <div className="flex" style={{ justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
          <h3 style={{ margin: 0 }}><Boxes size={15} /> {t('res.offeringPricing')}</h3>
          <button className="btn sm" onClick={autofillOfferings} title={t('res.autofillHint', { defaultValue: 'GPU 단가 × 코어비율로 자동 계산' })}>
            <Wand2 size={13} /> {t('res.autofill', { defaultValue: 'GPU 단가로 자동 채우기' })}
          </button>
        </div>
        <table>
          <thead><tr><th>{t('res.name')}</th><th>{t('res.model')}</th><th>{t('res.priceHour')}</th></tr></thead>
          <tbody>
            {offs.length === 0 && <tr><td colSpan={3} className="muted" style={{ padding: 14 }}>—</td></tr>}
            {offs.map((o) => (
              <tr key={o.id}>
                <td style={{ fontWeight: 600 }}><Dirty on={String(o.price) !== o.draft} />{o.name}</td>
                <td>{o.gpuType || (o.mode === 'cpu' ? 'CPU' : '—')}</td>
                <td><PriceInput value={o.draft} onChange={(v) => setOffs((cur) => cur.map((x) => (x.id === o.id ? { ...x, draft: v } : x)))} onCommit={() => commitOff(o)} /></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}

function num(v) { return Math.max(0, Number(v) || 0); }
function norm3(x) {
  return { ...x, hour: String(num(x.hour)), gb: String(num(x.gb)), core: String(num(x.core)),
    s_hour: num(x.hour), s_gb: num(x.gb), s_core: num(x.core) };
}
function Dirty({ on }) {
  if (!on) return null;
  return <span title="저장 대기" style={{ display: 'inline-block', width: 7, height: 7, borderRadius: '50%', background: 'var(--warn, #e0a800)', marginRight: 7, verticalAlign: 'middle' }} />;
}
function PriceInput({ value, unit, onChange, onCommit }) {
  return (
    <span className="flex gap" style={{ alignItems: 'center' }}>
      <input type="number" min={0} value={value} style={{ width: 90 }}
        onChange={(e) => onChange(e.target.value)}
        onBlur={() => onCommit && onCommit()}
        onKeyDown={(e) => { if (e.key === 'Enter') e.currentTarget.blur(); }} />
      {unit && <span className="muted" style={{ fontSize: 11.5, whiteSpace: 'nowrap' }}>{unit}</span>}
    </span>
  );
}

export default function Resources() {
  const { t } = useTranslation('consoleAdmin');
  const { config } = useSystemConfig();
  const creditMode = config.billing.mode === 'credit';
  const [tab, setTab] = useState('offerings');
  const title = tab === 'offerings' ? t('res.tabOfferings') : t('res.tabPricing');
  return (
    <div>
      <PageHead icon={Boxes} title={t('res.title')} subtitle={t('res.subtitle')} />
      <div className="legend mb">{t('res.catalogNote')}</div>
      <div className="subtabs">
        <span className={`st${tab === 'offerings' ? ' active' : ''}`} {...tabbable(() => setTab('offerings'), tab === 'offerings')}>{t('res.tabOfferings')}</span>
        {creditMode && <span className={`st${tab === 'pricing' ? ' active' : ''}`} {...tabbable(() => setTab('pricing'), tab === 'pricing')}>{t('res.tabPricing')}</span>}
      </div>
      {tab === 'offerings' ? (
        <OfferingsTab creditMode={creditMode} title={title} onGotoPricing={() => setTab('pricing')} />
      ) : (
        <PricingTab />
      )}
    </div>
  );
}
