// 실 백엔드 HTTP 클라이언트(fetch 래퍼).
// 세션키는 localStorage 'giosk_sessionkey' 에 보관하고 Authorization 헤더로 전송한다.
// 에러는 { code, message } 를 가진 Error 로 던진다(프론트가 err.code 로 분기).
import { translateError } from '../i18n/translateError';

const BASE = import.meta.env.VITE_API_URL || 'http://localhost:8080/api';
const KEY = 'giosk_sessionkey';
const SCOPE_KEY = 'giosk_console_scope'; // 멀티롤 활성 스코프("org:10" | "group:2")

export const getSessionKey = () => localStorage.getItem(KEY);
export const setSessionKey = (k) => (k ? localStorage.setItem(KEY, k) : localStorage.removeItem(KEY));

// 활성 콘솔 스코프 — 멀티롤 사용자가 전환기로 고른 역할. /console/* 요청에 X-Console-Scope 로 전송.
export const getConsoleScope = () => localStorage.getItem(SCOPE_KEY);
export const setConsoleScope = (s) => (s ? localStorage.setItem(SCOPE_KEY, s) : localStorage.removeItem(SCOPE_KEY));

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
  const scope = getConsoleScope();
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
  const scope = getConsoleScope();
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
    const scope = getConsoleScope();
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
  const scope = getConsoleScope();
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
