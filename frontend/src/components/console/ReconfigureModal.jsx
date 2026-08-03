import React, { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Users, Lock, HardDriveDownload, Check } from 'lucide-react';
import Modal from './Modal';
import Pill from './Pill';
import Select from './Select';
import Spinner from './Spinner';
import { useToast } from './Toast';
import { useSystemConfig } from '../../context/SystemConfigContext';
import { getOfferings, getGpuTypes, getImages, getAvailability } from '../../api/console/resources';
import { reconfigureSession } from '../../api/console/sessions';
import { c } from '../../lib/credit';
import { clickable } from '../../utils/a11y';

// 중단 세션의 계산자원 변경 — "CPU로 데이터 준비 → GPU 붙여서 학습"(그 반대도).
// 홈/볼륨/데이터셋은 그대로 두고 다음에 뜰 자원만 바꾼다. 세션을 새로 만들면 준비한 데이터를
// 다시 옮겨야 하므로, 자원만 갈아끼우는 길을 여기서 준다.
//
// 노드 고정: 세션 홈(/home/work)은 노드 로컬 디스크라 세션은 이전에 떴던 노드에서만 재개된다.
// 그래서 선택지도 그 노드가 실제로 줄 수 있는 GPU 로 좁힌다 — 클러스터 어딘가에 여유가 있어도
// 그 노드에 없으면 재개되지 않기 때문이다(서버도 같은 기준으로 막는다).
//
// 부모는 반드시 조건부로(그리고 세션별 key 로) 렌더한다 — 선택 상태를 현재 사양에서 시작하기 위해
// 마운트 시점에 초기화하고, 세션이 바뀌면 새로 마운트되게 하기 위해서다.
const MODES = [
  { key: 'cpu', icon: HardDriveDownload },
  { key: 'shared', icon: Users },
  { key: 'exclusive', icon: Lock },
];

function ModeBox({ on, onClick, icon, title, desc }) {
  const Icon = icon;
  return (
    <div className={`selbox${on ? ' on' : ''}`} {...clickable(onClick)}
      style={{ padding: 12, borderRadius: 10, cursor: 'pointer',
        border: '2px solid ' + (on ? 'var(--primary)' : 'var(--border)'),
        background: on ? 'var(--primary-soft)' : 'var(--surface)' }}>
      <div style={{ fontWeight: 800, fontSize: 14, display: 'flex', alignItems: 'center', gap: 7 }}>
        {on && <Check size={14} color="var(--primary)" />}<Icon size={15} /> {title}
      </div>
      <div className="muted" style={{ fontSize: 12, marginTop: 4 }}>{desc}</div>
    </div>
  );
}

