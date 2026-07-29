import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { notifyText } from '../../../utils/notify';
import { clickable } from '../../../utils/a11y';
import { BellRing, Plus, Trash2, Mail, Webhook, BellPlus, Cpu, Server } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import Pill from '../../../components/console/Pill';
import Select from '../../../components/console/Select';
import Toggle from '../../../components/console/Toggle';
import Modal from '../../../components/console/Modal';
import DataTable from '../../../components/console/DataTable';
import { useAuth } from '../../../context/AuthContext';
import { useToast } from '../../../components/console/Toast';
import { useConfirm } from '../../../components/console/Confirm';
import { useSystemConfig } from '../../../context/SystemConfigContext';
import { getAlerts, createAlert, deleteAlert, toggleAlert } from '../../../api/console/alerts';
import { getSshNodes } from '../../../api/console/sshNodes';
import { getUserNotify, saveUserNotify, getInbox, markInboxRead, markInboxAllRead } from '../../../api/console/notify';
import { getAvailability, getOfferings } from '../../../api/console/resources';
import { getMySessions } from '../../../api/console/sessions';

export default function NotificationCenter() {
  const { t } = useTranslation('consoleUser');
  const { user } = useAuth();
  const { toast } = useToast();
  const confirm = useConfirm();
  const { config } = useSystemConfig();
  const hybrid = config.deploymentMode === 'hybrid';
  // Dynamic 과금 모드에선 크레딧/예산 지표가 없으므로 알림 규칙에서도 제외.
  const creditMode = config.billing.mode === 'credit';
  const CREDIT_METRICS = ['credit_balance', 'budget_pct'];
  const [rules, setRules] = useState([]);
  const [emails, setEmails] = useState([]);
  const [webhooks, setWebhooks] = useState([]);
  const [newEmail, setNewEmail] = useState('');
  const [newWebhook, setNewWebhook] = useState('');
  const [loaded, setLoaded] = useState(false); // 최초 로드 전 저장 방지

  // 가용 알림 대상이 될 수 있는 GPU 자원(실시간: 전용 타입 + 공유 오퍼링).
  const [gpuTargets, setGpuTargets] = useState([]);
  // 세션 단위 알림 대상 — 내 세션 목록(활성 팀 기준). rule.target = 세션 instance_id.
  const [mySessions, setMySessions] = useState([]);
  useEffect(() => { getMySessions().then(setMySessions).catch(() => {}); }, []);

  // 가용성 알림(이전 '워크로드 알림') — GPU 자원/노드가 비면 알림.
  const [avail, setAvail] = useState([]);
  const [nodes, setNodes] = useState([]);
  const [availModal, setAvailModal] = useState(false);
  const [availForm, setAvailForm] = useState({ type: 'gpu', target: '' });

  // 백엔드에서 알림 규칙/채널 로드(없으면 백엔드 기본 규칙). 비크레딧 모드는 크레딧 지표 제외.
  useEffect(() => {
    getUserNotify().then((cfg) => {
      const rs = (cfg.rules || []).filter((r) => creditMode || !CREDIT_METRICS.includes(r.metric));
      // 기본 규칙은 미저장이라 id=0 → 고유 id 부여(중복 React key·updateRule 오작동 방지).
      setRules(rs.map((r, i) => ({ ...r, id: r.id || Date.now() + i })));
      setEmails(cfg.emails && cfg.emails.length ? cfg.emails : (user?.email ? [user.email] : []));
      setWebhooks(cfg.webhooks || []);
      setLoaded(true);
    });
  }, [creditMode]); // eslint-disable-line

  // 가용 GPU 대상 목록(드롭다운) — 실시간 가용 + 공유 오퍼링.
  useEffect(() => {
    Promise.all([getAvailability(), getOfferings()]).then(([a, o]) => {
      const exclusive = (a.byType || []).map((x) => `${x.gpuType} (전용)`);
      const shared = (o.items || []).filter((x) => x.gpuType && x.mode === 'fractional').map((x) => `${x.name} (공유)`);
      const list = [...exclusive, ...shared];
      setGpuTargets(list);
      setAvailForm((f) => (f.target ? f : { ...f, target: list[0] || '' }));
    });
  }, []);

  // 규칙/채널 변경 시 백엔드에 전체 저장(최초 로드 이후).
  useEffect(() => {
    if (!loaded) return;
    saveUserNotify({ rules, emails, webhooks });
  }, [rules, emails, webhooks, loaded]);

  useEffect(() => {
    getAlerts().then((d) => setAvail([...d.items]));
    if (hybrid) getSshNodes().then(setNodes);
  }, [hybrid]);

  // 받은 알림(인앱 수신함) — 알림 엔진이 규칙 위반 시 적재한 것.
  const [inbox, setInbox] = useState([]);
  const loadInbox = () => getInbox().then((d) => setInbox(d.items)).catch(() => {});
  useEffect(() => { loadInbox(); }, []);
  const readOne = async (id) => { await markInboxRead(id).catch(() => {}); setInbox((cur) => cur.map((n) => (n.id === id ? { ...n, read: true } : n))); };
  const readAll = async () => { await markInboxAllRead().catch(() => {}); setInbox((cur) => cur.map((n) => ({ ...n, read: true }))); };
  const inboxUnread = inbox.filter((n) => !n.read).length;
  const sevVariant = { err: 'err', warn: 'warn', info: 'primary' };
  // 받은 알림은 누적되므로 페이지네이션(10개/페이지).
  const INBOX_PAGE = 10;
  const [inboxPage, setInboxPage] = useState(0);
  const inboxPages = Math.max(1, Math.ceil(inbox.length / INBOX_PAGE));
  const inboxView = inbox.slice(inboxPage * INBOX_PAGE, inboxPage * INBOX_PAGE + INBOX_PAGE);

  const availTargetOptions = availForm.type === 'node'
    ? nodes.map((n) => ({ value: n.node, label: `${n.node} · ${n.gpu}` }))
    : gpuTargets.map((g) => ({ value: g, label: g }));
  const openAvailNew = () => { setAvailForm({ type: 'gpu', target: gpuTargets[0] || '' }); setAvailModal(true); };
  const setAvailType = (type) => setAvailForm({ type, target: type === 'node' ? (nodes[0]?.node || '') : (gpuTargets[0] || '') });
  const addAvail = async () => {
    if (!availForm.target) { toast(t('alerts.needTarget')); return; }
    const created = await createAlert({ type: availForm.type, target: availForm.target });
    setAvail((cur) => [{ id: created.id, type: created.type, target: created.target, enabled: created.enabled }, ...cur]);
    setAvailModal(false);
    toast(t('alerts.created'));
  };
  const toggleAvail = async (id) => { await toggleAlert(id); setAvail((cur) => cur.map((x) => (x.id === id ? { ...x, enabled: !x.enabled } : x))); };
  const removeAvail = async (id) => { if (!(await confirm({ title: t('alerts.delete'), message: t('confirmDelete') }))) return; await deleteAlert(id); setAvail((cur) => cur.filter((x) => x.id !== id)); toast(t('alerts.removed')); };
  const availTypePill = (type) => (type === 'node'
    ? <Pill variant="primary"><Server size={12} /> {t('alerts.typeNode')}</Pill>
    : <Pill variant="gpu"><Cpu size={12} /> {t('alerts.typeGpu')}</Pill>);

  // 실제로 발화하는 지표만 노출한다(서버 미평가 지표는 규칙을 추가해도 안 울려 혼란을 준다).
  //  전역: credit_balance(크레딧 잔액). 세션 단위: session_gpu/cpu/vram(대상 세션에서 평가).
  const METRICS = [
    ...(creditMode ? [{ key: 'credit_balance', label: t('notify.mCredit'), unit: 'C' }] : []),
    { key: 'session_gpu', label: t('notify.mSessGpu', { defaultValue: '세션 GPU 사용률' }), unit: '%', session: true },
    { key: 'session_cpu', label: t('notify.mSessCpu', { defaultValue: '세션 CPU 사용률' }), unit: '%', session: true },
    { key: 'session_vram', label: t('notify.mSessVram', { defaultValue: '세션 VRAM 사용률' }), unit: '%', session: true },
  ];
  const isSessionMetric = (m) => !!METRICS.find((x) => x.key === m)?.session;
  const OPS = [{ key: 'lte', label: t('notify.opLte') }, { key: 'gte', label: t('notify.opGte') }];
  const CHANNELS = [{ key: 'email', label: t('notify.chEmail') }, { key: 'webhook', label: t('notify.chWebhook') }];

  const metricUnit = (m) => METRICS.find((x) => x.key === m)?.unit || '';
  // 세션 instance_id → 표시 이름(현재 내 세션 목록 기준). 없으면 id 그대로.
  const sessName = (id) => mySessions.find((s) => s.id === id)?.name || id;
  // 신규 규칙 기본값: 세션 GPU 유휴(≤10%) 알림 — 첫 세션 대상(있으면). 크레딧 모드 무관하게 세션 지표는 항상 가능.
  const addRule = () => setRules([...rules, { id: Date.now(), metric: 'session_gpu', op: 'lte', value: 10, channel: 'email', on: true, target: mySessions[0]?.id || '' }]);
  // 지표를 세션↔전역으로 바꾸면 target 을 맞춰 초기화(세션 지표인데 target 없으면 첫 세션).
  const updateRule = (id, patch) => setRules(rules.map((r) => {
    if (r.id !== id) return r;
    const next = { ...r, ...patch };
    if (patch.metric !== undefined) next.target = isSessionMetric(patch.metric) ? (r.target || mySessions[0]?.id || '') : '';
    return next;
  }));
  const removeRule = (id) => setRules(rules.filter((r) => r.id !== id));
  const addEmail = () => { if (newEmail.trim()) { setEmails([...emails, newEmail.trim()]); setNewEmail(''); } };
  const addWebhook = () => { if (newWebhook.trim()) { setWebhooks([...webhooks, newWebhook.trim()]); setNewWebhook(''); } };

  return (
    <div>
      <PageHead title={t('notify.title')} subtitle={t('notify.subtitle')} />

      {/* 받은 알림(인앱 수신함) */}
      <div className="card mb">
        <h3 className="flex" style={{ justifyContent: 'space-between' }}>
          <span className="flex gap"><BellRing size={16} /> {t('notify.inboxTitle', { defaultValue: '받은 알림' })}
            {inboxUnread > 0 && <Pill variant="err">{inboxUnread}</Pill>}</span>
          {inboxUnread > 0 && <button className="btn sm" onClick={readAll}>{t('notify.readAll', { defaultValue: '모두 읽음' })}</button>}
        </h3>
        {inbox.length === 0 ? (
          <div className="muted" style={{ padding: '10px 0', fontSize: 13 }}>{t('notify.inboxEmpty', { defaultValue: '받은 알림이 없습니다.' })}</div>
        ) : (
          <div>
            {inboxView.map((n, i) => {
              const txt = notifyText(n, t);
              return (
              <div key={n.id} {...clickable(n.read ? undefined : () => readOne(n.id))}
                style={{ display: 'flex', gap: 12, alignItems: 'flex-start', padding: '12px 4px', borderTop: i ? '1px solid var(--border)' : 'none',
                  cursor: n.read ? 'default' : 'pointer', opacity: n.read ? 0.6 : 1 }}>
                {!n.read && <span style={{ marginTop: 6, flex: '0 0 auto', width: 8, height: 8, borderRadius: 4, background: 'var(--danger)' }} />}
                <div style={{ minWidth: 0, flex: 1 }}>
                  <div className="flex" style={{ gap: 8, alignItems: 'center', marginBottom: 3, flexWrap: 'wrap' }}>
                    <Pill variant={sevVariant[n.severity] || 'primary'}>{t(`notify.sev_${n.severity}`, { defaultValue: n.severity })}</Pill>
                    {/* 대상 구분: 세션 알림은 어느 세션인지, 그 외는 계정 전체 */}
                    {n.target
                      ? <Pill variant="gpu"><Server size={11} /> {sessName(n.target)}</Pill>
                      : <Pill variant="primary">{t('notify.scopeAccount', { defaultValue: '계정 전체' })}</Pill>}
                    <span style={{ fontWeight: 700, fontSize: 13.5 }}>{txt.title}</span>
                  </div>
                  <div className="muted" style={{ fontSize: 12.5 }}>{txt.body}</div>
                </div>
                <span className="muted mono" style={{ fontSize: 11, flex: '0 0 auto' }}>{new Date(n.createdAt).toLocaleString()}</span>
              </div>
              );
            })}
            {inboxPages > 1 && (
              <div className="flex" style={{ justifyContent: 'center', alignItems: 'center', gap: 10, marginTop: 12, paddingTop: 12, borderTop: '1px solid var(--border)' }}>
                <button className="btn sm" disabled={inboxPage === 0} onClick={() => setInboxPage((p) => Math.max(0, p - 1))}>{t('notify.prev', { defaultValue: '이전' })}</button>
                <span className="muted" style={{ fontSize: 12.5 }}>{inboxPage + 1} / {inboxPages}</span>
                <button className="btn sm" disabled={inboxPage >= inboxPages - 1} onClick={() => setInboxPage((p) => Math.min(inboxPages - 1, p + 1))}>{t('notify.next', { defaultValue: '다음' })}</button>
              </div>
            )}
          </div>
        )}
      </div>

      {config.features.workloadAlerts && (
        <div className="card mb">
          <h3 className="flex" style={{ justifyContent: 'space-between' }}>
            <span className="flex gap"><BellPlus size={16} /> {t('alerts.title')}</span>
            <button className="btn sm" onClick={openAvailNew}><BellPlus size={14} /> {t('alerts.add')}</button>
          </h3>
          <div className="legend mb">{t('alerts.hint')}</div>
          <DataTable
            rows={avail}
            rowKey={(x) => x.id}
            emptyText={t('alerts.none')}
            columns={[
              { key: 'type', header: t('alerts.type'), render: (x) => availTypePill(x.type) },
              { key: 'target', header: t('alerts.target'), render: (x) => <span style={{ fontWeight: 600 }}>{x.target}</span> },
              { key: 'cond', header: t('alerts.condition'), render: () => t('alerts.whenAvailable') },
              { key: 'enabled', header: t('alerts.enabled'), render: (x) => <Toggle checked={x.enabled} onChange={() => toggleAvail(x.id)} /> },
              { key: 'act', header: '', className: 'flex', render: (x) => <button className="btn sm danger" onClick={() => removeAvail(x.id)}>{t('alerts.delete')}</button> },
            ]}
          />
        </div>
      )}

      <div className="card mb">
        <h3 className="flex" style={{ justifyContent: 'space-between' }}>
          <span className="flex gap"><BellRing size={16} /> {t('notify.rules')}</span>
          <button className="btn sm primary" onClick={addRule}><Plus size={14} /> {t('notify.addRule')}</button>
        </h3>
        <div className="legend mb">{t('notify.rulesHint', { defaultValue: '알림 규칙과 받은 알림은 계정 단위입니다(모든 팀 공통). 세션 지표는 대상 세션에서 평가됩니다.' })}</div>
        {rules.length === 0 && <div className="muted" style={{ padding: '12px 0' }}>{t('notify.noRules')}</div>}
        {rules.map((r) => (
          <div key={r.id} className="flex gap wrap" style={{ padding: '14px 0', borderBottom: '1px solid var(--border)' }}>
            <Toggle checked={r.on} onChange={(v) => updateRule(r.id, { on: v })} />
            <span className="muted">{t('notify.cond')}</span>
            <Select size="sm" value={r.metric} onChange={(v) => updateRule(r.id, { metric: v })}
              options={METRICS.map((m) => ({ value: m.key, label: m.label }))} />
            {/* 세션 지표면 대상 세션 선택 */}
            {isSessionMetric(r.metric) && (
              mySessions.length > 0
                ? <Select size="sm" width={180} value={r.target} onChange={(v) => updateRule(r.id, { target: v })}
                    options={mySessions.map((s) => ({ value: s.id, label: s.name || s.id }))} placeholder={t('notify.selectSession', { defaultValue: '세션 선택' })} />
                : <span className="muted" style={{ fontSize: 12.5 }}>{t('notify.noSession', { defaultValue: '실행 중 세션 없음' })}</span>
            )}
            <Select size="sm" width={110} value={r.op} onChange={(v) => updateRule(r.id, { op: v })}
              options={OPS.map((o) => ({ value: o.key, label: o.label }))} />
            <input type="number" style={{ width: 90 }} value={r.value} onChange={(e) => updateRule(r.id, { value: Number(e.target.value) })} />
            <span className="muted">{metricUnit(r.metric)}</span>
            <span className="muted">→</span>
            <Select size="sm" width={120} value={r.channel} onChange={(v) => updateRule(r.id, { channel: v })}
              options={CHANNELS.map((c) => ({ value: c.key, label: c.label }))} />
            <button className="btn sm danger right" onClick={() => removeRule(r.id)}><Trash2 size={13} /></button>
          </div>
        ))}
      </div>

      <div className="grid cols-2">
        <div className="card">
          <h3><Mail size={16} /> {t('notify.emails')} <span className="muted">{t('notify.count', { n: emails.length })}</span></h3>
          {emails.map((e, i) => (
            <div className="avail" key={i}>
              <span className="flex gap"><Pill variant="ok">{t('notify.chEmail')}</Pill><span style={{ fontWeight: 600 }}>{e}</span></span>
              <button className="btn sm danger" onClick={() => setEmails(emails.filter((_, j) => j !== i))}><Trash2 size={13} /></button>
            </div>
          ))}
          <div className="flex gap mt">
            <input type="email" value={newEmail} onChange={(e) => setNewEmail(e.target.value)} placeholder="alert@giosk.io" />
            <button className="btn" onClick={addEmail}><Plus size={14} /> {t('notify.add')}</button>
          </div>
        </div>
        <div className="card">
          <h3><Webhook size={16} /> {t('notify.webhooks')} <span className="muted">{t('notify.count', { n: webhooks.length })}</span></h3>
          {webhooks.length === 0 && <div className="muted" style={{ padding: '10px 0' }}>{t('notify.noWebhook')}</div>}
          {webhooks.map((w, i) => (
            <div className="avail" key={i}>
              <span className="flex gap" style={{ minWidth: 0 }}><Pill variant="primary">{t('notify.chWebhook')}</Pill><span className="mono" style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>{w}</span></span>
              <button className="btn sm danger" onClick={() => setWebhooks(webhooks.filter((_, j) => j !== i))}><Trash2 size={13} /></button>
            </div>
          ))}
          <div className="flex gap mt">
            <input type="text" value={newWebhook} onChange={(e) => setNewWebhook(e.target.value)} placeholder="https://hooks.slack.com/..." />
            <button className="btn" onClick={addWebhook}><Plus size={14} /> {t('notify.add')}</button>
          </div>
        </div>
      </div>

      <div className="mt"><button className="btn primary" onClick={() => toast(t('notify.saved', { n: rules.length }))}>{t('notify.save')}</button></div>

      <Modal open={availModal} title={t('alerts.addTitle')} onClose={() => setAvailModal(false)} width={440}
        footer={(
          <>
            <button className="btn" onClick={() => setAvailModal(false)}>{t('alerts.cancel')}</button>
            <button className="btn primary" onClick={addAvail}>{t('alerts.save')}</button>
          </>
        )}>
        <label className="fld" id="user-notificationcenter-fld-0-lbl" style={{ marginTop: 0 }}>{t('alerts.type')}</label>
        <Select ariaLabelledBy="user-notificationcenter-fld-0-lbl" value={availForm.type} onChange={setAvailType}
          options={[
            { value: 'gpu', label: t('alerts.typeGpu') },
            ...(hybrid ? [{ value: 'node', label: t('alerts.typeNode') }] : []),
          ]} />
        <label className="fld" id="user-notificationcenter-fld-1-lbl">{t('alerts.target')}</label>
        <Select ariaLabelledBy="user-notificationcenter-fld-1-lbl" value={availForm.target} onChange={(v) => setAvailForm({ ...availForm, target: v })} options={availTargetOptions} placeholder={t('alerts.selectTarget')} />
        <div className="legend mt">{t('alerts.addHint')}</div>
      </Modal>
    </div>
  );
}
