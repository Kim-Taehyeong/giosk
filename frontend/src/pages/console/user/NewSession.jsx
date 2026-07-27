import React, { useEffect, useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Code2, NotebookPen, TerminalSquare, Check, Users, Lock, HardDriveDownload, Database, FolderInput, Boxes, Server, Zap, HardDrive, Cpu,
} from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import Pill from '../../../components/console/Pill';
import { useToast } from '../../../components/console/Toast';
import { getOfferings, getGpuTypes, getImages, getAvailability } from '../../../api/console/resources';
import { getWallet } from '../../../api/console/wallet';
import { getVolumes } from '../../../api/console/volumes';
import { createSession } from '../../../api/console/sessions';
import { getSshNodes } from '../../../api/console/sshNodes';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { c } from '../../../lib/credit';
import { useAuth } from '../../../context/AuthContext';
import { apiPut } from '../../../api/client';

// SshKeyField는 위저드의 SSH 공개키 입력. 계정에 이미 등록돼 있으면 긴 키를 다시 보여주지 않고
// 한 줄 요약 + "변경"만 노출한다(매번 빈 칸처럼 보이는 큰 상자가 뜨지 않게).
function SshKeyField({ t, registered, value, onChange }) {
  const [editing, setEditing] = useState(!registered);
  const short = registered ? `${registered.slice(0, 18)}…${registered.slice(-12)}` : '';
  if (!editing) {
    return (
      <>
        <label className="fld">{t('newSession.sshKey')}</label>
        <div className="flex" style={{ alignItems: 'center', justifyContent: 'space-between', gap: 10,
          padding: '9px 12px', borderRadius: 8, background: 'var(--surface-2)', border: '1px solid var(--border)' }}>
          <span className="mono" style={{ fontSize: 12, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{short}</span>
          <button type="button" className="btn sm" onClick={() => setEditing(true)}>
            {t('newSession.sshKeyChange', { defaultValue: '변경' })}
          </button>
        </div>
      </>
    );
  }
  return (
    <>
      <label className="fld">{t('newSession.sshKey')}</label>
      <textarea value={value} onChange={(e) => onChange(e.target.value)} placeholder="ssh-ed25519 AAAA..." style={{ minHeight: 70 }} />
    </>
  );
}

// saveSshKey는 위저드에서 입력/수정한 공개키를 계정(users.ssh_public_key)에 등록한다.
// 계정에 저장돼야 세션 컨테이너/물리노드 authorized_keys 에 주입된다(예전엔 localStorage 에만 두어 유실됐다).
// 저장 실패는 세션 생성을 막지 않는다 — 내정보 화면에서 다시 등록할 수 있다.
async function saveSshKey(key, current, refreshUser) {
  const k = (key || '').trim();
  if (!k || k === (current || '').trim()) return;
  try { await apiPut('/auth/me/ssh-key', { publicKey: k }); await refreshUser?.(); } catch { /* 무시 */ }
}

const CONN_ICON = { VSCode: Code2, Jupyter: NotebookPen, SSH: TerminalSquare };
// 커스텀 웹 채널(외부 이미지의 임의 포트명)은 CONN_ICON 에 없어 폴백 아이콘을 쓴다.
const connIcon = (c) => CONN_ICON[c] || Boxes;
const TYPE_ICON = { shared: Users, exclusive: Lock, cpu: HardDriveDownload };
const dsSizeVariant = { Large: 'primary', Medium: 'gpu', Small: 'free' };
// 오퍼링 슬라이스 크기 라벨 — GPU 지분(core%)으로 자동 산출(관리자 설정 불필요).
//   ≤25% Small · ≤50% Medium · 그 외 Large. 일반 사용자가 "얼마나 큰 조각인지" 바로 알 수 있게.
const sizeTierOf = (corePercent) => {
  const c = corePercent || 0;
  if (c <= 25) return 'small';
  if (c <= 50) return 'medium';
  return 'large';
};
const SIZE_VARIANT = { small: 'free', medium: 'gpu', large: 'primary' };
// CPU·메모리 최소 보장 = 최소 후보 노드 사양 × GPU 지분 × requestFactor. 지분은 "노드 GPU 총수 대비 내 몫".
//   전용 N개 → N/G · 분할(코어 c%) → (c/100)/G. (타임셰어링은 제거됨)
// 서버(applyGuarantee)와 동일한 식이며, Pod 에는 request 로만 걸려 여유 시 초과 사용 가능(상한 아님).
const shareOf = (mode, { gpuCount = 1, corePercent = 0, nodeGpus = 1 }) => {
  const g = Math.max(1, nodeGpus);
  if (mode === 'exclusive') return Math.max(1, gpuCount) / g;
  if (mode === 'shared') return (corePercent / 100) / g;
  return 0;
};
// 서버 applyGuarantee 와 동일한 requestFactor(0.5): 한 노드의 지분 합 ≤ 1 이므로 request 총합을 노드의
// 50% 이하로 눌러 CPU/Mem 부족으로 스케줄 실패하는 일을 원천 차단한다. 표기도 실제 예약값과 맞춰야
// "보장 32 vCPU" 처럼 실제(16)의 2배로 보이는 오표기가 안 생긴다. 서버 상수와 반드시 동일하게 유지.
const REQUEST_FACTOR = 0.5;
// gt=선택된 GPU 타입 정보(nodeCpu/nodeMemGb/nodeGpus). 정보가 없으면 null → 표기 생략.
const guaranteeOf = (gt, mode, opts) => {
  if (!gt || !gt.nodeCpu) return null;
  const sh = Math.min(1, shareOf(mode, { ...opts, nodeGpus: gt.nodeGpus })); // share 상한 1.0(서버와 동일)
  if (sh <= 0) return null;
  return {
    cpu: Math.max(1, Math.round(gt.nodeCpu * sh * REQUEST_FACTOR)),
    memGb: Math.max(1, Math.round(gt.nodeMemGb * sh * REQUEST_FACTOR)),
  };
};

function SelBox({ on, onClick, disabled, children }) {
  return (
    <div className={`selbox${on ? ' on' : ''}`} onClick={disabled ? undefined : onClick}
      style={{ padding: 18, borderRadius: 12, cursor: disabled ? 'not-allowed' : 'pointer', opacity: disabled ? 0.55 : 1,
        border: '2px solid ' + (on ? 'var(--primary)' : 'var(--border)'),
        background: on ? 'var(--primary-soft)' : 'var(--surface)' }}>
      {children}
    </div>
  );
}
const CheckMark = () => <span className="pop-in"><Check size={16} color="var(--primary)" /></span>;

// createErrMsg는 세션 생성 실패를 서버 에러코드에 맞춘 친절한 메시지로 변환한다.
function createErrMsg(e, t) {
  const code = e?.code;
  if (code === 'node_busy') return t('newSession.nodeBusy');
  if (code === 'session_limit') return t('newSession.sessionLimit');
  if (code === 'node_required') return t('newSession.nodeRequired');
  if (code === 'insufficient_credit') return t('newSession.insufficientCredit');
  return e?.message || t('newSession.createFailed');
}

// 데이터셋 1건을 정보가 풍부한 리스트 행으로 렌더(컨테이너 노드 선택 + SSH 캐시 보기 공용).
// 선택 시에도 사이즈 뱃지가 묻히지 않도록 배경은 유지하고 좌측 보더+체크박스로 선택 표시.
function DatasetRow({ t, d, selectable, on, onClick }) {
  const meta = [`${d.sizeGb} GB`, d.format, d.files, d.updatedAt && t('newSession.dsUpdatedAt', { d: d.updatedAt })].filter(Boolean);
  return (
    <div onClick={onClick}
      style={{ display: 'flex', gap: 11, alignItems: 'flex-start', padding: '10px 12px', borderRadius: 10,
        cursor: selectable ? 'pointer' : 'default', background: 'var(--surface-2)',
        border: '1.5px solid ' + (on ? 'var(--primary)' : 'var(--border)'),
        boxShadow: on ? '0 0 0 2px var(--primary-soft)' : 'none' }}>
      {selectable && (
        <span style={{ marginTop: 1, width: 18, height: 18, borderRadius: 5, flex: '0 0 auto', display: 'grid', placeItems: 'center',
          border: '1.5px solid ' + (on ? 'var(--primary)' : 'var(--border)'), background: on ? 'var(--primary)' : 'transparent' }}>
          {on && <Check size={12} color="#fff" />}
        </span>
      )}
      <div style={{ flex: 1, minWidth: 0 }}>
        <div className="flex" style={{ gap: 8, alignItems: 'center' }}>
          <span style={{ fontWeight: 700 }}>{d.name}</span>
          <Pill variant={dsSizeVariant[d.sizeClass]}>{d.sizeClass}</Pill>
        </div>
        {d.desc && <div className="muted" style={{ fontSize: 12.5, marginTop: 2 }}>{d.desc}</div>}
        <div className="muted" style={{ fontSize: 12, marginTop: 4, display: 'flex', gap: 6, flexWrap: 'wrap' }}>
          {meta.map((m, i) => <React.Fragment key={i}>{i > 0 && <span style={{ opacity: .5 }}>·</span>}<span>{m}</span></React.Fragment>)}
        </div>
        {d.path && <div className="mono" style={{ fontSize: 11.5, marginTop: 3, opacity: .7 }}>{d.path}</div>}
      </div>
    </div>
  );
}

// 좌측 스텝 사이드바 — 컨테이너/SSH 위저드 공용.
function StepSidebar({ steps, step, maxReached, summary, onGoto, labels }) {
  return (
    <div className="grid" style={{ gap: 12 }}>
      {steps.map((s, i) => {
        const reachable = i <= maxReached;
        return (
          <div key={i} onClick={() => reachable && onGoto(i)} className="selbox"
            style={{ display: 'flex', gap: 12, alignItems: 'flex-start', padding: 16, borderRadius: 12,
              border: '2px solid ' + (i === step ? 'var(--primary)' : 'var(--border)'),
              background: i === step ? 'var(--primary-soft)' : 'var(--surface)',
              cursor: reachable ? 'pointer' : 'default', opacity: reachable ? 1 : 0.5, boxShadow: 'var(--shadow)' }}>
            <div style={{ width: 30, height: 30, borderRadius: 8, flex: '0 0 auto', display: 'grid', placeItems: 'center', fontWeight: 800, fontSize: 14,
              background: i < step ? 'var(--free)' : i === step ? 'var(--primary)' : 'var(--surface-2)', color: i <= step ? '#fff' : 'var(--muted)' }}>{i < step ? <Check size={16} /> : i + 1}</div>
            <div style={{ minWidth: 0 }}>
              <div style={{ fontWeight: 700, fontSize: 14.5 }}>{s}</div>
              <div className="muted" style={{ fontSize: 12.5, marginTop: 2 }}>{summary[i] || (i === step ? labels.inProgress : labels.waiting)}</div>
            </div>
          </div>
        );
      })}
    </div>
  );
}

export default function NewSession() {
  const { t } = useTranslation('consoleUser');
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const { toast } = useToast();
  const { user, refreshUser } = useAuth();
  const { config } = useSystemConfig();
  const bill = config.billing;
  const idle = config.idle;
  const hybrid = config.deploymentMode === 'hybrid';

  // 실행 환경(하이브리드 전용): 컨테이너 vs SSH 물리노드 대여.
  const [env, setEnv] = useState('container');

  // 과금 모드는 시스템 설정에서 결정(크레딧 ↔ Dynamic ↔ Free 상호 배타). 세션 생성에선 고정.
  const billingMode = bill.mode;
  const isDynamic = billingMode === 'dynamic';
  const isFree = billingMode === 'free';

  const [step, setStep] = useState(0);
  const [maxReached, setMaxReached] = useState(0);
  const [creating, setCreating] = useState(false); // 생성 중(연타 중복 방지)
  const [offerings, setOfferings] = useState([]);
  const [gpuTypes, setGpuTypes] = useState([]); // 클러스터 노드 GPU 모델(전용 GPU 소스)
  const [cpuPrice, setCpuPrice] = useState(0); // CPU 전용 세션 시간당 단가(과금됨, 무료 아님)
  const [images, setImages] = useState([]); // 이미지 카탈로그(실 백엔드)
  const [nodes, setNodes] = useState([]);
  const [availMap, setAvailMap] = useState({}); // gpuType → {free,total,vramFree,vramTotal} (실시간 가용)
  const [fracNodes, setFracNodes] = useState([]); // 분할 노드별 VRAM/코어/슬롯 잔여(오퍼링 가용 계산)
  const [nodeGpu, setNodeGpu] = useState({}); // node → gpuType (빠른 생성 배지: 캐시노드 타입 매칭)
  const [nodesAvail, setNodesAvail] = useState([]); // byNode 원본 — 전용 GPU 개수 상한(단일 노드 free) 계산용
  const [wtype, setWtype] = useState(null);
  const [offeringId, setOfferingId] = useState(null);
  const [exGpuType, setExGpuType] = useState(null);
  const [sharedGpu, setSharedGpu] = useState(null); // 공유: 먼저 고른 GPU 모델(그 다음 오퍼링 선택)
  const [balance, setBalance] = useState(null); // 검토 단계 비용 확인용 잔액(크레딧 모드)
  const [gpuCount, setGpuCount] = useState(1);
  const [imageId, setImageId] = useState(null);
  const [vram, setVram] = useState(8192);
  const [cores, setCores] = useState(50);
  const [vols, setVols] = useState({ owned: [], shared: [], localHomes: [] });
  const [selVols, setSelVols] = useState([]); // [{ id, name, mountPath, perm }]
  const [localHomeNode, setLocalHomeNode] = useState(''); // 로컬 Home 선택(빈값=노드 로컬 임시 home, 영속은 ~/nfs)
  const [selNode, setSelNode] = useState('auto'); // 데이터셋 고급 선택 — 실제 노드 지정
  const [selDs, setSelDs] = useState([]); // 선택한 캐시 데이터셋명
  const [name, setName] = useState('');
  const [sshKey, setSshKey] = useState(user?.sshPublicKey || '');

  const TYPES = [
    { key: 'shared', title: t('newSession.tShared'), tag: t('newSession.tSharedTag'), desc: t('newSession.tSharedDesc'), variant: 'gpu' },
    { key: 'exclusive', title: t('newSession.tExclusive'), tag: t('newSession.tExclusiveTag'), desc: t('newSession.tExclusiveDesc'), variant: 'primary' },
    { key: 'cpu', title: t('newSession.tCpu'), tag: t('newSession.tCpuTag'), desc: t('newSession.tCpuDesc'), variant: 'free' },
  ];
  const typeOf = (k) => TYPES.find((x) => x.key === k);
  const STEPS = [t('newSession.step0'), t('newSession.step1'), t('newSession.step2'), t('newSession.stepVol'), t('newSession.step3')];
  const GPU_COUNTS = [{ n: 1, label: t('newSession.single') }, { n: 2, label: t('newSession.multi') }, { n: 4, label: t('newSession.multiLarge') }];

  useEffect(() => {
    getOfferings().then((d) => {
      const items = d.items.filter((o) => o.isActive);
      setOfferings(items);
      const p = params.get('preset');
      if (p === 'cpu') pickType('cpu', items);
      else if (p) pickType('shared', items);
    });
    getGpuTypes().then((d) => { setGpuTypes(d.items); setCpuPrice(d.cpuPricePerHour || 0); });
    getWallet().then((w) => setBalance(w.balance)).catch(() => {}); // 검토 단계 비용 확인(실패해도 무시)
    getImages().then((d) => setImages(d.items));
    getVolumes().then(setVols);
    getSshNodes().then(setNodes);
    getAvailability().then((a) => {
      const m = {};
      // free/total=전체(전용용), fracVram/Cores/Slots=분할(HAMi) 실자원 잔여(공유 오퍼링 가용 계산용).
      a.byType.forEach((x) => { m[x.gpuType] = {
        free: x.free, total: x.total,
        vramFree: x.fracVramFreeMb || 0, vramTotal: x.fracVramTotalMb || 0,
      }; });
      setAvailMap(m);
      const ng = {};
      (a.byNode || []).forEach((n) => { ng[n.node] = n.gpuType; });
      setNodeGpu(ng);
      setNodesAvail(a.byNode || []);
      // 분할 노드별 잔여(노드 단위 패킹으로 "오퍼링 몇 개 더 가능"을 정확 계산).
      setFracNodes((a.byNode || []).filter((n) => n.fractional).map((n) => ({
        gpuType: n.gpuType,
        vramFree: n.fracVramFreeMb || 0, vramTotal: n.fracVramTotalMb || 0,
        coresFree: n.fracCoresFree || 0, coresTotal: n.fracCoresTotal || 0,
        slotsFree: n.fracSlotsFree || 0, slotsTotal: n.fracSlotsTotal || 0,
      })));
    });
  }, []); // eslint-disable-line

  const toggleVol = (v) => setSelVols((cur) => cur.some((x) => x.id === v.id)
    ? cur.filter((x) => x.id !== v.id)
    : [...cur, { id: v.id, name: v.name, mountPath: `/data/${v.name}`, perm: v.perm }]);
  const setMount = (id, p) => setSelVols((cur) => cur.map((x) => (x.id === id ? { ...x, mountPath: p } : x)));

  const sharedOfferings = useMemo(() => offerings.filter((o) => o.gpuType && o.mode === 'fractional'), [offerings]);
  // CUDA 물리 지원 범위(min~max) — min=GPU 아키텍처(컴퓨트 능력), max=드라이버. gpuTypes 에서 모델별 조회.
  const fmtCuda = (min, max) => (min && max ? `${min}–${max}` : min ? `${min}+` : max ? `≤${max}` : '');
  const cudaOfType = (name) => { const g = gpuTypes.find((x) => x.name === name); return g ? fmtCuda(g.cudaMin, g.cudaMax) : ''; };
  // 공유 GPU 모델 목록(단계 선택 1단계) — 분할 오퍼링이 있는 GPU 모델을 유니크하게.
  //   각 모델의 분할 가용(HAMi 노드 잔여)과 오퍼링 개수를 함께 계산.
  const sharedGpuModels = useMemo(() => {
    const names = [...new Set(sharedOfferings.map((o) => o.gpuType))];
    return names.map((name) => {
      const a = availMap[name] || {};
      return {
        gpuType: name,
        cuda: cudaOfType(name),
        vramFreeGb: Math.round((a.vramFree || 0) / 1024),
        vramTotalGb: Math.round((a.vramTotal || 0) / 1024),
        count: sharedOfferings.filter((o) => o.gpuType === name).length,
      };
    });
  }, [sharedOfferings, availMap, gpuTypes]);

  // offeringFit — 이 오퍼링이 지금 몇 개 더 가능(free) / 비었을 때 최대 몇 개(total)인지.
  // 분할 노드별로 min(VRAM 여유/오퍼링VRAM, 코어 여유/오퍼링코어, 슬롯 여유)을 구해 합산(노드 단위 패킹).
  const offeringFit = (o, useTotal) => (fracNodes || [])
    .filter((n) => n.gpuType === o.gpuType)
    .reduce((sum, n) => {
      const vram = useTotal ? n.vramTotal : n.vramFree;
      const cores = useTotal ? n.coresTotal : n.coresFree;
      const slots = useTotal ? n.slotsTotal : n.slotsFree;
      const byVram = o.vramMb > 0 ? Math.floor(vram / o.vramMb) : 0;
      const byCore = o.corePercent > 0 ? Math.floor(cores / o.corePercent) : byVram;
      return sum + Math.max(0, Math.min(byVram, byCore, slots));
    }, 0);
  // 선택한 GPU 모델의 오퍼링(단계 선택 2단계).
  const offeringsForGpu = useMemo(
    () => sharedOfferings.filter((o) => o.gpuType === sharedGpu),
    [sharedOfferings, sharedGpu],
  );
  // 전용 GPU는 노드 인벤토리(클러스터 GPU 모델)에서 — 오퍼링과 무관. 풀 단가는 모델 단위로 정의.
  const exclusiveTypes = useMemo(
    () => gpuTypes.map((g) => ({ gpuType: g.name, fullPrice: g.fullPricePerHour || 0, cuda: fmtCuda(g.cudaMin, g.cudaMax) })),
    [gpuTypes],
  );

  const offering = offerings.find((o) => o.id === offeringId);
  // CPU 세션은 CPU 전용 이미지를 우선 보여주되, 카탈로그에 CPU 이미지가 없으면 전체 이미지로 폴백한다
  // (GPU/CUDA 이미지도 CPU 세션에서 정상 동작 — GPU 가속만 없을 뿐). 그래야 "CPU면 이미지가 아예 안 뜨는" 문제가 없다.
  // GPU 세션은 GPU 이미지만.
  const cpuImgs = images.filter((im) => !im.gpu);
  const imageChoices = wtype === 'cpu' ? (cpuImgs.length ? cpuImgs : images) : images.filter((im) => im.gpu);
  // 현재 선택된 GPU 모델(전용=exGpuType / 공유=오퍼링의 gpuType / CPU=없음).
  const selectedGpuType = wtype === 'exclusive' ? exGpuType
    : wtype === 'shared' ? offering?.gpuType
      : null;
  // 빠른 생성 = 이미지가 "내가 고른 GPU 모델을 가진 캐시 노드"에 있음(스케줄러가 그 노드 선호 → 풀 없이 시작).
  // CPU 세션은 타입 제약이 없으므로 캐시된 노드가 하나라도 있으면 빠름.
  const isFastImage = (im) => {
    const cached = im.cachedNodes || [];
    if (!cached.length) return false;
    if (wtype === 'cpu') return true;
    if (!selectedGpuType) return false;
    return cached.some((n) => nodeGpu[n] === selectedGpuType);
  };
  const image = images.find((i) => i.id === imageId) || imageChoices[0] || images[0];
  const exType = exclusiveTypes.find((x) => x.gpuType === exGpuType);
  // 전용 세션은 한 노드에서 N개를 co-locate 해야 하므로, 요청 가능 최대 = "단일 노드의 최대 free GPU 수".
  // (전체 free 합이 아니라 노드별 free 의 최댓값 — 파드는 노드에 걸칠 수 없음. 노드가 2장인데 1장 사용중이면 최대 1)
  const exPerNodeMaxFree = exGpuType
    ? nodesAvail.filter((n) => n.gpuType === exGpuType).reduce((mx, n) => Math.max(mx, n.gpuFree || 0), 0)
    : 0;

  const pricePerHour = wtype === 'cpu' ? cpuPrice
    : wtype === 'exclusive' ? (exType ? exType.fullPrice * gpuCount : 0)
      : (offering ? offering.pricePerHour : 0);

  const pickType = (k, list = offerings) => {
    setWtype(k); setExGpuType(null); setSharedGpu(null); setOfferingId(null); setSelNode('auto'); setSelDs([]);
    if (k === 'shared') {
      // 공유는 단계 선택 — 분할 가용한 GPU 모델을 자동 선택하되, 오퍼링은 사용자가 고른다.
      const frac = list.filter((o) => o.gpuType && o.mode === 'fractional');
      const firstModel = [...new Set(frac.map((o) => o.gpuType))]
        .find((n) => (availMap[n]?.vramFree ?? 0) > 0) || [...new Set(frac.map((o) => o.gpuType))][0];
      if (firstModel) pickSharedGpu(firstModel, list);
    } else if (k === 'cpu') {
      const first = list.filter((o) => !o.gpuType)[0];
      setOfferingId(first?.id || null);
    }
    const img = images.filter((im) => (k === 'cpu' ? !im.gpu : im.gpu))[0];
    setImageId(img?.id);
  };
  // 공유 1단계: GPU 모델 선택. 그 모델의 오퍼링이 하나면 자동 선택, 여럿이면 사용자가 2단계에서 고른다.
  const pickSharedGpu = (name, list = offerings) => {
    setSharedGpu(name);
    const os = list.filter((o) => o.gpuType === name && o.mode === 'fractional');
    if (os.length === 1) pickShared(os[0]);
    else { setOfferingId(null); setVram(0); setCores(0); }
  };
  // 공유 2단계: 오퍼링 선택.
  const pickShared = (o) => { setOfferingId(o.id); setVram(o.vramMb || 0); setCores(o.corePercent || 0); };

  const goto = (s) => { if (s <= maxReached) setStep(s); };
  const next = () => { const s = Math.min(STEPS.length - 1, step + 1); setStep(s); setMaxReached((m) => Math.max(m, s)); };
  const prev = () => setStep((s) => Math.max(0, s - 1));
  const canNext = () => {
    if (step === 0) return !!wtype;
    if (step === 1) { if (wtype === 'exclusive') return !!exGpuType; if (wtype === 'shared') return !!offeringId; return true; }
    return true;
  };

  // 선택 조합의 CPU·메모리 최소 보장(서버가 최종 산출; 화면은 동일 식으로 미리 보여줌).
  const selGt = gpuTypes.find((g) => g.name === selectedGpuType) || null;
  const guarantee = guaranteeOf(selGt, wtype, { gpuCount, corePercent: cores });
  const dc = guarantee || { cpu: 0, memGb: 0 };
  const priceText = pricePerHour ? `${c(pricePerHour)} C/h` : t('newSession.freeC');
  const resourceText = wtype === 'cpu' ? t('dash.kindCpu')
    : wtype === 'exclusive' ? `${exGpuType || ''} ${gpuCount}`
      : `${(vram / 1024).toFixed(0)}GB · ${cores}%${dc.cpu ? ` · ${t('newSession.guaranteeCpu', { n: dc.cpu })}` : ''}`;

  // 가용성 — 선택 자원 기준. 가용하지 않으면 생성 불가, 대신 알림 신청.
  const selGpuType = wtype === 'exclusive' ? exGpuType : (offering ? offering.gpuType : null);
  const rawAvail = selGpuType ? availMap[selGpuType] : null;
  // 공유(분할)는 선택 오퍼링 기준 "몇 개 더 가능"(VRAM/코어/슬롯 패킹). HAMi 노드 없으면 0 → 가용없음.
  const avail = wtype === 'cpu' ? { free: 1, total: 1 }
    : wtype === 'exclusive' ? (rawAvail || { free: 0, total: 0 })
      : (offering ? { free: offeringFit(offering, false), total: offeringFit(offering, true) } : null);
  const unavailable = avail ? avail.free === 0 : false;
  const availText = avail
    ? (unavailable ? t('newSession.unavailable') : t('newSession.availableN', { free: avail.free, total: avail.total }))
    : '—';
  const notify = () => toast(t('newSession.notifyRequested'));

  // 자원 카드 배지: 크레딧=가격 / 선착순(Dynamic)='선착순' / 자유(Free)='자유'.
  const ppTag = (creditText) => (isFree
    ? <Pill variant="free">{t('newSession.freeMode')}</Pill>
    : isDynamic
      ? <Pill variant="free">{t('newSession.firstCome')}</Pill>
      : <Pill variant="gpu">{creditText}</Pill>);

  // 비용 박스 과금 행. 자유=제한 없음 / 선착순=가용표기 / 크레딧=예상비용.
  const billingRows = isFree ? (
    <>
      <div className="row big"><span>{t('newSession.freeMode')}</span><span style={{ color: 'var(--free)' }}>{t('newSession.noLimit')}</span></div>
      <div className="row"><span>{t('newSession.availability')}</span>
        <span style={{ color: unavailable ? 'var(--danger)' : 'var(--free)' }}>{availText}</span></div>
    </>
  ) : isDynamic ? (
    <>
      <div className="row"><span>{t('newSession.dynMaxLease')}</span><span>{t('newSession.hoursN', { n: bill.dynamic.maxLeaseHours })}</span></div>
      <div className="row"><span>{t('newSession.dynCooldown')}</span><span>{t('newSession.hoursN', { n: bill.dynamic.cooldownHours })}</span></div>
      <div className="row"><span>{t('newSession.dynConcurrent')}</span><span>{t('newSession.countN', { n: bill.dynamic.maxConcurrentSessions })}</span></div>
      <div className="row big"><span>{t('newSession.firstCome')}</span>
        <span style={{ color: unavailable ? 'var(--danger)' : 'var(--free)' }}>{availText}</span></div>
    </>
  ) : (
    <div className="row big"><span>{t('newSession.estCost')}</span><span>{priceText}</span></div>
  );
  // 비용 확인(크레딧 모드) — 잔액과 "이 단가로 몇 시간 돌릴 수 있는지". 부족하면 경고.
  const costCheck = (!isFree && !isDynamic && balance !== null && pricePerHour > 0) ? (() => {
    const hours = Math.floor(balance / pricePerHour);
    const short = balance < pricePerHour; // 1시간치도 안 되면 생성 자체가 거부된다(서버 checkAffordable)
    return (
      <>
        <div className="row"><span>{t('newSession.balance')}</span><span>{c(balance)} C</span></div>
        <div className="row"><span>{t('newSession.runtimeLeft')}</span>
          <span style={{ color: short ? 'var(--danger)' : 'inherit', fontWeight: short ? 700 : 400 }}>
            {short ? t('newSession.notEnough') : t('newSession.hoursApprox', { n: hours })}
          </span></div>
      </>
    );
  })() : null;
  const surgeLegend = (!isDynamic && !isFree && bill.credit.pricing === 'dynamic' && pricePerHour > 0)
    ? <div className="legend" style={{ marginTop: 6 }}>{t('newSession.surgeNote', { n: bill.credit.surgeIncrement })}</div>
    : null;
  // 크레딧 모드에서만 별도 가용성 행(선착순/자유는 위 박스에 포함).
  const availRow = (avail && !isDynamic && !isFree) ? (
    <div className="row"><span>{t('newSession.availability')}</span>
      <span style={{ color: unavailable ? 'var(--danger)' : 'var(--free)', fontWeight: 700 }}>{availText}</span></div>
  ) : null;

  const submit = async () => {
    if (creating) return; // 연타 중복 생성 방지
    setCreating(true);
    await saveSshKey(sshKey, user?.sshPublicKey, refreshUser); // 세션 기동 전에 등록해야 컨테이너에 주입된다
    try {
      await createSession({
        instancename: name || 'session',
        imageId: image?.id,
        offeringId: wtype === 'shared' ? offeringId : undefined, // 공유는 무조건 오퍼링 기반.
        gpuMode: wtype,
        gpuType: selGpuType || undefined,
        gpuCount: wtype === 'exclusive' ? gpuCount : 1,
        // VRAM·코어% 는 공유(HAMi 분할) 전용 값이다. 전용/CPU 모드에선 백엔드가 무시하므로
        // 이전 단계의 기본값(8192·50%)이 그대로 실려 나가지 않게 0 으로 보낸다(요청 정합성).
        vramMb: wtype === 'shared' ? vram : 0,
        corePercent: wtype === 'shared' ? cores : 0,
        cpuCores: dc.cpu,
        memGb: dc.memGb,
        volumes: selVols.map((v) => ({ id: v.id, mountPath: v.mountPath })),
        localHomeNode: localHomeNode || undefined, // 선택 시 그 노드 로컬 디스크 home 을 /home/work 로(노드 핀)
        datasets: [], // 데이터셋은 서버가 승인된 전체를 자동 마운트
      });
    } catch (e) {
      toast(createErrMsg(e, t));
      setCreating(false);
      return;
    }
    toast(t('newSession.created'));
    navigate('/console/sessions');
  };

  const nodeText = selNode === 'auto' ? t('newSession.nodeAuto') : selNode;
  const summary = [
    wtype ? typeOf(wtype).title : null,
    wtype === 'exclusive' ? (exGpuType ? `${exGpuType} ${gpuCount}` : null)
      : wtype === 'cpu' ? (offering ? t('dash.kindCpu') : null)
        : (offering ? `${(offering.vramMb / 1024).toFixed(0)}GB·${offering.corePercent}%` : null),
    image && step > 1 ? image.name : null,
    step > 2 ? (selVols.length ? t('newSession.attachedN', { n: selVols.length }) : t('newSession.noAttach')) : null,
    name || null,
  ];
  const labels = { inProgress: t('newSession.inProgress'), waiting: t('newSession.waiting') };

  return (
    <div>
      <PageHead crumb={t('newSession.crumb')} title={t('newSession.title')} subtitle={t('newSession.subtitle')}
        actions={<button className="btn" onClick={() => navigate('/console/sessions')}>{t('newSession.cancel')}</button>} />

      {hybrid && (
        <div className="card mb" style={{ padding: 14 }}>
          <label className="fld" style={{ marginTop: 0, marginBottom: 8 }}>{t('newSession.envMode')}</label>
          <div className="grid cols-2" style={{ gap: 12 }}>
            {[
              { key: 'container', icon: Boxes, title: t('newSession.envContainer'), hint: t('newSession.envContainerHint') },
              { key: 'ssh', icon: TerminalSquare, title: t('newSession.envSsh'), hint: t('newSession.envSshHint') },
            ].map((b) => {
              const on = env === b.key;
              const BIcon = b.icon;
              return (
                <div key={b.key} className={`selbox${on ? ' on' : ''}`} onClick={() => setEnv(b.key)}
                  style={{ padding: 14, borderRadius: 12, cursor: 'pointer',
                    border: '2px solid ' + (on ? 'var(--primary)' : 'var(--border)'),
                    background: on ? 'var(--primary-soft)' : 'var(--surface)' }}>
                  <div style={{ fontWeight: 800, fontSize: 15, display: 'flex', alignItems: 'center', gap: 8 }}>
                    {on && <CheckMark />}<BIcon size={16} color="var(--primary)" /> {b.title}
                  </div>
                  <div className="muted" style={{ fontSize: 12.5, marginTop: 4 }}>{b.hint}</div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {env === 'ssh' ? (
        <SSHWizard t={t} navigate={navigate} toast={toast} nodes={nodes} vols={vols} labels={labels} />
      ) : (
      <>
      <div style={{ display: 'grid', gridTemplateColumns: '300px 1fr', gap: 20, alignItems: 'start' }}>
        <StepSidebar steps={STEPS} step={step} maxReached={maxReached} summary={summary} onGoto={goto} labels={labels} />

        <div className="card" style={{ minHeight: 380, display: 'flex', flexDirection: 'column' }}>
          <div style={{ flex: 1 }}>
            {step === 0 && (
              <div>
                <h3>{t('newSession.typeQuestion')}</h3>
                <div className="grid" style={{ gap: 14 }}>
                  {TYPES.map((tp) => {
                    const Icon = TYPE_ICON[tp.key];
                    return (
                      <SelBox key={tp.key} on={wtype === tp.key} onClick={() => pickType(tp.key)}>
                        <div style={{ display: 'flex', gap: 16, alignItems: 'center' }}>
                          <div style={{ width: 52, height: 52, borderRadius: 12, flex: '0 0 auto', display: 'grid', placeItems: 'center', background: 'var(--surface-2)', color: 'var(--primary)' }}><Icon size={26} /></div>
                          <div>
                            <div style={{ fontWeight: 800, fontSize: 17, display: 'flex', alignItems: 'center', gap: 8 }}>
                              {wtype === tp.key && <CheckMark />}{tp.title} <Pill variant={tp.variant}>{tp.tag}</Pill>
                            </div>
                            <div className="muted" style={{ fontSize: 13.5, marginTop: 4 }}>{tp.desc}</div>
                          </div>
                        </div>
                      </SelBox>
                    );
                  })}
                </div>
              </div>
            )}

            {step === 1 && (
              <div>
                {wtype === 'cpu' && (
                  <>
                    <h3>{t('newSession.cpuSession')}</h3>
                    <div className="cost-box"><div className="row big"><span>{t('newSession.cpuPool')}</span><span>{ppTag(`${c(cpuPrice)} C/h`)}</span></div>
                      <div className="legend">{t('newSession.cpuNote')}</div></div>
                  </>
                )}

                {wtype === 'exclusive' && (
                  <>
                    <h3>{t('newSession.pickGpu')}</h3>
                    <div className="grid cols-2" style={{ gap: 14 }}>
                      {exclusiveTypes.map((x) => {
                        const a = availMap[x.gpuType] || { free: 0, total: 0 };
                        const un = a.free === 0;
                        return (
                          <SelBox key={x.gpuType} on={exGpuType === x.gpuType} disabled={un} onClick={() => { setExGpuType(x.gpuType); setGpuCount(1); }}>
                            <div style={{ fontWeight: 800, fontSize: 15.5, display: 'flex', alignItems: 'center', gap: 8 }}>{exGpuType === x.gpuType && <CheckMark />}{x.gpuType}</div>
                            <div className="muted" style={{ fontSize: 13, margin: '6px 0', display: 'flex', gap: 8, alignItems: 'center' }}>
                              {t('newSession.dedicated1')}
                              {x.cuda && <span style={{ fontSize: 11, fontWeight: 700, padding: '1px 6px', borderRadius: 6, background: 'var(--surface-2)', border: '1px solid var(--border)' }}>CUDA {x.cuda}</span>}
                            </div>
                            <div className="flex" style={{ justifyContent: 'space-between', alignItems: 'center', gap: 8 }}>
                              {un ? <Pill variant="err">{t('newSession.unavailable')}</Pill> : ppTag(`${c(x.fullPrice)} C/h · 1`)}
                              {un
                                ? <button className="btn sm warn" onClick={(e) => { e.stopPropagation(); notify(); }}>{t('newSession.notifyAvail')}</button>
                                : <span className="muted" style={{ fontSize: 12 }}>{t('newSession.availableN', { free: a.free, total: a.total })}</span>}
                            </div>
                          </SelBox>
                        );
                      })}
                    </div>
                    {exGpuType && (
                      <div className="mt">
                        <label className="fld">{t('newSession.gpuCount')}</label>
                        <div className="grid cols-3" style={{ gap: 12 }}>
                          {GPU_COUNTS.map((g) => {
                            // 단일 노드의 "빈(free)" GPU 수를 넘는 개수는 막는다 — 파드는 여러 노드에 걸칠 수 없고,
                            // 노드가 2장이어도 1장이 이미 대여중이면 2개는 스케줄 불가(영구 Pending). 전체 free 합이 아니라
                            // 노드별 free 의 최댓값(exPerNodeMaxFree)이 실제 상한이다.
                            const un = g.n > exPerNodeMaxFree;
                            return (
                              <SelBox key={g.n} on={gpuCount === g.n} disabled={un} onClick={() => setGpuCount(g.n)}>
                                <div style={{ fontWeight: 800, fontSize: 18, display: 'flex', alignItems: 'center', gap: 8 }}>{gpuCount === g.n && <CheckMark />}{g.n}</div>
                                <div className="muted" style={{ fontSize: 12.5, marginTop: 4 }}>{g.label}</div>
                                {un ? <Pill variant="err">{t('newSession.unavailable')}</Pill> : ppTag(`${c((exType?.fullPrice || 0) * g.n)} C/h`)}
                              </SelBox>
                            );
                          })}
                        </div>
                      </div>
                    )}
                  </>
                )}

                {wtype === 'shared' && (
                  <>
                    {/* 단계 선택 — 1) GPU 모델 먼저 고르고, 2) 그 GPU의 세부 오퍼링을 고른다.
                        타임슬라이싱·고급(커스텀 분할)은 제거됨: 공유는 무조건 오퍼링 기반. */}
                    <h3>{t('newSession.pickGpuShared')}</h3>
                    <div className="grid cols-2" style={{ gap: 14 }}>
                      {sharedGpuModels.map((m) => {
                        const un = m.vramFreeGb === 0;
                        return (
                          <SelBox key={m.gpuType} on={sharedGpu === m.gpuType} disabled={un} onClick={() => pickSharedGpu(m.gpuType)}>
                            <div style={{ fontWeight: 800, fontSize: 15.5, display: 'flex', alignItems: 'center', gap: 8 }}>
                              {sharedGpu === m.gpuType && <CheckMark />}<Cpu size={16} /> {m.gpuType}
                              {m.cuda && <span style={{ fontSize: 11, fontWeight: 700, padding: '1px 6px', borderRadius: 6, background: 'var(--surface-2)', border: '1px solid var(--border)' }}>CUDA {m.cuda}</span>}
                            </div>
                            <div className="muted" style={{ fontSize: 13, margin: '6px 0' }}>{t('newSession.offeringCountN', { n: m.count })}</div>
                            <div className="flex" style={{ justifyContent: 'space-between', alignItems: 'center', gap: 8 }}>
                              {un ? <Pill variant="err">{t('newSession.unavailable')}</Pill>
                                : <span className="muted" style={{ fontSize: 12 }}>{t('newSession.vramFreeN', { free: m.vramFreeGb, total: m.vramTotalGb })}</span>}
                              {un && <button className="btn sm warn" onClick={(e) => { e.stopPropagation(); notify(); }}>{t('newSession.notifyAvail')}</button>}
                            </div>
                          </SelBox>
                        );
                      })}
                    </div>
                    {sharedGpuModels.length === 0 && <div className="muted" style={{ padding: '12px 0' }}>{t('newSession.noSharedGpu')}</div>}

                    {/* 2단계 — 선택한 GPU의 세부 오퍼링(크기 라벨·CUDA·가격). */}
                    {sharedGpu && offeringsForGpu.length > 0 && (
                      <div style={{ marginTop: 22, paddingTop: 18, borderTop: '1px solid var(--border)' }}>
                        <div style={{ fontWeight: 800, fontSize: 14 }}>{t('newSession.pickOfferingFor', { gpu: sharedGpu })}</div>
                        <div className="legend mb">{t('newSession.pickOfferingHint')}</div>
                        {/* 세부 오퍼링은 리스트형 — 크기·스펙·CUDA·가격을 한 줄에 정렬해 비교가 쉽게. */}
                        <div className="grid" style={{ gap: 8 }}>
                          {offeringsForGpu.map((o) => {
                            const af = offeringFit(o, false), at = offeringFit(o, true);
                            const un = af === 0;
                            const tier = sizeTierOf(o.corePercent);
                            const on = offeringId === o.id;
                            const cuda = cudaOfType(o.gpuType);
                            return (
                              <div key={o.id} onClick={un ? undefined : () => pickShared(o)}
                                style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '11px 14px', borderRadius: 10,
                                  cursor: un ? 'not-allowed' : 'pointer', opacity: un ? 0.55 : 1,
                                  border: '1.5px solid ' + (on ? 'var(--primary)' : 'var(--border)'),
                                  background: on ? 'var(--primary-soft)' : 'var(--surface)' }}>
                                <span style={{ width: 18, height: 18, borderRadius: '50%', flex: '0 0 auto', display: 'grid', placeItems: 'center',
                                  border: '2px solid ' + (on ? 'var(--primary)' : 'var(--border)') }}>
                                  {on && <span style={{ width: 9, height: 9, borderRadius: '50%', background: 'var(--primary)' }} />}
                                </span>
                                <Pill variant={SIZE_VARIANT[tier]}>{t(`newSession.size_${tier}`)}</Pill>
                                <div style={{ flex: 1, minWidth: 0 }}>
                                  <div style={{ fontWeight: 700, fontSize: 14.5 }}>{o.name}</div>
                                  <div className="muted" style={{ fontSize: 12.5, marginTop: 1 }}>{t('newSession.offeringSpec', { gb: (o.vramMb / 1024).toFixed(0), pct: o.corePercent })}</div>
                                </div>
                                {cuda && <span style={{ fontSize: 11, fontWeight: 700, padding: '2px 7px', borderRadius: 6, background: 'var(--surface-2)', border: '1px solid var(--border)', whiteSpace: 'nowrap' }}>CUDA {cuda}</span>}
                                <div style={{ textAlign: 'right', flex: '0 0 auto', minWidth: 96 }}>
                                  {un ? <Pill variant="err">{t('newSession.unavailable')}</Pill> : ppTag(`${c(o.pricePerHour)} C/h`)}
                                  <div style={{ marginTop: 3 }}>
                                    {un
                                      ? <button className="btn sm warn" onClick={(e) => { e.stopPropagation(); notify(); }}>{t('newSession.notifyAvail')}</button>
                                      : <span className="muted" style={{ fontSize: 11.5 }}>{t('newSession.availableN', { free: af, total: at })}</span>}
                                  </div>
                                </div>
                              </div>
                            );
                          })}
                        </div>
                        <div className="legend" style={{ marginTop: 6 }}>{t('newSession.grpGuaranteedTech')}</div>
                      </div>
                    )}
                  </>
                )}

                <div className="cost-box mt">
                  <div className="row"><span>{t('newSession.resource')}</span><span>{resourceText}</span></div>
                  {availRow}
                  {billingRows}
                  {surgeLegend}
                </div>
              </div>
            )}

            {step === 2 && (
              <div>
                <h3>{t('newSession.pickImage')} <span className="muted">{t('newSession.connByImage')}</span></h3>
                <div className="grid cols-2" style={{ gap: 14 }}>
                  {imageChoices.map((im) => (
                    <SelBox key={im.id} on={imageId === im.id} onClick={() => setImageId(im.id)}>
                      <div style={{ fontWeight: 800, fontSize: 15, display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                        {imageId === im.id && <CheckMark />}{im.name}
                        {isFastImage(im) && (
                          <Pill variant="free" title={(im.cachedNodes || []).join(', ')}><Zap size={12} /> {t('newSession.fastStart')}</Pill>
                        )}
                      </div>
                      <div className="muted" style={{ fontSize: 13, margin: '6px 0' }}>{im.desc}</div>
                      <div className="mono" style={{ fontSize: 12 }}>{im.base}</div>
                      <div className="flex gap mt" style={{ flexWrap: 'wrap' }}>
                        {im.conns.map((c) => { const Icon = connIcon(c); return <Pill key={c} variant="primary"><Icon size={12} /> {c}</Pill>; })}
                      </div>
                    </SelBox>
                  ))}
                </div>
              </div>
            )}

            {step === 3 && (
              <div>
                <h3>{t('newSession.pickVolume')} <span className="muted">{t('newSession.volumeOptional')}</span></h3>
                <div className="legend mb">{t('newSession.homeModelHint')}</div>
                <VolumePicker t={t} vols={vols} selVols={selVols} toggleVol={toggleVol} setMount={setMount} localHomeNode={localHomeNode} setLocalHomeNode={setLocalHomeNode} />
              </div>
            )}

            {step === 4 && (
              <div>
                <h3>{t('newSession.reviewStart')}</h3>
                <label className="fld" style={{ marginTop: 0 }}>{t('newSession.sessionName')}</label>
                <input type="text" value={name} onChange={(e) => setName(e.target.value)} placeholder="my-train" />
                <SshKeyField t={t} registered={user?.sshPublicKey} value={sshKey} onChange={setSshKey} />

                {wtype !== 'cpu' && config.features.datasets && (
                  <DatasetNodePicker t={t} nodes={nodes} selGpuType={selGpuType} selNode={selNode} setSelNode={setSelNode} selDs={selDs} setSelDs={setSelDs} />
                )}

                <div className="cost-box mt">
                  <div className="row"><span>{t('newSession.billingMode')}</span><span>{isFree ? t('newSession.freeMode') : isDynamic ? t('newSession.firstCome') : t('newSession.bmCredit')}</span></div>
                  <div className="row"><span>{t('newSession.type')}</span><span>{wtype && typeOf(wtype).title}</span></div>
                  <div className="row"><span>{t('newSession.resource')}</span><span>{resourceText}</span></div>
                  <div className="row"><span>{t('newSession.image')}</span><span>{image.name}</span></div>
                  <div className="row"><span>{t('newSession.conn')}</span><span>{image.conns.join(' · ')}</span></div>
                  <div className="row"><span>{t('newSession.volumes')}</span><span>{selVols.length ? selVols.map((v) => v.name).join(', ') : t('newSession.noAttach')}</span></div>
                  {wtype !== 'cpu' && <div className="row"><span>{t('newSession.nodeLabel')}</span><span>{nodeText}{selDs.length ? ` · ${t('newSession.dsCountN', { n: selDs.length })}` : ''}</span></div>}
                  <div className="row"><span>{t('newSession.idleReclaim')}</span><span>{t('newSession.minN', { n: idle.timeoutMin })}</span></div>
                  {/* 보장 자원 — GPU 지분에 비례. request(바닥)이지 상한이 아니다. */}
                  {guarantee && (
                    <div className="row"><span>{t('newSession.guaranteed')}</span>
                      <span>{t('newSession.guaranteeCpu', { n: guarantee.cpu })} · {t('newSession.guaranteeMem', { n: guarantee.memGb })}</span></div>
                  )}
                  {availRow}
                  {billingRows}
                  {/* 비용 확인 — 잔액 대비 얼마나 돌릴 수 있는지 명시(크레딧 모드). */}
                  {costCheck}
                  {surgeLegend}
                </div>
              </div>
            )}
          </div>

          <div className="modal-foot" style={{ margin: '20px -20px -20px', borderRadius: '0 0 12px 12px' }}>
            {step > 0 ? <button className="btn" onClick={prev}>{t('newSession.prev')}</button> : <span />}
            {step < STEPS.length - 1
              ? <button className="btn primary" onClick={next} disabled={!canNext()}>{t('newSession.next')}</button>
              : unavailable
                ? <button className="btn warn" onClick={notify}>{t('newSession.notifyAvail')}</button>
                : <button className="btn primary" onClick={submit} disabled={creating}>{creating ? t('newSession.creating') : t('newSession.start')}</button>}
          </div>
        </div>
      </div>
      </>
      )}
    </div>
  );
}

// 볼륨 선택 — 컨테이너/SSH 공용. NFS 기반 볼륨을 마운트.
function VolumePicker({ t, vols, selVols, toggleVol, setMount, localHomeNode, setLocalHomeNode }) {
  const groups = [
    { label: t('newSession.volMine'), list: vols.owned },
    { label: t('newSession.volShared'), list: vols.shared },
  ];
  const localHomes = vols.localHomes || [];
  const anyVol = vols.owned.length + vols.shared.length + localHomes.length > 0;
  if (!anyVol) return <div className="muted" style={{ padding: '16px 0' }}>{t('newSession.noVolumes')}</div>;
  return (
    <>
      {localHomes.length > 0 && (
        <div className="mb">
          <div className="muted" style={{ fontSize: 12.5, fontWeight: 700, margin: '4px 0 4px' }}>{t('newSession.localHome')}</div>
          <div className="legend mb">{t('newSession.localHomeHint')}</div>
          <div className="grid" style={{ gap: 10 }}>
            {localHomes.map((lh) => {
              const on = localHomeNode === lh.node;
              return (
                <SelBox key={lh.node} on={on} onClick={() => setLocalHomeNode(on ? '' : lh.node)}>
                  <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
                    <div style={{ width: 40, height: 40, borderRadius: 10, flex: '0 0 auto', display: 'grid', placeItems: 'center', background: 'var(--surface-2)', color: 'var(--warn)' }}><HardDrive size={20} /></div>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ fontWeight: 700, fontSize: 14.5, display: 'flex', alignItems: 'center', gap: 8 }}>{on && <CheckMark />}{lh.name}
                        <Pill variant="warn">{t('newSession.nodePinned')}</Pill></div>
                      <div className="muted" style={{ fontSize: 12.5, marginTop: 2 }}>{lh.node} · /home/work</div>
                    </div>
                  </div>
                </SelBox>
              );
            })}
          </div>
        </div>
      )}
      {groups.map((g) => g.list.length > 0 && (
        <div key={g.label} className="mb">
          <div className="muted" style={{ fontSize: 12.5, fontWeight: 700, margin: '4px 0 10px' }}>{g.label}</div>
          <div className="grid" style={{ gap: 10 }}>
            {g.list.map((v) => {
              const sel = selVols.find((x) => x.id === v.id);
              const ro = v.perm === 'ro';
              return (
                <SelBox key={v.id} on={!!sel} onClick={() => toggleVol(v)}>
                  <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
                    <div style={{ width: 40, height: 40, borderRadius: 10, flex: '0 0 auto', display: 'grid', placeItems: 'center', background: 'var(--surface-2)', color: 'var(--primary)' }}><Database size={20} /></div>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ fontWeight: 700, fontSize: 14.5, display: 'flex', alignItems: 'center', gap: 8 }}>{sel && <CheckMark />}{v.name}
                        <Pill variant={ro ? 'pause' : 'free'}>{ro ? t('newSession.permRo') : t('newSession.permRw')}</Pill></div>
                      <div className="muted" style={{ fontSize: 12.5, marginTop: 2 }}>{v.usedGb}/{v.capGb} GB</div>
                    </div>
                  </div>
                  {sel && (
                    <div className="mt" onClick={(e) => e.stopPropagation()}>
                      <label className="fld" style={{ marginTop: 0, display: 'flex', alignItems: 'center', gap: 6 }}><FolderInput size={13} /> {t('newSession.mountPath')}</label>
                      <input type="text" value={sel.mountPath} onChange={(e) => setMount(v.id, e.target.value)} placeholder="/data/dataset" />
                    </div>
                  )}
                </SelBox>
              );
            })}
          </div>
        </div>
      ))}
    </>
  );
}

// 데이터셋 고급 선택 — 실제 노드 지정 + 노드별 캐시 데이터셋을 박스로 클릭 선택.
function DatasetNodePicker({ t, nodes, selGpuType, selNode, setSelNode, selDs, setSelDs }) {
  const [open, setOpen] = useState(false);
  const [openDs, setOpenDs] = useState({}); // 노드별 '저장된 데이터셋' 펼침
  const matched = nodes.filter((n) => !selGpuType || n.gpu.includes(selGpuType));
  const pickNode = (node) => { setSelNode(node); setSelDs([]); };
  const toggleDs = (node, name) => {
    if (selNode !== node) { setSelNode(node); setSelDs([name]); return; }
    setSelDs((cur) => (cur.includes(name) ? cur.filter((x) => x !== name) : [...cur, name]));
  };

  return (
    <div className="mt">
      <button className="btn" onClick={() => setOpen((o) => !o)}>
        <Database size={14} /> {t('newSession.dsAdv')} {open ? '▾' : '▸'}
      </button>
      {open && (
        <div className="mt">
          <div className="legend mb">{t('newSession.dsAdvHint')}</div>
          <div className="grid" style={{ gap: 10 }}>
            <div className={`selbox${selNode === 'auto' ? ' on' : ''}`} onClick={() => pickNode('auto')}
              style={{ padding: 12, borderRadius: 10, cursor: 'pointer',
                border: '2px solid ' + (selNode === 'auto' ? 'var(--primary)' : 'var(--border)'),
                background: selNode === 'auto' ? 'var(--primary-soft)' : 'var(--surface)' }}>
              <span style={{ fontWeight: 700, display: 'flex', alignItems: 'center', gap: 8 }}>{selNode === 'auto' && <CheckMark />}{t('newSession.nodeAuto')}</span>
              <div className="muted" style={{ fontSize: 12.5, marginTop: 2 }}>{t('newSession.nodeAutoHint')}</div>
            </div>
            {matched.map((n) => {
              const on = selNode === n.node;
              return (
                <div key={n.node} className={`selbox${on ? ' on' : ''}`}
                  style={{ padding: 12, borderRadius: 10,
                    border: '2px solid ' + (on ? 'var(--primary)' : 'var(--border)'),
                    background: on ? 'var(--primary-soft)' : 'var(--surface)' }}>
                  <div className="flex" style={{ justifyContent: 'space-between', cursor: 'pointer' }} onClick={() => pickNode(n.node)}>
                    <span className="flex" style={{ gap: 8, fontWeight: 700 }}><Server size={15} color="var(--primary)" />{on && <CheckMark />}{n.node}</span>
                    <span className="muted" style={{ fontSize: 12.5 }}>{n.gpu}</span>
                  </div>
                  {n.cached.length > 0 && (() => {
                    const selN = on ? selDs.length : 0;
                    const show = openDs[n.node] || selN > 0;
                    return (
                      <div style={{ marginTop: 8 }}>
                        <button type="button" className="btn sm"
                          onClick={(e) => { e.stopPropagation(); setOpenDs((s) => ({ ...s, [n.node]: !show })); }}>
                          <Database size={13} /> {t('newSession.dsView')} ({n.cached.length}){selN > 0 ? ` · ${t('newSession.dsCountN', { n: selN })}` : ''} {show ? '▾' : '▸'}
                        </button>
                        {show && (
                          <div className="grid" style={{ gap: 8, marginTop: 8 }}>
                            {n.cached.map((d) => (
                              <DatasetRow key={d.name} t={t} d={d} selectable on={on && selDs.includes(d.name)}
                                onClick={() => toggleDs(n.node, d.name)} />
                            ))}
                          </div>
                        )}
                      </div>
                    );
                  })()}
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}

// SSH 물리노드 위저드 (하이브리드 전용) — 컨테이너와 동일한 좌측 스텝 + 우측 패널 레이아웃.
function SSHWizard({ t, navigate, toast, nodes, vols, labels }) {
  const { user, refreshUser } = useAuth();
  const { config } = useSystemConfig();
  const [step, setStep] = useState(0);
  const [maxReached, setMaxReached] = useState(0);
  const [creating, setCreating] = useState(false); // 생성 중(연타 중복 방지)
  const [sel, setSel] = useState(null);
  const [detail, setDetail] = useState(null);
  const [selVols, setSelVols] = useState([]);
  const [name, setName] = useState('');
  const [sshKey, setSshKey] = useState(user?.sshPublicKey || '');

  useEffect(() => { if (nodes.length && !sel) setSel(nodes[0].node); }, [nodes, sel]);
  const node = nodes.find((n) => n.node === sel);
  const unavailable = node ? !node.available : false;
  const toggleVol = (v) => setSelVols((cur) => cur.some((x) => x.id === v.id)
    ? cur.filter((x) => x.id !== v.id)
    : [...cur, { id: v.id, name: v.name, mountPath: `/data/${v.name}`, perm: v.perm }]);
  const setMount = (id, p) => setSelVols((cur) => cur.map((x) => (x.id === id ? { ...x, mountPath: p } : x)));

  const STEPS = [t('newSession.sshStepNode'), t('newSession.stepVol'), t('newSession.sshStepReview')];
  const goto = (s) => { if (s <= maxReached) setStep(s); };
  const next = () => { const s = Math.min(STEPS.length - 1, step + 1); setStep(s); setMaxReached((m) => Math.max(m, s)); };
  const prev = () => setStep((s) => Math.max(0, s - 1));
  const notify = () => toast(t('newSession.notifyRequested'));

  const start = async () => {
    if (creating) return; // 연타 중복 생성 방지
    setCreating(true);
    await saveSshKey(sshKey, user?.sshPublicKey, refreshUser); // 계정에도 등록(노드 authorized_keys 주입 근거)
    try {
      await createSession({ env: 'ssh', node: sel, instancename: name || 'ssh-session', sshpublickey: sshKey, volumes: selVols.map((v) => ({ id: v.id, mountPath: v.mountPath })) });
    } catch (e) {
      toast(createErrMsg(e, t));
      setCreating(false);
      return;
    }
    toast(t('newSession.sshStarted'));
    navigate('/console/sessions');
  };

  const summary = [sel || null, selVols.length ? t('newSession.attachedN', { n: selVols.length }) : t('newSession.noAttach'), name || null];

  return (
    <div style={{ display: 'grid', gridTemplateColumns: '300px 1fr', gap: 20, alignItems: 'start' }}>
      <StepSidebar steps={STEPS} step={step} maxReached={maxReached} summary={summary} onGoto={goto} labels={labels} />

      <div className="card" style={{ minHeight: 380, display: 'flex', flexDirection: 'column' }}>
        <div style={{ flex: 1 }}>
          {step === 0 && (
            <div>
              <h3><TerminalSquare size={16} /> {t('newSession.sshTitle')}</h3>
              <div className="legend mb">{t('newSession.sshDesc')}</div>
              <div className="grid" style={{ gap: 12 }}>
                {nodes.map((n) => {
                  const on = sel === n.node;
                  const dsTotal = n.cached.reduce((a, d) => a + d.sizeGb, 0);
                  return (
                    <div key={n.node} className={`selbox${on ? ' on' : ''}`} onClick={() => setSel(n.node)}
                      style={{ padding: 14, borderRadius: 12, cursor: 'pointer',
                        border: '2px solid ' + (on ? 'var(--primary)' : 'var(--border)'),
                        background: on ? 'var(--primary-soft)' : 'var(--surface)' }}>
                      <div className="flex" style={{ justifyContent: 'space-between' }}>
                        <span style={{ fontWeight: 800, fontSize: 15, display: 'flex', alignItems: 'center', gap: 8 }}>{on && <CheckMark />}{n.node}</span>
                        <span className="flex" style={{ gap: 8 }}>
                          <Pill variant={n.available ? 'ok' : 'wait'}>{n.available ? t('newSession.available1') : t('newSession.unavailable')}</Pill>
                          <span className="muted" style={{ fontSize: 12.5 }}>home {n.homeUsedGb} GB</span>
                        </span>
                      </div>
                      <div className="muted" style={{ fontSize: 12.5, margin: '6px 0' }}>{n.gpu} · {n.cpu} · {n.mem} · {n.disk}</div>
                      {config.features.datasets && (
                        <>
                          <div className="flex" style={{ gap: 8, flexWrap: 'wrap' }}>
                            <Pill variant="gpu">{t('newSession.sshCachedN', { n: n.cached.length, gb: dsTotal })}</Pill>
                            <button className="btn sm" onClick={(e) => { e.stopPropagation(); setDetail(detail === n.node ? null : n.node); }}>{t('newSession.sshCacheDetail')}</button>
                          </div>
                          {detail === n.node && (
                            <div className="mt grid" onClick={(e) => e.stopPropagation()} style={{ gap: 8, borderTop: '1px solid var(--border)', paddingTop: 10 }}>
                              {n.cached.length === 0 && <div className="muted" style={{ fontSize: 12.5 }}>{t('newSession.sshNoCache')}</div>}
                              {n.cached.map((d) => <DatasetRow key={d.name} t={t} d={d} />)}
                            </div>
                          )}
                        </>
                      )}
                    </div>
                  );
                })}
              </div>
            </div>
          )}

          {step === 1 && (
            <div>
              <h3>{t('newSession.pickVolume')} <span className="muted">{t('newSession.volumeOptional')}</span></h3>
              <VolumePicker t={t} vols={vols} selVols={selVols} toggleVol={toggleVol} setMount={setMount} />
            </div>
          )}

          {step === 2 && (
            <div>
              <h3>{t('newSession.reviewStart')}</h3>
              <label className="fld" style={{ marginTop: 0 }}>{t('newSession.sessionName')}</label>
              <input type="text" value={name} onChange={(e) => setName(e.target.value)} placeholder="ssh-session" />
              <SshKeyField t={t} registered={user?.sshPublicKey} value={sshKey} onChange={setSshKey} />
              <div className="cost-box mt">
                <div className="row"><span>{t('newSession.sshNodeLabel')}</span><span>{sel || '—'}</span></div>
                <div className="row"><span>{t('newSession.sshHome')}</span><span>{node ? `${node.homeUsedGb} GB` : '—'}</span></div>
                <div className="row"><span>{t('newSession.volumes')}</span><span>{selVols.length ? selVols.map((v) => v.name).join(', ') : t('newSession.noAttach')}</span></div>
                <div className="legend">{t('newSession.sshHomeHint')}</div>
              </div>
            </div>
          )}
        </div>

        <div className="modal-foot" style={{ margin: '20px -20px -20px', borderRadius: '0 0 12px 12px' }}>
          {step > 0 ? <button className="btn" onClick={prev}>{t('newSession.prev')}</button> : <span />}
          {step < STEPS.length - 1
            ? (unavailable
              ? <button className="btn warn" onClick={notify}>{t('newSession.notifyAvail')}</button>
              : <button className="btn primary" onClick={next} disabled={!sel}>{t('newSession.next')}</button>)
            : <button className="btn primary" onClick={start} disabled={!sel || creating}>{creating ? t('newSession.creating') : t('newSession.start')}</button>}
        </div>
      </div>
    </div>
  );
}
