import assert from 'node:assert/strict';
import { computeVisibleColumnWindow } from './gridHelpers';

// Empty columns
const empty = computeVisibleColumnWindow([], {}, 0, 800, 64, 4);
assert.equal(empty.startColIdx, 0);
assert.equal(empty.endColIdx, 0);
assert.equal(empty.leftSpacerWidth, 0);
assert.equal(empty.rightSpacerWidth, 0);
assert.equal(empty.totalWidth, 64);

// 100 columns x 120px = 12000px + 64px gutter
const cols = Array.from({ length: 100 }, (_, i) => `c${i}`);
const widths = Object.fromEntries(cols.map((c) => [c, 120]));

const atStart = computeVisibleColumnWindow(cols, widths, 0, 864, 64, 4);
assert.equal(atStart.startColIdx, 0);
assert.equal(atStart.leftSpacerWidth, 0);
// 800px viewport / 120px = 7 cols (0..6) + 4 overscan = 11
assert.equal(atStart.endColIdx, 11);
assert.equal(atStart.rightSpacerWidth, (100 - 11) * 120);
assert.equal(atStart.totalWidth, 64 + 100 * 120);

const mid = computeVisibleColumnWindow(cols, widths, 2400, 864, 64, 4);
// 2400 / 120 = col 20..26 visible -> with overscan 4: 16..31
assert.equal(mid.startColIdx, 16);
assert.equal(mid.endColIdx, 31);
assert.equal(mid.leftSpacerWidth, 16 * 120);
assert.equal(mid.rightSpacerWidth, (100 - 31) * 120);

const atEnd = computeVisibleColumnWindow(cols, widths, 12000, 864, 64, 4);
assert.equal(atEnd.endColIdx, 100);
assert.equal(atEnd.rightSpacerWidth, 0);

console.log('✅ gridHelpers.test.ts passed');
