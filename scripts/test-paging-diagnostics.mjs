import fs from 'fs';
import vm from 'vm';
import assert from 'assert';

console.log("--- Running Paging Diagnostics Test ---");

const html = fs.readFileSync('web/dist/index.html', 'utf8');

// CHECK 1: Initial HTML state of bottom pagination bar must be hidden
const paginationBarHtmlMatch = html.match(/<div id="grid-pagination-bar"[^>]*class="([^"]*)"/);
assert(paginationBarHtmlMatch, 'grid-pagination-bar must exist in HTML');
const paginationBarClasses = paginationBarHtmlMatch[1].split(/\s+/);
assert(paginationBarClasses.includes('hidden'), 'grid-pagination-bar must be hidden initially in HTML before any query has run');

// CHECK 2: Paging labels in HTML must be in English, not Indonesian
assert(!html.includes('Tampilkan:'), 'HTML should not contain Indonesian "Tampilkan:"');
assert(!html.includes('/ hal<'), 'HTML should not contain Indonesian "/ hal"');
assert(!html.includes('Hitung Akurat<'), 'HTML should not contain Indonesian "Hitung Akurat"');
assert(html.includes('Show:'), 'HTML should contain English "Show:"');
assert(html.includes('/ page<'), 'HTML should contain English "/ page"');
assert(html.includes('Exact Count<'), 'HTML should contain English "Exact Count"');

// CHECK 3: Top pagination buttons (#keyset-pagination) must be eliminated completely
assert(!html.includes('id="keyset-pagination"'), 'keyset-pagination should be removed completely from HTML');

// Mock DOM & runtime environment for JS checks
const domElements = new Map();
function createMockEl(id = '', tag = 'div', classes = '') {
  const classSet = new Set(classes ? classes.split(/\s+/).filter(Boolean) : []);
  return {
    id,
    tagName: tag.toUpperCase(),
    value: '',
    innerHTML: '',
    textContent: '',
    max: '',
    disabled: false,
    dataset: {},
    classList: {
      set: classSet,
      add(c) { this.set.add(c); },
      remove(c) { this.set.delete(c); },
      toggle(c, f) {
        if (f === undefined) { if (this.set.has(c)) this.set.delete(c); else this.set.add(c); }
        else if (f) this.set.add(c);
        else this.set.delete(c);
      },
      contains(c) { return this.set.has(c); }
    },
    style: {},
    children: [],
    querySelectorAll() { return []; },
    querySelector() { return null; },
    addEventListener() {},
    setAttribute(k, v) { this[k] = v; },
    getAttribute(k) { return this[k] || null; },
    removeAttribute(k) { delete this[k]; },
    focus() {},
    blur() {}
  };
}

function getEl(id, tag = 'div') {
  if (!domElements.has(id)) {
    domElements.set(id, createMockEl(id, tag));
  }
  return domElements.get(id);
}

const mockDocument = {
  getElementById: (id) => getEl(id),
  createElement: (tag) => createMockEl('', tag),
  querySelectorAll: () => [],
  querySelector: () => null,
  addEventListener: () => {}
};

const sandbox = {
  document: mockDocument,
  window: {
    addEventListener: () => {},
    location: { origin: 'http://localhost' },
    localStorage: { getItem: () => null, setItem: () => {} },
    sessionStorage: { getItem: () => null, setItem: () => {} }
  },
  localStorage: { getItem: () => null, setItem: () => {} },
  sessionStorage: { getItem: () => null, setItem: () => {} },
  setTimeout: () => {},
  setInterval: () => {},
  clearTimeout: () => {},
  clearInterval: () => {},
  console: console,
  fetch: () => {},
  apiFetch: async () => ({ ok: true, json: async () => ({}) }),
  CodeMirror: {
    fromTextArea: () => ({ getValue: () => '', setValue: () => {}, on: () => {}, getWrapperElement: () => createMockEl(), refresh: () => {} })
  }
};

const lastScriptStart = html.lastIndexOf('<script>');
const lastScriptEnd = html.lastIndexOf('</script>');
const scriptContent = html.substring(lastScriptStart + '<script>'.length, lastScriptEnd);

vm.createContext(sandbox);
vm.runInContext(scriptContent, sandbox);

// CHECK 4: clearGridToInitialState hides the pagination bar
const barEl = getEl('grid-pagination-bar');
barEl.classList.remove('hidden');
vm.runInContext(`clearGridToInitialState();`, sandbox);
assert(barEl.classList.contains('hidden'), 'clearGridToInitialState must hide grid-pagination-bar');

// CHECK 5: renderGridResults shows the bottom pagination bar and formats text in English
vm.runInContext(`
  tabs = [{
    id: 'tab-query',
    title: 'test.sql',
    isTableViewer: false,
    queryPage: 0,
    pageSize: 50
  }];
  activeTabId = 'tab-query';
  currentColumns = [{ name: 'id', type: 'INT' }, { name: 'val', type: 'VARCHAR' }];
  currentResults = Array.from({ length: 120 }, (_, i) => [i + 1, 'item_' + (i + 1)]);
  renderGridResults(currentColumns, currentResults);
`, sandbox);

assert(!barEl.classList.contains('hidden'), 'renderGridResults must show grid-pagination-bar');

const statsText = getEl('grid-bottom-stats').textContent;
assert(statsText.includes('Showing 1 - 50 of 120 rows'), 'Stats text must be English "Showing 1 - 50 of 120 rows", got: ' + statsText);
assert(!statsText.includes('Menampilkan'), 'Stats text must not contain Indonesian "Menampilkan"');
assert(!statsText.includes('baris'), 'Stats text must not contain Indonesian "baris"');

console.log("All paging diagnostic checks PASSED!");
