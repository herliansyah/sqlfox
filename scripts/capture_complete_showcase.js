const { chromium } = require('playwright');
const path = require('path');
const fs = require('fs');

const OUTPUT_DIR = path.resolve(__dirname, '../docs/assets/screenshots');
if (!fs.existsSync(OUTPUT_DIR)) fs.mkdirSync(OUTPUT_DIR, { recursive: true });

(async () => {
  console.log('🚀 Starting SqlFox Complete Showcase Capture Suite (Svelte 5 Edition)...');

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

  // Route interceptors for rich telemetry and DBA views
  await page.route('**/api/session/*/diagnostics/cluster', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        cluster_name: 'prod-ecommerce-cluster',
        node_name: 'prod-node-01 (Primary Leader)',
        cluster_size: 3,
        wsrep_cluster_status: 'Primary',
        wsrep_ready: 'ON',
        wsrep_connected: 'ON',
        wsrep_local_state_comment: 'Synced',
        wsrep_flow_control_paused: 0.0,
        replication_type: 'Multi-Region Galera Cluster + Read-Replica Async',
        replica_lag_seconds: 0.02,
        replica_io_running: 'Yes',
        replica_sql_running: 'Yes',
        last_io_error: '',
        bottleneck_severity: 'healthy',
        bottleneck_diagnosis: 'Galera multi-master mesh operates in full synchronization. Zero replication lag and zero flow-control stalls detected.',
        nodes: [
          { name: 'prod-node-01', address: '10.240.0.101:3306', state: 'Synced (Leader)', lag: 0.0, role: 'Primary Leader' },
          { name: 'prod-node-02', address: '10.240.0.102:3306', state: 'Synced', lag: 0.01, role: 'Active Peer' },
          { name: 'prod-node-03', address: '10.240.0.103:3306', state: 'Synced', lag: 0.02, role: 'Read Standby' }
        ]
      })
    });
  });

  await page.route('**/api/session/*/diagnostics/variables', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        variables: [
          { name: 'innodb_buffer_pool_size', value: '17,179,869,184 (16 GB)', category: 'memory', is_dynamic: true, description: 'Allocates memory buffer pool for caching table data and secondary indexes.', advice: 'Optimal for server with 32GB RAM (~50% memory threshold).' },
          { name: 'max_connections', value: '2500', category: 'connection', is_dynamic: true, description: 'Maximum permitted number of simultaneous client connections.', advice: 'Thread cache sizing should align with peak application pool.' },
          { name: 'innodb_flush_log_at_trx_commit', value: '1 (Strict ACID)', category: 'durability', is_dynamic: true, description: 'Redo log written and flushed to disk at each transaction commit.', advice: 'Required for non-negotiable financial audit durability.' },
          { name: 'tmp_table_size', value: '67,108,864 (64 MB)', category: 'memory', is_dynamic: true, description: 'Maximum size of internal in-memory temporary tables.', advice: 'Prevents complex join disk spills.' },
          { name: 'long_query_time', value: '0.500000 s', category: 'query', is_dynamic: true, description: 'Threshold in seconds above which queries are logged to slow query log.', advice: 'Recommended: <= 1.0s for OLTP workloads.' }
        ]
      })
    });
  });

  await page.route('**/api/session/*/processlist', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        current_thread_id: 95,
        threads: [
          { id: 142, user: 'app_payment_worker', host: '10.240.0.42:54120', db: 'production_store', command: 'Query', time: 14, state: 'Updating orders table', info: "UPDATE orders SET order_status = 'processing', total_amount = 14850.00 WHERE id = 10892;" },
          { id: 284, user: 'api_checkout_svc', host: '10.240.0.88:49210', db: 'production_store', command: 'Query', time: 8, state: 'Lock wait: row contention', info: 'SELECT * FROM orders WHERE id = 10892 FOR UPDATE;' },
          { id: 110, user: 'analytics_cron', host: '10.240.0.12:38901', db: 'production_store', command: 'Query', time: 3, state: 'Sending aggregated result sets', info: 'SELECT customer_id, COUNT(id), SUM(total_amount) FROM orders GROUP BY customer_id HAVING SUM(total_amount) > 10000;' },
          { id: 340, user: 'inventory_sync', host: '10.240.0.64:41200', db: 'production_store', command: 'Query', time: 1, state: 'Writing to binlog', info: "UPDATE products SET stock_quantity = stock_quantity - 2 WHERE sku = 'SKU-492-0042';" },
          { id: 412, user: 'audit_daemon', host: '10.240.0.19:55123', db: 'production_store', command: 'Execute', time: 0, state: 'Inserting records', info: "INSERT INTO audit_logs (actor_email, action, ip_address, created_at) VALUES ('secops@company.io', 'CONFIG_READ', '10.240.0.5', NOW());" },
          { id: 95, user: 'dba_admin', host: '10.240.0.5:60122', db: 'production_store', command: 'Sleep', time: 52, state: 'Idle connection pool', info: '' },
          { id: 104, user: 'web_session_mgr', host: '10.240.0.7:58911', db: 'production_store', command: 'Sleep', time: 120, state: 'Idle connection pool', info: '' }
        ],
        locks: [
          {
            is_root_blocker: true,
            blocking_thread_id: 142,
            blocking_user: 'app_payment_worker',
            lock_mode: 'EXCLUSIVE ROW LOCK (X)',
            locked_resource: 'orders (row #10892)',
            blocking_query: "UPDATE orders SET order_status = 'processing', total_amount = 14850.00 WHERE id = 10892;",
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
      })
    });
  });

  await page.route('**/api/sync/structure/compare', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        diffs: [
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
        ]
      })
    });
  });

  await page.route('**/api/sync/data/compare', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        table: 'orders',
        missing_count: 24,
        modified_count: 8,
        stale_count: 2,
        identical_count: 9966,
        operations: [
          { action: 'INSERT', pk_values: [10001, 10002], count: 24 },
          { action: 'UPDATE', pk_values: [10892, 10893], count: 8 },
          { action: 'DELETE', pk_values: [999, 1000], count: 2 }
        ]
      })
    });
  });

  // Seed AI drawer session
  await page.addInitScript(() => {
    localStorage.setItem(
      'sqlfox_ai_sessions',
      JSON.stringify([
        {
          id: 'session-showcase',
          title: 'Index Optimization Advisory',
          updatedAt: Date.now(),
          messages: [
            {
              id: 'msg-1',
              role: 'user',
              content:
                'How should I index the orders table with 10,000+ records to eliminate sort spills and optimize customer lookup with date ranges?',
              timestamp: Date.now() - 60000
            },
            {
              id: 'msg-2',
              role: 'assistant',
              content:
                'Analyzing schema & execution plan for `orders`:\n\nFrequent queries filter on `customer_id` (Equality) and sort by `created_at DESC` (Sort).\n\nApplying the **ESR (Equality, Sort, Range)** rule eliminates temporary tables and filesorts. Recommended composite index:\n\n```sql\nCREATE INDEX idx_orders_customer_created\nON orders (customer_id, created_at DESC);\n```\n\nEstimated query speedup: **~24x faster** (Index-only seek vs full table scan).',
              timestamp: Date.now() - 30000
            }
          ]
        }
      ])
    );
  });

  console.log('Navigating to SqlFox Svelte UI at http://127.0.0.1:8999...');
  await page.goto('http://127.0.0.1:8999', { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(2500);

  // Connect to Production Store DB
  console.log('Connecting to Production Store DB...');
  const connIds = await page.evaluate(async () => {
    const s = window.appState;
    const prod = s.connections.find((c) => c.name.includes('Production'));
    const staging = s.connections.find((c) => c.name.includes('Staging'));
    if (prod) await s.selectConnection(prod);
    return {
      prodId: prod?.id,
      stagingId: staging?.id
    };
  });
  await page.waitForTimeout(2000);

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

  await page.evaluate(async (sql) => {
    const s = window.appState;
    s.activeTab.sql = sql;
    await s.executeCurrentTab(sql);
  }, heroSQL);
  await page.waitForTimeout(2500);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '01_hero_workspace.png') });

  // =========================================================================
  // 2. SAFETY GUARD (Destructive Query Challenge Modal)
  // =========================================================================
  console.log('📸 [2/16] Capturing 02_safety_guard_destructive.png...');
  const dangerousSQL = `-- Accidental destructive statement on Production DB\nDROP TABLE customers;`;
  await page.evaluate((sql) => {
    const s = window.appState;
    s.activeTab.sql = sql;
    s.safetyChallenge = {
      isOpen: true,
      sql: sql,
      inspection: {
        is_destructive: true,
        requires_challenge: true,
        requires_affirmation: false,
        schema_scope_mismatch: false,
        reason: 'DROP TABLE permanently destroys table schema, indexes, and all 2,500 active customer records.',
        target_table: 'customers',
        target_database: 'Production Store DB'
      },
      onConfirm: () => {},
      onCancel: () => {}
    };
  }, dangerousSQL);
  await page.waitForTimeout(1000);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '02_safety_guard_destructive.png') });
  await page.evaluate(() => window.appState.cancelSafetyChallenge());
  await page.waitForTimeout(500);

  // =========================================================================
  // 3. FLIGHT RECORDER (Live On-Demand Performance Telemetry)
  // =========================================================================
  console.log('📸 [3/16] Capturing 03_flight_recorder_telemetry.png...');
  await page.evaluate(() => {
    const s = window.appState;
    s.openProcesslistTab('flight-recorder');
  });
  await page.waitForTimeout(1500);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '03_flight_recorder_telemetry.png') });

  // =========================================================================
  // 4. CLUSTER & REPLICATION INSPECTOR
  // =========================================================================
  console.log('📸 [4/16] Capturing 04_cluster_replication_inspector.png...');
  await page.evaluate(() => {
    const s = window.appState;
    s.openProcesslistTab('cluster');
  });
  await page.waitForTimeout(1500);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '04_cluster_replication_inspector.png') });

  // =========================================================================
  // 5. LOCK CONTENTION & ROOT BLOCKER TRIAGE
  // =========================================================================
  console.log('📸 [5/16] Capturing 05_lock_contention_root_blocker.png...');
  await page.evaluate(() => {
    const s = window.appState;
    s.isLockInspectorOpen = true;
  });
  await page.waitForTimeout(1200);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '05_lock_contention_root_blocker.png') });
  await page.evaluate(() => {
    window.appState.isLockInspectorOpen = false;
  });
  await page.waitForTimeout(500);

  // =========================================================================
  // 6. REAL-TIME PROCESSLIST
  // =========================================================================
  console.log('📸 [6/16] Capturing 06_realtime_processlist.png...');
  await page.evaluate(() => {
    const s = window.appState;
    s.openProcesslistTab('threads');
  });
  await page.waitForTimeout(1500);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '06_realtime_processlist.png') });

  // =========================================================================
  // 7. SMART SEED (FK-Aware Referential Topological Slicer)
  // =========================================================================
  console.log('📸 [7/16] Capturing 07_smart_seed_fk_exporter.png...');
  await page.evaluate(() => {
    const s = window.appState;
    s.isSmartSeedModalOpen = true;
  });
  await page.waitForTimeout(1200);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '07_smart_seed_fk_exporter.png') });
  await page.evaluate(() => {
    window.appState.isSmartSeedModalOpen = false;
  });
  await page.waitForTimeout(500);

  // =========================================================================
  // 8. STRUCTURE SYNC (Schema Diff & Safe DDL Migration Generator)
  // =========================================================================
  console.log('📸 [8/16] Capturing 08_structure_sync_schema_diff.png...');
  await page.evaluate(() => {
    const s = window.appState;
    s.isStructureSyncModalOpen = true;
  });
  await page.waitForTimeout(800);
  if (connIds.prodId && connIds.stagingId) {
    try {
      await page.evaluate(({ prodId, stagingId }) => {
        const src = document.getElementById('sync-src-conn');
        const tgt = document.getElementById('sync-tgt-conn');
        if (src) { src.value = prodId; src.dispatchEvent(new Event('change')); }
        if (tgt) { tgt.value = stagingId; tgt.dispatchEvent(new Event('change')); }
      }, { prodId: connIds.prodId, stagingId: connIds.stagingId });
      await page.waitForTimeout(600);
      const compareBtn = page.locator('button:has-text("Compare Schemas")');
      if (await compareBtn.count() > 0) {
        await compareBtn.first().click();
        await page.waitForTimeout(1200);
      }
    } catch (e) {
      console.warn('Note: Compare trigger skipped:', e.message);
    }
  }
  await page.screenshot({ path: path.join(OUTPUT_DIR, '08_structure_sync_schema_diff.png') });
  await page.evaluate(() => {
    window.appState.isStructureSyncModalOpen = false;
  });
  await page.waitForTimeout(500);

  // =========================================================================
  // 9. CROSS-CONNECTION DATA TRANSFER
  // =========================================================================
  console.log('📸 [9/16] Capturing 09_cross_connection_data_transfer.png...');
  await page.evaluate(() => {
    const s = window.appState;
    s.isDataTransferOpen = true;
  });
  await page.waitForTimeout(1000);
  if (connIds.prodId && connIds.stagingId) {
    try {
      await page.evaluate(({ prodId, stagingId }) => {
        const selects = document.querySelectorAll('div[role="dialog"] select');
        if (selects.length >= 3) {
          selects[0].value = prodId;
          selects[0].dispatchEvent(new Event('change'));
          selects[2].value = stagingId;
          selects[2].dispatchEvent(new Event('change'));
        }
      }, { prodId: connIds.prodId, stagingId: connIds.stagingId });
      await page.waitForTimeout(800);
    } catch (e) {
      console.warn('Note: DataTransfer options skipped:', e.message);
    }
  }
  await page.screenshot({ path: path.join(OUTPUT_DIR, '09_cross_connection_data_transfer.png') });
  await page.evaluate(() => {
    window.appState.isDataTransferOpen = false;
  });
  await page.waitForTimeout(500);

  // =========================================================================
  // 10. ROW-LEVEL DATA SYNC (Keyset Delta Reconciler)
  // =========================================================================
  console.log('📸 [10/16] Capturing 10_row_level_data_sync.png...');
  await page.evaluate(() => {
    const s = window.appState;
    s.isDataSyncOpen = true;
  });
  await page.waitForTimeout(1000);
  if (connIds.prodId && connIds.stagingId) {
    try {
      await page.evaluate(({ prodId, stagingId }) => {
        const selects = document.querySelectorAll('div[role="dialog"] select');
        if (selects.length >= 3) {
          selects[0].value = prodId;
          selects[0].dispatchEvent(new Event('change'));
          selects[2].value = stagingId;
          selects[2].dispatchEvent(new Event('change'));
        }
      }, { prodId: connIds.prodId, stagingId: connIds.stagingId });
      await page.waitForTimeout(800);
      const analyzeBtn = page.locator('div[role="dialog"] button:has-text("Analyze Diff")');
      if (await analyzeBtn.count() > 0) {
        await analyzeBtn.first().click();
        await page.waitForTimeout(1200);
      }
    } catch (e) {
      console.warn('Note: DataSync options skipped:', e.message);
    }
  }
  await page.screenshot({ path: path.join(OUTPUT_DIR, '10_row_level_data_sync.png') });
  await page.evaluate(() => {
    window.appState.isDataSyncOpen = false;
  });
  await page.waitForTimeout(500);

  // =========================================================================
  // 11. VISUAL INTERACTIVE ERD CANVAS
  // =========================================================================
  console.log('📸 [11/16] Capturing 11_visual_erd_canvas.png...');
  await page.evaluate(() => {
    const s = window.appState;
    s.openErdTab();
  });
  await page.waitForTimeout(3000);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '11_visual_erd_canvas.png') });

  // =========================================================================
  // 12. VISUAL TABLE DESIGNER (In-Place Column & Index Builder)
  // =========================================================================
  console.log('📸 [12/16] Capturing 12_visual_table_designer.png...');
  await page.evaluate(() => {
    const s = window.appState;
    s.openTableStructureTab('orders');
  });
  await page.waitForTimeout(2000);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '12_visual_table_designer.png') });

  // =========================================================================
  // 13. STORAGE & HEALTH DIAGNOSTICS
  // =========================================================================
  console.log('📸 [13/16] Capturing 13_storage_health_diagnostics.png...');
  await page.evaluate(() => {
    const s = window.appState;
    s.openDbDiagnosticsModal();
  });
  await page.waitForTimeout(1800);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '13_storage_health_diagnostics.png') });
  await page.evaluate(() => {
    window.appState.isDbDiagnosticsOpen = false;
  });
  await page.waitForTimeout(500);

  // =========================================================================
  // 14. SERVER VARIABLES & TUNING ADVISOR
  // =========================================================================
  console.log('📸 [14/16] Capturing 14_server_variables_tuning.png...');
  await page.evaluate(() => {
    const s = window.appState;
    s.openProcesslistTab('variables');
  });
  await page.waitForTimeout(1500);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '14_server_variables_tuning.png') });

  // =========================================================================
  // 15. VERTICAL RECORD INSPECTOR (Form View + Nested JSON)
  // =========================================================================
  console.log('📸 [15/16] Capturing 15_vertical_record_inspector.png...');
  await page.evaluate(async () => {
    const s = window.appState;
    const queryTab = s.tabs.find((t) => !t.isFlightRecorder && !t.isCluster && !t.isVariables && !t.isTableViewer && !t.isErdCanvas);
    if (queryTab) s.activeTabId = queryTab.id;
    const ordersSQL = `-- Inspect detailed order records with rich shipping & payment payloads\nSELECT * FROM orders ORDER BY id ASC LIMIT 50;`;
    s.activeTab.sql = ordersSQL;
    await s.executeCurrentTab(ordersSQL);
  });
  await page.waitForTimeout(2000);
  await page.evaluate(() => {
    const s = window.appState;
    s.recordInspectorRowIndex = 0;
    s.isRecordInspectorOpen = true;
  });
  await page.waitForTimeout(1500);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '15_vertical_record_inspector.png') });
  await page.evaluate(() => {
    window.appState.isRecordInspectorOpen = false;
  });
  await page.waitForTimeout(500);

  // =========================================================================
  // 16. AI COPILOT ASSISTANT (In-Context Index Tuning & Query Optimizer)
  // =========================================================================
  console.log('📸 [16/16] Capturing 16_ai_copilot_assistant.png...');
  await page.evaluate((sql) => {
    const s = window.appState;
    const queryTab = s.tabs.find((t) => !t.isFlightRecorder && !t.isCluster && !t.isVariables && !t.isTableViewer && !t.isErdCanvas);
    if (queryTab) s.activeTabId = queryTab.id;
    s.activeTab.sql = sql;
    s.toggleAiDrawer(true);
  }, heroSQL);
  await page.waitForTimeout(1500);
  await page.screenshot({ path: path.join(OUTPUT_DIR, '16_ai_copilot_assistant.png') });

  await browser.close();
  console.log('🎉 ALL 16 HIGH-RESOLUTION SHOWCASE SCREENSHOTS CAPTURED WITH FULL SVELTE UI DATA!');
})().catch((err) => {
  console.error('❌ Error during capture:', err);
  process.exit(1);
});
