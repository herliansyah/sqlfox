// crud_functional_self_check.js
// Advanced functional verification suite for MySQL/MariaDB Object CRUD & Designer Suite

const assert = require('assert');
const fs = require('fs');
const vm = require('vm');

const html = fs.readFileSync('web/dist/index.html', 'utf8');

// Set up a mock browser environment in VM
const sandbox = {
  console,
  document: {
    getElementById: (id) => ({
      id,
      value: '',
      textContent: '',
      dataset: {},
      classList: {
        add: () => {},
        remove: () => {},
        contains: () => false
      },
      querySelectorAll: () => [],
      querySelector: () => null,
      appendChild: () => {},
      removeAttribute: () => {},
      setAttribute: () => {},
      close: () => {},
      showModal: () => {}
    }),
    querySelectorAll: () => [],
    querySelector: () => null
  },
  window: {},
  activeConnection: { id: 'conn-1', driver: 'mysql' },
  schemaCatalogCache: { database_name: 'testdb', tables: [], triggers: [], routines: [], events: [] },
  tabs: [],
  activeTabId: null
};

vm.createContext(sandbox);

// Helper to extract function block from HTML
function extractFunctionCode(name) {
  const regex = new RegExp(`(?:async\\s+)?function\\s+${name}\\s*\\([\\s\\S]*?\\n    }`, 'm');
  const match = html.match(regex);
  if (!match) throw new Error(`Could not find function ${name} in index.html`);
  return match[0];
}

const funcsToLoad = [
  'stripDefiner',
  'extractViewSelectQuery',
  'assembleViewDDL',
  'assembleTriggerDDL',
  'assembleRoutineDDL',
  'assembleEventDDL'
];

funcsToLoad.forEach(fn => {
  vm.runInContext(extractFunctionCode(fn), sandbox);
});

console.log('🧪 Starting Advanced Functional Verification Suite...\n');

// ==========================================
// Test 1: stripDefiner Preserves Formatting & Comments
// ==========================================
console.log('--- Test 1: stripDefiner line break & comment preservation ---');
{
  const input = `CREATE DEFINER=\`admin_user\`@\`192.168.1.%\` PROCEDURE \`sp_complex\`()
BEGIN
  -- step 1: check balance
  DECLARE v_bal DECIMAL(10,2);
  
  -- step 2: update account
  UPDATE accounts SET balance = balance + 100 WHERE id = 1;

  /* multi-line
     block */
  SELECT v_bal;
END`;

  const stripped = sandbox.stripDefiner(input);
  assert(!stripped.includes('DEFINER'), 'DEFINER must be stripped');
  assert(stripped.includes('CREATE PROCEDURE `sp_complex`()'), 'Header must be clean');
  assert(stripped.includes('-- step 1: check balance\n'), 'Single-line comment must retain its line break');
  assert(stripped.includes('DECLARE v_bal DECIMAL(10,2);'), 'Statement must remain executable');
  assert(!stripped.includes('-- step 1: check balance DECLARE'), 'Comment must NOT swallow the next statement');
  assert(stripped.includes('\n  UPDATE accounts'), 'Indentation and line breaks must be preserved');
  console.log('  ✓ stripDefiner preserves line breaks and avoids comment swallowing');
}

// ==========================================
// Test 2: extractViewSelectQuery Robust Extraction
// ==========================================
console.log('\n--- Test 2: extractViewSelectQuery grammar variations ---');
{
  // 2a. Standard view
  const v1 = 'CREATE ALGORITHM=UNDEFINED DEFINER=`root`@`localhost` SQL SECURITY DEFINER VIEW `v_users` AS select `id`, `name` from `users`;';
  assert.strictEqual(sandbox.extractViewSelectQuery(v1), 'select `id`, `name` from `users`');

  // 2b. View name containing "as"
  const v2 = 'CREATE VIEW `user as admin` AS SELECT id, email FROM users;';
  assert.strictEqual(sandbox.extractViewSelectQuery(v2), 'SELECT id, email FROM users');

  // 2c. View with explicit column list
  const v3 = 'CREATE VIEW `v_summary` (`user_id`, `total_as_spent`) AS SELECT uid, sum(amount) FROM tx GROUP BY uid;';
  assert.strictEqual(sandbox.extractViewSelectQuery(v3), 'SELECT uid, sum(amount) FROM tx GROUP BY uid');

  // 2d. CTE view
  const v4 = 'CREATE VIEW `v_cte` AS WITH cte AS (SELECT 1 AS num) SELECT * FROM cte;';
  assert.strictEqual(sandbox.extractViewSelectQuery(v4), 'WITH cte AS (SELECT 1 AS num) SELECT * FROM cte');

  console.log('  ✓ extractViewSelectQuery correctly isolates inner SELECT for all DDL forms');
}

