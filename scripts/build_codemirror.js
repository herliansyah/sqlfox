const fs = require('fs');
const path = require('path');
const esbuild = require('esbuild');

const rootDir = path.resolve(__dirname, '..');
const entryFile = path.join(__dirname, 'codemirror6_entry.js');
const distVendorDir = path.join(rootDir, 'web', 'dist', 'vendor', 'codemirror');
const outJs = path.join(distVendorDir, 'codemirror.min.js');
const outCss = path.join(distVendorDir, 'codemirror.min.css');

console.log('--- Building CodeMirror 6 Bundle ---');

if (!fs.existsSync(distVendorDir)) {
  fs.mkdirSync(distVendorDir, { recursive: true });
}

// 1. Bundle JavaScript via esbuild
try {
  esbuild.buildSync({
    entryPoints: [entryFile],
    outfile: outJs,
    bundle: true,
    minify: true,
    format: 'iife',
    globalName: 'CodeMirrorBundle',
    sourcemap: false,
    target: ['es2020'],
    legalComments: 'none'
  });
  const jsSize = fs.statSync(outJs).size;
  console.log(`✅ CodeMirror 6 JS bundled: ${outJs} (${(jsSize / 1024).toFixed(1)} KB)`);
} catch (err) {
  console.error('❌ Failed to bundle CodeMirror 6 JS:', err);
  process.exit(1);
}

