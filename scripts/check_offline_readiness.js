const fs = require('fs');
const http = require('http');

async function testFrontendIndependence() {
  console.log('--- Phase 1 Feedback Loop: Frontend Self-Containment & Offline Readiness ---');
  let errors = [];

  const html = fs.readFileSync('web/dist/index.html', 'utf8');

  // Check 1: Zero remote CDN dependencies
  const remoteScripts = [...html.matchAll(/<script[^>]+src=["'](https?:[^"']+)["']/gi)].map(m => m[1]);
  const remoteStyles = [...html.matchAll(/<link[^>]+href=["'](https?:[^"']+)["']/gi)].map(m => m[1]);

  console.log(`Checking for external CDN scripts... found ${remoteScripts.length}:`, remoteScripts);
  console.log(`Checking for external CDN styles... found ${remoteStyles.length}:`, remoteStyles);

  if (remoteScripts.length > 0) {
    errors.push(`FAIL: Found ${remoteScripts.length} remote external scripts (blocks rendering & fails on offline/LAN): ${remoteScripts.join(', ')}`);
  }
  if (remoteStyles.length > 0) {
    errors.push(`FAIL: Found ${remoteStyles.length} remote external styles: ${remoteStyles.join(', ')}`);
  }

  // Check 2: Verify all referenced local vendor assets exist on disk
  const localScripts = [...html.matchAll(/<script[^>]+src=["'](\/vendor\/[^"']+)["']/gi)].map(m => m[1]);
  const localStyles = [...html.matchAll(/<link[^>]+href=["'](\/vendor\/[^"']+)["']/gi)].map(m => m[1]);

  for (const p of [...localScripts, ...localStyles]) {
    const diskPath = 'web/dist' + p;
    if (!fs.existsSync(diskPath)) {
      errors.push(`FAIL: Referenced local asset not found on disk: ${diskPath}`);
    }
  }

  // Check 3: Essential core styles must exist locally (not dependent on external JIT CDN)
  // If Tailwind is only loaded via cdn.tailwindcss.com, .hidden, flex, grid, etc. have no local definitions.
  const hasLocalTailwindOrStyles = fs.existsSync('web/dist/vendor/tailwind') || 
                                   html.includes('.hidden { display: none') || 
                                   fs.existsSync('web/dist/tailwind.css') ||
                                   fs.existsSync('web/dist/vendor/tailwind.min.css') ||
                                   html.includes('id="embedded-tailwind"');

  if (!hasLocalTailwindOrStyles && remoteScripts.some(s => s.includes('tailwindcss'))) {
    errors.push('FAIL: Application completely depends on cdn.tailwindcss.com! When opened from LAN or slow network, DOM will render unstyled ("tampilannya berantakan") and take long to load ("loadingnya lama").');
  }

  if (errors.length > 0) {
    console.error('\n❌ FEEDBACK LOOP FAILED:');
    errors.forEach(e => console.error('  - ' + e));
    process.exit(1);
  } else {
    console.log('\n✅ FEEDBACK LOOP PASSED: All frontend assets are self-contained and offline-ready!');
    process.exit(0);
  }
}

testFrontendIndependence().catch(err => {
  console.error('Test execution error:', err);
  process.exit(1);
});
