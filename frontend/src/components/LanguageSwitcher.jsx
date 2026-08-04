import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Globe, ChevronDown, Check, Search } from 'lucide-react';
import { LANGUAGES, langMeta, flagCC } from '../i18n/languages';
import { setLanguage } from '../i18n';

// 국기 SVG 를 파일 URL 로만 가져온다.
// flag-icons 의 통짜 CSS(약 430KB)는 138개국 배경 규칙을 전부 싣는데, 우리가 쓰는 건 언어 목록에
// 대응하는 40여 개뿐이다. URL 만 뽑아 <img> 로 그리면 CSS 비용이 사라지고, 실제 표시된 국기만 내려받는다.
const FLAG_URLS = import.meta.glob('/node_modules/flag-icons/flags/4x3/*.svg', { query: '?url', import: 'default', eager: true });
const flagUrl = (cc) => FLAG_URLS[`/node_modules/flag-icons/flags/4x3/${cc}.svg`];

// SVG 국기를 쓴다. emoji 와 달리 모든 OS 에서 같게 렌더된다. 대응 국가코드가 없으면 지구본으로 폴백한다.
function Flag({ flag, size = 20 }) {
  const cc = flagCC(flag);
  const url = cc && flagUrl(cc);
  if (!url) return <Globe size={15} style={{ color: 'var(--muted)' }} />;
  return (
    <img src={url} alt="" aria-hidden="true" width={size} height={Math.round(size * 0.75)}
      style={{ borderRadius: 2, flexShrink: 0, objectFit: 'cover', boxShadow: 'inset 0 0 0 1px rgba(0,0,0,.08)' }} />
  );
}

// 언어 선택 드롭다운이다. SVG 국기와 자국어명 검색 목록을 쓰고, 앱 CSS 변수로 라이트와 다크 테마에 대응한다.
export default function LanguageSwitcher({ align = 'right' }) {
  const { i18n } = useTranslation();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [dropUp, setDropUp] = useState(false);
  const ref = useRef(null);
  const searchRef = useRef(null);

  const current = i18n.resolvedLanguage || i18n.language || 'en';
  const meta = langMeta(current) || LANGUAGES[0];

  useEffect(() => {
    if (!open) return undefined;
    setQuery('');
    // 아래 공간이 부족하면 위로 열어 문서 오버플로(페이지가 길어지는 현상)를 막는다.
    const rect = ref.current?.getBoundingClientRect();
    setDropUp(!!rect && window.innerHeight - rect.bottom < 360);
    const t = setTimeout(() => searchRef.current?.focus(), 0);
    const onDoc = (e) => { if (ref.current && !ref.current.contains(e.target)) setOpen(false); };
    const onEsc = (e) => { if (e.key === 'Escape') setOpen(false); };
    document.addEventListener('mousedown', onDoc);
    document.addEventListener('keydown', onEsc);
    return () => { clearTimeout(t); document.removeEventListener('mousedown', onDoc); document.removeEventListener('keydown', onEsc); };
  }, [open]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return LANGUAGES;
    return LANGUAGES.filter(
      (l) => l.native.toLowerCase().includes(q) || l.en.toLowerCase().includes(q) || l.code.toLowerCase().includes(q),
    );
  }, [query]);

  // setLanguage 는 번역을 받은 뒤에 언어를 바꾼다. i18n.changeLanguage 를 직접 부르면
  // 번역이 오기 전에 영어로 한 번 그려진다.
  const change = (code) => {
    if (code !== current) setLanguage(code);
    setOpen(false);
  };

  const row = (active) => ({
    display: 'flex', alignItems: 'center', gap: 8, width: '100%', textAlign: 'start',
    padding: '8px 12px', border: 'none', background: 'transparent', cursor: 'pointer',
    fontSize: 13, color: active ? 'var(--primary)' : 'var(--text)', fontWeight: active ? 700 : 400,
  });

  return (
    <div ref={ref} style={{ position: 'relative' }}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="listbox"
        aria-expanded={open}
        style={{
          display: 'inline-flex', alignItems: 'center', gap: 7, padding: '7px 11px', fontSize: 13,
          border: '1px solid var(--border)', borderRadius: 'var(--r-control)', background: 'var(--surface)',
          color: 'var(--text)', cursor: 'pointer',
        }}
      >
        <Flag flag={meta?.flag} />
        <span style={{ fontWeight: 600 }}>{meta?.native ?? current}</span>
        <ChevronDown size={14} style={{ color: 'var(--muted)', transform: open ? 'rotate(180deg)' : 'none', transition: 'transform .15s' }} />
      </button>

      {open && (
        <div style={{
          position: 'absolute', [align === 'left' ? 'left' : 'right']: 0,
          [dropUp ? 'bottom' : 'top']: 'calc(100% + 6px)',
          width: 264, zIndex: 70, background: 'var(--surface)', border: '1px solid var(--border)',
          borderRadius: 'var(--r-container)', boxShadow: '0 16px 40px rgba(10,15,28,.22)', overflow: 'hidden',
        }}>
          <div style={{ padding: 8, borderBottom: '1px solid var(--border)' }}>
            <div style={{ position: 'relative' }}>
              <Search size={15} style={{ position: 'absolute', left: 9, top: '50%', transform: 'translateY(-50%)', color: 'var(--muted)' }} />
              <input
                ref={searchRef}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search language…"
                style={{
                  width: '100%', padding: '8px 10px 8px 30px', fontSize: 13, color: 'var(--text)',
                  background: 'var(--surface-2)', border: '1px solid var(--border)', borderRadius: 'var(--r-control)', outline: 'none',
                }}
              />
            </div>
          </div>
          <ul role="listbox" style={{ maxHeight: 288, overflowY: 'auto', margin: 0, padding: '4px 0', listStyle: 'none' }}>
            {filtered.map((l) => {
              const active = l.code === current;
              return (
                <li key={l.code}>
                  <button type="button" onClick={() => change(l.code)} style={row(active)}
                    onMouseEnter={(e) => { e.currentTarget.style.background = 'var(--surface-2)'; }}
                    onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent'; }}>
                    <Flag flag={l.flag} />
                    <span style={{ flex: 1, minWidth: 0 }}>
                      <span style={{ display: 'block', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{l.native}</span>
                      <span className="muted" style={{ display: 'block', fontSize: 11, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{l.en}</span>
                    </span>
                    {active && <Check size={15} style={{ flexShrink: 0 }} />}
                  </button>
                </li>
              );
            })}
            {filtered.length === 0 && <li className="muted" style={{ padding: '8px 12px', fontSize: 13 }}>No match</li>}
          </ul>
        </div>
      )}
    </div>
  );
}
