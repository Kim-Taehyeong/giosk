const { defineConfig } = require('@playwright/test');
module.exports = defineConfig({
  testDir: './e2e',
  timeout: 30000,
  retries: 0,
  reporter: [['line']],
  use: { headless: true, actionTimeout: 10000, baseURL: 'http://localhost:5173', locale: 'ko-KR' },
});