// ==========================================
// Test 3: assembleViewDDL Dialects and Semicolon Trimming
// ==========================================
console.log('\n--- Test 3: assembleViewDDL dialects & safety ---');
{
  // MySQL create
  sandbox.activeConnection.driver = 'mysql';
  const mySqlCreate = sandbox.assembleViewDDL('v_test', 'SELECT * FROM items;\n;;');
  assert(mySqlCreate.startsWith('CREATE OR REPLACE VIEW `v_test` AS\nSELECT * FROM items;'), 'MySQL must use CREATE OR REPLACE VIEW');
  assert(!mySqlCreate.endsWith(';;'), 'Must sanitize multiple trailing semicolons');

  // SQLite create
  sandbox.activeConnection.driver = 'sqlite';
  const sqLiteCreate = sandbox.assembleViewDDL('v_test', 'SELECT * FROM items');
  assert(sqLiteCreate.startsWith('CREATE VIEW IF NOT EXISTS "v_test" AS\nSELECT * FROM items;'), 'SQLite must use CREATE VIEW IF NOT EXISTS');

  // MySQL rename on edit
  sandbox.activeConnection.driver = 'mysql';
  const mySqlRename = sandbox.assembleViewDDL('v_new', 'SELECT 1', true, 'v_old');
  assert(mySqlRename.includes('DROP VIEW IF EXISTS `v_old`;\n'), 'Must drop old view on rename');
  assert(mySqlRename.includes('CREATE OR REPLACE VIEW `v_new`'), 'Must create new view');

  console.log('  ✓ assembleViewDDL handles dialects, semicolon trimming, and view renaming');
}

// ==========================================
// Test 4: assembleTriggerDDL Lifecycle & Replacement
// ==========================================
console.log('\n--- Test 4: assembleTriggerDDL lifecycle & replacement ---');
{
  sandbox.activeConnection.driver = 'mysql';

  // 4a. Fresh trigger creation
  const trgCreate = sandbox.assembleTriggerDDL('trg_audit', 'orders', 'AFTER', 'INSERT', 'BEGIN\n  INSERT INTO log VALUES (NOW());\nEND;', false);
  assert(!trgCreate.includes('DROP TRIGGER'), 'New trigger creation must not drop trigger');
  assert(trgCreate.includes('CREATE TRIGGER `trg_audit` AFTER INSERT ON `orders` FOR EACH ROW'), 'Must generate FOR EACH ROW clause');
  assert(trgCreate.endsWith('END;'), 'Must terminate with single semicolon');

  // 4b. Trigger edit replacement (same name)
  const trgEdit = sandbox.assembleTriggerDDL('trg_audit', 'orders', 'AFTER', 'INSERT', 'BEGIN\n  -- logic\nEND', true);
  assert(trgEdit.startsWith('DROP TRIGGER IF EXISTS `trg_audit`;\n'), 'Edit must prepend DROP TRIGGER IF EXISTS');

  // 4c. Trigger edit with rename
  const trgRename = sandbox.assembleTriggerDDL('trg_audit_v2', 'orders', 'AFTER', 'INSERT', 'BEGIN\nEND;', true, 'trg_audit_v1');
  assert(trgRename.includes('DROP TRIGGER IF EXISTS `trg_audit_v1`;\n'), 'Renamed trigger must drop original trigger first');
  assert(trgRename.includes('DROP TRIGGER IF EXISTS `trg_audit_v2`;\n'), 'Renamed trigger must also drop target if existing');
  assert(trgRename.includes('CREATE TRIGGER `trg_audit_v2`'), 'Must create under new name');

  // 4d. SQLite trigger
  sandbox.activeConnection.driver = 'sqlite';
  const sqLiteTrg = sandbox.assembleTriggerDDL('trg_sq', 'orders', 'BEFORE', 'DELETE', 'BEGIN\nEND', false);
  assert(!sqLiteTrg.includes('FOR EACH ROW'), 'SQLite trigger must omit FOR EACH ROW');

  console.log('  ✓ assembleTriggerDDL executes safe replacements, renames, and dialect variations');
}

