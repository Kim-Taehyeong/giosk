// 대규모 UI E2E — 전 라우트 렌더/요소/에러 + 모달/상세/역할별 네비 + 인터랙션. 수백 assert.
const { test } = require('@playwright/test');
const fs = require('fs');
const APP = 'http://localhost:5173';
let P = 0, Fbad = 0; const FAILS = [];
const ck = (c, l) => { if (c) P++; else { Fbad++; FAILS.push(l); } };
const IDS = JSON.parse(fs.readFileSync('/tmp/ui-ids.json', 'utf8'));
const ERRMARK = /something went wrong|error boundary|cannot read propert|is not a function|undefined is not|TypeError:|ReferenceError/i;

async function loginAs(page, u, pw) {
  await page.goto(APP + '/login');
  await page.fill('input[name=username]', u);
  await page.fill('input[name=password]', pw);
  await page.click('button[type=submit]');
  await page.waitForURL(/\/console/, { timeout: 15000 }).catch(() => {});
}

// 라우트 진입 후 공통 헬스체크(URL·에러마커·루트 렌더·헤딩) — 4 assert.
async function checkRoute(page, base, r, consoleErrs) {
  const before = consoleErrs.length;
  await page.goto(`${APP}${base}${r}`);
  await page.waitForLoadState('networkidle', { timeout: 15000 }).catch(() => {});
  await page.waitForTimeout(200);
  const url = page.url();
  ck(url.includes('/console'), `[${base}${r}] 콘솔 내 진입`);
  const body = (await page.textContent('body').catch(() => '')) || '';
  ck(!ERRMARK.test(body), `[${base}${r}] 에러마커 없음`);
  const rootKids = await page.locator('#root *, .console-root *, main *').count().catch(() => 0);
  ck(rootKids > 5, `[${base}${r}] 콘텐츠 렌더(${rootKids})`);
  const heads = await page.locator('h1,h2,h3,[class*=PageHead],[class*=page-head],.card h3').count().catch(() => 0);
  ck(heads > 0, `[${base}${r}] 헤딩/카드 존재(${heads})`);
  const newErrs = consoleErrs.slice(before).filter(e => !/favicon|manifest|ResizeObserver|404|Failed to load resource/i.test(e));
  ck(newErrs.length === 0, `[${base}${r}] 콘솔에러 없음(${newErrs.length}: ${newErrs[0] || ''})`);
}

