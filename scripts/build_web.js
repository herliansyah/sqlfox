// Thin wrapper: the frontend is now the Svelte 5 app in web/frontend/.
// Legacy HTML assembly pipeline (web/src) has been retired.
const { execSync } = require('child_process');
const path = require('path');

const rootDir = path.resolve(__dirname, '..');

console.log('--- Building Svelte frontend (vite) ---');
execSync('npm run build:svelte', { cwd: rootDir, stdio: 'inherit' });

// Keep companion assets that ship next to the bundle.
const srcChangelog = path.join(rootDir, 'CHANGELOG.md');
const dstChangelog = path.join(rootDir, 'web', 'dist', 'CHANGELOG.md');
try {
  require('fs').copyFileSync(srcChangelog, dstChangelog);
  console.log('✅ Copied CHANGELOG.md into web/dist/');
} catch (err) {
  console.warn('⚠️  CHANGELOG.md copy skipped:', err.message);
}

console.log('✅ Frontend build complete: web/dist/index.html (Svelte bundle)');
