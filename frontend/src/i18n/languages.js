// 지원 언어 레지스트리다. 스위처 목록과 i18n supportedLngs 의 단일 출처다.
//   code: BCP-47(간략), native: 자국어 표기, en: 영어명, flag: 대표 국기 이모지, dir: 'rtl' 만 표기(기본 ltr).
// 실제 번역 파일이 있는 언어는 그 번역을, 없으면 fallback(en)으로 우아하게 표시된다.
export const LANGUAGES = [
  { code: 'en', native: 'English', en: 'English', flag: '🇺🇸' },
  { code: 'ko', native: '한국어', en: 'Korean', flag: '🇰🇷' },
  { code: 'ja', native: '日本語', en: 'Japanese', flag: '🇯🇵' },
  { code: 'zh-Hans', native: '简体中文', en: 'Chinese (Simplified)', flag: '🇨🇳' },
  { code: 'zh-Hant', native: '繁體中文', en: 'Chinese (Traditional)', flag: '🇹🇼' },
  { code: 'es', native: 'Español', en: 'Spanish', flag: '🇪🇸' },
  { code: 'fr', native: 'Français', en: 'French', flag: '🇫🇷' },
  { code: 'de', native: 'Deutsch', en: 'German', flag: '🇩🇪' },
  { code: 'pt', native: 'Português', en: 'Portuguese', flag: '🇵🇹' },
  { code: 'pt-BR', native: 'Português (Brasil)', en: 'Portuguese (Brazil)', flag: '🇧🇷' },
  { code: 'it', native: 'Italiano', en: 'Italian', flag: '🇮🇹' },
  { code: 'ru', native: 'Русский', en: 'Russian', flag: '🇷🇺' },
  { code: 'uk', native: 'Українська', en: 'Ukrainian', flag: '🇺🇦' },
  { code: 'pl', native: 'Polski', en: 'Polish', flag: '🇵🇱' },
  { code: 'nl', native: 'Nederlands', en: 'Dutch', flag: '🇳🇱' },
  { code: 'sv', native: 'Svenska', en: 'Swedish', flag: '🇸🇪' },
  { code: 'tr', native: 'Türkçe', en: 'Turkish', flag: '🇹🇷' },
  { code: 'ar', native: 'العربية', en: 'Arabic', flag: '🇸🇦', dir: 'rtl' },
  { code: 'he', native: 'עברית', en: 'Hebrew', flag: '🇮🇱', dir: 'rtl' },
  { code: 'fa', native: 'فارسی', en: 'Persian', flag: '🇮🇷', dir: 'rtl' },
  { code: 'ur', native: 'اردو', en: 'Urdu', flag: '🇵🇰', dir: 'rtl' },
  { code: 'hi', native: 'हिन्दी', en: 'Hindi', flag: '🇮🇳' },
  { code: 'bn', native: 'বাংলা', en: 'Bengali', flag: '🇧🇩' },
  { code: 'ta', native: 'தமிழ்', en: 'Tamil', flag: '🇮🇳' },
  { code: 'te', native: 'తెలుగు', en: 'Telugu', flag: '🇮🇳' },
  { code: 'th', native: 'ไทย', en: 'Thai', flag: '🇹🇭' },
  { code: 'vi', native: 'Tiếng Việt', en: 'Vietnamese', flag: '🇻🇳' },
  { code: 'id', native: 'Bahasa Indonesia', en: 'Indonesian', flag: '🇮🇩' },
  { code: 'ms', native: 'Bahasa Melayu', en: 'Malay', flag: '🇲🇾' },
  { code: 'fil', native: 'Filipino', en: 'Filipino', flag: '🇵🇭' },
  { code: 'my', native: 'မြန်မာ', en: 'Burmese', flag: '🇲🇲' },
  { code: 'km', native: 'ខ្មែរ', en: 'Khmer', flag: '🇰🇭' },
  { code: 'el', native: 'Ελληνικά', en: 'Greek', flag: '🇬🇷' },
  { code: 'cs', native: 'Čeština', en: 'Czech', flag: '🇨🇿' },
  { code: 'ro', native: 'Română', en: 'Romanian', flag: '🇷🇴' },
  { code: 'hu', native: 'Magyar', en: 'Hungarian', flag: '🇭🇺' },
  { code: 'fi', native: 'Suomi', en: 'Finnish', flag: '🇫🇮' },
  { code: 'da', native: 'Dansk', en: 'Danish', flag: '🇩🇰' },
  { code: 'nb', native: 'Norsk', en: 'Norwegian', flag: '🇳🇴' },
  { code: 'sw', native: 'Kiswahili', en: 'Swahili', flag: '🇰🇪' },
];

// RTL 언어 코드 집합(문서 방향 전환용).
export const RTL_LANGS = new Set(LANGUAGES.filter((l) => l.dir === 'rtl').map((l) => l.code));

// 지원 코드 목록(i18n supportedLngs).
export const SUPPORTED_CODES = LANGUAGES.map((l) => l.code);

export const langMeta = (code) => {
  const base = (code || '').split('-')[0];
  return LANGUAGES.find((l) => l.code === code) || LANGUAGES.find((l) => l.code.split('-')[0] === base);
};

// 국기 이모지(regional indicator 2글자)를 ISO 3166 alpha-2 소문자로 바꾼다(flag-icons 클래스용).
export const flagCC = (flag) => {
  const cp = [...(flag || '')].map((c) => c.codePointAt(0));
  if (cp.length === 2 && cp[0] >= 0x1f1e6 && cp[0] <= 0x1f1ff) {
    return String.fromCharCode(cp[0] - 0x1f1e6 + 97) + String.fromCharCode(cp[1] - 0x1f1e6 + 97);
  }
  return '';
};
