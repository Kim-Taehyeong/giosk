#!/usr/bin/env node
/**
 * i18n 자동 번역 — en/*.json 을 레지스트리의 모든 언어로 번역한다.
 *   - {{placeholder}}, <tags>, $t(...) 는 ZZ<n>ZZ 센티넬로 마스킹해 보존.
 *   - resumable: 대상 파일에 이미 있는 키는 건너뜀 → 재실행하면 이어서 채움(레이트리밋 대응).
 *   - 백엔드 선택(env BACKEND): google(무키, 기본) | libre(LT_URL) | deepl(DEEPL_API_KEY).
 *
 * 사용:
 *   node scripts/i18n-translate.mjs                 # 전체 언어
 *   node scripts/i18n-translate.mjs ja zh-Hans es   # 특정 언어만
 *   BACKEND=libre LT_URL=http://localhost:5000 node scripts/i18n-translate.mjs
 *   BACKEND=deepl DEEPL_API_KEY=... node scripts/i18n-translate.mjs
 */
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const LOCALES = path.resolve(__dirname, '../src/locales');
const NS = ['common', 'consoleAdmin', 'consoleUser', 'errors'];
const SRC = 'en';
const SKIP = new Set(['en', 'ko']); // 이미 사람이 관리하는 원본 로케일

// 레지스트리 코드 → 번역 API 코드 매핑(다르면).
const API_CODE = {
  'zh-Hans': { google: 'zh-CN', deepl: 'ZH' },
  'zh-Hant': { google: 'zh-TW', deepl: 'ZH' },
  'pt-BR': { google: 'pt', deepl: 'PT-BR' },
  nb: { google: 'no', deepl: 'NB' },
  fil: { google: 'tl', deepl: null },
};

// 레지스트리에서 코드만 뽑기(languages.js 를 정규식으로 읽음).
function registryCodes() {
  const src = fs.readFileSync(path.resolve(__dirname, '../src/i18n/languages.js'), 'utf8');
  return [...src.matchAll(/code:\s*'([^']+)'/g)].map((m) => m[1]);
}

const BACKEND = process.env.BACKEND || 'google';
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// ── 플레이스홀더 마스킹 (ZZ<n>ZZ: 번역기가 위치만 옮겨도 온전히 보존, RTL 포함 검증) ──
const PAT = /(\{\{[^}]+\}\}|<[^>]+>|\$t\([^)]+\))/g;
function mask(text) {
  const toks = [];
  const masked = text.replace(PAT, (m) => { toks.push(m); return `ZZ${toks.length - 1}ZZ`; });
  return { masked, toks };
}
function unmask(text, toks) {
  return text.replace(/ZZ(\d+)ZZ/g, (_, i) => toks[Number(i)] ?? '');
}

// ── 백엔드 어댑터 ──
async function tGoogle(text, to) {
  const url = `https://translate.googleapis.com/translate_a/single?client=gtx&sl=en&tl=${encodeURIComponent(to)}&dt=t&q=${encodeURIComponent(text)}`;
  const r = await fetch(url);
  if (!r.ok) throw new Error(`google ${r.status}`);
  const j = await r.json();
  return (j[0] || []).map((seg) => seg[0]).join('');
}
async function tLibre(text, to) {
  const r = await fetch(`${process.env.LT_URL || 'http://localhost:5000'}/translate`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ q: text, source: 'en', target: to, format: 'text' }),
  });
  if (!r.ok) throw new Error(`libre ${r.status}`);
  return (await r.json()).translatedText;
}
async function tDeepl(text, to) {
  const r = await fetch('https://api-free.deepl.com/v2/translate', {
    method: 'POST',
    headers: { Authorization: `DeepL-Auth-Key ${process.env.DEEPL_API_KEY}`, 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({ text, source_lang: 'EN', target_lang: to }),
  });
  if (!r.ok) throw new Error(`deepl ${r.status}`);
  return (await r.json()).translations[0].text;
}
const ADAPTER = { google: tGoogle, libre: tLibre, deepl: tDeepl }[BACKEND];
if (!ADAPTER) throw new Error(`unknown BACKEND=${BACKEND}`);

const apiCode = (code) => API_CODE[code]?.[BACKEND] ?? code.split('-')[0];

const CONCURRENCY = Number(process.env.CONCURRENCY || 10);

// 캐시엔 "진행 중 Promise"를 저장 → 동시에 같은 문자열 요청 시 중복 호출 방지.
const cache = new Map();
function translate(text, apiTo) {
  if (!text || !text.trim()) return Promise.resolve(text);
  const key = `${apiTo}::${text}`;
  if (cache.has(key)) return cache.get(key);
  const p = (async () => {
    const { masked, toks } = mask(text);
    for (let attempt = 0; attempt < 4; attempt++) {
      try { return unmask(await ADAPTER(masked, apiTo), toks); }
      catch (e) {
        if (attempt === 3) { console.warn(`  ! ${apiTo} 스킵: ${e.message}`); return null; } // null → 키 생략(en 폴백)
        await sleep(700 * (attempt + 1));
      }
    }
    return null;
  })();
  cache.set(key, p);
  return p;
}

// 동시성 풀 — items 를 concurrency 개씩 병렬 처리.
async function runPool(items, worker, concurrency) {
  let i = 0;
  await Promise.all(Array.from({ length: concurrency }, async () => {
    while (i < items.length) await worker(items[i++]);
  }));
}

// 번역할 리프(대상에 없는 문자열 키)만 수집 = resumable.
function collectTasks(srcNode, dstNode, tasks) {
  for (const [k, v] of Object.entries(srcNode)) {
    if (typeof v === 'string') {
      if (typeof dstNode[k] === 'string' && dstNode[k].trim()) continue;
      tasks.push({ parent: dstNode, key: k, text: v });
    } else if (v && typeof v === 'object') {
      dstNode[k] ||= Array.isArray(v) ? [] : {};
      collectTasks(v, dstNode[k], tasks);
    }
  }
}

async function main() {
  const args = process.argv.slice(2);
  const targets = (args.length ? args : registryCodes()).filter((c) => !SKIP.has(c));
  console.log(`백엔드=${BACKEND}, 동시성=${CONCURRENCY}, 대상 언어 ${targets.length}개`);
  const srcFiles = Object.fromEntries(NS.map((ns) => [ns, JSON.parse(fs.readFileSync(path.join(LOCALES, SRC, `${ns}.json`), 'utf8'))]));

  for (const code of targets) {
    const apiTo = apiCode(code);
    const dir = path.join(LOCALES, code);
    fs.mkdirSync(dir, { recursive: true });
    let total = 0;
    for (const ns of NS) {
      const file = path.join(dir, `${ns}.json`);
      const dst = fs.existsSync(file) ? JSON.parse(fs.readFileSync(file, 'utf8')) : {};
      const tasks = [];
      collectTasks(srcFiles[ns], dst, tasks);
      await runPool(tasks, async (t) => {
        const out = await translate(t.text, apiTo);
        if (out != null) t.parent[t.key] = out;
      }, CONCURRENCY);
      fs.writeFileSync(file, JSON.stringify(dst, null, 2) + '\n');
      total += tasks.length;
    }
    console.log(`✓ ${code} (api=${apiTo}) — ${total} keys`);
  }
  console.log('완료.');
}
main().catch((e) => { console.error(e); process.exit(1); });
