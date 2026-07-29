import React from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { BookOpen, Rocket, Cpu, Database, Coins, Boxes, HardDrive, Building2, UserPlus, BellRing } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import { GUIDES } from '../../../config/guides';
import { clickable } from '../../../utils/a11y';

const GUIDE_ICON = {
  start: Rocket, session: Boxes, gpu: Cpu, volume: HardDrive, dataset: Database,
  credit: Coins, org: Building2, join: UserPlus, alerts: BellRing,
};

export default function Guide() {
  const { t, i18n } = useTranslation('consoleUser');
  const lang = i18n.language?.startsWith('en') ? 'en' : 'ko';
  const navigate = useNavigate();

  return (
    <div>
      <PageHead title={t('guide.title')} subtitle={t('guide.subtitle')} />
      <div className="card">
        <h3><BookOpen size={16} /> {t('guide.docs')}</h3>
        <div className="grid cols-4" style={{ gap: 14 }}>
          {GUIDES.map((g) => {
            const Icon = GUIDE_ICON[g.id] || BookOpen;
            return (
              <div className="task-card" key={g.id} {...clickable(() => navigate(`/console/guide/${g.id}`))}>
                <div className="ico" style={{ background: 'var(--primary-soft)', color: 'var(--primary)' }}><Icon size={22} /></div>
                <h4>{g.title[lang]}</h4>
                <p>{t('guide.readMore')} →</p>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