export default function ReconfigureModal({ session, onClose, onDone }) {
  const { t } = useTranslation('consoleUser');
  const { toast } = useToast();
  const { config } = useSystemConfig();
  const creditMode = config.billing.mode === 'credit';
  const spec = session.spec || {};

  const [data, setData] = useState(null); // { offerings, gpuTypes, cpuPrice, images, byNode }
  // 선택 상태는 현재 사양에서 출발한다. 카탈로그를 받고 나면 유효하지 않은 선택은 아래에서
  // "유효한 값으로 해석"해 쓴다(effective*) — 상태를 effect 로 되돌려 쓰면 렌더가 연쇄된다.
  const [mode, setMode] = useState(spec.gpuMode || 'cpu');
  const [gpuType, setGpuType] = useState(spec.gpuType || '');
  const [offeringId, setOfferingId] = useState(spec.offeringId || null);
  const [gpuCount, setGpuCount] = useState(spec.gpuCount || 1);
  const [imageId, setImageId] = useState(spec.imageId || null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let alive = true;
    Promise.all([getOfferings(), getGpuTypes(), getImages(), getAvailability()])
      .then(([o, g, im, av]) => alive && setData({
        offerings: (o.items || []).filter((x) => x.isActive),
        gpuTypes: g.items || [],
        cpuPrice: g.cpuPricePerHour || 0,
        images: im.items || [],
        byNode: av.byNode || [],
      }))
      .catch(() => alive && setData({ offerings: [], gpuTypes: [], cpuPrice: 0, images: [], byNode: [] }));
    return () => { alive = false; };
  }, []);

  // 이 세션이 묶인 노드(이전에 떴던 노드). 인벤토리에 없으면 제약을 걸지 않는다.
  const pinned = useMemo(() => {
    if (!data || !spec.node) return null;
    return data.byNode.find((n) => n.node === spec.node) || null;
  }, [data, spec.node]);
  // 노드 고정이면 후보 노드는 그 한 대뿐이다.
  const nodes = useMemo(() => (pinned ? [pinned] : (data?.byNode || [])), [data, pinned]);

  // 고른 모드에서 쓸 수 있는 GPU 모델(지금 여유가 있는 것만).
  const gpuChoices = useMemo(() => {
    if (!data) return [];
    const frac = new Set(data.offerings.filter((o) => o.gpuType && o.mode === 'fractional').map((o) => o.gpuType));
    const names = [...new Set(nodes.filter((n) => n.gpuType).map((n) => n.gpuType))];
    return names
      .filter((name) => (mode === 'shared'
        ? frac.has(name) && nodes.some((n) => n.gpuType === name && n.fractional && n.fracSlotsFree > 0)
        : nodes.some((n) => n.gpuType === name && n.gpuFree > 0)))
      .map((name) => ({ name, free: nodes.filter((n) => n.gpuType === name).reduce((sum, n) => sum + (n.gpuFree || 0), 0) }));
  }, [data, nodes, mode]);
  // 현재 선택이 이 모드에서 유효하지 않으면 첫 후보로 해석한다(상태는 그대로 두고 표시·전송만 보정).
  const selGpu = mode === 'cpu' ? ''
    : (gpuChoices.some((g) => g.name === gpuType) ? gpuType : (gpuChoices[0]?.name || ''));

  // 전용 GPU 개수 상한 = 단일 노드의 빈 GPU 최댓값(파드는 노드에 걸칠 수 없다).
  const maxGpu = useMemo(() => (selGpu
    ? nodes.filter((n) => n.gpuType === selGpu).reduce((mx, n) => Math.max(mx, n.gpuFree || 0), 0)
    : 0), [nodes, selGpu]);
  const selCount = Math.min(Math.max(1, gpuCount), Math.max(1, maxGpu));

  // 공유(분할) 오퍼링 — 선택한 모델의 것만. 그 노드에 실제로 들어가는지(fits)도 함께 판정.
  const offerings = useMemo(() => {
    if (!data || mode !== 'shared' || !selGpu) return [];
    const fracNodes = nodes.filter((n) => n.gpuType === selGpu && n.fractional);
    return data.offerings
      .filter((o) => o.gpuType === selGpu && o.mode === 'fractional')
      .map((o) => ({
        ...o,
        fits: fracNodes.some((n) => (n.fracSlotsFree || 0) >= 1
          && (!o.vramMb || (n.fracVramFreeMb || 0) >= o.vramMb)
          && (!o.corePercent || (n.fracCoresFree || 0) >= o.corePercent)),
      }));
  }, [data, nodes, mode, selGpu]);
  const selOfferingId = offerings.some((o) => o.id === offeringId && o.fits)
    ? offeringId
    : (offerings.find((o) => o.fits)?.id || null);

  // 이미지 — GPU 모드는 GPU 이미지, CPU 모드는 CPU 이미지 우선(없으면 전체).
  const images = useMemo(() => {
    if (!data) return [];
    if (mode === 'cpu') { const cpu = data.images.filter((im) => !im.gpu); return cpu.length ? cpu : data.images; }
    return data.images.filter((im) => im.gpu);
  }, [data, mode]);
  const selImageId = images.some((im) => im.id === imageId) ? imageId : (images[0]?.id || null);

  const offering = offerings.find((o) => o.id === selOfferingId);
  const gt = data?.gpuTypes.find((g) => g.name === selGpu);
  const price = mode === 'cpu' ? (data?.cpuPrice || 0)
    : mode === 'exclusive' ? (gt?.fullPricePerHour || 0) * selCount
      : (offering?.pricePerHour || 0);
  const specText = mode === 'cpu' ? t('reconf.cpuOnly')
    : mode === 'exclusive' ? (selGpu ? `${selGpu} ×${selCount}` : '—')
      : offering ? `${selGpu} · ${(offering.vramMb / 1024).toFixed(0)}GB · ${offering.corePercent}%` : '—';
  const ready = !!selImageId && (mode === 'cpu'
    || (mode === 'exclusive' ? !!selGpu && maxGpu >= selCount : !!offering));
  const changed = mode !== (spec.gpuMode || '') || selGpu !== (spec.gpuType || '')
    || (selOfferingId || null) !== (spec.offeringId || null) || (selImageId || null) !== (spec.imageId || null)
    || (mode === 'exclusive' && selCount !== spec.gpuCount);

  const apply = async (start) => {
    if (busy) return;
    setBusy(true);
    try {
      await reconfigureSession(session.id, {
        gpuMode: mode,
        gpuType: selGpu,
        gpuCount: mode === 'exclusive' ? selCount : 0,
        offeringId: mode === 'shared' ? selOfferingId : null,
        imageId: selImageId || undefined,
        start,
      });
      toast(start ? t('reconf.appliedStarted') : t('reconf.applied'));
      onDone?.();
      onClose?.();
    } catch (e) {
      toast(e?.message || t('reconf.failed'));
      setBusy(false);
    }
  };

  return (
    <Modal open title={t('reconf.title', { name: session.name || '' })} onClose={busy ? undefined : onClose} width={720}
      footer={(
        <>
          <button className="btn" onClick={onClose} disabled={busy}>{t('newSession.cancel')}</button>
          <span className="flex" style={{ gap: 8 }}>
            <button className="btn" onClick={() => apply(false)} disabled={busy || !ready || !changed}>{t('reconf.applyOnly')}</button>
            <button className="btn primary" onClick={() => apply(true)} disabled={busy || !ready}>{t('reconf.applyStart')}</button>
          </span>
        </>
      )}>
      {!data ? <Spinner pad /> : (
        <>
          <div className="legend mb">{t('reconf.hint')}</div>
          {pinned && (
            <div className="legend mb" style={{ color: 'var(--warn)' }}>
              {t('reconf.pinned', { node: pinned.node, gpu: pinned.gpuType || t('reconf.noGpu') })}
            </div>
          )}

          {/* 모드 전환 자체는 막지 않는다 — 그 모드에 자원이 있는지는 아래 선택지가 말한다. */}
          <div className="grid cols-3" style={{ gap: 10 }}>
            {MODES.map((m) => (
              <ModeBox key={m.key} on={mode === m.key} onClick={() => setMode(m.key)} icon={m.icon}
                title={t(`reconf.mode_${m.key}`)} desc={t(`reconf.modeDesc_${m.key}`)} />
            ))}
          </div>

          {mode !== 'cpu' && (
            <div className="mt">
              <label className="fld">{t('reconf.gpuModel')}</label>
              {gpuChoices.length === 0 ? (
                <div className="muted" style={{ fontSize: 12.5 }}>{t('reconf.noGpuAvail')}</div>
              ) : (
                <Select value={selGpu} onChange={setGpuType} ariaLabel={t('reconf.gpuModel')}
                  options={gpuChoices.map((g) => ({ value: g.name, label: `${g.name} (${t('reconf.freeN', { n: g.free })})` }))} />
              )}
            </div>
          )}

          {mode === 'exclusive' && selGpu && (
            <div className="mt">
              <label className="fld">{t('newSession.gpuCount')}</label>
              <Select value={String(selCount)} onChange={(v) => setGpuCount(Number(v))} ariaLabel={t('newSession.gpuCount')}
                options={[1, 2, 4].filter((n) => n <= Math.max(1, maxGpu)).map((n) => ({ value: String(n), label: String(n) }))} />
            </div>
          )}

          {mode === 'shared' && selGpu && (
            <div className="mt">
              <label className="fld">{t('reconf.offering')}</label>
              {offerings.length === 0 ? <div className="muted" style={{ fontSize: 12.5 }}>{t('reconf.noOffering')}</div> : (
                <div className="grid" style={{ gap: 8 }}>
                  {offerings.map((o) => {
                    const on = selOfferingId === o.id;
                    return (
                      <div key={o.id} {...clickable(o.fits ? () => setOfferingId(o.id) : undefined)} aria-pressed={on}
                        style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '9px 12px', borderRadius: 9,
                          cursor: o.fits ? 'pointer' : 'not-allowed', opacity: o.fits ? 1 : 0.55,
                          border: '1.5px solid ' + (on ? 'var(--primary)' : 'var(--border)'),
                          background: on ? 'var(--primary-soft)' : 'var(--surface)' }}>
                        <span style={{ width: 16, height: 16, borderRadius: '50%', flex: '0 0 auto', display: 'grid', placeItems: 'center',
                          border: '2px solid ' + (on ? 'var(--primary)' : 'var(--border)') }}>
                          {on && <span style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--primary)' }} />}
                        </span>
                        <div style={{ flex: 1, minWidth: 0 }}>
                          <div style={{ fontWeight: 700, fontSize: 14 }}>{o.name}</div>
                          <div className="muted" style={{ fontSize: 12 }}>{(o.vramMb / 1024).toFixed(0)}GB · GPU {o.corePercent}%</div>
                        </div>
                        {o.fits
                          ? (creditMode ? <Pill variant="gpu">{c(o.pricePerHour)} C/h</Pill> : null)
                          : <Pill variant="err">{t('newSession.unavailable')}</Pill>}
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          )}

          <div className="mt">
            <label className="fld">{t('reconf.image')}</label>
            <Select value={String(selImageId || '')} onChange={(v) => setImageId(Number(v))} ariaLabel={t('reconf.image')}
              options={images.map((im) => ({ value: String(im.id), label: im.name }))} />
            <div className="legend">{t('reconf.imageHint')}</div>
          </div>

          <div className="cost-box mt">
            <div className="row"><span>{t('reconf.current')}</span><span>{session.offering || '—'}</span></div>
            <div className="row big"><span>{t('reconf.next')}</span><span>{specText}</span></div>
            {creditMode && <div className="row"><span>{t('newSession.estCost')}</span><span>{price ? `${c(price)} C/h` : t('session.free')}</span></div>}
            <div className="legend">{t('reconf.keepData')}</div>
          </div>
        </>
      )}
    </Modal>
  );
}
