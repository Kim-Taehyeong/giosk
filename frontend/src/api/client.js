// 실 백엔드 HTTP 클라이언트(fetch 래퍼).
// 세션키는 localStorage 'giosk_sessionkey' 에 보관하고 Authorization 헤더로 전송한다.
// 에러는 { code, message } 를 가진 Error 로 던진다(프론트가 err.code 로 분기).
import { translateError } from '../i18n/translateError';

const BASE = import.meta.env.VITE_API_URL || 'http://localhost:8080/api';
const KEY = 'giosk_sessionkey';
// 스코프는 콘솔별로 분리 저장한다 — 관리자 콘솔(RoleSwitcher)과 사용자 콘솔(팀 선택)이 서로의
// 스코프를 오염시키면(예전엔 한 키를 공유) org 관리자가 사용자 뷰에서 고른 팀 스코프를 물려받아
// "내 조직이 안 보임 / 남의 팀이 보임" 같은 버그가 났다. 요청은 현재 콘솔(경로)에 맞는 키를 보낸다.
const SCOPE_KEY = 'giosk_console_scope';   // 관리자/매니저 콘솔 관리 스코프("org:10" | "group:2")
const USER_SCOPE_KEY = 'giosk_user_scope'; // 사용자 콘솔 활성 팀 스코프("group:2")

export const getSessionKey = () => localStorage.getItem(KEY);
export const setSessionKey = (k) => (k ? localStorage.setItem(KEY, k) : localStorage.removeItem(KEY));

const inAdminConsole = () => typeof window !== 'undefined' && window.location.pathname.startsWith('/console/admin');

// 관리자 콘솔 스코프(RoleSwitcher 전용).
export const getConsoleScope = () => localStorage.getItem(SCOPE_KEY);
export const setConsoleScope = (s) => (s ? localStorage.setItem(SCOPE_KEY, s) : localStorage.removeItem(SCOPE_KEY));
// 사용자 콘솔 팀 스코프(OrgGroupSelector 전용).
export const getUserScope = () => localStorage.getItem(USER_SCOPE_KEY);
export const setUserScope = (s) => (s ? localStorage.setItem(USER_SCOPE_KEY, s) : localStorage.removeItem(USER_SCOPE_KEY));
// 현재 콘솔(경로)에 맞는 스코프 — 요청 헤더/컨텍스트 초기화에 사용.
export const getScopeForRoute = () => (inAdminConsole() ? getConsoleScope() : getUserScope());
export const setScopeForRoute = (s) => (inAdminConsole() ? setConsoleScope(s) : setUserScope(s));

// wsURL은 웹터미널 등 WebSocket 접속 URL 을 만든다. http→ws / https→wss 변환하고,
// 브라우저 WebSocket 은 Authorization 헤더를 못 붙이므로 세션키를 access_token 쿼리로 실어 보낸다.
// BASE 가 상대경로('/api')면 현재 오리진을 붙여 same-origin 으로 접속(nginx 가 /api 를 API 로 프록시).
export function wsURL(path) {
  const abs = BASE.startsWith('http') ? BASE : window.location.origin + BASE;
  const u = new URL(abs + path);
  u.protocol = u.protocol === 'https:' ? 'wss:' : 'ws:';
  const key = getSessionKey();
  if (key) u.searchParams.set('access_token', key);
  return u.toString();
}

async function request(method, path, body) {
  const headers = { 'Content-Type': 'application/json' };
  const key = getSessionKey();
  if (key) headers.Authorization = `Bearer ${key}`;
  const scope = getScopeForRoute();
  if (scope) headers['X-Console-Scope'] = scope;

  const res = await fetch(BASE + path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });

  const text = await res.text();
  // 본문이 JSON 이 아닐 수 있다 — 라우트 미등록(gin 의 "404 page not found"), nginx 502/504 HTML,
  // 프록시 타임아웃 등. 그대로 JSON.parse 하면 SyntaxError 가 튀어 호출측 에러 처리가 통째로 무너진다.
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = null; }
  if (!res.ok) {
    const err = new Error(translateError(data?.error) || res.statusText || `HTTP ${res.status}`);
    err.code = data?.code;
    err.status = res.status;
    throw err;
  }
  return data;
}

// apiUpload는 multipart/form-data(파일 업로드)를 전송한다. Content-Type 은 브라우저가
// boundary 와 함께 자동 설정하므로 직접 지정하지 않는다(지정하면 boundary 누락으로 파싱 실패).
export async function apiUpload(path, formData) {
  const headers = {};
  const key = getSessionKey();
  if (key) headers.Authorization = `Bearer ${key}`;
  const scope = getScopeForRoute();
  if (scope) headers['X-Console-Scope'] = scope;
  const res = await fetch(BASE + path, { method: 'POST', headers, body: formData });
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = null; }
  if (!res.ok) {
    const err = new Error(translateError(data?.error) || res.statusText || `HTTP ${res.status}`);
    err.code = data?.code;
    err.status = res.status;
    throw err;
  }
  return data;
}

// apiUploadProgress는 업로드 진행률(%)을 실시간으로 콜백하는 multipart 전송이다.
// fetch 는 업로드 진행 이벤트를 노출하지 않으므로 XMLHttpRequest 를 쓴다. onProgress(pct 0~100).
// 대용량 파일을 올리는 동안 UI 가 멈춘 것처럼 보이지 않게 진행률을 준다.
export function apiUploadProgress(path, formData, onProgress) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open('POST', BASE + path);
    const key = getSessionKey();
    if (key) xhr.setRequestHeader('Authorization', `Bearer ${key}`);
    const scope = getScopeForRoute();
    if (scope) xhr.setRequestHeader('X-Console-Scope', scope);
    xhr.upload.onprogress = (e) => {
      if (onProgress && e.lengthComputable) onProgress(Math.round((e.loaded / e.total) * 100));
    };
    xhr.onload = () => {
      let data = null;
      try { data = xhr.responseText ? JSON.parse(xhr.responseText) : null; } catch { data = null; }
      if (xhr.status >= 200 && xhr.status < 300) { resolve(data); return; }
      const err = new Error(translateError(data?.error) || xhr.statusText || `HTTP ${xhr.status}`);
      err.code = data?.code; err.status = xhr.status;
      reject(err);
    };
    xhr.onerror = () => reject(new Error('network error'));
    xhr.send(formData);
  });
}

// apiPutRaw는 임의 바이너리 본문(Blob/청크)을 PUT 한다(청크 업로드용). JSON 이 아니라 원시 바디.
export async function apiPutRaw(path, body) {
  const headers = {};
  const key = getSessionKey();
  if (key) headers.Authorization = `Bearer ${key}`;
  const scope = getScopeForRoute();
  if (scope) headers['X-Console-Scope'] = scope;
  const res = await fetch(BASE + path, { method: 'PUT', headers, body });
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = null; }
  if (!res.ok) {
    const err = new Error(translateError(data?.error) || res.statusText || `HTTP ${res.status}`);
    err.code = data?.code; err.status = res.status;
    throw err;
  }
  return data;
}

export const apiGet = (path) => request('GET', path);
export const apiPost = (path, body) => request('POST', path, body);
export const apiPut = (path, body) => request('PUT', path, body);
export const apiPatch = (path, body) => request('PATCH', path, body);
export const apiDelete = (path) => request('DELETE', path);