// 2. Ensure CodeMirror CSS with token parity for embed tests and legacy styles
const cssContent = `/* SqlFox CodeMirror 6 Dark Theme Styles & Backward Compatibility Palette */
.CodeMirror, .cm-editor {
  height: 100% !important;
  width: 100% !important;
  font-family: 'JetBrains Mono', 'FiraCode Nerd Font', 'Fira Code', ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace !important;
  font-size: 14px !important;
  line-height: 1.65 !important;
  letter-spacing: 0.01em !important;
  background: transparent !important;
  color: #e0f2fe !important;
}
.cm-scroller {
  height: 100% !important;
  width: 100% !important;
  overflow: auto !important;
  font-family: inherit !important;
}
.cm-content {
  min-height: 100% !important;
  box-sizing: border-box !important;
  caret-color: #38bdf8 !important;
  padding: 8px 0 !important;
}
.CodeMirror-gutters, .cm-gutters {
  background: #090d16 !important;
  border-right: 1px solid #1e293b !important;
}
.CodeMirror-linenumber, .cm-lineNumbers .cm-gutterElement {
  color: #475569 !important;
  padding: 0 8px 0 4px !important;
}
.CodeMirror-cursor, .cm-cursor {
  border-left: 2px solid #38bdf8 !important;
}
.CodeMirror-selected, .cm-selectionBackground {
  background: rgba(56, 189, 248, 0.2) !important;
}
.CodeMirror-focused .CodeMirror-selected, .cm-focused .cm-selectionBackground {
  background: rgba(56, 189, 248, 0.25) !important;
}
.cm-keyword { color: #f472b6 !important; font-weight: 600 !important; }
.cm-string { color: #34d399 !important; }
.cm-number { color: #fbbf24 !important; font-variant-numeric: tabular-nums; }
.cm-variable { color: #e2e8f0 !important; }
.cm-variable-2, .cm-variable-3 { color: #7dd3fc !important; }
.cm-type { color: #c084fc !important; font-weight: 500 !important; }
.cm-comment { color: #64748b !important; font-style: italic !important; }
.cm-atom { color: #fb923c !important; }
.cm-builtin { color: #38bdf8 !important; }
.cm-operator { color: #94a3b8 !important; }
.cm-punctuation { color: #94a3b8 !important; }
.cm-bracket { color: #cbd5e1 !important; }
div.CodeMirror span.CodeMirror-matchingbracket, .cm-matchingBracket {
  color: #38bdf8 !important;
  background: rgba(56, 189, 248, 0.2) !important;
  outline: 1px solid rgba(56, 189, 248, 0.4) !important;
  border-radius: 2px !important;
}
div.CodeMirror span.CodeMirror-nonmatchingbracket, .cm-nonmatchingBracket {
  color: #f87171 !important;
  background: rgba(239, 68, 68, 0.2) !important;
  outline: 1px solid rgba(239, 68, 68, 0.4) !important;
  border-radius: 2px !important;
}
.cm-flash-exec {
  background-color: rgba(56, 189, 248, 0.35) !important;
  border-radius: 2px !important;
  transition: background-color 0.35s ease !important;
}
.CodeMirror-hints, .cm-tooltip.cm-tooltip-autocomplete {
  position: absolute;
  z-index: 100 !important;
  overflow: hidden;
  overflow-y: auto;
  max-height: 240px;
  background-color: #090d16 !important;
  border: 1px solid #1e293b !important;
  border-radius: 6px !important;
  box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.7), 0 8px 10px -6px rgba(0, 0, 0, 0.7) !important;
  font-family: 'JetBrains Mono', 'FiraCode Nerd Font', 'Fira Code', ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace !important;
  font-size: 13px !important;
}
.CodeMirror-hint, .cm-tooltip-autocomplete ul li {
  display: flex !important;
  align-items: center !important;
  padding: 4px 8px !important;
  border-radius: 4px !important;
  color: #cbd5e1 !important;
  cursor: pointer !important;
}
li.CodeMirror-hint-active, .cm-tooltip-autocomplete ul li[aria-selected] {
  background: #1e293b !important;
  color: #38bdf8 !important;
}
.cm-hint-badge {
  display: inline-flex !important;
  align-items: center !important;
  justify-content: center !important;
  font-size: 9px !important;
  font-weight: 700 !important;
  padding: 1px 4px !important;
  border-radius: 3px !important;
  margin-right: 8px !important;
  text-transform: uppercase !important;
  letter-spacing: 0.05em !important;
  flex-shrink: 0 !important;
  line-height: 1.2 !important;
}
.cm-badge-kw { background: #3b0764 !important; color: #f472b6 !important; border: 1px solid #701a75 !important; }
.cm-badge-tbl { background: #064e3b !important; color: #34d399 !important; border: 1px solid #047857 !important; }
.cm-badge-col { background: #082f49 !important; color: #38bdf8 !important; border: 1px solid #0284c7 !important; }
.cm-badge-db { background: #1e1b4b !important; color: #a78bfa !important; border: 1px solid #4338ca !important; }
.cm-badge-fn { background: #451a03 !important; color: #fbbf24 !important; border: 1px solid #b45309 !important; }
.cm-badge-snp { background: #4c0519 !important; color: #fb7185 !important; border: 1px solid #9f1239 !important; }
.cm-hint-label, .cm-completionLabel {
  font-weight: 500 !important;
  color: #e0f2fe !important;
}
.cm-hint-type, .cm-completionDetail {
  margin-left: auto !important;
  padding-left: 16px !important;
  font-size: 11px !important;
  color: #64748b !important;
  font-style: italic !important;
}
`;

fs.writeFileSync(outCss, cssContent, 'utf8');
console.log(`✅ CodeMirror CSS generated: ${outCss}`);

// 3. Ensure stub files for addon/hint and mode/sql exist to avoid 404s
const hintDir = path.join(distVendorDir, 'addon', 'hint');
const modeDir = path.join(distVendorDir, 'mode', 'sql');
fs.mkdirSync(hintDir, { recursive: true });
fs.mkdirSync(modeDir, { recursive: true });

fs.writeFileSync(path.join(hintDir, 'show-hint.min.css'), '/* CodeMirror 6 unified styles */\n', 'utf8');
fs.writeFileSync(path.join(hintDir, 'show-hint.min.js'), '// CodeMirror 6 native autocomplete included in bundle\n', 'utf8');
fs.writeFileSync(path.join(modeDir, 'sql.min.js'), '// CodeMirror 6 @codemirror/lang-sql included in bundle\n', 'utf8');

console.log('✅ Stub files verified for zero-404 asset compatibility.');
