import assert from 'node:assert/strict';
import fs from 'node:fs';
import http from 'node:http';
import path from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(process.cwd() + '/package.json');
const { chromium } = require('playwright');

const distDir = path.resolve('web/dist');
const chromiumPath = process.env.SQLFOX_CHROMIUM || '/home/ian/.cache/ms-playwright/chromium-1187/chrome-linux/chrome';
if (!fs.existsSync(chromiumPath)) {
  console.log('SKIP: Chromium binary not found at ' + chromiumPath + ' (set SQLFOX_CHROMIUM to override)');
  process.exit(0);
}

const mimeTypes = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'application/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.json': 'application/json',
  '.woff2': 'font/woff2',
};

const server = http.createServer((req, res) => {
  const urlPath = (req.url || '/').split('?')[0];
  let filePath = path.join(distDir, urlPath === '/' ? 'index.html' : urlPath);
  if (!fs.existsSync(filePath) || fs.statSync(filePath).isDirectory()) {
    filePath = path.join(distDir, 'index.html');
  }
  const ext = path.extname(filePath);
  res.setHeader('Content-Type', mimeTypes[ext] || 'application/octet-stream');
  res.end(fs.readFileSync(filePath));
});

await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
const port = server.address().port;

const mockThreads = [
  { id: '101', user: 'root', host: '127.0.0.1:50001', db: 'app', command: 'Query', time: 12, state: 'executing', info: 'SELECT * FROM users WHERE id = 1', trx_state: 'RUNNING', trx_started: '2026-11-17 08:00:00' },
  { id: '102', user: 'app_user', host: '127.0.0.1:50002', db: 'app', command: 'Query', time: 4, state: 'Sending data', info: 'SELECT * FROM orders ORDER BY created_at DESC LIMIT 100', trx_state: 'RUNNING', trx_started: '2026-11-17 08:00:08' },
  { id: '103', user: 'app_user', host: '127.0.0.1:50003', db: 'app', command: 'Sleep', time: 30, state: '', info: '', trx_state: '', trx_started: '' },
];

let browser;
try {
  browser = await chromium.launch({
    headless: true,
    executablePath: chromiumPath,
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
  });
  const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });

  await page.route('**/api/**', async (route) => {
    const url = route.request().url();
    const respond = (data) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(data) });

    if (url.includes('/api/connections')) {
      return respond([{ id: 'conn-1', name: 'Local MySQL', type: 'mysql', host: '127.0.0.1', port: 3306, user: 'root', database: 'app', environment: 'dev' }]);
    }
    if (url.includes('/connect')) {
      return respond({ session_id: 'sess-1', database: 'app', databases: ['app'], db_type: 'mysql', version: '8.0.35' });
    }
    if (url.includes('/databases')) return respond({ databases: ['app'] });
    if (url.includes('/tables')) return respond({ tables: [{ name: 'users', type: 'BASE TABLE' }] });
    if (url.includes('/processlist')) {
      return respond({ threads: mockThreads, capabilities: { can_kill_query: true, can_kill_connection: true, can_explain: true } });
    }
    if (url.includes('/snippets') || url.includes('/history') || url.includes('/notes')) return respond([]);
    if (url.includes('/ai/')) return respond({ configured: false });
    return respond({});
  });

  await page.goto(`http://127.0.0.1:${port}`, { waitUntil: 'networkidle' });

  const connItem = page.getByText('Local MySQL').first();
  await connItem.waitFor({ timeout: 5000 });
  await connItem.dblclick();

  const monitorBtn = page.getByRole('button', { name: /Monitor/i }).first();
  await monitorBtn.waitFor({ timeout: 5000 });
  await monitorBtn.click();

  const processlistItem = page.getByText('Processlist').first();
  await processlistItem.waitFor({ timeout: 5000 });
  await processlistItem.click();

  const menuBtn = page.locator('button[aria-label="Thread 101 menu"]');
  await menuBtn.waitFor({ timeout: 5000 });
  await menuBtn.click();

  await page.locator('button[aria-label="Close thread menu"]').waitFor({ timeout: 3000 });

  const result = await page.evaluate(() => {
    const btn101 = document.querySelector('button[aria-label="Thread 101 menu"]');
    const parent = btn101?.parentElement;
    const targetMenu = Array.from(parent?.children || []).find(
      (el) => el.tagName === 'DIV'
    );
    if (!targetMenu) return { found: false, itemCount: 0, occluded: [] };

    const items = Array.from(targetMenu.querySelectorAll('button, a, [role="menuitem"]'));
    const occluded = [];
    for (const item of items) {
      const rect = item.getBoundingClientRect();
      if (rect.width === 0 || rect.height === 0) continue;
      for (const fx of [0.15, 0.5, 0.85, 0.95]) {
        const cx = rect.left + rect.width * fx;
        const cy = rect.top + rect.height / 2;
        const topEl = document.elementFromPoint(cx, cy);
        if (topEl && !targetMenu.contains(topEl) && topEl !== targetMenu) {
          occluded.push({ itemText: item.textContent?.trim(), fx, coveredByTag: topEl.tagName });
        }
      }
    }
    return { found: true, itemCount: items.length, occluded };
  });

  assert(result.found, 'Thread action menu should be found in DOM');
  assert(result.itemCount > 0, 'Thread action menu should have items');
  assert.equal(result.occluded.length, 0, `Expected 0 occluded menu items, got: ${JSON.stringify(result.occluded)}`);
  console.log('✅ Processlist row action menu occlusion regression test passed');
} finally {
  if (browser) await browser.close();
  server.close();
}
