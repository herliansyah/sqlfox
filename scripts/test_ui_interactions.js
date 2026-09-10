const fs = require('fs');
const html = fs.readFileSync('web/dist/index.html', 'utf8');

let errors = [];

// Check 1: Is testConnectionFromModal defined in JavaScript?
if (!html.includes('function testConnectionFromModal') && !html.includes('testConnectionFromModal =')) {
  errors.push('BUG 1: testConnectionFromModal() is called in HTML button but NEVER defined in <script>!');
}

// Check 2: Is there an edit connection function or button?
if (!html.includes('editConnection') && !html.includes('openEditConnModal')) {
  errors.push('BUG 2: No editConnection() or edit button exists for connections in sidebar!');
}

// Check 3: Does selectConnection handle connection failures properly?
const selectConnMatch = html.match(/async function selectConnection[\s\S]*?catch \(err\)/);
if (selectConnMatch) {
  const code = selectConnMatch[0];
  if (!code.includes('if (!res.ok)') && !code.includes('if (!activeSession') && !code.includes('res.ok')) {
    errors.push('BUG 3: selectConnection() does not check `if (!res.ok)` on connect response, silently proceeding with failed session!');
  }
}

// Check 4: Does saveConnection support editing existing connection (has ID field)?
const saveConnMatch = html.match(/async function saveConnection[\s\S]*?const conn = {([\s\S]*?)};/);
if (saveConnMatch) {
  const fields = saveConnMatch[1];
  if (!fields.includes('id:')) {
    errors.push('BUG 4: saveConnection() does not preserve connection ID, so updating an existing connection always creates a duplicate or fails to update!');
  }
}

console.log('--- UI INTERACTION DIAGNOSTIC RESULT ---');
if (errors.length > 0) {
  console.log('FAIL (' + errors.length + ' bugs found):');
  errors.forEach(e => console.log('❌ ' + e));
  process.exit(1);
} else {
  console.log('PASS: All connection interaction seams are present and sound.');
  process.exit(0);
}
