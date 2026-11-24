import { execFileSync } from 'node:child_process';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';

const esbuildBin = resolve(
  'web/frontend/node_modules/.bin',
  process.platform === 'win32' ? 'esbuild.cmd' : 'esbuild'
);
const testFiles = process.argv.slice(2);
if (testFiles.length === 0) {
  console.error('Usage: node scripts/run-ts-tests.mjs <test-file.ts>...');
  process.exit(1);
}

const outDir = mkdtempSync(join(tmpdir(), 'sqlfox-ts-test-'));
try {
  const outFiles = testFiles.map((file, i) => {
    const outFile = join(outDir, `test-${i}.mjs`);
    execFileSync(esbuildBin, [
      file,
      '--bundle',
      '--platform=node',
      '--format=esm',
      `--outfile=${outFile}`,
    ], { stdio: 'inherit', shell: process.platform === 'win32' });
    return outFile;
  });
  execFileSync(process.execPath, ['--test', ...outFiles], { stdio: 'inherit' });
} finally {
  rmSync(outDir, { recursive: true, force: true });
}
