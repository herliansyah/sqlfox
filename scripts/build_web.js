const fs = require('fs');
const path = require('path');
const vm = require('vm');
const { execSync } = require('child_process');

const rootDir = path.resolve(__dirname, '..');
const srcDir = path.join(rootDir, 'web', 'src');
const srcJsDir = path.join(srcDir, 'js');
const distHtmlPath = path.join(rootDir, 'web', 'dist', 'index.html');

console.log('--- 0. Building CodeMirror 6 Bundle ---');
try {
  require('./build_codemirror.js');
} catch (err) {
  console.error('CodeMirror build failed:', err.message);
  process.exit(1);
}

console.log('--- 1. Assembling Modular Frontend ---');
const srcHtml = fs.readFileSync(path.join(srcDir, 'index.html'), 'utf8');

const jsFiles = fs.readdirSync(srcJsDir)
  .filter(f => f.endsWith('.js'))
  .sort();

console.log(`Concatenating ${jsFiles.length} JavaScript modules:`);
let combinedJs = '';
for (const f of jsFiles) {
  console.log(`  + web/src/js/${f}`);
  const content = fs.readFileSync(path.join(srcJsDir, f), 'utf8');
  combinedJs += `\n// ==================== MODULE: ${f} ====================\n` + content;
}

// Verify JavaScript syntax in memory
console.log('--- 2. Verifying JavaScript Syntax ---');
try {
  new vm.Script(combinedJs);
  console.log('✅ Combined JavaScript syntax is valid.');
} catch (err) {
  console.error('❌ Syntax error in combined JavaScript:', err);
  process.exit(1);
}

const placeholder = '<!-- @INJECT:SCRIPTS -->';
if (!srcHtml.includes(placeholder)) {
  console.error(`Placeholder "${placeholder}" not found in web/src/index.html`);
  process.exit(1);
}

// Use a replacer function () => combinedJs to prevent String.prototype.replace from
// treating patterns like $' or $& inside arbitrary SQL/JS code as special replacement tokens!
const finalHtml = srcHtml.replace(placeholder, () => combinedJs);
fs.writeFileSync(distHtmlPath, finalHtml, 'utf8');
console.log(`✅ Successfully generated ${distHtmlPath} (${finalHtml.split('\n').length} lines).`);

console.log('--- 3. Building Tailwind CSS ---');
try {
  execSync('npm run build:css', { cwd: rootDir, stdio: 'inherit' });
} catch (err) {
  console.error('Tailwind build failed:', err.message);
  process.exit(1);
}
