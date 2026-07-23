import React from 'react';
import { useTranslation } from 'react-i18next';
import { Boxes, TerminalSquare, Check, Wallet, Zap } from 'lucide-react';
import Toggle from '../console/Toggle';
import Select from '../console/Select';

// 위저드(/setup)와 관리자 Settings 가 공유하는 정책 폼 섹션.
// 각 섹션은 { config, update } 를 받아 해당 슬라이스를 읽고 중첩 patch 로 갱신한다.
// 문구는 모두 common.setup.* 네임스페이스를 사용한다(두 화면 공용).

/* ---------------- 공용 필드 헬퍼 ---------------- */
function Field({ label, hint, children, w }) {
  return (
    <div style={{ width: w || '100%', maxWidth: '100%' }}>
      <label className="fld" style={{ marginTop: 0 }}>{label}</label>
      {children}
      {hint && <div className="legend">{hint}</div>}
    </div>
  );
}

function NumInput({ value, onChange, min = 0, max, w = 120, suffix }) {
  return (
    <span className="flex" style={{ gap: 6 }}>
      <input type="number" value={value} min={min} max={max} style={{ width: w }}
        onChange={(e) => onChange(Number(e.target.value))} />
      {suffix && <span style={{ color: 'var(--muted)', fontSize: 13, fontWeight: 600 }}>{suffix}</span>}
    </span>
  );
}

function SwitchRow({ label, hint, checked, onChange }) {
  return (
    <div className="flex" style={{ justifyContent: 'space-between', alignItems: 'flex-start', gap: 14, padding: '12px 0' }}>
      <div style={{ minWidth: 0 }}>
        <div style={{ fontWeight: 700, fontSize: 13.5 }}>{label}</div>
        {hint && <div className="legend" style={{ marginTop: 2 }}>{hint}</div>}
      </div>
      <Toggle checked={checked} onChange={onChange} />
    </div>
  );
}

function SelCard({ on, onClick, icon: Icon, title, desc }) {
  return (
    <div className={`selbox${on ? ' on' : ''}`} onClick={onClick}
      style={{ flex: 1, minWidth: 220, padding: 16, borderRadius: 12, cursor: 'pointer',
        border: '2px solid ' + (on ? 'var(--primary)' : 'var(--border)'),
        background: on ? 'var(--primary-soft)' : 'var(--surface)' }}>
      <div className="flex" style={{ justifyContent: 'space-between' }}>
        <span className="flex" style={{ gap: 8, fontWeight: 800, fontSize: 15 }}>
          <Icon size={18} color="var(--primary)" /> {title}
        </span>
        {on && <Check size={16} color="var(--primary)" />}
      </div>
      <div className="legend" style={{ marginTop: 8, lineHeight: 1.5 }}>{desc}</div>
    </div>
  );
}

const Divider = () => <div style={{ height: 1, background: 'var(--border)', margin: '14px 0' }} />;
const SubHead = ({ children }) => (
  <div style={{ fontWeight: 800, fontSize: 13, color: 'var(--muted)', textTransform: 'uppercase', letterSpacing: '.04em', marginBottom: 4 }}>{children}</div>
);

/* ---------------- 1. 배포 모드 ---------------- */
export function ModeSection({ config, update }) {
  const { t } = useTranslation('common');
  const mode = config.deploymentMode;
  return (
    <div>
      <div className="flex" style={{ gap: 12, alignItems: 'stretch', flexWrap: 'wrap' }}>
        <SelCard on={mode === 'container'} onClick={() => update({ deploymentMode: 'container' })}
          icon={Boxes} title={t('setup.mode.container')} desc={t('setup.mode.containerDesc')} />
        <SelCard on={mode === 'hybrid'} onClick={() => update({ deploymentMode: 'hybrid' })}
          icon={TerminalSquare} title={t('setup.mode.hybrid')} desc={t('setup.mode.hybridDesc')} />
      </div>
      {mode === 'hybrid' && (
        <div className="legend" style={{ marginTop: 12, padding: '10px 12px', background: 'var(--warn-soft)', color: 'var(--warn)', borderRadius: 9, fontWeight: 600 }}>
          {t('setup.mode.hybridNote')}
        </div>
      )}
    </div>
  );
}

/* ---------------- 2. 과금 모델 (크레딧 ↔ Dynamic 상호 배타) ----------------
   hideModeSelect: 모드 선택 카드를 숨기고 현재 모드의 세부 정책만 렌더(설치 후 Settings 에서 사용).
   모드 전환은 설치 시점(/setup)에서만 가능하고, 세부 정책은 운영 중에도 변경 가능. */
