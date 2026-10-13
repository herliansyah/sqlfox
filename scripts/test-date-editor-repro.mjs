import fs from 'fs';
import vm from 'vm';
import assert from 'assert';

const html = fs.readFileSync('web/dist/index.html', 'utf-8');
const scriptMatches = [...html.matchAll(/<script>([\s\S]*?)<\/script>/gi)];
const mainScript = scriptMatches[scriptMatches.length - 1][1];

const elements = {};
function createMockElement(id = '', tag = 'div') {
  const el = {
    id,
    tagName: tag.toUpperCase(),
    value: '',
    checked: false,
    innerHTML: '',
    textContent: '',
    dataset: {},
    classList: {
      add(...cls) {
        this._classes = this._classes || new Set();
        cls.forEach(c => this._classes.add(c));
      },
      remove(...cls) {
        this._classes = this._classes || new Set();
        cls.forEach(c => this._classes.delete(c));
      },
      toggle(c) {
        this._classes = this._classes || new Set();
        if (this._classes.has(c)) this._classes.delete(c);
        else this._classes.add(c);
      },
      contains(c) {
        return Boolean(this._classes && this._classes.has(c));
      }
    },
    style: {},
    children: [],
    listeners: {},
    addEventListener(evt, fn) {
      if (!this.listeners[evt]) this.listeners[evt] = [];
      this.listeners[evt].push(fn);
    },
    trigger(evt, payload = {}) {
      const fns = this.listeners[evt] || [];
      fns.forEach(fn => fn(payload));
    },
    querySelectorAll(selector) {
      // Simple mock for querySelectorAll
      const results = [];
      function search(curr) {
        if (selector.startsWith('.') && curr.classList && curr.classList.contains(selector.slice(1))) {
          results.push(curr);
        }
        if (selector === 'button' && curr.tagName === 'BUTTON') results.push(curr);
        if (selector === 'input' && curr.tagName === 'INPUT') results.push(curr);
        (curr.children || []).forEach(search);
      }
      search(el);
      return results;
    },
    querySelector(selector) {
      if (selector === 'input, select') {
        const inp = (this.innerHTML || '').match(/<(input|select)[^>]*>/i);
        if (inp) {
          const valMatch = inp[0].match(/value="([^"]*)"/i);
          const typeMatch = inp[0].match(/type="([^"]*)"/i);
          return {
            tagName: inp[1].toUpperCase(),
            type: typeMatch ? typeMatch[1] : 'text',
            value: valMatch ? valMatch[1] : '',
            focus() {},
            select() {},
            addEventListener: this.addEventListener.bind(this),
            trigger: this.trigger.bind(this)
          };
        }
      }
      return null;
    },
    focus() {},
    blur() {},
    setAttribute(k, v) { this[k] = v; },
    removeAttribute(k) { delete this[k]; }
  };
  return el;
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
  setTimeout(fn) { if (typeof fn === 'function') fn(); },
  setInterval() {},
  clearTimeout() {},
  clearInterval() {},
  console: console,
  fetch() {},
  navigator: { clipboard: { writeText() {} } },
  CodeMirror: Object.assign(function() {
    return {
      getValue() { return ''; },
      setValue() {},
      getSelection() { return ''; },
      on() {},
      getWrapperElement() { return createMockElement(); },
      refresh() {}
    };
  }, {
    fromTextArea() {
      return {
        getValue() { return ''; },
        setValue() {},
        getSelection() { return ''; },
        on() {},
        getWrapperElement() { return createMockElement(); },
        refresh() {}
      };
    }
  })
};

vm.createContext(sandbox);

try {
  vm.runInContext(mainScript, sandbox);
  console.log('Script loaded successfully into sandbox!');
} catch (e) {
  console.error('Script load error:', e.message);
  process.exit(1);
}

