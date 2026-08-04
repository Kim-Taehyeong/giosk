import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { SlidersHorizontal, UserPlus } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import Toggle from '../../../components/console/Toggle';
import Spinner from '../../../components/console/Spinner';
import { useToast } from '../../../components/console/Toast';
import { getGroups, updateGroup } from '../../../api/console/governance';

// 중간관리자(조직이나 그룹) 설정이다. 지금은 그룹별 가입 신청 수락 여부만 있고 스코프 안의 그룹만 보인다.
export default function ManagerSettings() {
  const { t } = useTranslation('consoleAdmin');
  const { toast } = useToast();
  const [groups, setGroups] = useState(null);

  const load = () => getGroups().then((d) => setGroups(d.items));
  useEffect(() => { load(); }, []);

  const setAccepts = async (g, on) => {
    setGroups((cur) => cur.map((x) => (x.id === g.id ? { ...x, acceptsJoin: on } : x)));
    try { await updateGroup(g.id, { displayName: g.displayName, acceptsJoin: on }); toast(on ? t('mset.accepted', { g: g.displayName }) : t('mset.closed', { g: g.displayName })); }
    catch { toast(t('mset.fail')); load(); }
  };

  if (!groups) return <Spinner pad label={t('mset.loading', { defaultValue: '…' })} />;

  return (
    <div>
      <PageHead icon={SlidersHorizontal} title={t('mset.title')} subtitle={t('mset.subtitle')} />
      <div className="card">
        <h3><UserPlus size={16} /> {t('mset.joinTitle')}</h3>
        <div className="legend mb">{t('mset.joinNote')}</div>
        {groups.length === 0 ? <div className="legend">{t('mset.noGroups')}</div> : groups.map((g, i) => (
          <div key={g.id} className="flex" style={{ justifyContent: 'space-between', alignItems: 'center', padding: '12px 0', borderTop: i ? '1px solid var(--border)' : 'none' }}>
            <div>
              <div style={{ fontWeight: 700, fontSize: 14 }}>{g.displayName}</div>
              <div className="legend" style={{ marginTop: 2 }}>{g.orgName || ''}</div>
            </div>
            <Toggle checked={!!g.acceptsJoin} onChange={(v) => setAccepts(g, v)} />
          </div>
        ))}
      </div>
    </div>
  );
}
