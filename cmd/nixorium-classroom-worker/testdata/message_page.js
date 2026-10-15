// Runs view/message.js against stubs of the document and of the helpers of
// app.js it uses, and checks the message dialog and the exam mode flows.
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
    document: { getElementById: get, querySelector: () => get('exam-button') },
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

const review = (id, action, rows, extra = {}) => ({ ready: true, id, title: action, rows, expires: Date.now() + 240000, confirm: 'x', ...extra });
const okRows = (...names) => names.map((name) => ({ name }));
// What the page reads with the list of computers.
const listed = (...entries) => entries.map(([name, steps]) => ({ name, state: 'viewing', ...(steps ? { exam: { steps, since: 1 } } : {}) }));

(async () => {
  // Start: three steps, one confirmation, the result of each computer.
  {
    const s = page({
      plan: (body) => review('id-' + body.action, body.action, [{ name: 'pc01' }, { name: 'pc02', skip: true, note: 'Switched off or not reachable.' }, { name: 'pc03' }]),
      apply: (body) => ({ state: 'partial', rows: [
        { name: 'pc01', outcome: 'done' },
        { name: 'pc02', outcome: 'off', skip: true, note: 'Switched off or not reachable.' },
        { name: 'pc03', outcome: body.id === 'id-lock' ? 'skipped' : 'done', skip: body.id === 'id-lock', note: 'Nobody is signed in.' }] }),
    });
    s.tools.exam(['pc01', 'pc02', 'pc03']);
    assert(s.get('exam-dialog').open, 'the exam dialog opens');
    assert.strictEqual(s.get('exam-text').value, 'Exam in progress. Wait for the teacher.');
    await s.get('exam-review').fire('click');
    await settle();
    assert.deepStrictEqual(s.plans.map((plan) => plan.action).sort(), ['internet-block', 'lock', 'message']);
    assert(s.plans.every((plan) => plan.exam === 'start'), 'every step is tagged for the service to remember');
    assert.deepStrictEqual(s.plans.find((plan) => plan.action === 'message').message, { text: 'Exam in progress. Wait for the teacher.', untilUnlock: true });
    assert.strictEqual(s.get('action-title').textContent, 'Exam mode');
    assert(s.get('action-message').textContent.startsWith('This will block Internet, lock the screens and show this message:'));
    assert.strictEqual(s.get('action-confirm').textContent, 'Start exam mode on 2 computers');
    assert.strictEqual(s.context.actionRows.rows.length, 3);
    assert.strictEqual(s.context.actionRows.rows[1].skip, true);
    await s.get('action-confirm').fire('click');
    assert.deepStrictEqual(s.applies.map((apply) => apply.id), ['id-internet-block', 'id-lock', 'id-message'], 'the steps apply in order');
    const [title, result] = s.results[0];
    assert.strictEqual(title, 'Exam mode');
    const byName = Object.fromEntries(result.rows.map((row) => [row.name, row]));
    assert.strictEqual(byName.pc01.note, 'Internet blocked · Locked · Message shown');
    assert.strictEqual(byName.pc02.outcome, 'off');
    assert.strictEqual(byName.pc03.outcome, 'skipped');
    assert(byName.pc03.note.includes('Screen: Nobody is signed in.'));
    assert.deepStrictEqual([...s.context.chosen].sort(), ['pc02', 'pc03'], 'computers that did not take it stay selected');
    assert.strictEqual(s.seen.refreshes, 1, 'the list is read again to show what the service noted');
    assert(!s.tools.inExam('pc01'), 'the page itself remembers nothing until it reads the list');
  }

  // A page opened later reads exam mode from the list and can end it.
  {
    const s = page({
      plan: (body) => review('id-' + body.action, body.action, okRows('pc01', 'pc03')),
      apply: () => ({ state: 'completed', rows: [{ name: 'pc01', outcome: 'done' }, { name: 'pc03', outcome: 'done' }] }),
    });
    s.tools.read(listed(['pc01', ['internet', 'lock', 'message']], ['pc02'], ['pc03', ['internet']]));
    assert(s.tools.inExam('pc01') && s.tools.inExam('pc03') && !s.tools.inExam('pc02'));
    s.tools.exam(['pc01', 'pc02', 'pc03']);
    await settle();
    assert(!s.get('exam-dialog').open, 'a selection in exam mode ends it instead of asking again');
    // One review per step, each on the computers where the step is in effect.
    const bySteps = Object.fromEntries(s.plans.map((plan) => [plan.action, plan.computers]));
    assert.deepStrictEqual(bySteps, { 'message-hide': ['pc01'], unlock: ['pc01'], 'internet-allow': ['pc01', 'pc03'] });
    assert(s.plans.every((plan) => plan.exam === 'end'));
    assert.strictEqual(s.get('action-title').textContent, 'End exam mode');
    assert.strictEqual(s.get('action-message').textContent, 'This will remove the message, unlock the screens and allow Internet again.');
    assert.strictEqual(s.get('action-confirm').textContent, 'End exam mode on 2 computers');
    await s.get('action-confirm').fire('click');
    assert.deepStrictEqual(s.applies.map((apply) => apply.id), ['id-message-hide', 'id-unlock', 'id-internet-allow'], 'ending runs the steps the other way round');
    assert.deepStrictEqual(s.log.at(-1), ['runAction', 'collect', ['pc01', 'pc03']], 'the collect review is offered, not run');
    // The next list no longer shows them.
    s.tools.read(listed(['pc01'], ['pc03']));
    assert(!s.tools.inExam('pc01') && !s.tools.inExam('pc03'));
  }

  // Nothing chosen, or a bad message, stays in the dialog.
  {
    const s = page({ plan: () => assert.fail('no review'), apply: () => assert.fail('no apply') });
    s.tools.exam(['pc01']);
    for (const key of ['internet', 'lock', 'message']) s.get('exam-' + key).checked = false;
    await s.get('exam-review').fire('click');
    assert.strictEqual(s.get('exam-problem').textContent, 'Choose at least one thing to do.');
    s.get('exam-message').checked = true;
    s.get('exam-text').value = '   ';
    await s.get('exam-review').fire('click');
    assert.strictEqual(s.get('exam-problem').textContent, 'Write the message first.');
    s.get('exam-text').value = 'a\tb';
    await s.get('exam-review').fire('click');
    assert(s.get('exam-problem').textContent.includes('control characters'));
    assert(s.get('exam-dialog').open);
  }

  // A review about to end is made again; other computers qualifying asks the teacher again.
  {
    let calls = 0;
    const s = page({
      plan: (body) => { calls += 1; return review('id' + calls + '-' + body.action, body.action, okRows('pc01'), { expires: calls <= 3 ? Date.now() + 5000 : Date.now() + 240000 }); },
      apply: () => ({ state: 'completed', rows: [{ name: 'pc01', outcome: 'done' }] }),
    });
    s.tools.exam(['pc01']);
    await s.get('exam-review').fire('click');
    await settle();
    await s.get('action-confirm').fire('click');
    assert.strictEqual(calls, 6, 'the three reviews were made again');
    assert(s.applies.every((apply) => /^id[456]-/.test(apply.id)), JSON.stringify(s.applies));

    let again = 0;
    const t = page({
      plan: (body) => { again += 1; return review('r' + again, body.action, again <= 3 ? okRows('pc01') : [{ name: 'pc01', skip: true, note: 'Nobody is signed in.' }], { expires: again <= 3 ? Date.now() + 5000 : Date.now() + 240000, ready: !(again > 3 && body.action === 'lock') }); },
      apply: () => assert.fail('nothing is applied while the computers changed'),
    });
    t.tools.exam(['pc01']);
    await t.get('exam-review').fire('click');
    await settle();
    await t.get('action-confirm').fire('click');
    assert.strictEqual(t.applies.length, 0);
    assert(t.get('action-dialog').open, 'the new review stays for the teacher');
    assert(t.context.actionWarnings.lines.some((line) => line.startsWith('Screen:')));
  }

  // The service does not answer when the reviews are made again.
  {
    let calls = 0;
    const s = page({ plan: (body) => { calls += 1; if (calls > 3) throw new Error('down'); return review('x' + calls, body.action, okRows('pc01'), { expires: Date.now() + 1000 }); }, apply: () => assert.fail('no apply') });
    s.tools.exam(['pc01']);
    await s.get('exam-review').fire('click');
    await settle();
    await s.get('action-confirm').fire('click');
    assert.strictEqual(s.toasts[0][0], 'The controller did not answer; exam mode was not started.');
    assert(!s.get('action-dialog').open);
  }

  // A step whose review expired is reported; the others still run.
  {
    const s = page({
      plan: (body) => review('id-' + body.action, body.action, okRows('pc01')),
      apply: (body) => (body.id === 'id-lock' ? { state: 'expired', rows: [] } : { state: 'completed', rows: [{ name: 'pc01', outcome: 'done' }] }),
    });
    s.tools.exam(['pc01']);
    await s.get('exam-review').fire('click');
    await settle();
    await s.get('action-confirm').fire('click');
    assert.strictEqual(s.applies.length, 3);
    const row = s.results[0][1].rows[0];
    assert.strictEqual(row.outcome, 'skipped');
    assert.strictEqual(row.note, 'Internet blocked · Screen: the review expired · Message shown');
  }

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
