import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { SlidersHorizontal, Lock, Boxes, TerminalSquare, Wallet, Zap, Sparkles, RefreshCcw } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import Pill from '../../../components/console/Pill';
import Toggle from '../../../components/console/Toggle';
import BrandLogo, { BRAND_ICONS, LogoMark } from '../../../components/BrandLogo';
import { useToast } from '../../../components/console/Toast';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { IdleSection, FeaturesSection } from '../../../components/setup/sections';
import { putSystemConfig } from '../../../api/console/systemconfig';

// 크레딧 정기 재충전 설정(크레딧 모드). 관리자가 주기/양/모드를 정하면 백엔드 잡이 리필한다.
function RechargeCard({ config, refresh, toast, t }) {
  const rc = config.billing?.credit?.recharge || {};
  const [d, setD] = useState({ enabled: !!rc.enabled, intervalDays: rc.intervalDays ?? 30, carryover: !!rc.carryover });
  useEffect(() => { setD({ enabled: !!rc.enabled, intervalDays: rc.intervalDays ?? 30, carryover: !!rc.carryover }); /* eslint-disable-next-line */ }, [rc.enabled, rc.intervalDays, rc.carryover]);
  const save = async (patch) => {
    const next = { ...d, ...patch }; setD(next);
    try { await putSystemConfig({ recharge: next }); await refresh(); toast(t('settings.rechargeSaved', { defaultValue: '재충전 설정을 저장했습니다.' })); }
    catch { toast(t('settings.saveFailed')); }
  };
  return (
    <div className="card mb">
      <h3><RefreshCcw size={15} /> {t('settings.rechargeTitle', { defaultValue: '정기 크레딧 리필(기본/상한)' })}</h3>
      <div className="legend mb">{t('settings.rechargeNote', { defaultValue: '플랫폼 기본 리필 주기와 이월 정책입니다. 조직·팀·개인은 이보다 짧은 주기를 정할 수 있지만 더 길게는 못 하며, 상위가 이월불가면 그 경계에서 하위 이월분도 소멸합니다.' })}</div>
      <div className="flex" style={{ justifyContent: 'space-between', alignItems: 'center', padding: '8px 0' }}>
        <div style={{ fontWeight: 700, fontSize: 13.5 }}>{t('settings.rechargeEnabled', { defaultValue: '정기 리필 사용' })}</div>
        <Toggle checked={d.enabled} onChange={(v) => save({ enabled: v })} />
      </div>
      {d.enabled && (
        <div className="grid cols-2" style={{ gap: 14, marginTop: 8, alignItems: 'end' }}>
          <div><label className="fld" style={{ marginTop: 0 }}>{t('settings.rechargeInterval', { defaultValue: '기본 리필 주기(일) · 하위 상한' })}</label>
            <input type="number" min={1} value={d.intervalDays} onChange={(e) => setD({ ...d, intervalDays: Number(e.target.value) })} onBlur={() => save({})} /></div>
          <div><label className="fld" style={{ marginTop: 0 }}>{t('settings.rechargeCarryover', { defaultValue: '이월 허용' })}</label>
            <div className="flex" style={{ gap: 8, alignItems: 'center', height: 38 }}>
              <Toggle checked={d.carryover} onChange={(v) => save({ carryover: v })} />
              <span style={{ fontSize: 12.5 }}>{d.carryover ? t('settings.carryoverOn', { defaultValue: '미사용분 이월' }) : t('settings.carryoverOff', { defaultValue: '주기마다 소멸' })}</span>
            </div>
          </div>
        </div>
      )}
      <div className="legend" style={{ marginTop: 10 }}>{t('settings.rechargeAmountHint', { defaultValue: '조직·팀·개인별 리필 양/주기/이월은 각 상세 화면에서 설정합니다(주기는 상위보다 길 수 없음).' })}</div>
    </div>
  );
}

// 관리자 시스템 설정.
//  - 운영 모드 / 과금 모델: 설치 시점(첫 실행 마법사)에만 결정 → 여기선 읽기 전용 + 재설치 안내.
//  - 유휴 등 운영 정책: 운영 중 조정 가능.
const SUBHEAD = { fontWeight: 800, fontSize: 12.5, color: 'var(--muted)', textTransform: 'uppercase', letterSpacing: '.04em', margin: '4px 0 10px' };

