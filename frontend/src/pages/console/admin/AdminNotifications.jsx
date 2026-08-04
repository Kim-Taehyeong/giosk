import React, { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { BellRing, Plus, Trash2, Mail, Webhook } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import Pill from '../../../components/console/Pill';
import { useToast } from '../../../components/console/Toast';
import { getAdminNotify, saveAdminNotify } from '../../../api/console/notify';

export default function AdminNotifications() {
  const { t } = useTranslation('consoleAdmin');
  const { toast } = useToast();
  const [rules, setRules] = useState([]);
  const [emails, setEmails] = useState([]);
  const [webhooks, setWebhooks] = useState([]);
  const [newEmail, setNewEmail] = useState('');
  const [newWebhook, setNewWebhook] = useState('');

  // 백엔드에서 관리자(전역) 알림 규칙/채널 로드(없으면 백엔드 기본 규칙).
  useEffect(() => {
    getAdminNotify().then((cfg) => {
      // 기본 규칙은 저장돼 있지 않아 id 가 0 이라 고유 id 를 부여한다(React key 중복과 updateRule 오작동 방지).
      setRules((cfg.rules || []).map((r, i) => ({ ...r, id: r.id || Date.now() + i })));
      setEmails(cfg.emails || []);
      setWebhooks(cfg.webhooks || []);
    }).catch(() => {}); // 플랫폼 전용이라 매니저가 URL 로 직접 들어오면 403 이다. 미처리 rejection(화이트스크린)을 막는다.
  }, []);

  const save = async () => { await saveAdminNotify({ rules, emails, webhooks }); toast(t('anotify.saved', { n: rules.length })); };

  // 알림 엔진이 실제 평가하는 메트릭만 노출.
  const METRICS = [
    { key: 'gpu_util', label: t('anotify.mGpu'), unit: '%' },
    { key: 'gpu_temp', label: t('anotify.mTemp'), unit: '℃' },
    { key: 'node_down', label: t('anotify.mNode'), unit: '대' },
    { key: 'node_idle', label: t('anotify.mNodeIdle', { defaultValue: '노드 유휴(GPU 사용률 이하)' }), unit: '%' },
    { key: 'capacity', label: t('anotify.mCapacity', { defaultValue: '클러스터 GPU 소진' }), unit: '%' },
    { key: 'disk_usage', label: t('anotify.mDisk', { defaultValue: '노드 디스크 사용률' }), unit: '%' },
    { key: 'volume_usage', label: t('anotify.mVolume', { defaultValue: '볼륨 사용률' }), unit: '%' },
  ];
  const OPS = [{ key: 'lte', label: t('anotify.opLte') }, { key: 'gte', label: t('anotify.opGte') }];
  const CHANNELS = [{ key: 'email', label: t('anotify.chEmail') }, { key: 'webhook', label: t('anotify.chWebhook') }];

  const metricUnit = (m) => METRICS.find((x) => x.key === m)?.unit || '';
  const addRule = () => setRules([...rules, { id: Date.now(), metric: 'gpu_util', op: 'gte', value: 90, channel: 'webhook', on: true }]);
  const updateRule = (id, patch) => setRules(rules.map((r) => (r.id === id ? { ...r, ...patch } : r)));
  const removeRule = (id) => setRules(rules.filter((r) => r.id !== id));
  const addEmail = () => { if (newEmail.trim()) { setEmails([...emails, newEmail.trim()]); setNewEmail(''); } };
  const addWebhook = () => { if (newWebhook.trim()) { setWebhooks([...webhooks, newWebhook.trim()]); setNewWebhook(''); } };

  return (
    <div>
      <PageHead title={t('anotify.title')} subtitle={t('anotify.subtitle')}
        actions={<button className="btn primary" onClick={addRule}><Plus size={15} /> {t('anotify.addRule')}</button>} />

      <div className="card mb">
        <h3><BellRing size={16} /> {t('anotify.rules')}</h3>
        {rules.length === 0 && <div className="muted" style={{ padding: '12px 0' }}>{t('anotify.noRules')}</div>}
        {rules.map((r) => (
          <div key={r.id} className="flex gap wrap" style={{ padding: '14px 0', borderBottom: '1px solid var(--border)' }}>
            <input type="checkbox" style={{ width: 'auto' }} checked={r.on} onChange={(e) => updateRule(r.id, { on: e.target.checked })} />
            <span className="muted">{t('anotify.cond')}</span>
            <select style={{ width: 'auto' }} value={r.metric} onChange={(e) => updateRule(r.id, { metric: e.target.value })}>
              {METRICS.map((m) => <option key={m.key} value={m.key}>{m.label}</option>)}
            </select>
            <select style={{ width: 'auto' }} value={r.op} onChange={(e) => updateRule(r.id, { op: e.target.value })}>
              {OPS.map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}
            </select>
            <input type="number" style={{ width: 90 }} value={r.value} onChange={(e) => updateRule(r.id, { value: Number(e.target.value) })} />
            <span className="muted">{metricUnit(r.metric)}</span>
            <span className="muted">→</span>
            <select style={{ width: 'auto' }} value={r.channel} onChange={(e) => updateRule(r.id, { channel: e.target.value })}>
              {CHANNELS.map((c) => <option key={c.key} value={c.key}>{c.label}</option>)}
            </select>
            <button className="btn sm danger right" onClick={() => removeRule(r.id)}><Trash2 size={13} /></button>
          </div>
        ))}
      </div>

      <div className="grid cols-2 mb">
        <div className="card">
          <h3><Mail size={16} /> {t('anotify.emails')} <span className="muted">{t('anotify.count', { n: emails.length })}</span></h3>
          {emails.map((e, i) => (
            <div className="avail" key={i}>
              <span className="flex gap"><Pill variant="ok">{t('anotify.chEmail')}</Pill><span style={{ fontWeight: 600 }}>{e}</span></span>
              <button className="btn sm danger" onClick={() => setEmails(emails.filter((_, j) => j !== i))}><Trash2 size={13} /></button>
            </div>
          ))}
          <div className="flex gap mt">
            <input type="email" value={newEmail} onChange={(e) => setNewEmail(e.target.value)} placeholder="alert@giosk.io" />
            <button className="btn" onClick={addEmail}><Plus size={14} /> {t('anotify.add')}</button>
          </div>
        </div>
        <div className="card">
          <h3><Webhook size={16} /> {t('anotify.webhooks')} <span className="muted">{t('anotify.count', { n: webhooks.length })}</span></h3>
          {webhooks.length === 0 && <div className="muted" style={{ padding: '10px 0' }}>{t('anotify.noWebhook')}</div>}
          {webhooks.map((w, i) => (
            <div className="avail" key={i}>
              <span className="flex gap" style={{ minWidth: 0 }}><Pill variant="primary">{t('anotify.chWebhook')}</Pill><span className="mono" style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>{w}</span></span>
              <button className="btn sm danger" onClick={() => setWebhooks(webhooks.filter((_, j) => j !== i))}><Trash2 size={13} /></button>
            </div>
          ))}
          <div className="flex gap mt">
            <input type="text" value={newWebhook} onChange={(e) => setNewWebhook(e.target.value)} placeholder="https://hooks.slack.com/..." />
            <button className="btn" onClick={addWebhook}><Plus size={14} /> {t('anotify.add')}</button>
          </div>
        </div>
      </div>

      <button className="btn primary" onClick={save}>{t('anotify.save')}</button>
    </div>
  );
}
