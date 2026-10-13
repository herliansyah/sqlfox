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
    selectionStart: 0,
    selectionEnd: 0,
    setSelectionRange(s, e) {
      this.selectionStart = s;
      this.selectionEnd = e;
    },
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
      return [];
    },
    querySelector(selector) {
      if (selector === 'input, select' || selector === 'input') {
        const inp = (this.innerHTML || '').match(/<(input|select)[^>]*>/i);
        if (inp) {
          const valMatch = inp[0].match(/value="([^"]*)"/i);
          const typeMatch = inp[0].match(/type="([^"]*)"/i);
          const mockInput = {
            tagName: inp[1].toUpperCase(),
            type: typeMatch ? typeMatch[1] : 'text',
            value: valMatch ? valMatch[1] : '',
            selectionStart: 0,
            selectionEnd: 0,
            setSelectionRange(s, e) {
              this.selectionStart = s;
              this.selectionEnd = e;
            },
            focus() {},
            select() {},
            addEventListener: this.addEventListener.bind(this),
            trigger: this.trigger.bind(this)
          };
          return mockInput;
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

// Set up mock table
vm.runInContext(`
  activeConnection = { id: 'c1', name: 'test_conn', database_name: 'test_db' };
  tabs = [{ id: 'tab-1', title: 'users', tableName: 'orders', isTableViewer: true, database: 'test_db' }];
  activeTabId = 'tab-1';
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

const cols = [
  { name: 'id', type: 'int' },
  { name: 'order_date', type: 'date' },
  { name: 'created_at', type: 'datetime' }
];
const rows = [
  [1, '2026-01-31', '2026-01-31 10:20:30']
];

sandbox.renderGridResults(cols, rows, 'id', 'orders', 'test_db');

const failures = [];

// TEST 1: Check startInlineEdit renders visible inline stepper buttons inside cell
console.log('\n--- Running TEST 1: Inline cell stepper buttons ---');
const mockTd = createMockElement('td-0-1', 'td');
sandbox.startInlineEdit(mockTd, 0, 1, 'id', 'orders');

// Stepper buttons should be integrated into the cell input container
const hasInlineStepper = mockTd.innerHTML.includes('▲') && mockTd.innerHTML.includes('▼');
// Should NOT just be <input type="date"> which hides arrows and prevents selectionStart
const usesDateInputType = mockTd.innerHTML.includes('type="date"');

console.log('Has stepper buttons in cell HTML:', hasInlineStepper);
console.log('Uses restrictive type="date":', usesDateInputType);
if (usesDateInputType) {
  failures.push('FAIL: editor should use type="text" with font-mono so caret position/selectionStart works for date parts');
}

// TEST 2: Dynamic segment stepping (month on Jan 31 -> Feb 28 clamping!)
console.log('\n--- Running TEST 2: Clamping 2026-01-31 + 1 month -> 2026-02-28 ---');
const activeCell = vm.runInContext('activeEditCell', sandbox);
if (activeCell && activeCell.input) {
  activeCell.input.value = '2026-01-31';
  // Cursor placed at index 6 (inside "01" month segment: "YYYY-MM-DD")
  activeCell.input.selectionStart = 6;
  activeCell.input.selectionEnd = 6;

  if (typeof sandbox.stepActiveDatePart === 'function') {
    sandbox.stepActiveDatePart(1);
    console.log('Stepped month from 2026-01-31 (+1m):', activeCell.input.value);
    if (activeCell.input.value !== '2026-02-28') {
      failures.push(`FAIL: 2026-01-31 + 1 month must clamp to "2026-02-28", got "${activeCell.input.value}"`);
    }
  } else {
    failures.push('FAIL: stepActiveDatePart function not implemented');
  }
}

// TEST 3: Dynamic segment stepping for Year, Day, and Datetime
console.log('\n--- Running TEST 3: Dynamic segment stepping for Year, Day, Time ---');
if (activeCell && activeCell.input && typeof sandbox.stepActiveDatePart === 'function') {
  // Reset to 2026-02-28
  activeCell.input.value = '2026-02-28';
  // Year segment (index 2)
  activeCell.input.selectionStart = 2;
  activeCell.input.selectionEnd = 2;
  sandbox.stepActiveDatePart(1);
  console.log('Stepped year from 2026-02-28 (+1y):', activeCell.input.value);
  if (activeCell.input.value !== '2027-02-28') {
    failures.push(`FAIL: 2026-02-28 + 1 year must be "2027-02-28", got "${activeCell.input.value}"`);
  }

  // Day segment (index 9)
  activeCell.input.selectionStart = 9;
  activeCell.input.selectionEnd = 9;
  sandbox.stepActiveDatePart(1);
  console.log('Stepped day from 2027-02-28 (+1d):', activeCell.input.value);
  if (activeCell.input.value !== '2027-03-01') {
    failures.push(`FAIL: 2027-02-28 + 1 day must be "2027-03-01", got "${activeCell.input.value}"`);
  }
}

// TEST 4: Datetime hour/minute/second dynamic stepping
console.log('\n--- Running TEST 4: Datetime dynamic hour/minute/second stepping ---');
const mockDtTd = createMockElement('td-0-2', 'td');
sandbox.startInlineEdit(mockDtTd, 0, 2, 'id', 'orders');
const dtCell = vm.runInContext('activeEditCell', sandbox);
if (dtCell && dtCell.input && typeof sandbox.stepActiveDatePart === 'function') {
  dtCell.input.value = '2026-01-31 10:20:30';
  // Hour segment (index 12: "10")
  dtCell.input.selectionStart = 12;
  dtCell.input.selectionEnd = 12;
  sandbox.stepActiveDatePart(1);
  console.log('Stepped hour (+1h):', dtCell.input.value);
  if (dtCell.input.value !== '2026-01-31 11:20:30') {
    failures.push(`FAIL: 10:20:30 + 1 hour must be "2026-01-31 11:20:30", got "${dtCell.input.value}"`);
  }

  // Minute segment (index 15: "20")
  dtCell.input.selectionStart = 15;
  dtCell.input.selectionEnd = 15;
  sandbox.stepActiveDatePart(-1);
  console.log('Stepped minute (-1m):', dtCell.input.value);
  if (dtCell.input.value !== '2026-01-31 11:19:30') {
    failures.push(`FAIL: 11:20:30 - 1 min must be "2026-01-31 11:19:30", got "${dtCell.input.value}"`);
  }

  // Second segment (index 18: "30")
  dtCell.input.selectionStart = 18;
  dtCell.input.selectionEnd = 18;
  sandbox.stepActiveDatePart(1);
  console.log('Stepped second (+1s):', dtCell.input.value);
  if (dtCell.input.value !== '2026-01-31 11:19:31') {
    failures.push(`FAIL: 11:19:30 + 1 sec must be "2026-01-31 11:19:31", got "${dtCell.input.value}"`);
  }
}

if (failures.length > 0) {
  console.error('\n' + failures.join('\n'));
  process.exit(1);
} else {
  console.log('\nALL TESTS PASSED!');
}