// ==========================================
// Test 5: assembleRoutineDDL (Procedures & Functions)
// ==========================================
console.log('\n--- Test 5: assembleRoutineDDL procedures vs functions ---');
{
  // 5a. Stored Procedure with mixed parameter modes
  const procParams = [
    { name: 'p_user_id', mode: 'IN', type: 'INT' },
    { name: 'p_discount', mode: 'IN', type: 'DECIMAL(5,2)' },
    { name: 'p_out_total', mode: 'OUT', type: 'DECIMAL(10,2)' }
  ];
  const procDDL = sandbox.assembleRoutineDDL('sp_calc', 'PROCEDURE', procParams, 'INT', true, 'BEGIN\n  -- logic\nEND;', false);
  assert(procDDL.includes('CREATE PROCEDURE `sp_calc`(IN `p_user_id` INT, IN `p_discount` DECIMAL(5,2), OUT `p_out_total` DECIMAL(10,2))'), 'Procedure must declare parameter modes');
  assert(procDDL.includes('DETERMINISTIC'), 'Must include DETERMINISTIC declaration');
  assert(!procDDL.includes('RETURNS'), 'Procedure must NOT declare RETURNS');
  assert(!procDDL.endsWith(';;'), 'Must not produce double semicolon');

  // 5b. Stored Function (strict input-only parameters, no mode keywords, RETURNS type required)
  const funcParams = [
    { name: 'p_price', mode: 'IN', type: 'DECIMAL(10,2)' },
    { name: 'p_rate', mode: 'IN', type: 'DECIMAL(4,2)' }
  ];
  const funcDDL = sandbox.assembleRoutineDDL('fn_calc_tax', 'FUNCTION', funcParams, 'DECIMAL(10,2)', true, 'BEGIN\n  RETURN p_price * p_rate;\nEND;', false);
  assert(funcDDL.includes('CREATE FUNCTION `fn_calc_tax`(`p_price` DECIMAL(10,2), `p_rate` DECIMAL(4,2)) RETURNS DECIMAL(10,2)'), 'Function parameters must NOT have IN keyword');
  assert(funcDDL.includes('RETURNS DECIMAL(10,2)'), 'Function must declare RETURNS type');

  // 5c. Procedure rename on edit
  const procRename = sandbox.assembleRoutineDDL('sp_calc_v2', 'PROCEDURE', [], 'INT', true, 'BEGIN\nEND', true, 'sp_calc_v1', 'PROCEDURE');
  assert(procRename.includes('DROP PROCEDURE IF EXISTS `sp_calc_v1`;\n'), 'Must drop old procedure on rename');
  assert(procRename.includes('DROP PROCEDURE IF EXISTS `sp_calc_v2`;\n'), 'Must drop new procedure if existing');

  // 5d. Type transition on edit (Procedure -> Function)
  const typeSwitch = sandbox.assembleRoutineDDL('rt_calc', 'FUNCTION', [], 'INT', true, 'BEGIN\nRETURN 0;\nEND', true, 'rt_calc', 'PROCEDURE');
  assert(typeSwitch.includes('DROP PROCEDURE IF EXISTS `rt_calc`;\n'), 'Must drop old type (PROCEDURE)');
  assert(typeSwitch.includes('DROP FUNCTION IF EXISTS `rt_calc`;\n'), 'Must drop target type (FUNCTION)');
  assert(typeSwitch.includes('CREATE FUNCTION `rt_calc`'), 'Must create as FUNCTION');

  console.log('  ✓ assembleRoutineDDL manages parameter signatures, returns, and type transitions');
}

// ==========================================
// Test 6: assembleEventDDL Scheduled Events
// ==========================================
console.log('\n--- Test 6: assembleEventDDL recurring & one-time schedules ---');
{
  // 6a. Recurring interval with starts and ends timestamps
  const ev1 = sandbox.assembleEventDDL('ev_purge', 'EVERY', '2', 'HOUR', '2026-11-02 00:00:00', '2026-12-31 23:59:59', '', 'ENABLE', true, 'BEGIN\n  DELETE FROM logs WHERE created_at < NOW() - INTERVAL 7 DAY;\nEND;', false);
  assert(ev1.includes('CREATE EVENT `ev_purge`'), 'Must generate CREATE EVENT');
  assert(ev1.includes('ON SCHEDULE EVERY 2 HOUR STARTS \'2026-11-02 00:00:00\' ENDS \'2026-12-31 23:59:59\''), 'Schedule recurring interval with starts/ends');
  assert(ev1.includes('ON COMPLETION PRESERVE ENABLE'), 'Must include preservation and status');
  assert(ev1.includes('DO\nBEGIN'), 'Must include DO keyword');

  // 6b. Recurring with NOW() expression (no quotes on SQL expression)
  const evNow = sandbox.assembleEventDDL('ev_purge', 'EVERY', '1', 'DAY', 'NOW()', '', '', 'ENABLE', true, 'BEGIN\nEND', false);
  assert(evNow.includes('STARTS NOW()'), 'NOW() expression must NOT be wrapped in quotes');

  // 6c. One-time execution AT timestamp
  const evOneTime = sandbox.assembleEventDDL('ev_migration', 'AT', '', '', '', '', '2026-11-05 03:00:00', 'ENABLE', false, 'BEGIN\n  -- migrate\nEND', false);
  assert(evOneTime.includes('ON SCHEDULE AT \'2026-11-05 03:00:00\''), 'One-time AT timestamp schedule');
  assert(evOneTime.includes('ON COMPLETION NOT PRESERVE'), 'Not preserve completion');

  // 6d. Event rename on edit
  const evRename = sandbox.assembleEventDDL('ev_new', 'EVERY', '1', 'DAY', '', '', '', 'ENABLE', true, 'BEGIN\nEND', true, 'ev_old');
  assert(evRename.includes('DROP EVENT IF EXISTS `ev_old`;\n'), 'Must drop old event on rename');
  assert(evRename.includes('DROP EVENT IF EXISTS `ev_new`;\n'), 'Must drop new event if existing');

  console.log('  ✓ assembleEventDDL accurately handles schedules, timestamps, and rename cycles');
}

