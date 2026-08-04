import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';

import { SUPPORTED_CODES, RTL_LANGS } from './languages';

// 로케일을 지연 로딩한다. src/locales/<lang>/<ns>.json 을 glob 으로 등록해두고 실제로 쓰는 언어만 받아온다.
// (전부 eager 로 묶으면 40개 언어 × 4 네임스페이스가 첫 번들에 들어간다. 이 콘솔은 VPN·저대역폭에서
//  열리는 일이 많아 그 비용이 그대로 첫 화면 지연이 된다.)
// 번역 파일을 새로 추가하면 별도 배선 없이 자동 인식되는 성질은 그대로다.
const modules = import.meta.glob('../locales/*/*.json');

// path 별 { lng, ns } 색인
const index = {};
for (const path of Object.keys(modules)) {
  const m = path.match(/\/locales\/([^/]+)\/([^/]+)\.json$/);
  if (!m) continue;
  const [, lng, ns] = m;
  (index[lng] ||= {})[ns] = path;
}

const NS = ['common', 'consoleAdmin', 'consoleUser', 'errors'];
const loaded = new Set();

// 한 언어의 모든 네임스페이스를 받아 { ns: bundle } 로 돌려준다.
async function fetchLanguage(lng) {
  const entries = await Promise.all(
    NS.filter((ns) => index[lng]?.[ns]).map(async (ns) => [ns, (await modules[index[lng][ns]]()).default]),
  );
  return Object.fromEntries(entries);
}

// init 이후에 언어를 추가로 받아 넣는다.
// (init 전에는 i18next 의 리소스 저장소가 없어 addResourceBundle 을 부를 수 없다. 그래서 초기 언어는
//  아래에서 미리 받아 init 의 resources 로 넘긴다.)
//
// 진행 중인 요청은 loading 에 담아 같은 언어를 두 번 받지 않는다. loaded 표시는 실제로 다 넣은
// 뒤에 한다. 먼저 표시하면 아직 안 들어온 언어를 "이미 있다"고 보고 그냥 지나친다.
const loading = new Map();
function loadLanguage(lng) {
  if (!lng || !index[lng] || loaded.has(lng)) return Promise.resolve();
  if (loading.has(lng)) return loading.get(lng);
  const p = fetchLanguage(lng)
    .then((bundles) => {
      for (const [ns, bundle] of Object.entries(bundles)) i18n.addResourceBundle(lng, ns, bundle, true, true);
      loaded.add(lng);
    })
    .finally(() => loading.delete(lng));
  loading.set(lng, p);
  return p;
}

// 언어 전환은 번역을 받은 뒤에 한다. 먼저 바꾸면 아직 번역이 없어 영어로 한 번 그려지고,
// 뒤늦게 도착한 번역이 화면에 반영되지 않는다(바로 그 증상이 있었다).
export async function setLanguage(lng) {
  await loadLanguage(lng);
  await i18n.changeLanguage(lng);
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

// 감지기가 고를 언어를 미리 계산해, init 전에 그 언어와 폴백(en)만 받아둔다.
// (init 이후에 받으면 첫 렌더가 키 문자열로 번쩍인다.)
const detected = (() => {
  if (typeof localStorage !== 'undefined') {
    const saved = localStorage.getItem('giosk_lang');
    if (saved && SUPPORTED_CODES.includes(saved)) return saved;
  }
  const navs = typeof navigator !== 'undefined' ? (navigator.languages || [navigator.language]) : [];
  for (const n of navs) {
    if (!n) continue;
    if (SUPPORTED_CODES.includes(n)) return n;
    const base = n.split('-')[0];
    if (SUPPORTED_CODES.includes(base)) return base;
  }
  return 'en';
})();

// 초기 언어(감지 결과 + 폴백 en)는 init 의 resources 로 직접 넘긴다.
const initial = [...new Set(['en', detected])];
const initialBundles = await Promise.all(initial.map(fetchLanguage));
const resources = {};
initial.forEach((lng, i) => { resources[lng] = initialBundles[i]; loaded.add(lng); });

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources, // 나머지 언어는 languageChanged 에서 addResourceBundle 로 덧붙인다
    fallbackLng: 'en',
    supportedLngs: SUPPORTED_CODES,
    // nonExplicitSupportedLngs 는 쓰지 않는다: zh-Hans/pt-BR 처럼 스크립트/지역 서브태그가 있는 코드를
    // 언어파트(zh/pt)만으로 지원여부를 검사해 목록에 없으면 영어로 폴백시켜버린다. 명시적 코드로 직접 매칭.
    defaultNS: 'common',
    ns: NS,
    interpolation: { escapeValue: false },
    detection: {
      order: ['localStorage', 'navigator'],
      lookupLocalStorage: 'giosk_lang',
      caches: ['localStorage'],
    },
    // 번역을 지연 로딩하므로 리소스가 나중에 들어온다. bindI18nStore 를 켜지 않으면
    // addResourceBundle 로 번역이 도착해도 컴포넌트가 다시 그려지지 않아 영어가 그대로 남는다.
    react: { bindI18nStore: 'added' },
  });

// setLanguage 를 거치지 않고 언어가 바뀌는 경로(감지기 등)를 위한 그물이다.
// 번역이 없으면 받아오고, 도착하면 bindI18nStore 가 다시 그린다.
i18n.on('languageChanged', (lng) => {
  applyDir(lng);
  loadLanguage(lng);
});
applyDir(i18n.resolvedLanguage || i18n.language);

export default i18n;
