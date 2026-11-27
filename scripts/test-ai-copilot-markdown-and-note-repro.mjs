import fs from 'node:fs';
import path from 'node:path';
import http from 'node:http';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';

const require = createRequire(process.cwd() + '/package.json');
const { chromium } = require('playwright');

const distDir = path.resolve(process.cwd(), 'web/dist');
const chromiumPath = process.env.SQLFOX_CHROMIUM || chromium.executablePath();
if (!fs.existsSync(chromiumPath)) {
  console.log('SKIP: Chromium binary not found at ' + chromiumPath + ' (set SQLFOX_CHROMIUM to override)');
  process.exit(0);
}

const mimeTypes = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.json': 'application/json',
  '.woff2': 'font/woff2',
};

const aiResponseMarkdown = `### Query Analysis

Here is the **optimized** query using \`idx_users_email\`:

- Filters by indexed column
- Avoids full table scan

\`\`\`sql
SELECT id, email FROM users WHERE email = 'a@b.com';
\`\`\`

1. Step one
2. Step two`;

const server = http.createServer((req, res) => {
  const url = (req.url || '/').split('?')[0];
  if (url.startsWith('/api/')) {
    res.setHeader('Content-Type', 'application/json');
    if (url === '/api/health') return res.end(JSON.stringify({ status: 'ok', version: '0.18.0' }));
    if (url === '/api/ai/config') return res.end(JSON.stringify({ configured: true, provider: 'openai', model: 'gpt-4o' }));
    if (url === '/api/settings') return res.end(JSON.stringify({}));
    if (url === '/api/ai/chat' && req.method === 'POST') {
      return res.end(JSON.stringify({ reply: aiResponseMarkdown, response: aiResponseMarkdown, content: aiResponseMarkdown }));
    }
    return res.end(JSON.stringify([]));
  }

  let filePath = path.join(distDir, url === '/' ? 'index.html' : url);
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
  const page = await browser.newPage({ viewport: { width: 1400, height: 900 } });
  const pageErrors = [];
  page.on('pageerror', (e) => pageErrors.push(e.message));

  await page.goto(`http://127.0.0.1:${port}/`, { waitUntil: 'networkidle' });

  const aiBtn = page.locator('button:has-text("Copilot"), button[title*="Copilot"], button[title*="AI"]').first();
  await aiBtn.click();
  await page.waitForSelector('textarea[placeholder*="Ask Copilot"]', { timeout: 5000 });

  const input = page.locator('textarea[placeholder*="Ask Copilot"]');
  await input.fill('Tolong cek **query** dengan `SELECT * FROM users`');
  await input.press('Enter');

  await page.waitForSelector('button:has-text("Save to Notes")', { timeout: 5000 });

  const userBubbleInfo = await page.evaluate(() => {
    const bubbles = Array.from(document.querySelectorAll('div.rounded-xl'));
    const userBubble = bubbles.find((el) => el.textContent?.includes('Tolong cek'));
    if (!userBubble) return null;
    const cs = getComputedStyle(userBubble);
    const m = cs.backgroundColor.match(/\d+/g)?.map(Number) || [0, 0, 0];
    const luminance = (0.2126 * m[0] + 0.7152 * m[1] + 0.0722 * m[2]) / 255;
    return {
      bg: cs.backgroundColor,
      luminance,
      hasStrong: !!userBubble.querySelector('strong'),
      hasCode: !!userBubble.querySelector('code'),
      codeHasEm: !!userBubble.querySelector('code em'),
    };
  });

  const assistantInfo = await page.evaluate(() => {
    const bubbles = Array.from(document.querySelectorAll('div.rounded-xl'));
    const aiBubble = bubbles.find((el) => el.textContent?.includes('Query Analysis'));
    if (!aiBubble) return null;
    return {
      hasH3: !!aiBubble.querySelector('h3'),
      hasStrong: !!aiBubble.querySelector('strong'),
      hasPre: !!aiBubble.querySelector('pre'),
      codeHasEm: !!aiBubble.querySelector('code em'),
    };
  });

  await page.locator('button:has-text("Save to Notes")').first().click();
  await page.waitForSelector('textarea', { timeout: 5000 });

  const noteComposerContent = await page.evaluate(() => {
    const textareas = Array.from(document.querySelectorAll('textarea'));
    const noteTa = textareas.find((t) => t.value.includes('Query Analysis') || t.value.includes('Query Snapshot'));
    return noteTa ? noteTa.value : '';
  });

  await page.locator('button:has-text("Preview")').first().click();
  const previewHtml = await page.evaluate(() => {
    const previewBox = document.querySelector('div.min-h-\\[144px\\]');
    return {
      hasPreviewBox: !!previewBox,
      hasH3: !!previewBox?.querySelector('h3'),
      hasStrong: !!previewBox?.querySelector('strong'),
      hasPre: !!previewBox?.querySelector('pre'),
      hasUl: !!previewBox?.querySelector('ul'),
      codeHasEm: !!previewBox?.querySelector('code em'),
    };
  });

  assert.ok(userBubbleInfo && userBubbleInfo.luminance < 0.15, `User bubble background too bright/glaring: ${userBubbleInfo?.bg}`);
  assert.ok(userBubbleInfo.hasStrong && userBubbleInfo.hasCode, 'User bubble must render markdown (**bold** and `code`)');
  assert.equal(assistantInfo.codeHasEm, false, 'Inline code `idx_users_email` must not have <em> injected inside <code>');
  assert.equal(noteComposerContent.startsWith('### Query Snapshot\n\n```sql\n### Query Analysis'), false, 'Save to Notes from AI Copilot must not wrap markdown response in ```sql fence');
  assert.ok(previewHtml.hasPreviewBox && previewHtml.hasH3 && previewHtml.hasStrong && previewHtml.hasPre && previewHtml.hasUl, 'Note Preview must render markdown headings, bold, lists, and code blocks');
  assert.equal(previewHtml.codeHasEm, false, 'Note Preview inline code must not have <em> injected inside <code>');
  assert.equal(pageErrors.length, 0, `Unexpected page errors: ${pageErrors.join(', ')}`);

  console.log('✅ AI Copilot bubble, markdown inline code, and Save-to-Notes preview tests passed!');
} finally {
  if (browser) await browser.close();
  server.close();
}
