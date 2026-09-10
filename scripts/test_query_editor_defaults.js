const fs = require('fs');
const path = require('path');
const assert = require('assert');

const htmlPath = path.join(__dirname, '..', 'web', 'dist', 'index.html');
const content = fs.readFileSync(htmlPath, 'utf8');

console.log('Testing query editor defaults...');

// Check 1: No hardcoded 'SELECT * FROM users LIMIT 50;' in initial tabs state
const initialTabsMatch = content.match(/let\s+tabs\s*=\s*\[\s*\{[^}]*sql:\s*'([^']*)'[^}]*\}\s*\]/);
assert(initialTabsMatch, 'Could not find initial tabs definition');
assert.strictEqual(
  initialTabsMatch[1],
  '',
  `Initial tab should have empty SQL default, found: "${initialTabsMatch[1]}"`
);

// Check 2: No hardcoded 'SELECT * FROM users LIMIT 50;' in addNewQueryTab
const addNewTabMatch = content.match(/function\s+addNewQueryTab\(\)\s*\{([\s\S]*?switchTab\([^)]+\);\s*\})/);
assert(addNewTabMatch, 'Could not find addNewQueryTab definition');
const sqlInNewTab = addNewTabMatch[1].match(/sql:\s*'([^']*)'/);
assert(sqlInNewTab, 'Could not find sql property in addNewQueryTab');
assert.strictEqual(
  sqlInNewTab[1],
  '',
  `addNewQueryTab should have empty SQL default, found: "${sqlInNewTab[1]}"`
);

// Check 3: Placeholder in textarea should not contain 'SELECT * FROM users'
const textareaMatch = content.match(/<textarea\s+id="sql-editor-textarea"[^>]*placeholder="([^"]*)"/);
assert(textareaMatch, 'Could not find sql-editor-textarea placeholder');
assert(
  !textareaMatch[1].includes('SELECT * FROM users'),
  `Textarea placeholder should not reference 'SELECT * FROM users', found: "${textareaMatch[1]}"`
);

console.log('All query editor default tests passed!');
