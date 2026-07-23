import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';

import { SUPPORTED_CODES, RTL_LANGS } from './languages';

// 로케일 자동 로딩 — src/locales/<lang>/<ns>.json 을 전부 eager import 해서 resources 구성.
// 번역 파일을 새로 추가하면(스크립트가 생성) 별도 배선 없이 자동 인식된다.
// 파일이 없는 언어는 fallbackLng(en)로 우아하게 폴백(선택은 가능, 표시는 영어).
const modules = import.meta.glob('../locales/*/*.json', { eager: true });
const resources = {};
for (const [path, mod] of Object.entries(modules)) {
  const m = path.match(/\/locales\/([^/]+)\/([^/]+)\.json$/);
  if (!m) continue;
  const [, lng, ns] = m;
  (resources[lng] ||= {})[ns] = mod.default || mod;
}

// 문서 방향/언어 속성 반영(RTL 언어 지원).
const applyDir = (lng) => {
  const base = (lng || 'en').split('-')[0];
  const rtl = RTL_LANGS.has(lng) || RTL_LANGS.has(base);
  if (typeof document !== 'undefined') {
    document.documentElement.dir = rtl ? 'rtl' : 'ltr';
    document.documentElement.lang = lng || 'en';
  }
};

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources,
    fallbackLng: 'en',
    supportedLngs: SUPPORTED_CODES,
    // nonExplicitSupportedLngs 는 쓰지 않는다: zh-Hans/pt-BR 처럼 스크립트/지역 서브태그가 있는 코드를
    // 언어파트(zh/pt)만으로 지원여부를 검사해 목록에 없으면 영어로 폴백시켜버린다. 명시적 코드로 직접 매칭.
    defaultNS: 'common',
    ns: ['common', 'consoleAdmin', 'consoleUser', 'errors'],
    interpolation: { escapeValue: false },
    detection: {
      order: ['localStorage', 'navigator'],
      lookupLocalStorage: 'giosk_lang',
      caches: ['localStorage'],
    },
  });

i18n.on('languageChanged', applyDir);
applyDir(i18n.resolvedLanguage || i18n.language);

export default i18n;
