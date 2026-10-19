const { chromium } = require('playwright');
const path = require('path');

const OUTPUT_DIR = path.resolve(__dirname, '../docs/assets/threads_screenshots');

async function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

(async () => {
  const browser = await chromium.launch({
    headless: true,
    executablePath: '/home/ian/.cache/ms-playwright/chromium-1187/chrome-linux/chrome'
  });

  const context = await browser.newContext({
    viewport: { width: 1540, height: 960 },
    deviceScaleFactor: 2
  });

  const page = await context.newPage();
  console.log('Navigating to SqlFox at http://127.0.0.1:8999...');
  await page.goto('http://127.0.0.1:8999');
  await page.waitForTimeout(2000);

  // Connect to our dummy database
  console.log('Connecting to Production Store DB...');
  await page.evaluate(async () => {
    if (window.connections && window.connections.length > 0) {
      await window.selectConnection(window.connections[0].id);
    }
  });
  await page.waitForTimeout(2000);

  // -------------------------------------------------------------
  // SHOT 1: HERO WORKSPACE (Editor with SQL + Result Virtual Grid)
  // -------------------------------------------------------------
  console.log('Generating Shot 1: Hero Workspace...');
  const heroSQL = `-- Top Customers by Lifetime Value (LTV) & Order Frequency
SELECT 
    u.id, 
    u.name, 
    u.email, 
    u.country,
    COUNT(o.id) AS total_orders, 
    PRINTF('$%,.2f', SUM(o.total_amount)) AS total_spent
FROM users u
LEFT JOIN orders o ON u.id = o.user_id
GROUP BY u.id
ORDER BY SUM(o.total_amount) DESC;`;

  await page.evaluate((sql) => {
    window.setEditorSQL(sql);
    window.runActiveQuery();
  }, heroSQL);

  await page.waitForTimeout(2500);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '01_hero_workspace.png'), fullPage: false });
  console.log('✅ Shot 1 captured: 01_hero_workspace.png');

  // -------------------------------------------------------------
  // SHOT 2: SAFETY GUARD (Destructive Interception Modal)
  // -------------------------------------------------------------
  console.log('Generating Shot 2: Safety Guard...');
  const dangerousSQL = `-- Accidental destructive statement on Production DB\nDROP TABLE users;`;
  await page.evaluate((sql) => {
    window.setEditorSQL(sql);
    window.openSafetyModal(sql, {
      reason: "DROP TABLE permanently destroys table schema, indexes, and all 10 customer records.",
      target_table: "users",
      target_database: "Production Store DB",
      requires_challenge: true,
      requires_affirmation: false,
      schema_scope_mismatch: false
    });
  }, dangerousSQL);

  await page.waitForTimeout(1000);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '02_safety_guard_destructive.png'), fullPage: false });
  console.log('✅ Shot 2 captured: 02_safety_guard_destructive.png');

  // Close safety modal
  await page.evaluate(() => {
    window.closeSafetyModal();
  });
  await page.waitForTimeout(800);

  // -------------------------------------------------------------
  // SHOT 3: VISUAL ERD CANVAS
  // -------------------------------------------------------------
  console.log('Generating Shot 3: Visual ERD Canvas...');
  await page.evaluate(async () => {
    await window.openERDCanvas();
  });
  await page.waitForTimeout(2500);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '03_visual_erd_canvas.png'), fullPage: false });
  console.log('✅ Shot 3 captured: 03_visual_erd_canvas.png');

  // Close ERD dialog
  await page.evaluate(() => {
    const dlg = document.getElementById('erd-dialog');
    if (dlg) dlg.close();
  });
  await page.waitForTimeout(800);

  // -------------------------------------------------------------
  // SHOT 4: VERTICAL RECORD INSPECTOR (Form View + JSON Payload)
  // -------------------------------------------------------------
  console.log('Generating Shot 4: Vertical Record Inspector...');
  const ordersSQL = `-- Inspect detailed order records with nested JSON metadata\nSELECT * FROM orders ORDER BY id ASC;`;
  await page.evaluate((sql) => {
    window.setEditorSQL(sql);
    window.runActiveQuery();
  }, ordersSQL);
  await page.waitForTimeout(2000);

  await page.evaluate(() => {
    window.openRecordInspector(0);
  });
  await page.waitForTimeout(1500);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '04_vertical_record_inspector.png'), fullPage: false });
  console.log('✅ Shot 4 captured: 04_vertical_record_inspector.png');

  // Close record inspector dialog
  await page.evaluate(() => {
    window.closeRecordInspector();
  });
  await page.waitForTimeout(800);

  // -------------------------------------------------------------
  // SHOT 5: AI COPILOT SIDEBAR ASSISTANT
  // -------------------------------------------------------------
  console.log('Generating Shot 5: AI Copilot Sidebar...');
  await page.evaluate((sql) => {
    window.setEditorSQL(sql);
    window.runActiveQuery();
    window.toggleAISidebar(true);
    const container = document.getElementById('ai-messages');
    if (container) {
      container.innerHTML = `
        <div class="flex flex-col space-y-3.5">
          <div class="bg-slate-800/90 p-3.5 rounded-lg border border-slate-700/70 text-xs text-slate-200 shadow-sm">
            <div class="font-bold text-sky-400 mb-1.5 flex items-center space-x-1.5">
              <span>👤</span><span>You</span>
            </div>
            How should I index the orders table to optimize customer lookup and date filtering?
          </div>
          <div class="bg-app-card p-3.5 rounded-lg border border-app-border text-xs text-slate-200 leading-relaxed space-y-2.5 shadow-sm">
            <div class="font-bold text-emerald-400 mb-1 flex items-center space-x-2">
              <span>🤖</span><span>SqlFox Copilot</span>
              <span class="text-[10px] bg-emerald-950 text-emerald-400 px-1.5 py-0.5 rounded border border-emerald-800 font-mono font-medium">ESR Rule Guided</span>
            </div>
            <p class="text-slate-300">Analyzing schema for <code>orders</code>: Queries frequently filter by equality on <code>user_id</code> and sort by timestamp on <code>created_at</code>.</p>
            <p class="text-slate-300">To achieve index-only lookups without sort spills, I recommend adding this composite index:</p>
            <div class="bg-slate-950 p-2.5 rounded border border-slate-800 font-mono text-[11px] text-sky-300 shadow-inner">
              CREATE INDEX idx_orders_user_created<br/>
              ON orders (user_id, created_at DESC);
            </div>
            <div class="pt-1 flex items-center space-x-2">
              <span class="bg-sky-600/30 hover:bg-sky-600/50 text-sky-300 border border-sky-500/40 px-2.5 py-1 rounded text-[11px] font-medium cursor-pointer">▶ Insert & Run</span>
              <span class="bg-slate-800 hover:bg-slate-700 text-slate-300 border border-slate-700 px-2.5 py-1 rounded text-[11px] cursor-pointer">📋 Copy DDL</span>
            </div>
          </div>
        </div>
      `;
    }
  }, heroSQL);

  await page.waitForTimeout(1500);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '05_ai_copilot_assistant.png'), fullPage: false });
  console.log('✅ Shot 5 captured: 05_ai_copilot_assistant.png');

  await browser.close();
  console.log('🎉 Finished capturing all 5 screenshots in high resolution!');
})();
