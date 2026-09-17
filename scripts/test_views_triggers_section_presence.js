const fs = require('fs');
const assert = require('assert');

// Read web/dist/index.html
const htmlContent = fs.readFileSync('web/dist/index.html', 'utf8');

const vm = require('vm');

class MockElement {
  constructor(id) {
    this.id = id;
    this.innerHTML = '';
    this.textContent = '';
    this.classList = {
      _classes: new Set(),
      add: (c) => this.classList._classes.add(c),
      remove: (c) => this.classList._classes.delete(c),
      contains: (c) => this.classList._classes.has(c)
    };
  }
}

const elements = {
  'schema-tree': new MockElement('schema-tree'),
  'database-selector-container': new MockElement('database-selector-container'),
  'database-switcher-active-name': new MockElement('database-switcher-active-name'),
  'database-selector': new MockElement('database-selector'),
  'table-count-badge': new MockElement('table-count-badge')
};

const sandbox = {
  document: {
    getElementById: (id) => elements[id] || null,
    addEventListener: () => {}
  },
  escapeHtml: (str) => String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;'),
  schemaFilterQuery: '',
  renderDatabaseDropdownList: () => {}
};

// Extract renderSchemaTree implementation from index.html
const fnMatch = htmlContent.match(/function renderSchemaTree\(schema\) \{([\s\S]*?)\n    \}\n\n    \/\/ --- MySQL/);
if (!fnMatch) {
  console.error("Could not find renderSchemaTree function in web/dist/index.html");
  process.exit(1);
}

const fnCode = `function renderSchemaTree(schema) {${fnMatch[1]}}`;
vm.createContext(sandbox);
vm.runInContext(fnCode, sandbox);

console.log("=== Running Feedback Loop for Views & Triggers presence ===");

// Test Case 1: Database with tables, but NO views and NO triggers
console.log("\nTest 1: Database with tables, 0 views, 0 triggers (q = '')");
const schema1 = {
  database_name: 'testdb',
  databases: ['testdb'],
  tables: [
    { name: 'users', type: 'BASE TABLE', row_count: 5 }
  ],
  triggers: []
};

sandbox.schemaFilterQuery = '';
sandbox.renderSchemaTree(schema1);
const html1 = elements['schema-tree'].innerHTML;

assert(html1.includes('Tables (1)'), 'Expected "Tables (1)" header');
assert(html1.includes('openCreateTableModal()'), 'Expected openCreateTableModal() button');
assert(html1.includes('Views (0)'), 'Expected "Views (0)" header');
assert(html1.includes('openCreateViewModal()'), 'Expected openCreateViewModal() button');
assert(html1.includes('Triggers (0)'), 'Expected "Triggers (0)" header');
assert(html1.includes('openCreateTriggerModal()'), 'Expected openCreateTriggerModal() button');
console.log("✓ Test 1 passed");

// Test Case 2: Brand new empty database (0 tables, 0 views, 0 triggers)
console.log("\nTest 2: Empty database with 0 tables, 0 views, 0 triggers (q = '')");
const schema2 = {
  database_name: 'emptydb',
  databases: ['emptydb'],
  tables: [],
  triggers: []
};

sandbox.schemaFilterQuery = '';
sandbox.renderSchemaTree(schema2);
const html2 = elements['schema-tree'].innerHTML;

assert(html2.includes('Tables (0)'), 'Expected "Tables (0)" in empty db');
assert(html2.includes('openCreateTableModal()'), 'Expected openCreateTableModal() in empty db');
assert(html2.includes('Views (0)'), 'Expected "Views (0)" in empty db');
assert(html2.includes('openCreateViewModal()'), 'Expected openCreateViewModal() in empty db');
assert(html2.includes('Triggers (0)'), 'Expected "Triggers (0)" in empty db');
assert(html2.includes('openCreateTriggerModal()'), 'Expected openCreateTriggerModal() in empty db');
console.log("✓ Test 2 passed");

// Test Case 3: Search filter active
console.log("\nTest 3: Search filter active (q = 'users')");
sandbox.schemaFilterQuery = 'users';
sandbox.renderSchemaTree(schema1);
const html3 = elements['schema-tree'].innerHTML;

assert(html3.includes('Tables (1/1)'), 'Expected Tables section when query matches table');
assert(!html3.includes('Views ('), 'Views section should be hidden when filtered out');
assert(!html3.includes('Triggers ('), 'Triggers section should be hidden when filtered out');
console.log("✓ Test 3 passed");

// Test Case 4: Search filter matches nothing
console.log("\nTest 4: Search filter matches nothing (q = 'nonexistent')");
sandbox.schemaFilterQuery = 'nonexistent';
sandbox.renderSchemaTree(schema1);
const html4 = elements['schema-tree'].innerHTML;

assert(html4.includes('No objects matching "nonexistent"'), 'Expected no objects matching message');
assert(html4.includes('clearSchemaFilter()'), 'Expected clear filter button');
console.log("✓ Test 4 passed");

console.log("\n🎉 ALL TESTS PASSED!");
