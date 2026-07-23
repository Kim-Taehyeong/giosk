import React from 'react';
import { useParams, useNavigate, Navigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { ArrowLeft, BookOpen, Rocket, Cpu, Database, Coins, Boxes, HardDrive, Building2, UserPlus, BellRing } from 'lucide-react';
import PageHead from '../../../components/console/PageHead';
import Markdown from '../../../components/console/Markdown';
import { GUIDES } from '../../../config/guides';

const GUIDE_ICON = {
  start: Rocket, session: Boxes, gpu: Cpu, volume: HardDrive, dataset: Database,
  credit: Coins, org: Building2, join: UserPlus, alerts: BellRing,
};

export default function GuideDetail() {
  const { t, i18n } = useTranslation('consoleUser');
  const lang = i18n.language?.startsWith('en') ? 'en' : 'ko';
  const { id } = useParams();
  const navigate = useNavigate();
  const guide = GUIDES.find((g) => g.id === id);
  if (!guide) return <Navigate to="/console/guide" replace />;
  const Icon = GUIDE_ICON[guide.id] || BookOpen;

  return (
    <div>
      <PageHead crumb={t('guide.title')} title={guide.title[lang]}
        actions={<button className="btn" onClick={() => navigate('/console/guide')}><ArrowLeft size={15} /> {t('guide.back')}</button>} />
      <article style={{ maxWidth: 760, margin: '0 auto', paddingBottom: 40 }}>
        <div className="flex gap" style={{ alignItems: 'center', paddingBottom: 16, marginBottom: 8, borderBottom: '1px solid var(--border)' }}>
          <span style={{ width: 40, height: 40, borderRadius: 11, flex: '0 0 auto', display: 'grid', placeItems: 'center', background: 'var(--primary-soft)', color: 'var(--primary)' }}><Icon size={22} /></span>
          <div>
            <div className="muted" style={{ fontSize: 12, fontWeight: 700 }}>{t('guide.docs')}</div>
            <div style={{ fontSize: 20, fontWeight: 800, letterSpacing: '-.4px' }}>{guide.title[lang]}</div>
          </div>
        </div>
        <Markdown>{guide.body[lang]}</Markdown>
      </article>
    </div>
  );
}
