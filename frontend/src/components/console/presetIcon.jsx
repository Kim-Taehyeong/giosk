import React from 'react';
import {
  Sprout, MonitorSmartphone, Rocket, HardDriveDownload, Layers, Box,
  Cpu, Brain, FlaskConical, Database, Zap, Server,
} from 'lucide-react';

// 선택 가능한 아이콘 팔레트 (key → Icon).
export const ICON_OPTIONS = [
  { key: 'sprout', Icon: Sprout },
  { key: 'laptop', Icon: MonitorSmartphone },
  { key: 'rocket', Icon: Rocket },
  { key: 'download', Icon: HardDriveDownload },
  { key: 'layers', Icon: Layers },
  { key: 'cpu', Icon: Cpu },
  { key: 'brain', Icon: Brain },
  { key: 'flask', Icon: FlaskConical },
  { key: 'database', Icon: Database },
  { key: 'zap', Icon: Zap },
  { key: 'server', Icon: Server },
  { key: 'box', Icon: Box },
];

const REG = Object.fromEntries(ICON_OPTIONS.map((o) => [o.key, o.Icon]));
// 기본 프리셋 name → 아이콘 key 매핑(아이콘 미지정 시 폴백).
const NAME_MAP = { light: 'sprout', standard: 'laptop', large: 'rocket', cpu: 'download', mig: 'layers' };

export function resolveIconKey({ icon, name } = {}) {
  if (icon && REG[icon]) return icon;
  if (name && NAME_MAP[name]) return NAME_MAP[name];
  return 'box';
}

export function presetIconFor(name) {
  return REG[NAME_MAP[name]] || Box;
}

// icon(명시 key) 우선, 없으면 name 기반 폴백.
export default function PresetIcon({ name, icon, size = 18 }) {
  const Icon = REG[resolveIconKey({ icon, name })] || Box;
  return <Icon size={size} />;
}
