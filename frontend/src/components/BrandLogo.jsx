import React from 'react';
import { Cpu, Server, Boxes, Cloud, Rocket, Zap, Layers, Hexagon, CircuitBoard, Atom, MemoryStick, Database } from 'lucide-react';
import { useSystemConfig } from '../context/SystemConfigContext';

// 브랜드 아이콘 세트(Lucide)다. 설치자가 콘솔 로고 아이콘을 고를 수 있고 key 가 저장값이다.
export const BRAND_ICONS = [
  { key: '', Icon: null }, // 기본(칩 마크)
  { key: 'cpu', Icon: Cpu }, { key: 'memory', Icon: MemoryStick }, { key: 'circuit', Icon: CircuitBoard },
  { key: 'server', Icon: Server }, { key: 'boxes', Icon: Boxes }, { key: 'layers', Icon: Layers },
  { key: 'cloud', Icon: Cloud }, { key: 'rocket', Icon: Rocket }, { key: 'zap', Icon: Zap },
  { key: 'hexagon', Icon: Hexagon }, { key: 'atom', Icon: Atom }, { key: 'database', Icon: Database },
];
const ICON_MAP = Object.fromEntries(BRAND_ICONS.filter((x) => x.Icon).map((x) => [x.key, x.Icon]));

// Giosk 로고 마크(GPU 칩 모티프)다. 색은 currentColor 를 따라 테마와 문맥에 맞춰 상속한다.
export function LogoMark({ size = 28, style }) {
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" fill="none" aria-hidden="true"
      style={{ color: 'var(--primary)', flex: '0 0 auto', ...style }}>
      {/* 둥근 사각 배경 */}
      <rect x="2" y="2" width="28" height="28" rx="8" fill="currentColor" />
      {/* 칩 다리(핀) */}
      <g fill="#fff">
        <rect x="12" y="5.6" width="1.8" height="3.4" rx="0.9" />
        <rect x="18.2" y="5.6" width="1.8" height="3.4" rx="0.9" />
        <rect x="12" y="23" width="1.8" height="3.4" rx="0.9" />
        <rect x="18.2" y="23" width="1.8" height="3.4" rx="0.9" />
        <rect x="5.6" y="12" width="3.4" height="1.8" rx="0.9" />
        <rect x="5.6" y="18.2" width="3.4" height="1.8" rx="0.9" />
        <rect x="23" y="12" width="3.4" height="1.8" rx="0.9" />
        <rect x="23" y="18.2" width="3.4" height="1.8" rx="0.9" />
      </g>
      {/* 칩 본체 */}
      <rect x="9" y="9" width="14" height="14" rx="3.5" fill="#fff" />
      {/* 코어 */}
      <rect x="12.9" y="12.9" width="6.2" height="6.2" rx="1.7" fill="currentColor" />
    </svg>
  );
}

// 마크 + 워드마크. 브랜드명/색은 시스템 설정(branding)에서 읽어 조직별로 바뀐다.
// ConsoleLayout 의 .logo 컨테이너 안에서 쓰면 .logo-name 스타일이 적용된다.
export default function BrandLogo({ markSize = 28, badge }) {
  const { config } = useSystemConfig();
  const b = config?.branding || {};
  const name = b.name?.trim() || 'Giosk';
  const accent = b.accent;
  // 서브타이틀: branding.subtitle 이 지정되면 그 값(빈 문자열이면 숨김), 아니면 기본 badge("Console").
  const sub = b.subtitle !== undefined && b.subtitle !== null ? b.subtitle : badge;
  const CustomIcon = b.icon && ICON_MAP[b.icon];
  return (
    <>
      {b.iconUrl
        ? <img src={b.iconUrl} alt="" style={{ width: markSize, height: markSize, flex: '0 0 auto', borderRadius: markSize * 0.28, objectFit: 'cover' }} />
        : CustomIcon
          ? <span style={{ width: markSize, height: markSize, flex: '0 0 auto', borderRadius: markSize * 0.28, display: 'grid', placeItems: 'center', background: accent || 'var(--primary)', color: '#fff' }}>
              <CustomIcon size={markSize * 0.6} strokeWidth={2.2} />
            </span>
          : <LogoMark size={markSize} style={accent ? { color: accent } : undefined} />}
      <span className="logo-name">
        <strong>{name}</strong>
        {sub && <small>{sub}</small>}
      </span>
    </>
  );
}