function LockedItem({ icon: Icon, title, value, sub }) {
  return (
    <div style={{ display: 'flex', gap: 12, alignItems: 'flex-start', padding: 14, borderRadius: 12, border: '1px solid var(--border)', background: 'var(--surface-2)' }}>
      <span style={{ width: 38, height: 38, borderRadius: 10, flex: '0 0 auto', display: 'grid', placeItems: 'center', background: 'var(--primary-soft)', color: 'var(--primary)' }}>
        <Icon size={19} />
      </span>
      <div style={{ minWidth: 0 }}>
        <div className="muted" style={{ fontSize: 11.5, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '.03em' }}>{title}</div>
        <div style={{ fontWeight: 800, fontSize: 15, marginTop: 2 }}>{value}</div>
        {sub && <div className="muted" style={{ fontSize: 12.5, marginTop: 2 }}>{sub}</div>}
      </div>
    </div>
  );
}

export default function Settings() {
  const { t } = useTranslation('consoleAdmin');
  const { t: tc } = useTranslation('common');
  const { config, update, refresh } = useSystemConfig();
  const { toast } = useToast();

  const hybrid = config.deploymentMode === 'hybrid';
  const creditMode = config.billing.mode === 'credit';
  const freeMode = config.billing.mode === 'free';

  const modeValue = hybrid ? tc('setup.mode.hybrid') : tc('setup.mode.container');
  const billingValue = freeMode ? tc('setup.billing.modeFree') : creditMode ? tc('setup.billing.modeCredit') : tc('setup.billing.modeDynamic');

  // 설치시 고정(읽기전용) 요약 — 과금모델/동시세션/데이터셋/임대 등 스키마·인프라 영향 항목.
  const pricingValue = creditMode
    ? (config.billing.credit.pricing === 'dynamic'
        ? tc('setup.billing.pricingDynamic') + ` (+${config.billing.credit.surgeIncrement})`
        : tc('setup.billing.pricingStatic'))
    : '—';
  const concurrency = freeMode ? '∞' : creditMode ? config.billing.credit.maxConcurrentSessions : config.billing.dynamic.maxConcurrentSessions;

  // 운영 중 조정 항목만 백엔드에 영속(유휴 타임아웃 + 단순 기능 토글). 무거운 항목은 무시됨.
  const save = async () => {
    try {
      await putSystemConfig({
        idle: { timeoutMin: config.idle.timeoutMin },
        reclaim: config.reclaim, // 중단 세션 홈 회수(방치 일수 · 디스크 임계)
        features: config.features,
      });
      await refresh();
      toast(t('settings.saved'));
    } catch {
      toast(t('settings.saveFailed'));
    }
  };

  return (
    <div>
      <PageHead icon={SlidersHorizontal} title={t('settings.title')} subtitle={t('settings.subtitle')}
        actions={<Pill variant={hybrid ? 'primary' : 'gpu'} dot>{t(`settings.mode.${config.deploymentMode}`)}</Pill>} />

      {/* 브랜딩(좌) · 설치 시 고정(우) — 1:1 */}
      <div className="grid cols-2 mb" style={{ gap: 16, alignItems: 'stretch' }}>
        {/* 브랜딩 — 운영 중 변경 가능 */}
        <div className="card" style={{ margin: 0 }}>
          <h3><Sparkles size={15} /> {t('settings.brandingTitle')}</h3>
          <div className="legend mb">{t('settings.brandingNote')}</div>
          <label className="fld" style={{ marginTop: 0 }}>{t('settings.brandName')}</label>
          <input type="text" value={config.branding?.name || ''} placeholder="Giosk"
            onChange={(e) => update({ branding: { name: e.target.value } })} />
          <label className="fld">{t('settings.brandSubtitle', { defaultValue: '부제(서브타이틀)' })}</label>
          <input type="text" value={config.branding?.subtitle ?? 'Console'} placeholder="Console"
            onChange={(e) => update({ branding: { subtitle: e.target.value } })} />
          <div className="legend" style={{ marginTop: 4 }}>{t('settings.brandSubtitleHint', { defaultValue: '로고 옆 작은 글씨. 비우면 표시하지 않습니다.' })}</div>
          <label className="fld">{t('settings.brandIcon', { defaultValue: '아이콘' })}</label>
          <div className="flex gap wrap">
            {BRAND_ICONS.map(({ key, Icon }) => {
              const on = (config.branding?.icon || '') === key;
              return (
                <button key={key || 'default'} type="button" onClick={() => update({ branding: { icon: key } })} title={key || 'default'}
                  style={{ width: 40, height: 40, borderRadius: 10, display: 'grid', placeItems: 'center', cursor: 'pointer',
                    border: '2px solid ' + (on ? 'var(--primary)' : 'var(--border)'),
                    background: on ? 'var(--primary-soft)' : 'var(--surface)', color: on ? 'var(--primary)' : 'var(--text)' }}>
                  {Icon ? <Icon size={18} /> : <LogoMark size={20} />}
                </button>
              );
            })}
          </div>
          <label className="fld">{t('settings.brandIconUpload', { defaultValue: '아이콘 업로드 (선택)' })}</label>
          <div className="flex" style={{ gap: 10, alignItems: 'center' }}>
            {config.branding?.iconUrl && <img src={config.branding.iconUrl} alt="icon" style={{ width: 40, height: 40, borderRadius: 10, objectFit: 'cover', border: '1px solid var(--border)' }} />}
            <input type="file" accept="image/png,image/jpeg,image/svg+xml,image/webp" onChange={(e) => {
              const f = e.target.files?.[0]; if (!f) return;
              if (f.size > 120 * 1024) { toast(t('settings.brandIconTooBig', { defaultValue: '아이콘은 120KB 이하여야 합니다.' })); return; }
              const r = new FileReader(); r.onload = () => update({ branding: { iconUrl: String(r.result) } }); r.readAsDataURL(f);
            }} />
            {config.branding?.iconUrl && <button className="btn sm" onClick={() => update({ branding: { iconUrl: '' } })}>{t('settings.brandIconClear', { defaultValue: '제거' })}</button>}
          </div>
          <div className="legend" style={{ marginTop: 4 }}>{t('settings.brandIconUploadHint', { defaultValue: '업로드하면 위 Lucide 아이콘보다 우선 적용됩니다(120KB 이하 PNG/SVG 권장).' })}</div>
          <label className="fld">{t('settings.brandAccent')}</label>
          <div className="flex" style={{ gap: 10, alignItems: 'center' }}>
            <input type="color" value={config.branding?.accent || '#2563eb'} style={{ width: 46, height: 34, padding: 2, borderRadius: 8, cursor: 'pointer' }}
              onChange={(e) => update({ branding: { accent: e.target.value } })} />
            <button className="btn sm" onClick={() => update({ branding: { accent: '' } })}>{t('settings.brandAccentReset')}</button>
          </div>
          <div className="legend" style={{ marginTop: 6 }}>{t('settings.brandAccentHint')}</div>
          <div className="legend" style={{ marginTop: 16, marginBottom: 6 }}>{t('settings.brandPreview')}</div>
          <div className="logo" style={{ border: '1px solid var(--border)', borderRadius: 12, padding: '16px 20px', background: 'var(--surface-2)' }}>
            <BrandLogo markSize={30} badge={t('badge')} />
          </div>
        </div>

        {/* 설치 시 고정 — 읽기 전용 */}
        <div className="card" style={{ margin: 0, display: 'flex', flexDirection: 'column' }}>
          <h3><Lock size={15} /> {t('settings.lockedTitle')}</h3>
          <div className="legend mb">{t('settings.lockedNote')}</div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            <LockedItem icon={hybrid ? TerminalSquare : Boxes} title={t('settings.tab.mode')} value={modeValue} />
            <LockedItem icon={freeMode ? Sparkles : creditMode ? Wallet : Zap} title={t('settings.tab.billing')} value={billingValue} sub={`${t('settings.lockPricing')}: ${pricingValue}`} />
            <LockedItem icon={Boxes} title={t('settings.lockDatasets')} value={config.features?.datasets ? t('settings.on') : t('settings.off')} sub={t('settings.lockDatasetsSub')} />
          </div>
          <div className="legend" style={{ marginTop: 'auto', paddingTop: 16, textAlign: 'right' }}>{t('settings.lockedHelmNote')}</div>
        </div>
      </div>

      {/* 크레딧 정기 재충전 — 크레딧 모드 전용. 주기마다 사용자 잔액을 리필. */}
      {creditMode && <RechargeCard config={config} refresh={refresh} toast={toast} t={t} />}

      {/* 운영 중 조정 가능 — 단순 정책만(유휴 타임아웃 + 기능 토글). 무거운 항목은 위 '설치시 고정'에. */}
      <div className="card">
        <h3><SlidersHorizontal size={15} /> {t('settings.runtimeTitle')}</h3>
        <div className="legend mb">{t('settings.runtimeNote')}</div>

        <div style={SUBHEAD}>{t('settings.tab.idle')}</div>
        <IdleSection config={config} update={update} />

        <div style={{ height: 1, background: 'var(--border)', margin: '18px 0' }} />

        <div style={SUBHEAD}>{t('settings.featuresHead')}</div>
        <FeaturesSection config={config} update={update} omit={['datasets']} />

        <div className="flex" style={{ justifyContent: 'flex-end', marginTop: 18, paddingTop: 16, borderTop: '1px solid var(--border)' }}>
          <button className="btn primary" onClick={save}>{t('settings.save')}</button>
        </div>
      </div>
    </div>
  );
}
