import fs from 'fs';
import vm from 'vm';
import assert from 'assert';

const html = fs.readFileSync('web/dist/index.html', 'utf-8');
const scriptMatches = [...html.matchAll(/<script>([\s\S]*?)<\/script>/gi)];
const mainScript = scriptMatches[scriptMatches.length - 1][1];

const elements = {};
function createMockElement(id = '', tag = 'div') {
  return {
    id,
    tagName: tag.toUpperCase(),
    value: '',
    checked: false,
    innerHTML: '',
    textContent: '',
    dataset: {},
    classList: {
      add() {},
      remove() {},
      toggle() {},
      contains() { return false; }
    },
    style: {},
    children: [],
    querySelectorAll() { return []; },
    querySelector() { return null; },
    addEventListener() {},
    focus() {},
    blur() {},
    setAttribute() {},
    removeAttribute() {}
  };
}

const mockDoc = {
  getElementById(id) {
    if (!elements[id]) elements[id] = createMockElement(id);
    return elements[id];
  },
  createElement(tag) { return createMockElement('', tag); },
  querySelectorAll() { return []; },
  querySelector() { return null; },
  addEventListener() {}
};

const sandbox = {
  document: mockDoc,
  window: {
    addEventListener() {},
    location: { origin: 'http://localhost' },
    localStorage: { getItem() { return null; }, setItem() {} }
  },
  localStorage: { getItem() { return null; }, setItem() {} },
  setTimeout() {},
  setInterval() {},
  clearTimeout() {},
  clearInterval() {},
  console: console,
  fetch() {},
  navigator: { clipboard: { writeText() {} } },
  CodeMirror: {
    fromTextArea() {
      return {
        getValue() { return ''; },
        setValue() {},
        on() {},
        getWrapperElement() { return createMockElement(); },
        refresh() {}
      };
    }
  }
};

vm.createContext(sandbox);

try {
  vm.runInContext(mainScript, sandbox);
  console.log('Script loaded successfully into sandbox!');
} catch (e) {
  console.error('Script load error:', e.message);
  process.exit(1);
}

// Set up mock connection & initial tables
vm.runInContext(`
  activeConnection = { id: 'c1', name: 'test' };
  schemaCatalogCache = { database_name: 'testdb', tables: [{ name: 'users' }, { name: 'products' }] };
  
  // Mock apiFetch to return rows for table requests
  apiFetch = async (url) => {
    if (url.includes('/table/users/rows')) {
      const rows = Array.from({ length: 10 }, (_, i) => [i + 1, 'User ' + (i + 1)]);
      return {
        ok: true,
        json: async () => ({ columns: [{ name: 'id', is_primary_key: true }, { name: 'name' }], rows, pk_col: 'id' })
      };
    }
    if (url.includes('/table/products/rows')) {
      const rows = Array.from({ length: 10 }, (_, i) => [100 + i + 1, 'Product ' + (i + 1)]);
      return {
        ok: true,
        json: async () => ({ columns: [{ name: 'id', is_primary_key: true }, { name: 'title' }], rows, pk_col: 'id' })
      };
    }
    return { ok: false, statusText: 'Not found', json: async () => ({}) };
  };
`, sandbox);

// STEP 1: Open users table
vm.runInContext(`
  openTableViewer('users');
`, sandbox);
await vm.runInContext(`
  loadTableRows('users');
`, sandbox);

// STEP 2: Select all rows in users table
vm.runInContext(`
  toggleSelectAllRows(true);
`, sandbox);

const selectedCountBefore = vm.runInContext(`selectedRowIndices.size`, sandbox);
console.log('Selected count in users table after select-all:', selectedCountBefore);
assert.strictEqual(selectedCountBefore, 10, 'Expected 10 rows selected in users table');

// STEP 3: Switch table to products
vm.runInContext(`
  openTableViewer('products');
`, sandbox);
await vm.runInContext(`
  loadTableRows('products');
`, sandbox);

const selectedCountAfter = vm.runInContext(`selectedRowIndices.size`, sandbox);
console.log('Selected count in products table after switching table:', selectedCountAfter);

// User requirement:
// "saat saya pindah tabel...sepuluh row yang terceklis masih terceklis harusnya ke release uncheck lagi"
// The selection MUST be released / cleared (size === 0) when switching table.
assert.strictEqual(selectedCountAfter, 0, `Expected 0 selected rows after switching table, but got ${selectedCountAfter}!`);

// STEP 4: Select some rows in products
vm.runInContext(`
  toggleSelectRow(0, true);
  toggleSelectRow(1, true);
`, sandbox);
const selectedProducts = vm.runInContext(`selectedRowIndices.size`, sandbox);
assert.strictEqual(selectedProducts, 2, 'Expected 2 rows selected in products');

// STEP 5: Switch back to users tab
const usersTabId = vm.runInContext(`tabs.find(t => t.tableName === 'users').id`, sandbox);
vm.runInContext(`
  switchTab('${usersTabId}');
`, sandbox);
await vm.runInContext(`
  loadTableRows('users');
`, sandbox);
const selectedAfterSwitchBack = vm.runInContext(`selectedRowIndices.size`, sandbox);
console.log('Selected count in users table after switching back:', selectedAfterSwitchBack);
assert.strictEqual(selectedAfterSwitchBack, 0, `Expected 0 selected rows after switching back to users tab, but got ${selectedAfterSwitchBack}!`);

console.log('ALL REGRESSION TESTS PASSED!');
