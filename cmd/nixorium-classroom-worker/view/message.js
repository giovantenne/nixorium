// Message and exam mode of the classroom view page. This script follows
// app.js and uses its dialog, review and notice handling (runAction,
// showResult and the like); app.js calls it only through classroomTools.
//
// A message is one more reviewed action. Exam mode is the page's own
// composition of three existing reviewed actions (block Internet, lock,
// show a message), confirmed once; the computers keep no exam state, and the
// page only remembers which computers it started exam mode on while it is
// open.
'use strict';

const classroomTools = (() => {
  const messageDialog = document.getElementById('message-dialog');
  const messageText = document.getElementById('message-text');
  const messageCount = document.getElementById('message-count');
  const messageDuration = document.getElementById('message-duration');
  const messageProblem = document.getElementById('message-problem');
  const examDialog = document.getElementById('exam-dialog');
  const examText = document.getElementById('exam-text');
  const examMessage = document.getElementById('exam-message');
  const examProblem = document.getElementById('exam-problem');
  const examButton = document.querySelector('.toolbar button[data-action="exam"]');

  // The controller checks the same rules again; these catch most mistakes
  // before the review.
  function problemWith(text) {
    if (!text.trim()) return 'Write the message first.';
    if (text.length > 300) return 'The message is longer than 300 characters.';
    if (text.split('\n').length > 10) return 'The message has more than 10 lines.';
    if (/[\u0000-\u0009\u000b-\u001f\u007f-\u009f\u2028\u2029]/.test(text)) return 'The message cannot contain tabs or other control characters.';
    return '';
  }

  function say(line, text) {
    line.textContent = text;
    line.hidden = !text;
  }

  // The keyboard goes to the dialog, not to a computer under remote control.
  function takeKeyboard() {
    if (controlling) setControl(false);
  }

  // Message: the dialog asks for the text and how long it stays; the
  // review follows like any action.
  let messageComputers = [];

  function openMessage(computers) {
    takeKeyboard();
    messageComputers = computers;
    say(messageProblem, '');
    messageCount.textContent = String(messageText.value.length);
    messageDialog.showModal();
    messageText.focus();
  }

  messageText.addEventListener('input', () => {
    messageCount.textContent = String(messageText.value.length);
  });
  document.getElementById('message-cancel').addEventListener('click', () => messageDialog.close());
  document.getElementById('message-review').addEventListener('click', () => {
    const problem = problemWith(messageText.value);
    if (problem) {
      say(messageProblem, problem);
      return;
    }
    const choice = messageDuration.value;
    const message = { text: messageText.value.trim() };
    if (choice === 'unlock') message.untilUnlock = true;
    else if (choice !== 'close') message.seconds = Number(choice);
    messageDialog.close();
    runAction('message', messageComputers, undefined, { message });
  });

  // Exam mode. Each step is an existing action and has its own review; the
  // page shows them as one and applies them in order, the way back in the
  // reverse order. The words are for the review (will) and the result (did),
  // starting (0) and ending (1).
  const steps = {
    internet: { label: 'Internet', actions: ['internet-block', 'internet-allow'], will: ['Internet will be blocked', 'Internet will be allowed'], did: ['Internet blocked', 'Internet allowed'], plan: ['block Internet', 'allow Internet again'] },
    lock: { label: 'Screen', actions: ['lock', 'unlock'], will: ['Screen will be locked', 'Screen will be unlocked'], did: ['Locked', 'Unlocked'], plan: ['lock the screens', 'unlock the screens'] },
    message: { label: 'Message', actions: ['message', 'message-hide'], will: ['Message will be shown', 'Message will be removed'], did: ['Message shown', 'Message removed'], plan: ['show this message', 'remove the message'] },
  };
  const startOrder = ['internet', 'lock', 'message'];
  const endOrder = ['message', 'lock', 'internet'];
  const examDefaultText = 'Exam in progress. Wait for the teacher.';
  // Computers exam mode was started on from this page, and with which steps.
  const started = new Map();
  // The computers the exam dialog was opened for, and the chain waiting for
  // the teacher's confirmation in the action dialog.
  let examTargets = [];
  let waiting = null;

  const count = (number) => number + (number === 1 ? ' computer' : ' computers');
  const names = (run) => [...new Set(run.parts.flatMap((part) => part.computers))].sort();
  const rowOf = (rows, name) => (rows || []).find((row) => row.name === name);

  function updateExamButton() {
    const ending = [...chosen].some((name) => started.has(name));
    examButton.textContent = ending ? 'End exam mode' : 'Exam mode';
    examButton.classList.toggle('primary', ending);
  }

  function openExam(computers) {
    takeKeyboard();
    examTargets = computers;
    if (!examText.value) examText.value = examDefaultText;
    examText.disabled = !examMessage.checked;
    say(examProblem, '');
    examDialog.showModal();
  }

  examMessage.addEventListener('change', () => {
    examText.disabled = !examMessage.checked;
  });
  document.getElementById('exam-cancel').addEventListener('click', () => examDialog.close());
  document.getElementById('exam-review').addEventListener('click', () => {
    const keys = startOrder.filter((key) => document.getElementById('exam-' + key).checked);
    const problem = !keys.length ? 'Choose at least one thing to do.' : (keys.includes('message') ? problemWith(examText.value) : '');
    if (problem) {
      say(examProblem, problem);
      return;
    }
    // The message stays until the screens are unlocked; on a computer that
    // is not locked the student can close it.
    const extra = { message: { text: examText.value.trim(), untilUnlock: true } };
    const parts = keys.map((key) => ({ key, action: steps[key].actions[0], computers: examTargets, extra: key === 'message' ? extra : undefined }));
    examDialog.close();
    review({ phase: 0, title: 'Exam mode', parts, keys, text: extra.message.text });
  });

  // The action dialog says what is being checked.
  function checking() {
    actionID = '';
    actionTitle.textContent = 'Checking the computers…';
    actionMessage.textContent = '';
    showRows(actionRows, []);
    showText(actionWarnings, []);
    actionConfirm.hidden = true;
    actionCancel.textContent = 'Cancel';
    if (!actionDialog.open) actionDialog.showModal();
  }

  async function plan(run) {
    await Promise.all(run.parts.map(async (part) => {
      part.review = await postJSON('/api/actions/plan', { action: part.action, computers: part.computers, ...part.extra });
    }));
  }

  async function review(run) {
    checking();
    try {
      await plan(run);
    } catch (error) {
      actionTitle.textContent = 'The action could not start';
      actionMessage.textContent = 'The controller did not answer. Try again.';
      return;
    }
    show(run);
  }

  // One review for the steps that can be done: a line for each computer
  // that says what happens to it, and one confirmation.
  function show(run) {
    const ready = run.parts.filter((part) => part.review.ready);
    const rows = names(run).map((name) => {
      const notes = [];
      let usable = false;
      for (const part of ready) {
        const row = rowOf(part.review.rows, name);
        if (!row) continue;
        const step = steps[part.key];
        if (row.skip) {
          notes.push(step.label + ': ' + (row.note || 'not possible'));
        } else {
          usable = true;
          notes.push(row.note === 'Already like this.' ? step.label + ': already like this' : step.will[run.phase]);
        }
      }
      return { name, note: notes.join(' · '), skip: !usable };
    });
    const warnings = [];
    for (const part of run.parts.filter((item) => !item.review.ready)) warnings.push(steps[part.key].label + ': ' + (part.review.message || 'not possible now.'));
    for (const part of ready) for (const line of part.review.warnings || []) if (!warnings.includes(line)) warnings.push(line);
    const usable = rows.filter((row) => !row.skip).length;
    const words = ready.map((part) => steps[part.key].plan[run.phase]);
    const list = words.length > 1 ? words.slice(0, -1).join(', ') + ' and ' + words[words.length - 1] : words[0];
    actionID = '';
    actionTitle.textContent = run.title;
    if (!usable) actionMessage.textContent = 'Exam mode is not possible on these computers now.';
    else if (run.phase === 0 && ready.some((part) => part.key === 'message')) actionMessage.textContent = 'This will ' + list + ':\n\n“' + run.text + '”';
    else actionMessage.textContent = 'This will ' + list + '.';
    showRows(actionRows, rows);
    showText(actionWarnings, warnings);
    actionCancel.textContent = 'Cancel';
    actionConfirm.hidden = !usable;
    if (usable) {
      actionConfirm.textContent = (run.phase === 0 ? 'Start exam mode on ' : 'End exam mode on ') + count(usable);
      waiting = run;
      actionConfirm.focus();
    }
  }

  // A review that is about to end could expire between two steps and leave
  // exam mode half started. Close to the end, the reviews are made again;
  // the chain goes on only if the same computers qualify, and otherwise the
  // teacher confirms the new review.
  const margin = 60 * 1000;
  const eligible = (part) => (part.review.rows || []).filter((row) => !row.skip).map((row) => row.name).join(',');

  async function applyChain(run) {
    let parts = run.parts.filter((part) => part.review.ready);
    if (parts.some((part) => part.review.expires - Date.now() < margin)) {
      actionMessage.textContent = 'Checking the computers again…';
      const before = parts.map(eligible);
      try {
        await plan(run);
      } catch (error) {
        return { unanswered: true };
      }
      parts = run.parts.filter((part) => part.review.ready);
      if (parts.length !== before.length || parts.some((part, index) => eligible(part) !== before[index])) {
        show(run);
        return { shown: true };
      }
    }
    const results = [];
    for (const part of parts) {
      actionMessage.textContent = 'Working… ' + steps[part.key].label;
      let result;
      try {
        result = await postJSON('/api/actions/apply', { id: part.review.id });
      } catch (error) {
        result = { state: 'unknown', rows: [] };
      }
      results.push({ part, result });
    }
    return { results };
  }

  // Every computer's line lists the steps. A computer counts as done when
  // each step was; off when none of them could reach it; failed when a step
  // failed; skipped otherwise (nobody signed in, for example).
  function summarize(run, results) {
    return names(run).map((name) => {
      const items = run.parts.filter((part) => part.computers.includes(name)).map((part) => {
        const step = steps[part.key];
        const entry = results.find((item) => item.part === part);
        if (!entry) return { outcome: 'skipped', text: step.label + ': not done' };
        const { result } = entry;
        const row = rowOf(result.rows, name);
        if (result.state === 'expired') return { outcome: 'skipped', text: step.label + ': the review expired' };
        if (!row) return { outcome: 'failed', text: step.label + ' failed: ' + (result.message || 'the controller did not answer') };
        switch (row.outcome) {
          case 'done': return { outcome: 'done', text: step.did[run.phase] };
          case 'off': return { outcome: 'off', text: step.label + ': switched off or not reachable' };
          case 'skipped': return { outcome: 'skipped', text: step.label + ': ' + row.note };
          default: return { outcome: 'failed', text: step.label + ' failed: ' + row.note };
        }
      });
      const outcomes = items.map((item) => item.outcome);
      let outcome = 'skipped';
      if (outcomes.every((item) => item === 'done')) outcome = 'done';
      else if (outcomes.every((item) => item === 'off')) outcome = 'off';
      else if (outcomes.includes('failed')) outcome = 'failed';
      return { name, outcome, skip: outcome !== 'done', note: outcome === 'off' ? 'Switched off or not reachable.' : items.map((item) => item.text).join(' · '), items };
    });
  }

  actionConfirm.addEventListener('click', async () => {
    const run = waiting;
    if (!run || !run.parts || actionID) return;
    waiting = null;
    actionConfirm.hidden = true;
    actionMessage.textContent = 'Working…';
    const outcome = await applyChain(run);
    // The dialog already shows a new review for the teacher to confirm.
    if (outcome.shown) return;
    if (outcome.unanswered) {
      actionDialog.close();
      showToast('The controller did not answer; exam mode was not ' + (run.phase === 0 ? 'started.' : 'ended.'), { sticky: true });
      return;
    }
    const rows = summarize(run, outcome.results);
    for (const row of rows) {
      const done = row.items.some((item) => item.outcome === 'done');
      if (run.phase === 0 && done) started.set(row.name, run.keys);
      // A computer where a step failed stays in exam mode, to end it again.
      if (run.phase === 1 && !row.items.some((item) => item.outcome === 'failed')) started.delete(row.name);
    }
    showResult(run.title, { state: rows.every((row) => row.outcome === 'done') ? 'completed' : 'partial', rows });
    chosen.clear();
    for (const row of rows) if (row.skip) chosen.add(row.name);
    updateSelection();
    // Ending an exam leads to collecting the work, with the usual review;
    // nothing is collected without it. The notice stays while it is decided.
    const offer = run.phase === 1 ? rows.filter((row) => row.outcome !== 'off').map((row) => row.name) : [];
    if (offer.length) {
      clearTimeout(toastTimer);
      runAction('collect', offer);
    } else {
      actionDialog.close();
    }
  });
  actionDialog.addEventListener('close', () => {
    waiting = null;
  });

  // End exam mode: the review of the way back on the computers the page
  // started it on, with the steps it started there.
  function endExam(computers) {
    const parts = endOrder.map((key) => ({ key, computers: computers.filter((name) => (started.get(name) || []).includes(key)) })).filter((part) => part.computers.length)
      .map((part) => ({ ...part, action: steps[part.key].actions[1] }));
    review({ phase: 1, title: 'End exam mode', parts, keys: [] });
  }

  return {
    message: openMessage,
    exam: (computers) => {
      const running = computers.filter((name) => started.has(name));
      if (running.length) endExam(running);
      else openExam(computers);
    },
    inExam: (name) => started.has(name),
    updateExamButton,
  };
})();
