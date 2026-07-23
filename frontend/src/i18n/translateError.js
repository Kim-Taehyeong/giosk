import i18n from './index';
import { KO_TO_SLUG } from './errorMap';

// 백엔드가 내려준 한국어 에러 메시지를 현지화한다. 카탈로그에 없으면(변수 포함 등) 원문 유지.
// client.js 가 에러를 던지기 전에 호출 → 모든 표시 지점(토스트/에러문구)이 자동 번역된다.
export function translateError(msg) {
  if (!msg || typeof msg !== 'string') return msg;
  const slug = KO_TO_SLUG[msg.trim()];
  return slug ? i18n.t(slug, { ns: 'errors', defaultValue: msg }) : msg;
}
