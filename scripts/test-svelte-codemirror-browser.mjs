import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import http from 'node:http';
import { createRequire } from 'node:module';

const require = createRequire(process.cwd() + '/package.json');
const { buildSync } = require(process.cwd() + '/node_modules/esbuild');
const { chromium } = require('playwright');
const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'sqlfox-cm-browser-'));
const root = process.cwd();
const chromiumPath = process.env.SQLFOX_CHROMIUM || '/home/ian/.cache/ms-playwright/chromium-1187/chrome-linux/chrome';
if (!fs.existsSync(chromiumPath)) {
  console.log('SKIP: Chromium binary not found at ' + chromiumPath + ' (set SQLFOX_CHROMIUM to override)');
  process.exit(0);
}
const fixture = `
  import { codemirror } from ${JSON.stringify(root + '/web/frontend/src/lib/actions/codemirror.ts')};
  import { formatSQL } from ${JSON.stringify(root + '/web/frontend/src/lib/utils/sqlFormatter.ts')};
  const catalogs = {
    app: { database_name: 'app', tables: [
      { name: 'users', type: 'BASE TABLE', columns: [{name:'id',type:'int'},{name:'email',type:'text'}] },
      { name: 'orders', type: 'BASE TABLE', columns: [{name:'total',type:'decimal'}] },
    ]},
    analytics: { database_name: 'analytics', tables: [
      { name: 'users', type: 'BASE TABLE', columns: [{name:'analytics_id',type:'bigint'},{name:'region',type:'text'}] },
    ]},
  };
  const node = document.getElementById('editor');
  let options = {
    id: 'tab-1', doc: 'SELECT * FROM analytics.us', catalogs,
    snippets: [{id:'s1',title:'Active Users',sql:'SELECT 1;',tags:['users']}],
    onExecute: (sql) => window.calls.execute.push(sql),
    onExecuteAll: (sql) => window.calls.all.push(sql),
    onExplain: (sql) => window.calls.explain.push(sql),
    onFormat: (target) => window.controller.replaceTarget(formatSQL(target.text)),
    onSnippet: (snippet, insert) => { window.calls.snippets.push(snippet.title); insert(snippet.sql); },
    onChange: (doc) => { window.calls.changes++; options.doc = doc; },
    onReady: (controller) => { window.controller = controller; },
  };
  window.calls = { execute: [], all: [], explain: [], snippets: [], changes: 0 };
  const action = codemirror(node, options);
  window.cmUpdate = (patch) => { options = {...options, ...patch}; action.update(options); };
  window.cmDestroy = () => action.destroy();
`;
buildSync({ stdin: { contents: fixture, resolveDir: root, sourcefile: 'cm-browser-fixture.js', loader: 'js' }, outfile: path.join(dir, 'fixture.js'), bundle: true, format: 'iife', platform: 'browser', logLevel: 'silent' });
const html = '<!doctype html><html><body style="margin:0;background:#090d16"><div id="editor" style="height:400px;width:900px"></div><script src="/fixture.js"></script></body></html>';
const server = http.createServer((req, res) => {
  if (req.url === '/fixture.js') { res.setHeader('Content-Type', 'text/javascript'); res.end(fs.readFileSync(path.join(dir, 'fixture.js'))); }
  else { res.setHeader('Content-Type', 'text/html'); res.end(html); }
});
await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
const port = server.address().port;
let browser;
try {
  browser = await chromium.launch({ headless: true, executablePath: chromiumPath, args: ['--no-sandbox', '--disable-dev-shm-usage'] });
  const page = await browser.newPage({ viewport: { width: 1000, height: 650 } });
  const errors = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.goto(`http://127.0.0.1:${port}`);
  await page.locator('.cm-content').click();
  await page.keyboard.press('Control+End');
  await page.keyboard.press('Control+Space');
  await page.locator('.cm-tooltip-autocomplete').waitFor();
  await page.locator('.cm-tooltip-autocomplete li', { hasText: 'users' }).first().waitFor();
  const suggestions = await page.locator('.cm-tooltip-autocomplete li').allTextContents();
  assert(suggestions.some((text) => text.includes('users')));
  assert(!suggestions.some((text) => text.includes('orders')));
  assert(await page.locator('.cm-hint-badge').first().textContent() === 'TBL');
  await page.keyboard.press('Enter');
  assert.equal(await page.evaluate(() => window.controller.getDocument()), 'SELECT * FROM analytics.users');

  await page.evaluate(() => window.cmUpdate({ doc: 'SELECT analytics.users.' }));
  await page.locator('.cm-content').click();
  await page.keyboard.press('Control+End');
  await page.keyboard.press('Control+Space');
  await page.locator('.cm-tooltip-autocomplete').waitFor();
  await page.locator('.cm-tooltip-autocomplete li', { hasText: 'analytics_id' }).first().waitFor();
  const columns = await page.locator('.cm-completionLabel').allTextContents();
  assert.deepEqual(new Set(columns), new Set(['analytics_id', 'region']));
  await page.keyboard.press('Escape');

  await page.evaluate(() => window.cmUpdate({ doc: "SELECT 'analytics.users." }));
  await page.locator('.cm-content').click();
  await page.keyboard.press('Control+End');
  await page.keyboard.press('Control+Space');
  await page.waitForTimeout(100);
  assert.equal(await page.locator('.cm-tooltip-autocomplete').count(), 0);

  await page.evaluate(() => window.cmUpdate({ doc: 'SELECT 1;\nDELETE FROM logs;\nSELECT 3;' }));
  await page.locator('.cm-content').click();
  await page.keyboard.press('Control+Home');
  await page.keyboard.press('Control+Enter');
  assert.deepEqual(await page.evaluate(() => window.calls.execute), ['SELECT 1;']);
  await page.keyboard.press('Control+Shift+Enter');
  assert.deepEqual(await page.evaluate(() => window.calls.explain), ['SELECT 1;']);
  await page.keyboard.press('Control+Alt+Enter');
  assert.deepEqual(await page.evaluate(() => window.calls.all), ['SELECT 1;\nDELETE FROM logs;\nSELECT 3;']);
  await page.keyboard.press('F5');
  assert.equal(await page.evaluate(() => window.calls.all.length), 2);

  await page.keyboard.press('Control+Home');
  await page.keyboard.press('Control+Shift+f');
  const formatted = await page.evaluate(() => window.controller.getDocument());
  assert(formatted.startsWith('SELECT 1'));
  assert(formatted.includes('DELETE FROM logs;'));
  assert(formatted.includes('SELECT 3;'));

  await page.evaluate(() => window.cmUpdate({ doc: 'snip:Active' }));
  await page.locator('.cm-content').click();
  await page.keyboard.press('Control+End');
  await page.keyboard.press('Control+Space');
  await page.locator('.cm-tooltip-autocomplete li', { hasText: 'Active Users' }).first().waitFor();
  await page.keyboard.press('Enter');
  assert.deepEqual(await page.evaluate(() => window.calls.snippets), ['Active Users']);
  assert.equal(await page.evaluate(() => window.controller.getDocument()), 'SELECT 1;');

  // Per-tab state, undo and external updates.
  await page.evaluate(() => { window.calls.changes = 0; window.cmUpdate({ id: 'tab-1', doc: 'SELECT 1;' }); });
  await page.locator('.cm-content').click();
  await page.keyboard.press('Control+End');
  await page.keyboard.insertText(' SELECT 2;');
  await page.evaluate(() => window.cmUpdate({ id: 'tab-2', doc: 'SELECT 9;' }));
  await page.locator('.cm-content').click();
  await page.keyboard.press('Control+End');
  await page.keyboard.insertText(' SELECT 10;');
  await page.evaluate(() => window.cmUpdate({ id: 'tab-1', doc: 'SELECT 1; SELECT 2;' }));
  assert.equal(await page.evaluate(() => window.controller.getDocument()), 'SELECT 1; SELECT 2;');
  await page.keyboard.press('Control+z');
  assert.equal(await page.evaluate(() => window.controller.getDocument()), 'SELECT 1;');
  const changesBeforeExternal = await page.evaluate(() => window.calls.changes);
  await page.evaluate(() => window.cmUpdate({ doc: 'SELECT 42;' }));
  assert.equal(await page.evaluate(() => window.controller.getDocument()), 'SELECT 42;');
  assert.equal(await page.evaluate(() => window.calls.changes), changesBeforeExternal, 'external updates must not echo through onChange');

  // A stale document on a restored tab must be reconciled with the incoming doc.
  await page.evaluate(() => window.cmUpdate({ id: 'tab-9', doc: 'SELECT 5;' }));
  await page.evaluate(() => window.cmUpdate({ id: 'tab-8', doc: 'SELECT 6;' }));
  await page.evaluate(() => window.cmUpdate({ id: 'tab-9', doc: 'SELECT 55;' }));
  assert.equal(await page.evaluate(() => window.controller.getDocument()), 'SELECT 55;');

  // Dialect switching must reconfigure the restored tab's parser.
  await page.evaluate(() => window.cmUpdate({ id: 'tab-1', doc: 'SELECT 1;', dialect: 'postgres' }));
  await page.evaluate(() => window.cmUpdate({ id: 'tab-2', doc: 'SELECT 9;', dialect: 'mysql' }));
  await page.evaluate(() => window.cmUpdate({ id: 'tab-1', doc: 'SELECT 1;', dialect: 'postgres' }));
  const postgresDialect = await page.evaluate(() => window.controller.getDocument());
  assert.equal(postgresDialect, 'SELECT 1;');

  await page.evaluate(() => window.cmUpdate({ readOnly: true }));
  assert.equal(await page.locator('.cm-content').getAttribute('contenteditable'), 'false');
  await page.evaluate(() => window.cmUpdate({ readOnly: false }));
  await page.locator('.cm-content').click();
  await page.keyboard.press('Control+f');
  await page.locator('.cm-search').waitFor();
  await page.keyboard.press('Escape');

  assert(await page.locator('.cm-lineWrapping').count() > 0);
  const fontSize = await page.locator('.cm-editor').evaluate((element) => getComputedStyle(element).fontSize);
  assert.equal(fontSize, '14px');
  assert.deepEqual(errors, []);
  console.log('✅ Chromium CodeMirror smoke tests passed');
} finally {
  if (browser) await browser.close();
  await new Promise((resolve) => server.close(resolve));
  fs.rmSync(dir, { recursive: true, force: true });
}
