// Runs view/message.js against stubs of the document and of the helpers of
// app.js it uses, and checks the message dialog.
// Usage: node message_page.js view/message.js
'use strict';
const fs = require('fs');
const vm = require('vm');
const assert = require('assert');
// The script runs in its own context: compare through JSON.
assert.deepStrictEqual = (actual, expected, message) => assert.strictEqual(JSON.stringify(actual), JSON.stringify(expected), message);
const settle = () => new Promise((resolve) => setTimeout(resolve, 5));
const source = fs.readFileSync(process.argv[2], 'utf8');

function element(id) {
  const listeners = {};
  return {
    id, value: '', checked: true, disabled: false, hidden: false, textContent: '', open: false,
    classList: { toggle() {} },
    addEventListener(name, listener) { (listeners[name] ||= []).push(listener); },
    async fire(name) { for (const listener of listeners[name] || []) await listener({}); },
    showModal() { this.open = true; },
    close() { this.open = false; (listeners.close || []).forEach((listener) => listener({})); },
    focus() {},
  };
}

// A page with the helpers of app.js. scenario.plan and scenario.apply play
// the classroom service.
function page(scenario) {
  const elements = {};
  const get = (id) => (elements[id] ||= element(id));
  const seen = { log: [], toasts: [], results: [], plans: [], applies: [], refreshes: 0 };
  const context = {
    document: { getElementById: get },
    controlling: false, setControl() {},
    chosen: new Set(['pc01', 'pc02', 'pc03']),
    actionID: '', toastTimer: 0,
    actionDialog: get('action-dialog'), actionTitle: get('action-title'), actionMessage: get('action-message'),
    actionRows: { rows: [] }, actionWarnings: { lines: [] }, actionConfirm: get('action-confirm'), actionCancel: get('action-cancel'),
    showRows(list, rows) { list.rows = rows; },
    showText(list, lines) { list.lines = lines; },
    async postJSON(path, body) {
      if (path === '/api/actions/plan') {
        seen.plans.push(body);
        return scenario.plan(body);
      }
      seen.applies.push(body);
      return scenario.apply(body);
    },
    runAction(...args) { seen.log.push(['runAction', ...args]); },
    showResult(title, result) { seen.results.push([title, result]); },
    showToast(line, options) { seen.toasts.push([line, options]); },
    updateSelection() {},
    refresh() { seen.refreshes += 1; },
    clearTimeout() {}, Date, Promise, Set, Map, Number, String, Array, Object, JSON, console,
  };
  vm.createContext(context);
  vm.runInContext(source + '\nthis.classroomTools = classroomTools;', context);
  return { context, get, tools: context.classroomTools, seen, ...seen };
}

(async () => {
  // Message dialog: the review follows with the duration chosen.
  {
    const s = page({ plan: () => assert.fail('no review'), apply: () => assert.fail('no apply') });
    s.tools.message(['pc01', 'pc02']);
    assert(s.get('message-dialog').open);
    s.get('message-text').value = '  Hello\nclass ';
    s.get('message-duration').value = 'unlock';
    await s.get('message-review').fire('click');
    assert.deepStrictEqual(s.log.at(-1), ['runAction', 'message', ['pc01', 'pc02'], undefined, { message: { text: 'Hello\nclass', untilUnlock: true } }]);
    s.get('message-text').value = 'x';
    s.get('message-duration').value = '120';
    s.get('message-dialog').open = true;
    await s.get('message-review').fire('click');
    assert.deepStrictEqual(s.log.at(-1)[4], { message: { text: 'x', seconds: 120 } });
    s.get('message-text').value = 'x'.repeat(301);
    s.get('message-dialog').open = true;
    await s.get('message-review').fire('click');
    assert(s.get('message-problem').textContent.includes('300 characters') && s.get('message-dialog').open);
  }
  console.log('message.js scenarios passed');
})().catch((error) => { console.error(error); process.exit(1); });
