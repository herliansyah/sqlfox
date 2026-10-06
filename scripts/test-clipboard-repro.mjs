import fs from 'fs';
import vm from 'vm';
import assert from 'assert';

const html = fs.readFileSync('web/dist/index.html', 'utf-8');
const scriptMatches = [...html.matchAll(/<script>([\s\S]*?)<\/script>/gi)];
const mainScript = scriptMatches[scriptMatches.length - 1][1];

let alertCalls = [];

const sandbox = {
  document: {
    getElementById(id) {
      return {
        id,
        tagName: 'DIV',
        value: '',
        innerHTML: '',
        textContent: '',
        dataset: {},
        classList: { add() {}, remove() {}, toggle() {}, contains() { return false; } },
        style: {},
        children: [],
        querySelectorAll() { return []; },
        querySelector() { return null; },
        addEventListener() {}
      };
    },
    createElement(tag) {
      return {
        tagName: tag.toUpperCase(),
        value: '',
        style: {},
        select() {},
        focus() {},
        remove() {},
        setAttribute() {},
        setSelectionRange() {}
      };
    },
    body: {
      appendChild() {},
      removeChild() {}
    },
    execCommand(cmd) { return true; },
    querySelectorAll() { return []; },
    querySelector() { return null; },
    addEventListener() {}
  },
  window: {
    addEventListener() {},
    location: { origin: 'http://192.168.7.8:8085' },
    isSecureContext: false
  },
  navigator: {
    // Over HTTP via IP, navigator.clipboard is UNDEFINED in browsers!
    clipboard: undefined
  },
  localStorage: { getItem() { return null; }, setItem() {} },
  setTimeout() {},
  setInterval() {},
  clearTimeout() {},
  clearInterval() {},
  console: console,
  fetch() {},
  CodeMirror: {
    fromTextArea() {
      return {
        getValue() { return ''; },
        setValue() {},
        on() {},
        getWrapperElement() { return { classList: { add() {}, remove() {} } }; },
        refresh() {}
      };
    }
  }
};

vm.createContext(sandbox);

try {
  vm.runInContext(mainScript, sandbox);
  console.log('Script loaded successfully!');
} catch (e) {
  console.error('Script load error:', e.message);
  process.exit(1);
}

// Set up table data
vm.runInContext(`
  showNativeAlert = (msg, title) => {
    alertCalls.push({ msg, title });
  };
  alertCalls = [];
  currentColumns = [{ name: 'id' }, { name: 'name' }];
  currentResults = [[1, 'Alice'], [2, 'Bob']];
`, sandbox);

console.log('Testing copyGridAsTSV when navigator.clipboard is undefined...');
try {
  await vm.runInContext(`copyGridAsTSV()`, sandbox);
  const alerts = vm.runInContext(`alertCalls`, sandbox);
  console.log('Alert calls for TSV:', alerts);
  assert.ok(alerts.length > 0 && alerts[0].title === 'Copied TSV', 'Should copy TSV successfully with fallback');
  console.log('copyGridAsTSV passed!');

  // Reset alertCalls and test copyGridAsMarkdown
  vm.runInContext(`alertCalls = [];`, sandbox);
  await vm.runInContext(`copyGridAsMarkdown()`, sandbox);
  const mdAlerts = vm.runInContext(`alertCalls`, sandbox);
  console.log('Alert calls for Markdown:', mdAlerts);
  assert.ok(mdAlerts.length > 0 && mdAlerts[0].title === 'Copied Markdown', 'Should copy Markdown successfully with fallback');
  console.log('copyGridAsMarkdown passed!');

  console.log('ALL CLIPBOARD TESTS PASSED!');
} catch (err) {
  console.error('Test failed with error:', err.message);
  process.exit(1);
}
