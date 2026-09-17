const fs = require('fs');

console.log('--- Phase 1 Feedback Loop: Database Creator Charset & Collation Auto-Sync ---');
const html = fs.readFileSync('web/dist/index.html', 'utf8');

let errors = [];

// 1. Check if new-db-charset has onchange handler
if (!html.includes('id="new-db-charset"') || !html.includes('onchange="onDBCharsetChange()"')) {
  errors.push('FAIL: #new-db-charset does not trigger onDBCharsetChange() when selected!');
}

// 2. Check if onDBCharsetChange function exists
if (!html.includes('function onDBCharsetChange(')) {
  errors.push('FAIL: onDBCharsetChange function does not exist in client scripts!');
}

// 3. Check if latin1_swedish_ci is supported
if (!html.includes('latin1_swedish_ci')) {
  errors.push('FAIL: latin1_swedish_ci is missing from supported collations!');
}

// 4. Check if Server Default option is available
if (!html.includes('Server Default') && !html.includes('(Server Default)')) {
  errors.push('FAIL: Server Default option missing from Database Creator!');
}

if (errors.length > 0) {
  console.error('\n❌ FEEDBACK LOOP FAILED:');
  errors.forEach(e => console.error('  - ' + e));
  process.exit(1);
} else {
  console.log('\n✅ FEEDBACK LOOP PASSED: Charset-collation auto-sync and legacy collations verified!');
  process.exit(0);
}
