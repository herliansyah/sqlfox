const fs = require('fs');

let errors = [];

// 1. Check frontend for BYOK settings modal and toggleAISettings
const html = fs.readFileSync('web/dist/index.html', 'utf8');

if (!html.includes('id="ai-settings-modal"')) {
  errors.push('BUG 1: #ai-settings-modal is missing in index.html!');
}

if (!html.includes('function toggleAISettings') && !html.includes('toggleAISettings =')) {
  errors.push('BUG 2: toggleAISettings() is called on line 217 button but NEVER defined in <script>!');
}

if (!html.includes('id="database-selector"') && !html.includes('id="db-select"')) {
  errors.push('BUG 3: Database selector dropdown is missing in sidebar to browse/switch databases when connecting to a server!');
}

// 2. Check mysql driver for database discovery when database_name is empty
const mysqlCode = fs.readFileSync('internal/driver/mysql/mysql.go', 'utf8');
if (!mysqlCode.includes('SHOW DATABASES') && !mysqlCode.includes('information_schema.SCHEMATA')) {
  errors.push('BUG 4: MySQL driver does not query SHOW DATABASES / SCHEMATA when database name is empty, returning zero tables!');
}

// 3. Check AI client for proxy endpoint normalization
const aiCode = fs.readFileSync('internal/ai/client.go', 'utf8');
if (!aiCode.includes('/chat/completions') || (!aiCode.includes('TrimRight') && !aiCode.includes('HasSuffix'))) {
  errors.push('BUG 5: AI client does not normalize proxy endpoints (like 9router http://ip:port/v1)!');
}

console.log('--- BYOK & DB SELECTION DIAGNOSTIC RESULT ---');
if (errors.length > 0) {
  console.log('FAIL (' + errors.length + ' bugs found):');
  errors.forEach(e => console.log('❌ ' + e));
  process.exit(1);
} else {
  console.log('PASS: All database selection and BYOK settings seams are sound.');
  process.exit(0);
}