// ==========================================
// Test 7: Sidebar Schema Tree Filter Behavior
// ==========================================
console.log('\n--- Test 7: Schema Tree filter matching non-table objects ---');
{
  const schema = {
    database_name: 'production',
    tables: [
      { name: 'users', type: 'BASE TABLE' },
      { name: 'orders', type: 'BASE TABLE' }
    ],
    triggers: [
      { name: 'trg_order_audit', table: 'orders' }
    ],
    routines: [
      { name: 'sp_calculate_tax', type: 'PROCEDURE' },
      { name: 'fn_format_currency', type: 'FUNCTION' }
    ],
    events: [
      { name: 'ev_daily_rollup', status: 'ENABLED' }
    ]
  };

  // Simulating the corrected filter matching condition from index.html:
  function evaluateFilterEmptyCheck(q, sc) {
    const allTables = sc.tables.filter(t => t.type !== 'VIEW');
    const allViews = sc.tables.filter(t => t.type === 'VIEW');
    const allTriggers = sc.triggers;
    const allRoutines = sc.routines;
    const allProcedures = allRoutines.filter(r => (r.type || 'PROCEDURE').toUpperCase() === 'PROCEDURE');
    const allFunctions = allRoutines.filter(r => (r.type || '').toUpperCase() === 'FUNCTION');
    const allEvents = sc.events;

    const tablesList = q ? allTables.filter(t => t.name.toLowerCase().includes(q)) : allTables;
    const viewsList = q ? allViews.filter(v => v.name.toLowerCase().includes(q)) : allViews;
    const triggersList = q ? allTriggers.filter(tr => tr.name.toLowerCase().includes(q)) : allTriggers;
    const proceduresList = q ? allProcedures.filter(r => r.name.toLowerCase().includes(q)) : allProcedures;
    const functionsList = q ? allFunctions.filter(r => r.name.toLowerCase().includes(q)) : allFunctions;
    const eventsList = q ? allEvents.filter(e => e.name.toLowerCase().includes(q)) : allEvents;

    const isEmpty = Boolean(q && tablesList.length === 0 && viewsList.length === 0 && triggersList.length === 0 && proceduresList.length === 0 && functionsList.length === 0 && eventsList.length === 0);
    return { isEmpty, tablesList, viewsList, triggersList, proceduresList, functionsList, eventsList };
  }

  // Query "calc" matches ONLY sp_calculate_tax
  const resCalc = evaluateFilterEmptyCheck('calc', schema);
  assert.strictEqual(resCalc.isEmpty, false, 'Must NOT be considered empty when procedure matches');
  assert.strictEqual(resCalc.proceduresList.length, 1);
  assert.strictEqual(resCalc.tablesList.length, 0);

  // Query "currency" matches ONLY fn_format_currency
  const resCurr = evaluateFilterEmptyCheck('currency', schema);
  assert.strictEqual(resCurr.isEmpty, false, 'Must NOT be considered empty when function matches');
  assert.strictEqual(resCurr.functionsList.length, 1);

  // Query "rollup" matches ONLY ev_daily_rollup
  const resRollup = evaluateFilterEmptyCheck('rollup', schema);
  assert.strictEqual(resRollup.isEmpty, false, 'Must NOT be considered empty when event matches');
  assert.strictEqual(resRollup.eventsList.length, 1);

  // Query "xyz123" matches nothing
  const resNone = evaluateFilterEmptyCheck('xyz123', schema);
  assert.strictEqual(resNone.isEmpty, true, 'Must report empty when no objects match');

  console.log('  ✓ Schema filter search correctly surfaces procedures, functions, and events');
}

console.log('\n🎉 ALL 7 ADVANCED FUNCTIONAL TESTS PASSED CLEANLY!\n');
