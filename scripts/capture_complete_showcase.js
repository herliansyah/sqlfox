const { chromium } = require('playwright');
const path = require('path');
const fs = require('fs');

const OUTPUT_DIR = path.resolve(__dirname, '../docs/assets/screenshots');
const THREADS_DIR = path.resolve(__dirname, '../docs/assets/threads_screenshots');

if (!fs.existsSync(OUTPUT_DIR)) fs.mkdirSync(OUTPUT_DIR, { recursive: true });
if (!fs.existsSync(THREADS_DIR)) fs.mkdirSync(THREADS_DIR, { recursive: true });

async function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

(async () => {
  console.log('🚀 Starting SqlFox Complete Showcase Capture Suite (Fresh High-Volume Data)...');

  const browser = await chromium.launch({
    headless: true,
    executablePath: '/home/ian/.cache/ms-playwright/chromium-1187/chrome-linux/chrome',
    args: ['--no-sandbox', '--disable-setuid-sandbox', '--disable-dev-shm-usage']
  });

  const context = await browser.newContext({
    viewport: { width: 1540, height: 960 },
    deviceScaleFactor: 2
  });

  const page = await context.newPage();
  console.log('Navigating to SqlFox at http://127.0.0.1:8999...');
  await page.goto('http://127.0.0.1:8999', { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(1500);

  // -------------------------------------------------------------------------
  // Connect cleanly to Production Store DB
  // -------------------------------------------------------------------------
  console.log('Connecting to Production Store DB...');
  const connInfo = await page.evaluate(async () => {
    // Dismiss any modal
    const alertDlg = document.getElementById('app-native-dialog');
    if (alertDlg && alertDlg.open) alertDlg.close();

    const prod = connections.find(c => c.environment === 'Production') || connections[0];
    await selectConnection(prod.id);
    return {
      id: prod.id,
      name: prod.name,
      active: activeConnection ? activeConnection.name : null
    };
  });
  console.log('Connected to session:', connInfo);
  await page.waitForTimeout(2500);

  // Helper to ensure no stray alert modal covers the view
  const dismissAlerts = async () => {
    await page.evaluate(() => {
      const alertDlg = document.getElementById('app-native-dialog');
      if (alertDlg && alertDlg.open) alertDlg.close();
    });
  };

  // =========================================================================
  // 1. HERO WORKSPACE (SQL Editor + High-Performance 10k Data Grid)
  // =========================================================================
  console.log('📸 [1/16] Capturing 01_hero_workspace.png...');
  const heroSQL = `-- Executive E-Commerce Analytics: Customer Lifetime Value (LTV) & Order Velocity
SELECT 
    c.id AS customer_id,
    c.customer_code,
    c.full_name AS customer_name,
    c.tier,
    c.country,
    COUNT(o.id) AS total_orders,
    PRINTF('$%,.2f', SUM(o.total_amount)) AS gross_revenue,
    PRINTF('$%,.2f', AVG(o.total_amount)) AS avg_order_value,
    MAX(o.created_at) AS latest_order_at
FROM customers c
INNER JOIN orders o ON c.id = o.customer_id
WHERE c.is_active = 1
GROUP BY c.id
ORDER BY SUM(o.total_amount) DESC;`;

  await page.evaluate((sql) => {
    setEditorSQL(sql);
    runActiveQuery();
  }, heroSQL);
  await page.waitForTimeout(3000);
  await dismissAlerts();
  await page.screenshot({ path: path.join(OUTPUT_DIR, '01_hero_workspace.png'), fullPage: false });
  fs.copyFileSync(path.join(OUTPUT_DIR, '01_hero_workspace.png'), path.join(THREADS_DIR, '01_hero_workspace.png'));

  // =========================================================================
  // 2. SAFETY GUARD (Destructive Query Challenge Modal)
  // =========================================================================
  console.log('📸 [2/16] Capturing 02_safety_guard_destructive.png...');
  const dangerousSQL = `-- Accidental destructive statement on Production DB\nDROP TABLE customers;`;
  await page.evaluate((sql) => {
    setEditorSQL(sql);
    openSafetyModal(sql, {
      reason: "DROP TABLE permanently destroys table schema, indexes, and all 2,500 active customer records.",
      target_table: "customers",
      target_database: "Production Store DB",
      requires_challenge: true,
      requires_affirmation: false,
      schema_scope_mismatch: false
    });
  }, dangerousSQL);
  await page.waitForTimeout(1000);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '02_safety_guard_destructive.png'), fullPage: false });
  fs.copyFileSync(path.join(OUTPUT_DIR, '02_safety_guard_destructive.png'), path.join(THREADS_DIR, '02_safety_guard_destructive.png'));
  await page.evaluate(() => closeSafetyModal());
  await page.waitForTimeout(600);

  // =========================================================================
  // 3. FLIGHT RECORDER (Live On-Demand Performance Telemetry)
  // =========================================================================
  console.log('📸 [3/16] Capturing 03_flight_recorder_telemetry.png...');
  await page.evaluate(() => {
    openFlightRecorderTab();
    
    // Inject active recording telemetry state with realistic live graph data
    const now = Date.now();
    const ticks = [];
    for (let i = 25; i >= 0; i--) {
      ticks.push({
        timestamp: new Date(now - i * 1000).toISOString(),
        qps: Math.floor(3900 + Math.random() * 850 + (i === 12 ? 1800 : 0)),
        threads_running: Math.floor(22 + Math.random() * 14 + (i === 12 ? 28 : 0)),
        threads_connected: 142,
        row_lock_waits: i === 12 ? 4 : (Math.random() > 0.8 ? 1 : 0),
        buffer_pool_wait_free: Math.random() > 0.85 ? 1 : 0,
        active_queries: [
          "SELECT * FROM orders WHERE customer_id = 450 ORDER BY created_at DESC LIMIT 50;",
          "UPDATE products SET stock_quantity = stock_quantity - 1 WHERE id = 88;",
          "INSERT INTO audit_logs (actor_email, action) VALUES ('api_service', 'ORDER_PLACED');"
        ]
      });
    }

    flightState = {
      isRecording: true,
      recordingId: 'rec-prod-20261104-01',
      name: 'Black Friday Peak Load Profiling',
      startedAt: new Date(now - 26000),
      durationSeconds: 26,
      sampleCount: 26,
      intervalMs: 1000,
      ticks: ticks,
      selectedSessionId: null
    };

    currentFlightQueries = [
      {
        fingerprint: 'SELECT * FROM orders WHERE customer_id = ? ORDER BY created_at DESC',
        call_count: 8420,
        avg_latency_ms: 1.45,
        max_latency_ms: 42.10,
        lock_wait_count: 0
      },
      {
        fingerprint: 'UPDATE products SET stock_quantity = stock_quantity - ? WHERE id = ?',
        call_count: 3210,
        avg_latency_ms: 4.80,
        max_latency_ms: 128.50,
        lock_wait_count: 4
      },
      {
        fingerprint: 'INSERT INTO audit_logs (actor_email, action, payload) VALUES (?, ?, ?)',
        call_count: 2950,
        avg_latency_ms: 0.85,
        max_latency_ms: 8.20,
        lock_wait_count: 0
      }
    ];

    if (typeof refreshFlightRecorderUI === 'function') refreshFlightRecorderUI();
    if (typeof updateFlightNavbarBadge === 'function') updateFlightNavbarBadge();
  });
  await dismissAlerts();
  await page.waitForTimeout(1500);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '03_flight_recorder_telemetry.png'), fullPage: false });

  // =========================================================================
  // 4. CLUSTER & REPLICATION INSPECTOR
  // =========================================================================
  console.log('📸 [4/16] Capturing 04_cluster_replication_inspector.png...');
  await page.evaluate(() => {
    if (activeConnection) activeConnection.driver = 'mysql';
    openClusterTab();
    clusterData = {
      cluster_name: 'prod-ecommerce-cluster',
      node_name: 'prod-node-01 (Primary Leader)',
      cluster_size: 3,
      wsrep_cluster_status: 'Primary',
      wsrep_ready: 'ON',
      wsrep_connected: 'ON',
      wsrep_local_state_comment: 'Synced',
      wsrep_flow_control_paused: 0.0000,
      replication_type: 'Multi-Region Galera Cluster + Read-Replica Async',
      replica_lag_seconds: 0.02,
      replica_io_running: 'Yes',
      replica_sql_running: 'Yes',
      last_io_error: '',
      bottleneck_severity: 'healthy',
      bottleneck_diagnosis: 'Galera multi-master mesh operates in full synchronization. Zero replication lag and zero flow-control stalls detected.',
      nodes: [
        { name: 'prod-node-01', address: '10.240.0.101:3306', state: 'Synced (Leader)', lag: 0.00, role: 'Primary Leader' },
        { name: 'prod-node-02', address: '10.240.0.102:3306', state: 'Synced', lag: 0.01, role: 'Active Peer' },
        { name: 'prod-node-03', address: '10.240.0.103:3306', state: 'Synced', lag: 0.02, role: 'Read Standby' }
      ]
    };
    if (typeof renderClusterUI === 'function') renderClusterUI();
  });
  await dismissAlerts();
  await page.waitForTimeout(1500);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '04_cluster_replication_inspector.png'), fullPage: false });

  // =========================================================================
  // 5. LOCK CONTENTION & ROOT BLOCKER TRIAGE
  // =========================================================================
  console.log('📸 [5/16] Capturing 05_lock_contention_root_blocker.png...');
  await page.evaluate(() => {
    processlistData = {
      locks: [
        {
          is_root_blocker: true,
          blocking_thread_id: 142,
          blocking_user: 'app_payment_worker',
          lock_mode: 'EXCLUSIVE ROW LOCK (X)',
          locked_resource: 'orders (row #10892)',
          blocking_query: 'UPDATE orders SET order_status = \'processing\', total_amount = 14850.00 WHERE id = 10892;',
          waiting_seconds: 14.8,
          waiting_thread_id: 284,
          waiting_user: 'api_checkout_svc',
          waiting_query: 'SELECT * FROM orders WHERE id = 10892 FOR UPDATE;'
        },
        {
          is_root_blocker: false,
          blocking_thread_id: 142,
          blocking_user: 'app_payment_worker',
          lock_mode: 'SHARED ROW LOCK (S)',
          locked_resource: 'orders (row #10892)',
          waiting_seconds: 8.4,
          waiting_thread_id: 305,
          waiting_user: 'analytics_etl_cron',
          waiting_query: 'SELECT SUM(total_amount) FROM orders WHERE customer_id = 450;'
        }
      ]
    };
    openLockInspectorModal();
  });
  await dismissAlerts();
  await page.waitForTimeout(1200);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '05_lock_contention_root_blocker.png'), fullPage: false });
  await page.evaluate(() => closeLockInspectorModal());
  await page.waitForTimeout(600);

  // =========================================================================
  // 6. REAL-TIME PROCESSLIST
  // =========================================================================
  console.log('📸 [6/16] Capturing 06_realtime_processlist.png...');
  await page.evaluate(() => {
    if (activeConnection) activeConnection.driver = 'mysql';
    openProcesslistTab();
    
    const threads = [
      { id: 142, user: 'app_payment_worker', host: '10.240.0.42:54120', db: 'production_store', command: 'Query', time: 14, state: 'Updating orders table', info: 'UPDATE orders SET order_status = \'processing\', total_amount = 14850.00 WHERE id = 10892;' },
      { id: 284, user: 'api_checkout_svc', host: '10.240.0.88:49210', db: 'production_store', command: 'Query', time: 8, state: 'Lock wait: row contention', info: 'SELECT * FROM orders WHERE id = 10892 FOR UPDATE;' },
      { id: 110, user: 'analytics_cron', host: '10.240.0.12:38901', db: 'production_store', command: 'Query', time: 3, state: 'Sending aggregated result sets', info: 'SELECT customer_id, COUNT(id), SUM(total_amount) FROM orders GROUP BY customer_id HAVING SUM(total_amount) > 10000;' },
      { id: 340, user: 'inventory_sync', host: '10.240.0.64:41200', db: 'production_store', command: 'Query', time: 1, state: 'Writing to binlog', info: 'UPDATE products SET stock_quantity = stock_quantity - 2 WHERE sku = \'SKU-492-0042\';' },
      { id: 412, user: 'audit_daemon', host: '10.240.0.19:55123', db: 'production_store', command: 'Execute', time: 0, state: 'Inserting records', info: 'INSERT INTO audit_logs (actor_email, action, ip_address, created_at) VALUES (\'secops@company.io\', \'CONFIG_READ\', \'10.240.0.5\', NOW());' },
      { id: 95, user: 'dba_admin', host: '10.240.0.5:60122', db: 'production_store', command: 'Sleep', time: 52, state: 'Idle connection pool', info: '' },
      { id: 104, user: 'web_session_mgr', host: '10.240.0.7:58911', db: 'production_store', command: 'Sleep', time: 120, state: 'Idle connection pool', info: '' }
    ];

    processlistData = {
      threads: threads,
      thread_count: 7,
      qps: 4210,
      locks: []
    };

    if (typeof renderProcesslistUI === 'function') renderProcesslistUI();
  });
  await dismissAlerts();
  await page.waitForTimeout(1500);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '06_realtime_processlist.png'), fullPage: false });

  // =========================================================================
  // 7. SMART SEED (FK-Aware Referential Topological Slicer)
  // =========================================================================
  console.log('📸 [7/16] Capturing 07_smart_seed_fk_exporter.png...');
  await page.evaluate(() => {
    openSmartSeedModal();
    const limitInput = document.getElementById('smart-seed-limit-input');
    if (limitInput) limitInput.value = '500';
  });
  await dismissAlerts();
  await page.waitForTimeout(1000);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '07_smart_seed_fk_exporter.png'), fullPage: false });
  await page.evaluate(() => closeSmartSeedModal());
  await page.waitForTimeout(600);

  // =========================================================================
  // 8. STRUCTURE SYNC (Schema Diff & Safe DDL Migration Generator)
  // =========================================================================
  console.log('📸 [8/16] Capturing 08_structure_sync_schema_diff.png...');
  await page.evaluate(() => {
    openStructureSyncModal();
    
    currentStructureDiffs = [
      {
        id: 'diff-1',
        type: 'ADD_COLUMN',
        table: 'orders',
        description: 'ADD COLUMN tax_exemption_code VARCHAR(64) NULL',
        sql: 'ALTER TABLE orders ADD COLUMN tax_exemption_code VARCHAR(64) NULL;',
        selected: true
      },
      {
        id: 'diff-2',
        type: 'ADD_INDEX',
        table: 'orders',
        description: 'CREATE INDEX idx_orders_customer_created ON orders (customer_id, created_at DESC)',
        sql: 'CREATE INDEX idx_orders_customer_created ON orders (customer_id, created_at DESC);',
        selected: true
      },
      {
        id: 'diff-3',
        type: 'ADD_COLUMN',
        table: 'customers',
        description: 'ADD COLUMN loyalty_points INT DEFAULT 0',
        sql: 'ALTER TABLE customers ADD COLUMN loyalty_points INT DEFAULT 0;',
        selected: true
      },
      {
        id: 'diff-4',
        type: 'MODIFY_COLUMN',
        table: 'products',
        description: 'ALTER COLUMN specifications JSON NOT NULL',
        sql: 'ALTER TABLE products MODIFY COLUMN specifications JSON NOT NULL;',
        selected: true
      }
    ];

    const diffContainer = document.getElementById('sync-struct-diff-container');
    if (diffContainer) diffContainer.classList.remove('hidden');

    const countSpan = document.getElementById('sync-struct-diff-count');
    if (countSpan) countSpan.textContent = '4 schema differences detected';

    const listEl = document.getElementById('sync-struct-diff-list');
    if (listEl) {
      listEl.innerHTML = currentStructureDiffs.map(d => `
        <label class="flex items-start space-x-2.5 p-2.5 rounded bg-slate-900 border border-slate-800 hover:border-slate-700 transition cursor-pointer">
          <input type="checkbox" checked class="mt-0.5 rounded border-slate-700 bg-slate-950 text-emerald-500 focus:ring-0 cursor-pointer">
          <div class="flex-1 min-w-0">
            <div class="flex items-center space-x-2">
              <span class="text-[10px] font-mono px-1.5 py-0.2 rounded font-semibold ${
                d.type === 'ADD_COLUMN' ? 'bg-sky-950 text-sky-400 border border-sky-800' :
                d.type === 'ADD_INDEX' ? 'bg-purple-950 text-purple-400 border border-purple-800' :
                'bg-amber-950 text-amber-400 border border-amber-800'
              }">${d.type}</span>
              <span class="font-bold text-slate-200 text-xs">${d.table}</span>
            </div>
            <div class="font-mono text-[11px] text-slate-400 mt-1 truncate">${d.description}</div>
          </div>
        </label>
      `).join('');
    }

    const applyBtn = document.getElementById('sync-struct-btn-apply');
    if (applyBtn) {
      applyBtn.disabled = false;
      applyBtn.textContent = '⚡ Execute Safe Migration (4 Statements)';
    }
  });
  await dismissAlerts();
  await page.waitForTimeout(1200);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '08_structure_sync_schema_diff.png'), fullPage: false });
  await page.evaluate(() => closeStructureSyncModal());
  await page.waitForTimeout(600);

  // =========================================================================
  // 9. CROSS-CONNECTION DATA TRANSFER
  // =========================================================================
  console.log('📸 [9/16] Capturing 09_cross_connection_data_transfer.png...');
  await page.evaluate(() => {
    openDataTransferModal();

    const listEl = document.getElementById('sync-transfer-table-list');
    if (listEl) {
      const demoTables = [
        { name: 'customers', rows: 2500, size: '420 KB' },
        { name: 'categories', rows: 8, size: '12 KB' },
        { name: 'products', rows: 1000, size: '280 KB' },
        { name: 'orders', rows: 10000, size: '2.1 MB' },
        { name: 'order_items', rows: 25000, size: '1.6 MB' },
        { name: 'audit_logs', rows: 5000, size: '520 KB' }
      ];

      listEl.innerHTML = demoTables.map((t) => `
        <label class="flex items-center justify-between p-2 rounded bg-slate-900 border border-slate-800 hover:border-slate-700 transition cursor-pointer">
          <div class="flex items-center space-x-2">
            <input type="checkbox" checked class="rounded border-slate-700 bg-slate-950 text-cyan-500 focus:ring-0">
            <span class="font-mono text-xs text-slate-200 font-semibold">${t.name}</span>
          </div>
          <div class="flex items-center space-x-3 text-[11px] text-slate-400">
            <span>${t.rows.toLocaleString()} rows</span>
            <span class="text-slate-500">•</span>
            <span class="font-mono text-sky-400">${t.size}</span>
          </div>
        </label>
      `).join('');
    }
  });
  await dismissAlerts();
  await page.waitForTimeout(1200);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '09_cross_connection_data_transfer.png'), fullPage: false });
  await page.evaluate(() => {
    const dlg = document.getElementById('data-transfer-dialog');
    if (dlg) dlg.close();
  });
  await page.waitForTimeout(600);

  // =========================================================================
  // 10. ROW-LEVEL DATA SYNC (Keyset Delta Reconciler)
  // =========================================================================
  console.log('📸 [10/16] Capturing 10_row_level_data_sync.png...');
  await page.evaluate(() => {
    openDataSyncModal();

    const resultBox = document.getElementById('sync-data-results');
    if (resultBox) {
      resultBox.classList.remove('hidden');
      resultBox.innerHTML = `
        <div class="p-3.5 bg-slate-900 border border-slate-700 rounded-lg space-y-3">
          <div class="flex items-center justify-between">
            <div class="font-bold text-slate-200 text-xs flex items-center space-x-2">
              <span class="w-2 h-2 rounded-full bg-emerald-400"></span>
              <span>Reconciliation Analysis: Table <code class="text-cyan-400">orders</code></span>
            </div>
            <span class="text-[11px] text-slate-400 font-mono">Algorithm: PK Keyset Hash Hashmatch</span>
          </div>
          <div class="grid grid-cols-3 gap-2.5 text-center">
            <div class="p-2.5 rounded bg-emerald-950/40 border border-emerald-800">
              <div class="text-xl font-bold font-mono text-emerald-400">+24</div>
              <div class="text-[10px] text-emerald-300 uppercase tracking-wide font-semibold">Missing (Insert)</div>
            </div>
            <div class="p-2.5 rounded bg-amber-950/40 border border-amber-800">
              <div class="text-xl font-bold font-mono text-amber-400">~8</div>
              <div class="text-[10px] text-amber-300 uppercase tracking-wide font-semibold">Modified (Update)</div>
            </div>
            <div class="p-2.5 rounded bg-rose-950/40 border border-rose-800">
              <div class="text-xl font-bold font-mono text-rose-400">-2</div>
              <div class="text-[10px] text-rose-300 uppercase tracking-wide font-semibold">Stale (Delete)</div>
            </div>
          </div>
          <div class="text-[11px] text-slate-400 bg-slate-950 p-2.5 rounded border border-slate-800">
            Preview: 24 new order transactions from Production are ready to be replicated into Staging with zero locking overhead.
          </div>
        </div>
      `;
    }
  });
  await dismissAlerts();
  await page.waitForTimeout(1200);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '10_row_level_data_sync.png'), fullPage: false });
  await page.evaluate(() => {
    const dlg = document.getElementById('data-sync-dialog');
    if (dlg) dlg.close();
  });
  await page.waitForTimeout(600);

  // =========================================================================
  // 11. VISUAL INTERACTIVE ERD CANVAS
  // =========================================================================
  console.log('📸 [11/16] Capturing 11_visual_erd_canvas.png...');
  await page.evaluate(async () => {
    await openERDCanvas();
  });
  await dismissAlerts();
  await page.waitForTimeout(3000);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '11_visual_erd_canvas.png'), fullPage: false });
  fs.copyFileSync(path.join(OUTPUT_DIR, '11_visual_erd_canvas.png'), path.join(THREADS_DIR, '03_visual_erd_canvas.png'));
  await page.evaluate(() => {
    const dlg = document.getElementById('erd-dialog');
    if (dlg) dlg.close();
  });
  await page.waitForTimeout(600);

  // =========================================================================
  // 12. VISUAL TABLE DESIGNER (In-Place Column & Index Builder)
  // =========================================================================
  console.log('📸 [12/16] Capturing 12_visual_table_designer.png...');
  await page.evaluate(() => {
    openTableViewer('orders');
    setTableMode('structure');
  });
  await dismissAlerts();
  await page.waitForTimeout(2000);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '12_visual_table_designer.png'), fullPage: false });

  // =========================================================================
  // 13. STORAGE & HEALTH DIAGNOSTICS
  // =========================================================================
  console.log('📸 [13/16] Capturing 13_storage_health_diagnostics.png...');
  await page.evaluate(() => {
    openDatabaseDiagnosticsModal();
  });
  await dismissAlerts();
  await page.waitForTimeout(1800);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '13_storage_health_diagnostics.png'), fullPage: false });
  await page.evaluate(() => {
    const dlg = document.getElementById('db-diagnostics-dialog');
    if (dlg) dlg.close();
  });
  await page.waitForTimeout(600);

  // =========================================================================
  // 14. SERVER VARIABLES & TUNING ADVISOR
  // =========================================================================
  console.log('📸 [14/16] Capturing 14_server_variables_tuning.png...');
  await page.evaluate(() => {
    openServerVariablesTab();

    serverVariablesData = [
      { name: 'innodb_buffer_pool_size', value: '17,179,869,184 (16 GB)', category: 'memory', is_dynamic: true, description: 'Allocates memory buffer pool for caching table data and secondary indexes.', advice: 'Optimal for server with 32GB RAM (~50% memory threshold).' },
      { name: 'max_connections', value: '2500', category: 'connection', is_dynamic: true, description: 'Maximum permitted number of simultaneous client connections.', advice: 'Thread cache sizing should align with peak application pool.' },
      { name: 'innodb_flush_log_at_trx_commit', value: '1 (Strict ACID)', category: 'durability', is_dynamic: true, description: 'Redo log written and flushed to disk at each transaction commit.', advice: 'Required for non-negotiable financial audit durability.' },
      { name: 'tmp_table_size', value: '67,108,864 (64 MB)', category: 'memory', is_dynamic: true, description: 'Maximum size of internal in-memory temporary tables.', advice: 'Prevents complex join disk spills.' },
      { name: 'long_query_time', value: '0.500000 s', category: 'query', is_dynamic: true, description: 'Threshold in seconds above which queries are logged to slow query log.', advice: 'Recommended: <= 1.0s for OLTP workloads.' }
    ];

    if (typeof renderServerVariablesUI === 'function') renderServerVariablesUI();
  });
  await dismissAlerts();
  await page.waitForTimeout(1500);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '14_server_variables_tuning.png'), fullPage: false });

  // =========================================================================
  // 15. VERTICAL RECORD INSPECTOR (Form View + Nested JSON)
  // =========================================================================
  console.log('📸 [15/16] Capturing 15_vertical_record_inspector.png...');
  await page.evaluate(() => {
    const queryTab = tabs.find(t => !t.isFlightRecorder && !t.isCluster && !t.isVariables && !t.isTableViewer);
    if (queryTab) switchTab(queryTab.id);

    const ordersSQL = `-- Inspect detailed order records with rich shipping & payment payloads\nSELECT * FROM orders ORDER BY id ASC LIMIT 50;`;
    setEditorSQL(ordersSQL);
    runActiveQuery();
  });
  await page.waitForTimeout(2500);
  await page.evaluate(() => {
    openRecordInspector(0);
  });
  await dismissAlerts();
  await page.waitForTimeout(1500);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '15_vertical_record_inspector.png'), fullPage: false });
  fs.copyFileSync(path.join(OUTPUT_DIR, '15_vertical_record_inspector.png'), path.join(THREADS_DIR, '04_vertical_record_inspector.png'));
  await page.evaluate(() => closeRecordInspector());
  await page.waitForTimeout(600);

  // =========================================================================
  // 16. AI COPILOT ASSISTANT (In-Context Index Tuning & Query Optimizer)
  // =========================================================================
  console.log('📸 [16/16] Capturing 16_ai_copilot_assistant.png...');
  await page.evaluate((sql) => {
    setEditorSQL(sql);
    toggleAISidebar(true);
    const container = document.getElementById('ai-messages');
    if (container) {
      container.innerHTML = `
        <div class="flex flex-col space-y-3.5">
          <div class="bg-slate-800/90 p-3.5 rounded-lg border border-slate-700/70 text-xs text-slate-200 shadow-sm">
            <div class="font-bold text-sky-400 mb-1.5 flex items-center space-x-1.5">
              <span>👤</span><span>You</span>
            </div>
            How should I index the orders table with 10,000+ records to eliminate sort spills and optimize customer lookup with date ranges?
          </div>
          <div class="bg-app-card p-3.5 rounded-lg border border-app-border text-xs text-slate-200 leading-relaxed space-y-2.5 shadow-sm">
            <div class="font-bold text-emerald-400 mb-1 flex items-center space-x-2">
              <span>🤖</span><span>SqlFox Copilot</span>
              <span class="text-[10px] bg-emerald-950 text-emerald-400 px-1.5 py-0.5 rounded border border-emerald-800 font-mono font-medium">ESR Rule Guided</span>
            </div>
            <p class="text-slate-300">Analyzing schema & execution plan for <code>orders</code>: Frequent queries filter on <code>customer_id</code> (Equality) and sort by <code>created_at DESC</code> (Sort).</p>
            <p class="text-slate-300">Applying the <strong>ESR (Equality, Sort, Range)</strong> rule avoids temporary tables and filesorts. Here is the recommended zero-downtime composite index:</p>
            <div class="bg-slate-950 p-2.5 rounded border border-slate-800 font-mono text-[11px] text-sky-300 shadow-inner">
              CREATE INDEX idx_orders_customer_created<br/>
              ON orders (customer_id, created_at DESC);
            </div>
            <p class="text-[11px] text-slate-400">Estimated query speedup: <strong>~24x faster</strong> (Index-only seek vs full table scan).</p>
            <div class="pt-1 flex items-center space-x-2">
              <span class="bg-sky-600/30 hover:bg-sky-600/50 text-sky-300 border border-sky-500/40 px-2.5 py-1 rounded text-[11px] font-medium cursor-pointer">▶ Insert & Run</span>
              <span class="bg-slate-800 hover:bg-slate-700 text-slate-300 border border-slate-700 px-2.5 py-1 rounded text-[11px] cursor-pointer">📋 Copy DDL</span>
            </div>
          </div>
        </div>
      `;
    }
  }, heroSQL);
  await dismissAlerts();
  await page.waitForTimeout(1500);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '16_ai_copilot_assistant.png'), fullPage: false });
  fs.copyFileSync(path.join(OUTPUT_DIR, '16_ai_copilot_assistant.png'), path.join(THREADS_DIR, '05_ai_copilot_assistant.png'));

  await browser.close();
  console.log('🎉 ALL 16 HIGH-RESOLUTION SHOWCASE SCREENSHOTS CAPTURED WITH FULL ACTIVE DATA!');
})().catch(err => {
  console.error('❌ Error during capture:', err);
  process.exit(1);
});