// Set up mock connection & table
vm.runInContext(`
  activeConnection = { id: 'c1', name: 'test_conn', database_name: 'test_db' };
  tabs = [{ id: 'tab-1', title: 'users', tableName: 'orders', isTableViewer: true, database: 'test_db' }];
  activeTabId = 'tab-1';
  activeTab = tabs[0];
  activeGridTable = 'orders';
  activeGridDB = 'test_db';

  schemaCatalogCache = {
    database_name: 'test_db',
    tables: [{
      name: 'orders',
      columns: [
        { name: 'id', type: 'int', is_primary_key: true },
        { name: 'order_date', type: 'date', is_primary_key: false },
        { name: 'created_at', type: 'datetime', is_primary_key: false }
      ]
    }]
  };
`, sandbox);

// TEST 1: Check rendering in grid
console.log('\n--- Running TEST 1: Grid date and datetime display ---');
const cols = [
  { name: 'id', type: 'int' },
  { name: 'order_date', type: 'date' },
  { name: 'created_at', type: 'datetime' }
];
const rows = [
  [1, '2026-10-13T00:00:00Z', '2026-10-13T11:04:49Z'],
  [2, '2026-10-14 00:00:00', '2026-10-14 15:30:00']
];

sandbox.renderGridResults(cols, rows, 'id', 'orders', 'test_db');
const gridTbodyHtml = sandbox.document.getElementById('grid-tbody').innerHTML;

// Assert date display in row 0
const hasIsoDateRow0 = gridTbodyHtml.includes('2026-10-13T00:00:00Z');
const hasIsoDateRow1 = gridTbodyHtml.includes('2026-10-14 00:00:00');
const hasDateOnlyRow0 = gridTbodyHtml.includes('>2026-10-13<');
const hasDateOnlyRow1 = gridTbodyHtml.includes('>2026-10-14<');

console.log('Row 0 order_date contains raw timestamp (T00:00:00Z):', hasIsoDateRow0);
console.log('Row 1 order_date contains raw timestamp ( 00:00:00):', hasIsoDateRow1);
console.log('Row 0 order_date is formatted as date-only (2026-10-13):', hasDateOnlyRow0);
console.log('Row 1 order_date is formatted as date-only (2026-10-14):', hasDateOnlyRow1);

// Assert datetime display in row 0
const hasRawIsoDatetime = gridTbodyHtml.includes('2026-10-13T11:04:49Z');
const hasCleanDatetime = gridTbodyHtml.includes('>2026-10-13 11:04:49<');
console.log('Row 0 created_at contains raw ISO string (T11:04:49Z):', hasRawIsoDatetime);
console.log('Row 0 created_at is formatted as clean SQL datetime (2026-10-13 11:04:49):', hasCleanDatetime);

// TEST 2: Check startInlineEdit on date cell
console.log('\n--- Running TEST 2: Inline editor on date cell ---');
const mockTd = createMockElement('td-0-1', 'td');
sandbox.startInlineEdit(mockTd, 0, 1, 'id', 'orders');
console.log('Editor HTML rendered on date cell:\n', mockTd.innerHTML);

const inputMatch = mockTd.innerHTML.match(/<input[^>]*value="([^"]*)"[^>]*>/i);
const editorVal = inputMatch ? inputMatch[1] : null;
console.log('Date editor default input value:', editorVal);

const hasUpButton = mockTd.innerHTML.includes('▲') || mockTd.innerHTML.includes('title="Increment date') || mockTd.innerHTML.includes('step-up');
const hasDownButton = mockTd.innerHTML.includes('▼') || mockTd.innerHTML.includes('title="Decrement date') || mockTd.innerHTML.includes('step-down');
console.log('Has up stepper button:', hasUpButton);
console.log('Has down stepper button:', hasDownButton);

