import React, { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Users, Lock, HardDriveDownload, Check } from 'lucide-react';
import Modal from './Modal';
import Pill from './Pill';
import Select from './Select';
import Spinner from './Spinner';
import { useToast } from './Toast';
import { useConfirm } from './Confirm';
import { useSystemConfig } from '../../context/SystemConfigContext';
import { getOfferings, getGpuTypes, getImages, getAvailability } from '../../api/console/resources';
import { reconfigureSession, reallocateSession } from '../../api/console/sessions';
import { c } from '../../lib/credit';
import { clickable } from '../../utils/a11y';

// 중단 세션의 계산자원을 바꾼다. CPU로 데이터를 준비하고 GPU를 붙여 학습하는 흐름(반대도)이다.
// 홈/볼륨/데이터셋은 그대로 두고 다음에 뜰 자원만 바꾼다. 세션을 새로 만들면 준비한 데이터를
// 다시 옮겨야 하므로, 자원만 갈아끼우는 길을 여기서 준다.
//
// 노드 고정: 세션 홈(/home/work)은 노드 로컬 디스크라 세션은 이전에 떴던 노드에서만 재개된다.
// 그래서 선택지도 그 노드가 실제로 줄 수 있는 GPU 로 좁힌다. 클러스터 어딘가에 여유가 있어도
// 그 노드에 없으면 재개되지 않기 때문이다(서버도 같은 기준으로 막는다).
//
// 부모는 반드시 조건부로(그리고 세션별 key 로) 렌더한다. 선택 상태를 현재 사양에서 시작하기 위해
// 마운트 시점에 초기화하고, 세션이 바뀌면 새로 마운트되게 하기 위해서다.
const MODES = [
  { key: 'cpu', icon: HardDriveDownload },
  { key: 'shared', icon: Users },
  { key: 'exclusive', icon: Lock },
];

// 쓸 수 없는 모드는 누르지 못하게 하고 그 자리에 이유를 적는다. 눌러 놓고 나중에 막으면
// 사용자는 무엇이 잘못됐는지 모른 채 되돌아 나와야 한다.
function ModeBox({ on, onClick, icon, title, desc, disabled, reason, current, currentLabel }) {
  const Icon = icon;
  return (
    <div className={`selbox${on ? ' on' : ''}`} {...(disabled ? { 'aria-disabled': true } : clickable(onClick))}
      title={disabled ? reason : undefined}
      style={{ padding: 12, borderRadius: 10,
        cursor: disabled ? 'not-allowed' : 'pointer',
        opacity: disabled ? 0.5 : 1,
        filter: disabled ? 'saturate(.3)' : 'none',
        border: '2px solid ' + (on ? 'var(--primary)' : 'var(--border)'),
        background: on ? 'var(--primary-soft)' : 'var(--surface)' }}>
      <div style={{ fontWeight: 800, fontSize: 14, display: 'flex', alignItems: 'center', gap: 7 }}>
        {on && <Check size={14} color="var(--primary)" />}<Icon size={15} /> {title}
        {current && <Pill variant="primary">{currentLabel}</Pill>}
      </div>
      <div className="muted" style={{ fontSize: 12, marginTop: 4 }}>{desc}</div>
      {disabled && reason && (
        <div style={{ fontSize: 11.5, marginTop: 6, color: 'var(--warn)' }}>{reason}</div>
      )}
    </div>
  );
}

