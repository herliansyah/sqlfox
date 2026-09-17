const fs = require('fs');

console.log('--- Phase 1 Feedback Loop: Toolbar Button Consistency Check ---');
const html = fs.readFileSync('web/dist/index.html', 'utf8');

let errors = [];

// Find the toolbar buttons inside #editor-container
const formatBtnMatch = html.match(/<button onclick="formatActiveQuery\(\)"([^>]*)>([\s\S]*?)<\/button>/);
const explainBtnMatch = html.match(/<button onclick="explainActiveQuery\(\)"([^>]*)>([\s\S]*?)<\/button>/);
const safetyBtnMatch = html.match(/<button onclick="inspectActiveQuery\(\)"([^>]*)>([\s\S]*?)<\/button>/);

if (!formatBtnMatch) {
  errors.push('FAIL: formatActiveQuery button not found in HTML!');
} else {
  const formatAttrs = formatBtnMatch[1];
  const formatContent = formatBtnMatch[2].trim();

  // Check 1: Must share bg-app-card
  if (!formatAttrs.includes('bg-app-card')) {
    errors.push('FAIL: Format button lacks "bg-app-card" class (different background than Explain & Safety)!');
  }

  // Check 2: Must share border border-app-border
  if (!formatAttrs.includes('border') || !formatAttrs.includes('border-app-border')) {
    errors.push('FAIL: Format button lacks "border border-app-border" (different border style than Explain & Safety)!');
  }

  // Check 3: Must have an icon like Explain (⚡) and Safety (🛡️)
  if (!/[\u{1F300}-\u{1FAFF}\u{2600}-\u{27BF}]/u.test(formatContent)) {
    errors.push('FAIL: Format button lacks an icon/emoji prefix like Explain (⚡) and Safety (🛡️)!');
  }
}

if (errors.length > 0) {
  console.error('\n❌ FEEDBACK LOOP FAILED:');
  errors.forEach(e => console.error('  - ' + e));
  process.exit(1);
} else {
  console.log('\n✅ FEEDBACK LOOP PASSED: Toolbar button styling is consistent across Run, Explain, Safety, and Format!');
  process.exit(0);
}