// TEST 3: Stepping date with stepActiveDate
console.log('\n--- Running TEST 3: Date stepping logic ---');
let steppedUpVal = null;
let steppedDownVal = null;
if (typeof sandbox.stepActiveDate === 'function') {
  // Stepping on date cell (cell 0, 1)
  sandbox.stepActiveDate(1);
  steppedUpVal = vm.runInContext('activeEditCell ? activeEditCell.input.value : null', sandbox);
  console.log('Stepped up (+1d) value:', steppedUpVal);
  sandbox.stepActiveDate(-2);
  steppedDownVal = vm.runInContext('activeEditCell ? activeEditCell.input.value : null', sandbox);
  console.log('Stepped down (-2d) value:', steppedDownVal);

  // Test keyboard ArrowUp on active input
  const curInput = vm.runInContext('activeEditCell ? activeEditCell.input : null', sandbox);
  let defaultPrevented = false;
  curInput.trigger('keydown', { key: 'ArrowUp', preventDefault() { defaultPrevented = true; } });
  const keySteppedVal = vm.runInContext('activeEditCell ? activeEditCell.input.value : null', sandbox);
  console.log('Keyboard ArrowUp stepped value:', keySteppedVal, 'preventDefault called:', defaultPrevented);
}

// TEST 4: Datetime cell inline editor and stepping
console.log('\n--- Running TEST 4: Datetime cell inline editor ---');
const mockDtTd = createMockElement('td-0-2', 'td');
sandbox.startInlineEdit(mockDtTd, 0, 2, 'id', 'orders');
const dtMatch = mockDtTd.innerHTML.match(/<input[^>]*value="([^"]*)"[^>]*>/i);
const dtEditorVal = dtMatch ? dtMatch[1] : null;
console.log('Datetime editor default input value:', dtEditorVal);
const hasDtUpButton = mockDtTd.innerHTML.includes('▲');
const hasDtDownButton = mockDtTd.innerHTML.includes('▼');

// Step datetime up +1d
sandbox.stepActiveDate(1);
const dtSteppedUpVal = vm.runInContext('activeEditCell ? activeEditCell.input.value : null', sandbox);
console.log('Datetime stepped up (+1d) value:', dtSteppedUpVal);

// Verification check
let failures = [];
if (hasIsoDateRow0 || !hasDateOnlyRow0) {
  failures.push('FAIL: order_date row 0 should only display date (2026-10-13), but displayed timestamp with time');
}
if (hasIsoDateRow1 || !hasDateOnlyRow1) {
  failures.push('FAIL: order_date row 1 should only display date (2026-10-14), but displayed timestamp with time');
}
if (hasRawIsoDatetime || !hasCleanDatetime) {
  failures.push('FAIL: created_at row 0 should display clean datetime (2026-10-13 11:04:49)');
}
if (editorVal !== '2026-10-13') {
  failures.push(`FAIL: date cell editor default value must be "2026-10-13", got "${editorVal}"`);
}
if (!hasUpButton || !hasDownButton) {
  failures.push('FAIL: date cell editor must have up and down buttons');
}
if (typeof sandbox.stepActiveDate !== 'function') {
  failures.push('FAIL: stepActiveDate function is not defined');
}
if (dtEditorVal !== '2026-10-13 11:04:49') {
  failures.push(`FAIL: datetime editor default value must be "2026-10-13 11:04:49", got "${dtEditorVal}"`);
}
if (!hasDtUpButton || !hasDtDownButton) {
  failures.push('FAIL: datetime cell editor must have up and down buttons');
}

if (steppedUpVal !== '2026-10-14') {
  failures.push(`FAIL: steppedUpVal must be "2026-10-14", got "${steppedUpVal}"`);
}
if (steppedDownVal !== '2026-10-12') {
  failures.push(`FAIL: steppedDownVal must be "2026-10-12", got "${steppedDownVal}"`);
}
if (dtSteppedUpVal !== '2026-10-14 11:04:49') {
  failures.push(`FAIL: dtSteppedUpVal must be "2026-10-14 11:04:49", got "${dtSteppedUpVal}"`);
}

if (failures.length > 0) {
  console.error('\n' + failures.join('\n'));
  process.exit(1);
} else {
  console.log('\nALL TESTS PASSED!');
}