export default function ReconfigureModal({ session, onClose, onDone }) {
  const { t } = useTranslation('consoleUser');
  const { toast } = useToast();
  const confirm = useConfirm();
  const { config } = useSystemConfig();
  const creditMode = config.billing.mode === 'credit';
  const spec = session.spec || {};

  const [data, setData] = useState(null); // { offerings, gpuTypes, cpuPrice, images, byNode }
  // 선택 상태는 현재 사양에서 출발한다. 카탈로그를 받고 나면 유효하지 않은 선택은 아래에서
  // 유효한 값으로 해석해 쓴다(effective*). 상태를 effect 로 되돌려 쓰면 렌더가 연쇄된다.
  const [modeSel, setMode] = useState(spec.gpuMode || 'cpu');
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
  // 선택지는 클러스터 전체에서 고른다. 노드 핀으로 목록을 좁히지 않는다.
  // 홈을 버리고 다른 노드로 재할당하는 길이 있으므로, 노드 때문에 고를 수조차 없게 만들 이유가 없다.
  // "지금 이 노드에서 뜨는가"는 아래 canStart 가 따로 본다.
  const nodes = useMemo(() => (data?.byNode || []), [data]);
  // 지금 노드에서 바로 뜰 수 있는지 판정할 때만 쓰는 후보(핀이 없으면 전체).
  const pinNodes = useMemo(() => (pinned ? [pinned] : nodes), [pinned, nodes]);

  // 각 모드를 이 노드에서 쓸 수 있는지. 못 쓰면 왜 못 쓰는지까지 같이 준다.
  // 구조적으로 불가한 것(공유가 꺼진 노드, 카드가 없는 노드)만 여기서 막는다.
  // "지금 자리가 없다"는 구조가 아니라 상태라서 막지 않는다. 사양은 저장해 두고 나중에 시작하면 된다.
  const modeState = useMemo(() => {
    const withGpu = nodes.filter((n) => n.gpuType);
    const frac = withGpu.filter((n) => n.fractional);
    const hasOffering = (name) => !!data?.offerings.some((o) => o.gpuType === name && o.mode === 'fractional');
    const shared = withGpu.length === 0 ? { ok: false, why: t('reconf.whyNoGpu') }
      : frac.length === 0 ? { ok: false, why: t('reconf.whyShareOff') }
        : !frac.some((n) => hasOffering(n.gpuType)) ? { ok: false, why: t('reconf.whyNoOffering') }
          : { ok: true };
    const exclusive = withGpu.length === 0 ? { ok: false, why: t('reconf.whyNoGpu') }
      : withGpu.every((n) => n.fractional) ? { ok: false, why: t('reconf.whyAllShared') }
        : { ok: true };
    return { cpu: { ok: true }, shared, exclusive };
  }, [data, nodes, t]);
  // 못 쓰는 모드가 골라져 있으면(현재 사양이 그랬거나 노드 설정이 바뀐 경우) 쓸 수 있는 모드로 해석한다.
  // 상태는 그대로 두고 표시와 전송만 보정한다.
  const mode = modeState[modeSel]?.ok ? modeSel : (MODES.find((m) => modeState[m.key]?.ok)?.key || 'cpu');

  // 고른 모드에서 이 노드가 원리상 줄 수 있는 GPU 모델. 지금 여유가 없어도 목록에 남긴다.
  // 여유가 없다고 목록에서 빼면 사양을 저장조차 못 해, 자리가 날 때까지 아무것도 예약해 둘 수 없다.
  // 지금 뜰 수 있는지는 아래 canStart 가 따로 판정한다.
  const gpuChoices = useMemo(() => {
    if (!data) return [];
    const frac = new Set(data.offerings.filter((o) => o.gpuType && o.mode === 'fractional').map((o) => o.gpuType));
    const names = [...new Set(nodes.filter((n) => n.gpuType).map((n) => n.gpuType))];
    return names
      .filter((name) => (mode === 'shared'
        ? frac.has(name) && nodes.some((n) => n.gpuType === name && n.fractional)
        : true))
      // 여유는 모드에 맞는 단위로 센다. 공유는 분할 슬롯, 전용은 통짜 카드다.
      // (HAMi 노드는 통짜를 주지 않아 gpuFree 가 0 이다. 그걸 공유에 쓰면 멀쩡히 빈 노드가 "여유 없음"이 된다.)
      .map((name) => {
        const own = nodes.filter((n) => n.gpuType === name);
        const key = mode === 'shared' ? 'fracSlotsFree' : 'gpuFree';
        return { name, free: own.reduce((sum, n) => sum + (n[key] || 0), 0) };
      });
  }, [data, nodes, mode]);
  // 현재 선택이 이 모드에서 유효하지 않으면 첫 후보로 해석한다(상태는 그대로 두고 표시·전송만 보정).
  const selGpu = mode === 'cpu' ? ''
    : (gpuChoices.some((g) => g.name === gpuType) ? gpuType : (gpuChoices[0]?.name || ''));

  // 전용 GPU 개수 상한 = 단일 노드의 빈 GPU 최댓값(파드는 노드에 걸칠 수 없다).
  // 상한은 클러스터 전체 기준이다. 지금 노드에서 뜨는지는 maxGpuHere 가 따로 본다.
  const maxOn = (list) => (selGpu
    ? list.filter((n) => n.gpuType === selGpu).reduce((mx, n) => Math.max(mx, n.gpuFree || 0), 0)
    : 0);
  const maxGpu = maxOn(nodes);
  const maxGpuHere = maxOn(pinNodes);
  const selCount = Math.min(Math.max(1, gpuCount), Math.max(1, maxGpu));

  // 공유(분할) 오퍼링은 선택한 모델의 것만 본다. 그 노드에 실제로 들어가는지(fits)도 함께 판정한다.
  const offerings = useMemo(() => {
    if (!data || mode !== 'shared' || !selGpu) return [];
    const frac = (list) => list.filter((n) => n.gpuType === selGpu && n.fractional);
    const roomIn = (list, o) => frac(list).some((n) => (n.fracSlotsFree || 0) >= 1
      && (!o.vramMb || (n.fracVramFreeMb || 0) >= o.vramMb)
      && (!o.corePercent || (n.fracCoresFree || 0) >= o.corePercent));
    return data.offerings
      .filter((o) => o.gpuType === selGpu && o.mode === 'fractional')
      .map((o) => ({
        ...o,
        // fits = 클러스터 어딘가에 자리가 있다. fitsHere = 지금 이 세션의 노드에 자리가 있다.
        // capable = 노드 총량으로 애초에 담을 수 있다. 셋을 갈라야
        // "만석", "여기선 안 되지만 옮기면 됨", "어디서도 안 됨"을 다르게 말할 수 있다.
        fits: roomIn(nodes, o),
        fitsHere: roomIn(pinNodes, o),
        capable: frac(nodes).some((n) => (!o.vramMb || (n.fracVramTotalMb || 0) >= o.vramMb)
          && (!o.corePercent || (n.fracCoresTotal || 100) >= o.corePercent)),
      }));
  }, [data, nodes, pinNodes, mode, selGpu]);
  // 담을 수 있는 오퍼링 중에서 고른다. 지금 자리가 있는 것을 우선하되, 만석이면 그중 첫 번째로 둔다.
  // (담을 수 없는 오퍼링은 아예 고를 수 없다.)
  const usableOfferings = offerings.filter((o) => o.capable);
  const selOfferingId = usableOfferings.some((o) => o.id === offeringId)
    ? offeringId
    : (usableOfferings.find((o) => o.fits)?.id || usableOfferings[0]?.id || null);

  // 이미지는 GPU 모드면 GPU 이미지를, CPU 모드면 CPU 이미지를 우선한다(없으면 전체).
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
  // 사양이 저장 가능한가(적용만)와 지금 뜰 수 있는가(적용하고 시작)는 다른 질문이다.
  // 자리가 없으면 저장은 되고 시작만 막힌다. 나중에 자리가 나면 재시작만 누르면 된다.
  const specValid = !!selImageId && (mode === 'cpu'
    || (mode === 'exclusive' ? !!selGpu : !!offering));
  // 지금 이 세션의 노드에서 바로 뜰 수 있는가(= 홈 그대로 재시작).
  const canStart = specValid && (mode === 'cpu'
    || (mode === 'exclusive' ? maxGpuHere >= selCount : !!offering?.fitsHere));
  // 홈을 버리고 옮기면 뜰 수 있는가. 이게 참인데 canStart 가 거짓이면
  // 막은 것은 자원이 아니라 홈의 위치다.
  const placeableSomewhere = specValid && (mode === 'cpu'
    || (mode === 'exclusive' ? maxGpu >= selCount : !!offering?.fits));
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

  // 이 노드에서는 못 뜨지만 클러스터 어딘가에는 자리가 있는 경우.
  // 홈을 옮기지 않고 버린 뒤 다른 노드에서 새로 시작한다(홈은 작업 공간이지 영속 저장소가 아니다).
  const canElsewhere = specValid && !canStart && !!pinned && placeableSomewhere;
  const applyElsewhere = async () => {
    if (busy) return;
    const ok = await confirm({
      title: t('reconf.reallocTitle'),
      message: t('reconf.reallocWarn', { node: pinned?.node || '' }),
      confirmText: t('reconf.reallocConfirm'),
      danger: true,
    });
    if (!ok) return;
    setBusy(true);
    try {
      // 사양을 먼저 저장하고(시작 없이), 그다음 홈을 버리고 다른 노드에서 띄운다.
      await reconfigureSession(session.id, {
        gpuMode: mode,
        gpuType: selGpu,
        gpuCount: mode === 'exclusive' ? selCount : 0,
        offeringId: mode === 'shared' ? selOfferingId : null,
        imageId: selImageId || undefined,
        start: false,
      });
      await reallocateSession(session.id, true);
      toast(t('reconf.reallocDone'));
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
            <button className="btn" onClick={() => apply(false)} disabled={busy || !specValid || !changed}>{t('reconf.applyOnly')}</button>
            {/* 이 노드에서 못 뜨는데 다른 노드엔 자리가 있으면, 홈을 버리고 옮겨 갈 길을 준다. */}
            {canElsewhere && (
              <button className="btn danger" onClick={applyElsewhere} disabled={busy}>{t('reconf.applyElsewhere')}</button>
            )}
            <button className="btn primary" onClick={() => apply(true)} disabled={busy || !canStart}
              title={specValid && !canStart ? t('reconf.startBlocked') : undefined}>{t('reconf.applyStart')}</button>
          </span>
        </>
      )}>
      {!data ? <Spinner pad /> : (
        <>
          <div className="legend mb">{t('reconf.hint')}</div>

          {/* 모드 전환 자체는 막지 않는다 — 그 모드에 자원이 있는지는 아래 선택지가 말한다. */}
          <div className="grid cols-3" style={{ gap: 10 }}>
            {MODES.map((m) => (
              <ModeBox key={m.key} on={mode === m.key} onClick={() => setMode(m.key)} icon={m.icon}
                title={t(`reconf.mode_${m.key}`)} desc={t(`reconf.modeDesc_${m.key}`)}
                disabled={!modeState[m.key]?.ok} reason={modeState[m.key]?.why}
                current={(spec.gpuMode || 'cpu') === m.key} currentLabel={t('reconf.currentBadge')} />
            ))}
          </div>

          {mode !== 'cpu' && (
            <div className="mt">
              <label className="fld">{t('reconf.gpuModel')}</label>
              {gpuChoices.length === 0 ? (
                // 왜 고를 게 없는지를 갈라서 말한다. "GPU 가 없다"와 "공유가 꺼져 있다"는
                // 사용자가 할 일이 다르다(포기 vs 관리자에게 요청).
                <div className="muted" style={{ fontSize: 12.5 }}>
                  {mode === 'shared' && nodes.some((n) => n.gpuType && !n.fractional)
                    ? t('reconf.shareOff')
                    : t('reconf.noGpuAvail')}
                </div>
              ) : (
                <Select value={selGpu} onChange={setGpuType} ariaLabel={t('reconf.gpuModel')}
                  options={gpuChoices.map((g) => ({
                    value: g.name,
                    // 지금 쓰고 있는 모델과 여유 개수를 라벨에 같이 적는다.
                    label: `${g.name} (${g.free > 0 ? t('reconf.freeN', { n: g.free }) : t('reconf.full')})`
                      + (spec.gpuType === g.name ? ` · ${t('reconf.currentBadge')}` : ''),
                  }))} />
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
                    // 담을 수 없는 오퍼링만 잠근다. 지금 만석인 것은 고를 수 있게 두고
                    // (사양은 저장 가능) "여유 없음"만 알린다.
                    return (
                      <div key={o.id} {...(o.capable ? clickable(() => setOfferingId(o.id)) : { 'aria-disabled': true })}
                        aria-pressed={on} title={o.capable ? undefined : t('reconf.tooBig')}
                        style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '9px 12px', borderRadius: 9,
                          cursor: o.capable ? 'pointer' : 'not-allowed', opacity: o.capable ? 1 : 0.5,
                          filter: o.capable ? 'none' : 'saturate(.3)',
                          border: '1.5px solid ' + (on ? 'var(--primary)' : 'var(--border)'),
                          background: on ? 'var(--primary-soft)' : 'var(--surface)' }}>
                        <span style={{ width: 16, height: 16, borderRadius: '50%', flex: '0 0 auto', display: 'grid', placeItems: 'center',
                          border: '2px solid ' + (on ? 'var(--primary)' : 'var(--border)') }}>
                          {on && <span style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--primary)' }} />}
                        </span>
                        <div style={{ flex: 1, minWidth: 0 }}>
                          <div style={{ fontWeight: 700, fontSize: 14, display: 'flex', alignItems: 'center', gap: 6 }}>
                            {o.name}
                            {spec.offeringId === o.id && <Pill variant="primary">{t('reconf.currentBadge')}</Pill>}
                          </div>
                          <div className="muted" style={{ fontSize: 12 }}>{(o.vramMb / 1024).toFixed(0)}GB · GPU {o.corePercent}%</div>
                        </div>
                        {!o.capable ? <Pill variant="err">{t('reconf.tooBig')}</Pill>
                          : !o.fits ? <Pill variant="warn">{t('reconf.full')}</Pill>
                            : (creditMode ? <Pill variant="gpu">{c(o.pricePerHour)} C/h</Pill> : null)}
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
            {pinned && <div className="row"><span>{t('reconf.node')}</span><span>{pinned.node}</span></div>}
            <div className="legend">{t('reconf.keepData')}</div>
            {/* 왜 "적용하고 시작"이 꺼져 있는지 말해준다. 사양은 저장할 수 있다.
                노드 제약은 실제로 막혔을 때만 꺼낸다. 막지도 않았는데 미리 경고하면 잡음이다. */}
            {specValid && !canStart && (
              <div className="legend" style={{ color: 'var(--warn)' }}>
                {canElsewhere
                  ? t('reconf.needsOtherNode', { node: pinned?.node || '' })
                  : t('reconf.startBlocked')}
              </div>
            )}
          </div>
        </>
      )}
    </Modal>
  );
}
