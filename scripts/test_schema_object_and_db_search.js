const fs = require('fs');

console.log('--- Testing Schema Object and Database Search & Filter ---');
const html = fs.readFileSync('web/dist/index.html', 'utf8');

let errors = [];

// 1. Schema Object Filter elements
if (!html.includes('id="schema-filter-input"')) {
  errors.push('FAIL: #schema-filter-input is missing from sidebar!');
}
if (!html.includes('oninput="filterSchemaTree(this.value)"')) {
  errors.push('FAIL: #schema-filter-input lacks oninput handler filterSchemaTree(this.value)!');
}
if (!html.includes('id="schema-filter-clear-btn"')) {
  errors.push('FAIL: #schema-filter-clear-btn is missing from sidebar!');
}

// 2. Schema Object Filter logic
if (!html.includes('function filterSchemaTree(')) {
  errors.push('FAIL: filterSchemaTree function is not defined!');
}
if (!html.includes('function clearSchemaFilter(')) {
  errors.push('FAIL: clearSchemaFilter function is not defined!');
}

// 3. Searchable Database Switcher elements
if (!html.includes('id="database-switcher-btn"')) {
  errors.push('FAIL: #database-switcher-btn is missing from sidebar!');
}
if (!html.includes('id="database-dropdown-menu"')) {
  errors.push('FAIL: #database-dropdown-menu popover is missing!');
}
if (!html.includes('id="database-search-input"')) {
  errors.push('FAIL: #database-search-input is missing from database dropdown!');
}
if (!html.includes('oninput="filterDatabaseList(this.value)"')) {
  errors.push('FAIL: #database-search-input lacks oninput handler filterDatabaseList(this.value)!');
}
if (!html.includes('id="database-list-tree"')) {
  errors.push('FAIL: #database-list-tree container is missing!');
}

// 4. Database Filter logic
if (!html.includes('function toggleDatabaseDropdown(')) {
  errors.push('FAIL: toggleDatabaseDropdown function is not defined!');
}
if (!html.includes('function filterDatabaseList(')) {
  errors.push('FAIL: filterDatabaseList function is not defined!');
}
if (!html.includes('function selectDatabase(')) {
  errors.push('FAIL: selectDatabase function is not defined!');
}

if (errors.length > 0) {
  console.error('\n❌ TEST FAILED:');
  errors.forEach(e => console.error('  - ' + e));
  process.exit(1);
} else {
  console.log('\n✅ TEST PASSED: All Schema and Database search/filter elements and handlers verified!');
  process.exit(0);
}
