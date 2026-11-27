import assert from 'node:assert/strict';
import fs from 'node:fs';
import http from 'node:http';
import path from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(process.cwd() + '/package.json');
const { chromium } = require('playwright');

const distDir = path.resolve('web/dist');
const chromiumPath = process.env.SQLFOX_CHROMIUM || chromium.executablePath();
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

let browser;
try {
  browser = await chromium.launch({
    headless: true,
    executablePath: chromiumPath,
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
  });
  const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });

  const pageErrors = [];
  page.on('pageerror', (err) => pageErrors.push(err.message));

  await page.route('**/api/**', async (route) => {
    const url = route.request().url();
    const respond = (data) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(data) });

    if (url.includes('/logs/stream')) {
      return route.fulfill({ status: 200, contentType: 'text/event-stream', body: '' });
    }
    if (url.includes('/api/connections')) {
      return respond([{ id: 'conn-1', name: 'Local MySQL', type: 'mysql', host: '127.0.0.1', port: 3306, user: 'root', database: 'app', environment: 'dev' }]);
    }
    if (url.includes('/connect')) {
      return respond({
        session_id: 'conn-1',
        current_database: 'app',
        databases: ['app'],
        db_type: 'mysql',
        version: '8.0.35',
        schema: {
          database: 'app',
          databases: ['app'],
          tables: [
            { name: 'users', type: 'BASE TABLE', columns: [{ name: 'id', type: 'int' }] },
            { name: 'orders', type: 'BASE TABLE', columns: [{ name: 'id', type: 'int' }, { name: 'total_amount', type: 'decimal' }] },
            { name: 'invoices', type: 'BASE TABLE', columns: [{ name: 'id', type: 'int' }] },
          ],
          views: [],
          routines: [],
          events: [],
        },
      });
    }
    if (url.includes('/schema') || url.includes('/refresh-schema')) {
      return respond({
        database: 'app',
        databases: ['app'],
        tables: [
          { name: 'users', type: 'BASE TABLE', columns: [{ name: 'id', type: 'int' }] },
          { name: 'orders', type: 'BASE TABLE', columns: [{ name: 'id', type: 'int' }, { name: 'total_amount', type: 'decimal' }] },
          { name: 'invoices', type: 'BASE TABLE', columns: [{ name: 'id', type: 'int' }] },
        ],
        views: [],
        routines: [],
        events: [],
      });
    }
    if (url.includes('/fast-catalog')) {
      return respond({
        app: [
          { name: 'users', type: 'BASE TABLE', columns: [{ name: 'id', type: 'int' }] },
          { name: 'orders', type: 'BASE TABLE', columns: [{ name: 'id', type: 'int' }, { name: 'total_amount', type: 'decimal' }] },
          { name: 'invoices', type: 'BASE TABLE', columns: [{ name: 'id', type: 'int' }] },
        ],
      });
    }
    if (url.includes('/databases')) return respond({ databases: ['app'] });
    if (url.includes('/snippets') || url.includes('/history') || url.includes('/notes')) return respond([]);
    if (url.includes('/ai/')) return respond({ configured: false });
    return respond({});
  });

  await page.goto(`http://127.0.0.1:${port}`, { waitUntil: 'networkidle' });

  const connItem = page.getByText('Local MySQL').first();
  await connItem.waitFor({ timeout: 5000 });
  await connItem.dblclick();
  await page.waitForTimeout(500);

  const omnibarBtn = page.getByTitle(/Search tables/i).first();
  await omnibarBtn.waitFor({ timeout: 5000 });
  await omnibarBtn.click();

  const paletteInput = page.locator('input[placeholder*="Jump to a table" i]').first();
  await paletteInput.waitFor({ state: 'visible', timeout: 5000 });

  // 1. Table search (>= 2 chars) must filter and not throw
  await paletteInput.fill('orders');
  await page.waitForTimeout(150);

  const modalContainer = paletteInput.locator('xpath=ancestor::div[contains(@class,"fixed")]').first();
  const tableSearchText = await modalContainer.innerText();
  assert.strictEqual(pageErrors.length, 0, `Unexpected page errors: ${pageErrors.join('; ')}`);
  assert.ok(tableSearchText.includes('orders'), 'Expected "orders" in omni search results');
  assert.ok(!tableSearchText.includes('invoices'), 'Expected "invoices" to be filtered out when searching "orders"');
  assert.ok(!tableSearchText.includes('Shut Down SqlFox Daemon'), 'Expected unrelated actions to be filtered out');

  // 2. Column search (#prefix) must find matching columns across catalogs
  await paletteInput.fill('#total_amount');
  await page.waitForTimeout(150);

  const colSearchText = await modalContainer.innerText();
  assert.strictEqual(pageErrors.length, 0, `Unexpected page errors in column mode: ${pageErrors.join('; ')}`);
  assert.ok(colSearchText.includes('orders.total_amount'), 'Expected "orders.total_amount" in #column search results');

  console.log('PASS: Omni search filters tables and #columns without errors');
} finally {
  if (browser) await browser.close();
  server.close();
}
