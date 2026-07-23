// 모드별 UI 게이팅 E2E — 현재 API 모드를 읽고 그에 맞게 콘솔 라우트 게이팅을 검증한다.
// 오케스트레이터가 각 설치 모드로 API 를 재기동한 뒤 이 스펙을 돌린다.
const { test, expect } = require('@playwright/test');

const APP = 'http://localhost:5173';
const API = 'http://localhost:8080/api';

test('mode-based console gating', async ({ page, request }) => {
  const cfg = await (await request.get(`${API}/config`)).json();
  const mode = cfg.billing.mode;
  const dsOn = !!cfg.features.datasets;

  // 로그인(admin)
  await page.goto(`${APP}/login`);
  await page.fill('input[name=username]', 'admin');
  await page.fill('input[name=password]', 'giosk123');
  await page.click('button[type=submit]');
  await page.waitForURL(/\/console/, { timeout: 15000 });

  // 지갑(크레딧 모드 전용) — CreditOnlyRoute
  await page.goto(`${APP}/console/wallet`);
  await page.waitForLoadState('networkidle');
  const walletUrl = page.url();
  if (mode === 'credit') expect(walletUrl, 'credit: 지갑 렌더').toContain('/console/wallet');
  else expect(walletUrl, `${mode}: 지갑 리다이렉트`).not.toContain('/console/wallet');

  // 데이터셋(기능 on 전용) — DatasetRoute
  await page.goto(`${APP}/console/datasets`);
  await page.waitForLoadState('networkidle');
  const dsUrl = page.url();
  if (dsOn) expect(dsUrl, 'datasets ON: 렌더').toContain('/console/datasets');
  else expect(dsUrl, 'datasets OFF: 리다이렉트').not.toContain('/console/datasets');

  console.log(`  ✓ MODE=${mode} datasets=${dsOn} | wallet→${walletUrl.replace(APP, '')} datasets→${dsUrl.replace(APP, '')}`);
});