test('UI 대규모 검증', async ({ page }) => {
  test.setTimeout(600000);
  const consoleErrs = [];
  page.on('console', m => { if (m.type() === 'error') consoleErrs.push(m.text()); });
  page.on('pageerror', e => consoleErrs.push('PAGEERROR: ' + e.message));

  const bodyHas = async (s) => (((await page.textContent('body').catch(() => '')) || '').includes(s));

  await loginAs(page, 'admin', 'giosk123');
  ck(page.url().includes('/console'), 'admin 로그인→콘솔');

  // ── A. 관리자 전 라우트 (목록/기능) ──
  const adminRoutes = ['dashboard/ops', 'dashboard/infra', 'sessions', 'nodes', 'volumes', 'users',
    'groups', 'orgs', 'resources', 'policies', 'datasets', 'images', 'announcements',
    'billing', 'audit', 'notifications', 'settings', 'manage-settings', 'approvals'];
  for (const r of adminRoutes) await checkRoute(page, '/console/admin/', r, consoleErrs);

  // ── B. 관리자 상세 라우트 (실 엔티티 id) ──
  const detail = [];
  if (IDS.org) detail.push('orgs/' + IDS.org);
  if (IDS.group) detail.push('groups/' + IDS.group);
  if (IDS.user) detail.push('users/' + IDS.user);
  if (IDS.session) detail.push('sessions/' + IDS.session);
  if (IDS.image) detail.push('images/' + IDS.image);
  const node1 = await page.evaluate(async () => {
    try { const r = await fetch('/api/admin/nodes', { headers: { Authorization: 'Bearer ' + localStorage.getItem('giosk_sessionkey') } }); const d = await r.json(); return (d.items || d || [])[0]?.node || (d[0] && d[0].node); } catch { return null; }
  }).catch(() => null);
  if (node1) detail.push('nodes/' + node1);
  for (const r of detail) await checkRoute(page, '/console/admin/', r, consoleErrs);

  // ── C. 사용자 전 라우트 ──
  const userRoutes = ['dashboard', 'sessions', 'sessions/new', 'volumes', 'datasets', 'wallet',
    'notifications', 'account', 'guide'];
  for (const r of userRoutes) await checkRoute(page, '/console/', r, consoleErrs);
  if (IDS.session) await checkRoute(page, '/console/', 'sessions/' + IDS.session, consoleErrs);

  // ── D. 관리자 사이드바 네비 항목(href 기반, 견고) ──
  await page.goto(APP + '/console/admin/dashboard/ops');
  await page.waitForLoadState('networkidle').catch(() => {});
  const navLinks = await page.locator('a[href*="/console/admin/"]').count().catch(() => 0);
  ck(navLinks >= 10, `관리자 사이드바 링크 ${navLinks}개`);
  for (const p of ['dashboard/ops', 'sessions', 'nodes', 'users', 'groups', 'orgs', 'resources',
    'policies', 'images', 'settings', 'audit', 'billing', 'datasets', 'announcements', 'volumes', 'notifications']) {
    const cnt = await page.locator(`a[href*="/console/admin/${p}"]`).count().catch(() => 0);
    ck(cnt > 0, `네비 링크 '${p}' 존재`);
  }

  // ── E. 상단바 요소 ──
  const bell = await page.locator('.topbar-bell, button[title*=알림], svg').count().catch(() => 0);
  ck(bell > 0, '상단바 벨/아이콘 존재');
  // 벨 클릭 → 드롭다운
  const bellBtn = page.locator('.topbar-bell').first();
  if (await bellBtn.count()) {
    await bellBtn.click().catch(() => {});
    await page.waitForTimeout(300);
    const dd = await page.getByText(/알림센터에서 전체 보기|받은 알림이 없습니다|알림/).count().catch(() => 0);
    ck(dd > 0, '벨 드롭다운 렌더');
    await page.keyboard.press('Escape').catch(() => {});
    await page.mouse.click(5, 5).catch(() => {});
  }

  // ── F. 모달/폼 열림 검증 ──
  // 조직 생성 모달
  await page.goto(APP + '/console/admin/orgs'); await page.waitForLoadState('networkidle').catch(() => {});
  const createOrgBtn = page.getByRole('button', { name: /조직 생성|생성|추가/ }).first();
  if (await createOrgBtn.count()) {
    await createOrgBtn.click().catch(() => {}); await page.waitForTimeout(300);
    ck(await page.locator('.modal, [class*=modal], .console-modal-bg').count() > 0, '조직 생성 모달 열림');
    ck(await page.locator('input').count() > 0, '조직 모달 입력필드');
    await page.keyboard.press('Escape').catch(() => {});
  }
  // 오퍼링/단가 페이지 모달
  await page.goto(APP + '/console/admin/resources'); await page.waitForLoadState('networkidle').catch(() => {});
  ck(await bodyHas('오퍼링') || await bodyHas('단가'), '오퍼링/단가 화면');
  const addOff = page.getByRole('button', { name: /오퍼링 추가|추가/ }).first();
  if (await addOff.count()) { await addOff.click().catch(() => {}); await page.waitForTimeout(300); ck(await page.locator('.modal, .console-modal-bg').count() > 0, '오퍼링 모달 열림'); await page.keyboard.press('Escape').catch(() => {}); }
  // 설정 화면 카드들
  await page.goto(APP + '/console/admin/settings'); await page.waitForLoadState('networkidle').catch(() => {});
  for (const t of ['브랜드', '리필', '운영 중', '이월']) ck(await bodyHas(t), `설정 '${t}' 섹션`);

  // ── G. 상세페이지 섹션 검증 ──
  if (IDS.org) {
    await page.goto(APP + '/console/admin/orgs/' + IDS.org); await page.waitForLoadState('networkidle').catch(() => {});
    for (const t of ['정기 크레딧', '크레딧', '그룹']) ck(await bodyHas(t), `조직상세 '${t}'`);
  }
  if (IDS.user) {
    await page.goto(APP + '/console/admin/users/' + IDS.user); await page.waitForLoadState('networkidle').catch(() => {});
    for (const t of ['소속', '권한', '리필', '세션']) ck(await bodyHas(t), `사용자상세 '${t}'`);
  }
  if (IDS.group) {
    await page.goto(APP + '/console/admin/groups/' + IDS.group); await page.waitForLoadState('networkidle').catch(() => {});
    for (const t of ['멤버', '리필']) ck(await bodyHas(t), `그룹상세 '${t}'`);
  }
  if (IDS.session) {
    await page.goto(APP + '/console/admin/sessions/' + IDS.session); await page.waitForLoadState('networkidle').catch(() => {});
    ck(!ERRMARK.test((await page.textContent('body')) || ''), '세션상세 에러없음');
  }

  // ── H. 사용자 화면 요소 ──
  await page.goto(APP + '/console/wallet'); await page.waitForLoadState('networkidle').catch(() => {});
  ck(await bodyHas('크레딧') || await bodyHas('잔액') || await bodyHas('리필'), '지갑 화면 크레딧 정보');
  await page.goto(APP + '/console/sessions/new'); await page.waitForLoadState('networkidle').catch(() => {});
  ck(await page.locator('button, .card, select, input').count() > 3, '세션 생성 폼 요소');
  await page.goto(APP + '/console/notifications'); await page.waitForLoadState('networkidle').catch(() => {});
  ck(await bodyHas('알림') || await bodyHas('규칙'), '알림센터 화면');

  console.log(`\nADMIN_PHASE PASS=${P} FAIL=${Fbad}`);

  // ── I. 역할별 UI: member ──
  await page.evaluate(() => localStorage.clear());
  await loginAs(page, 'ui_mb', 'giosk123');
  ck(page.url().includes('/console'), 'member 로그인→콘솔');
  // member 는 사용자 콘솔. 관리자 라우트 접근 시 리다이렉트/차단
  await page.goto(APP + '/console/admin/orgs'); await page.waitForLoadState('networkidle').catch(() => {});
  ck(!page.url().includes('/console/admin/orgs') || (await page.getByText(/조직/).count() === 0), 'member: 관리자 조직 접근 차단');
  await page.goto(APP + '/console/dashboard'); await page.waitForLoadState('networkidle').catch(() => {});
  ck(await page.locator('.card, main *').count() > 5, 'member: 사용자 대시보드 렌더');
  for (const r of ['sessions', 'wallet', 'volumes', 'notifications', 'account']) await checkRoute(page, '/console/', r, consoleErrs);

  // ── J. 역할별 UI: org_admin ──
  await page.evaluate(() => localStorage.clear());
  await loginAs(page, 'ui_oa', 'giosk123');
  ck(page.url().includes('/console'), 'org_admin 로그인→콘솔');
  await page.goto(APP + '/console/admin/dashboard/ops'); await page.waitForLoadState('networkidle').catch(() => {});
  ck(await page.locator('.card, main *').count() > 5, 'org_admin: 운영 대시보드 렌더');
  // 스코프 관리 라우트 접근
  for (const r of ['orgs', 'groups', 'users', 'billing', 'audit', 'policies']) await checkRoute(page, '/console/admin/', r, consoleErrs);
  // 인프라(플랫폼 전용) 접근 시도 — 차단/빈화면 허용
  await page.goto(APP + '/console/admin/nodes'); await page.waitForLoadState('networkidle').catch(() => {});
  ck(!ERRMARK.test((await page.textContent('body')) || ''), 'org_admin: 노드 접근시 크래시 없음');

  // ── K. member 전 사용자 라우트 풀 스윕 + 관리자 라우트 차단 ──
  await page.evaluate(() => localStorage.clear());
  await loginAs(page, 'ui_mb', 'giosk123');
  for (const r of ['dashboard', 'sessions', 'sessions/new', 'volumes', 'datasets', 'wallet', 'notifications', 'account', 'guide'])
    await checkRoute(page, '/console/', r, consoleErrs);
  for (const r of ['nodes', 'settings', 'billing', 'audit', 'policies', 'resources']) {
    await page.goto(APP + '/console/admin/' + r); await page.waitForLoadState('networkidle').catch(() => {});
    const u = page.url();
    ck(!ERRMARK.test((await page.textContent('body')) || ''), `member 관리자라우트 ${r} 크래시없음`);
    ck(!u.endsWith('/console/admin/' + r) || (await page.locator('main *').count() > 0), `member ${r} 접근처리`);
  }

  // ── L. org_admin 관리자 라우트 풀 스윕 ──
  await page.evaluate(() => localStorage.clear());
  await loginAs(page, 'ui_oa', 'giosk123');
  for (const r of ['dashboard/ops', 'orgs', 'groups', 'users', 'billing', 'audit', 'policies', 'announcements', 'volumes', 'notifications'])
    await checkRoute(page, '/console/admin/', r, consoleErrs);
  // org_admin 상세 접근
  if (IDS.org) await checkRoute(page, '/console/admin/', 'orgs/' + IDS.org, consoleErrs);
  if (IDS.user) await checkRoute(page, '/console/admin/', 'users/' + IDS.user, consoleErrs);

  // ── M. project_admin 스윕 ──
  await page.evaluate(() => localStorage.clear());
  await loginAs(page, 'ui_pa', 'giosk123');
  ck(page.url().includes('/console'), 'project_admin 로그인→콘솔');
  for (const r of ['dashboard/ops', 'groups', 'users', 'announcements', 'notifications'])
    await checkRoute(page, '/console/admin/', r, consoleErrs);

  // ── N. 다중 엔티티 상세 순회(admin) ──
  await page.evaluate(() => localStorage.clear());
  await loginAs(page, 'admin', 'giosk123');
  const orgIds = await page.evaluate(async () => {
    try { const r = await fetch('/api/console/orgs', { headers: { Authorization: 'Bearer ' + localStorage.getItem('giosk_sessionkey') } }); const d = await r.json(); return (d.items || []).slice(0, 5).map(x => x.id); } catch { return []; }
  }).catch(() => []);
  for (const id of orgIds) await checkRoute(page, '/console/admin/', 'orgs/' + id, consoleErrs);
  const userIds = await page.evaluate(async () => {
    try { const r = await fetch('/api/console/users?size=8', { headers: { Authorization: 'Bearer ' + localStorage.getItem('giosk_sessionkey') } }); const d = await r.json(); return (d.items || []).slice(0, 8).map(x => x.id); } catch { return []; }
  }).catch(() => []);
  for (const id of userIds) await checkRoute(page, '/console/admin/', 'users/' + id, consoleErrs);
  const grpIds = await page.evaluate(async () => {
    try { const r = await fetch('/api/console/groups', { headers: { Authorization: 'Bearer ' + localStorage.getItem('giosk_sessionkey') } }); const d = await r.json(); return (d.items || []).slice(0, 5).map(x => x.id); } catch { return []; }
  }).catch(() => []);
  for (const id of grpIds) await checkRoute(page, '/console/admin/', 'groups/' + id, consoleErrs);

  // ── O. 테마 토글 + 반응형 ──
  await page.goto(APP + '/console/admin/dashboard/ops'); await page.waitForLoadState('networkidle').catch(() => {});
  for (const vp of [{ width: 1440, height: 900 }, { width: 1024, height: 768 }, { width: 768, height: 1024 }]) {
    await page.setViewportSize(vp);
    await page.waitForTimeout(150);
    const scrollW = await page.evaluate(() => document.documentElement.scrollWidth);
    ck(scrollW <= vp.width + 40, `반응형 ${vp.width}px 가로overflow 없음(${scrollW})`);
    ck(!ERRMARK.test((await page.textContent('body')) || ''), `반응형 ${vp.width} 크래시없음`);
  }
  await page.setViewportSize({ width: 1440, height: 900 });

  console.log(`\nUI RESULT PASS=${P} FAIL=${Fbad} TOTAL=${P + Fbad}`);
  FAILS.slice(0, 60).forEach(f => console.log('  FAIL:', f));
});