export function BillingSection({ config, update, hideModeSelect }) {
  const { t } = useTranslation('common');
  const { mode, credit, dynamic } = config.billing;
  const setMode = (m) => update({ billing: { mode: m } });
  const setCredit = (patch) => update({ billing: { credit: patch } });
  const setDynamic = (patch) => update({ billing: { dynamic: patch } });
  return (
    <div>
      {!hideModeSelect && (
        <>
          <div className="flex" style={{ gap: 12, alignItems: 'stretch', flexWrap: 'wrap' }}>
            <SelCard on={mode === 'credit'} onClick={() => setMode('credit')}
              icon={Wallet} title={t('setup.billing.modeCredit')} desc={t('setup.billing.modeCreditDesc')} />
            <SelCard on={mode === 'dynamic'} onClick={() => setMode('dynamic')}
              icon={Zap} title={t('setup.billing.modeDynamic')} desc={t('setup.billing.modeDynamicDesc')} />
          </div>
          <Divider />
        </>
      )}

      {mode === 'credit' ? (
        <div>
          <div className="flex" style={{ gap: 18, flexWrap: 'wrap', alignItems: 'flex-start' }}>
            <Field label={t('setup.billing.pricing')} hint={t('setup.billing.pricingHint')} w={260}>
              <Select value={credit.pricing} onChange={(v) => setCredit({ pricing: v })}
                options={[
                  { value: 'static', label: t('setup.billing.pricingStatic') },
                  { value: 'dynamic', label: t('setup.billing.pricingDynamic') },
                ]} />
            </Field>
            <Field label={t('setup.billing.maxConcurrent')} w={160}>
              <NumInput value={credit.maxConcurrentSessions} min={1} onChange={(v) => setCredit({ maxConcurrentSessions: v })} suffix={t('setup.unit.sessions')} />
            </Field>
          </div>
          {credit.pricing === 'dynamic' && (
            <div style={{ marginTop: 14 }}>
              <Field label={t('setup.billing.surge')} hint={t('setup.billing.surgeHint')}>
                <NumInput value={credit.surgeIncrement} min={0} onChange={(v) => setCredit({ surgeIncrement: v })} suffix={t('setup.unit.creditPerStep')} />
              </Field>
            </div>
          )}
        </div>
      ) : (
        <div className="flex" style={{ gap: 18, flexWrap: 'wrap' }}>
          <Field label={t('setup.billing.maxLease')} w={160}>
            <NumInput value={dynamic.maxLeaseHours} min={1} onChange={(v) => setDynamic({ maxLeaseHours: v })} suffix={t('setup.unit.hour')} />
          </Field>
          <Field label={t('setup.billing.cooldown')} w={160}>
            <NumInput value={dynamic.cooldownHours} min={0} onChange={(v) => setDynamic({ cooldownHours: v })} suffix={t('setup.unit.hour')} />
          </Field>
          <Field label={t('setup.billing.maxConcurrent')} w={160}>
            <NumInput value={dynamic.maxConcurrentSessions} min={1} onChange={(v) => setDynamic({ maxConcurrentSessions: v })} suffix={t('setup.unit.sessions')} />
          </Field>
        </div>
      )}
    </div>
  );
}

/* ---------------- 4. 사용자 기능 on/off ----------------
   omit: 표시에서 제외할 키(설치시 고정 항목은 Settings 런타임 영역에서 제외).
   datasets 기능 자체는 인프라(NFS) 의존이라 설치시 고정 → 런타임에선 omit=['datasets']. */
export function FeaturesSection({ config, update, omit = [] }) {
  const { t } = useTranslation('common');
  const f = config.features;
  const set = (patch) => update({ features: patch });
  const rows = [
    { key: 'signupRequest', label: t('setup.features.signup'), hint: t('setup.features.signupHint') },
    { key: 'datasets', label: t('setup.features.datasetFeature'), hint: t('setup.features.datasetFeatureHint') },
    // 데이터셋 등록은 데이터셋 기능이 켜진 경우에만 의미.
    ...(f.datasets ? [{ key: 'datasetRegister', label: t('setup.features.dataset'), hint: t('setup.features.datasetHint') }] : []),
    { key: 'workloadAlerts', label: t('setup.features.alerts'), hint: t('setup.features.alertsHint') },
    { key: 'groupJoinRequest', label: t('setup.features.groupJoin'), hint: t('setup.features.groupJoinHint') },
    // 크레딧 충전 요청은 크레딧 과금 모드에서만 의미 있음.
    ...(config.billing.mode === 'credit'
      ? [{ key: 'creditRequest', label: t('setup.features.credit'), hint: t('setup.features.creditHint') }] : []),
  ].filter((r) => !omit.includes(r.key));
  return (
    <div>
      {rows.map((r) => (
        <SwitchRow key={r.key} label={r.label} hint={r.hint} checked={!!f[r.key]} onChange={(v) => set({ [r.key]: v })} />
      ))}
    </div>
  );
}

/* ---------------- 4. 물리 노드 임대 연장 (하이브리드 전용) ---------------- */
export function LeaseSection({ config, update }) {
  const { t } = useTranslation('common');
  const l = config.lease || { extensionHours: 2, maxExtensions: 2 };
  return (
    <div>
      <Field label={t('setup.lease.extHours')} hint={t('setup.lease.extHoursHint')} w={240}>
        <NumInput value={l.extensionHours} min={1} w={140} onChange={(v) => update({ lease: { extensionHours: v } })} suffix={t('setup.unit.hour')} />
      </Field>
      <Field label={t('setup.lease.maxExt')} hint={t('setup.lease.maxExtHint')} w={240}>
        <NumInput value={l.maxExtensions} min={0} w={140} onChange={(v) => update({ lease: { maxExtensions: v } })} suffix={t('setup.unit.times')} />
      </Field>
    </div>
  );
}

/* ---------------- 3. 유휴 핸들링 ---------------- */
export function IdleSection({ config, update }) {
  const { t } = useTranslation('common');
  return (
    <div>
      <Field label={t('setup.idle.timeout')} hint={t('setup.idle.timeoutHint')} w={220}>
        <NumInput value={config.idle.timeoutMin} min={1} w={140} onChange={(v) => update({ idle: { timeoutMin: v } })} suffix={t('setup.unit.min')} />
      </Field>
      <div className="legend" style={{ marginTop: 14, lineHeight: 1.7 }}>
        • {t('setup.idle.containerNote')}<br />
        • {t('setup.idle.sshNote')}
      </div>
    </div>
  );
}

