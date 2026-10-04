// Classroom view: a grid of the student computers' screens, refreshed every
// two seconds. A computer opens in its own window (the same page with
// ?screen=NAME), which the teacher can move, resize or put on full screen.
// Images come only from this controller; nothing is stored.
'use strict';

const grid = document.getElementById('computers');
const summary = document.getElementById('summary');
const message = document.getElementById('message');
const viewerName = document.getElementById('viewer-name');
const viewerDetail = document.getElementById('viewer-detail');
const viewerImage = document.getElementById('viewer-image');
const controlButton = document.getElementById('viewer-control');
const controllingNotice = document.getElementById('viewer-controlling');
const fullscreenButton = document.getElementById('viewer-fullscreen');
const viewerPanel = document.getElementById('viewer-panel');
const cards = new Map();
let selected = null;
const screenName = new URLSearchParams(location.search).get('screen');
const screenMode = /^[a-z0-9-]{1,32}$/.test(screenName || '');

const stateText = {
  connecting: 'Connecting…',
  viewing: '',
  'no-session': 'Nobody is signed in',
  'no-agent': 'Classroom view is not running here',
  locked: 'The screen is locked',
  unreachable: 'Switched off or not reachable',
  failed: 'The screen could not be captured',
};

// Computers chosen with their check boxes for the toolbar actions.
const chosen = new Set();
const selectAll = document.getElementById('select-all');
const selectedCount = document.getElementById('selected-count');
const toolbarButtons = document.querySelectorAll('.toolbar button[data-action]');

function updateSelection() {
  for (const [name, entry] of cards) {
    entry.check.checked = chosen.has(name);
    entry.item.classList.toggle('chosen', chosen.has(name));
  }
  selectedCount.textContent = chosen.size === 0 ? 'None selected' : chosen.size + ' selected';
  selectAll.checked = chosen.size > 0 && chosen.size === cards.size;
  selectAll.indeterminate = chosen.size > 0 && chosen.size < cards.size;
  for (const button of toolbarButtons) button.disabled = chosen.size === 0;
}

selectAll.addEventListener('change', () => {
  chosen.clear();
  if (selectAll.checked) for (const name of cards.keys()) chosen.add(name);
  updateSelection();
});

function card(computer) {
  let entry = cards.get(computer.name);
  if (entry) return entry;
  const item = document.createElement('li');
  item.className = 'computer';
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'open';
  button.title = 'Open the screen of ' + computer.name;
  const screen = document.createElement('div');
  screen.className = 'screen';
  const image = document.createElement('img');
  image.alt = 'Screen of ' + computer.name;
  image.hidden = true;
  const placeholder = document.createElement('span');
  screen.append(image, placeholder);
  button.append(screen);
  button.addEventListener('click', () => openWindow(computer.name));
  const label = document.createElement('div');
  label.className = 'label';
  const name = document.createElement('label');
  name.className = 'name';
  const check = document.createElement('input');
  check.type = 'checkbox';
  check.addEventListener('change', () => {
    if (check.checked) chosen.add(computer.name); else chosen.delete(computer.name);
    updateSelection();
  });
  name.append(check, computer.name);
  const state = document.createElement('span');
  state.className = 'state';
  label.append(name, state);
  const badges = document.createElement('div');
  badges.className = 'badges';
  item.append(button, label, badges);
  grid.append(item);
  entry = { item, check, image, placeholder, state, badges, badgeText: '', imageAt: 0 };
  cards.set(computer.name, entry);
  return entry;
}

function screenAge(computer) {
  if (!computer.screenAt) return '';
  const seconds = Math.round((Date.now() - computer.screenAt) / 1000);
  return seconds < 60 ? '' : 'No change for ' + Math.round(seconds / 60) + ' min';
}

function render(computers) {
  let viewing = 0;
  for (const computer of computers) {
    const entry = card(computer);
    const text = stateText[computer.state] ?? computer.detail;
    // Without an image the screen area already says why; avoid repeating it.
    entry.state.textContent = computer.state === 'viewing' ? screenAge(computer) : (computer.hasImage ? text : '');
    entry.state.classList.toggle('warning', computer.state !== 'viewing' && computer.state !== 'connecting');
    if (computer.hasImage && computer.imageAt !== entry.imageAt && !selected) {
      entry.imageAt = computer.imageAt;
      entry.image.src = '/api/thumbnail?name=' + encodeURIComponent(computer.name) + '&at=' + computer.imageAt;
      entry.image.hidden = false;
      entry.placeholder.textContent = '';
    } else if (!computer.hasImage) {
      entry.placeholder.textContent = text;
    }
    const labels = [];
    if (computer.internet === 'blocked') labels.push('Internet off');
    if (computer.power) labels.push(computer.power);
    if (labels.join('|') !== entry.badgeText) {
      entry.badgeText = labels.join('|');
      entry.badges.replaceChildren(...labels.map((text) => {
        const badge = document.createElement('span');
        badge.className = 'badge';
        badge.textContent = text;
        return badge;
      }));
    }
    if (computer.state === 'viewing') viewing += 1;
    if (computer.name === selected) {
      viewerDetail.textContent = computer.state === 'viewing' ? '' : text;
    }
  }
  summary.textContent = viewing + ' of ' + computers.length + ' screens visible. Student computers show a sharing notice while you watch.';
  updateSelection();
}

// Actions: the controller reviews the computers first; nothing happens until
// the teacher confirms that review in the dialog.
const actionDialog = document.getElementById('action-dialog');
const actionTitle = document.getElementById('action-title');
const actionMessage = document.getElementById('action-message');
const actionRows = document.getElementById('action-rows');
const actionWarnings = document.getElementById('action-warnings');
const actionWordLabel = document.getElementById('action-word-label');
const actionWord = document.getElementById('action-word');
const actionWordInput = document.getElementById('action-word-input');
const actionCancel = document.getElementById('action-cancel');
const actionConfirm = document.getElementById('action-confirm');
let actionID = '';

function showRows(list, rows) {
  list.replaceChildren(...(rows || []).map((row) => {
    const item = document.createElement('li');
    item.classList.toggle('skip', Boolean(row.skip));
    item.append(row.name);
    if (row.note) {
      const note = document.createElement('span');
      note.className = 'note';
      note.textContent = ' — ' + row.note;
      item.append(note);
    }
    return item;
  }));
}

function showText(list, lines) {
  list.replaceChildren(...(lines || []).map((line) => {
    const item = document.createElement('li');
    item.textContent = line;
    return item;
  }));
}

async function postJSON(path, body) {
  const response = await fetch(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  if (!response.ok) throw new Error(await response.text());
  return response.json();
}

async function runAction(action, computers) {
  // The dialog takes the keyboard: let go of every key on the student's side.
  if (controlling) {
    pressedCodes.clear();
    send([], true);
  }
  actionID = '';
  actionTitle.textContent = 'Checking the computers…';
  actionMessage.textContent = '';
  showRows(actionRows, []);
  showText(actionWarnings, []);
  actionWordLabel.hidden = true;
  actionWordInput.value = '';
  actionConfirm.hidden = true;
  actionCancel.textContent = 'Cancel';
  if (!actionDialog.open) actionDialog.showModal();
  try {
    const review = await postJSON('/api/actions/plan', { action, computers });
    actionTitle.textContent = review.title;
    actionMessage.textContent = review.ready ? '' : (review.message || 'This action is not possible now.');
    showRows(actionRows, review.rows);
    showText(actionWarnings, review.warnings);
    if (review.ready) {
      actionID = review.id;
      actionConfirm.textContent = review.confirm;
      actionConfirm.hidden = false;
      actionWordLabel.hidden = !review.word;
      actionWord.textContent = review.word || '';
      if (review.word) actionWordInput.focus(); else actionConfirm.focus();
    }
  } catch (error) {
    actionTitle.textContent = 'The action could not start';
    actionMessage.textContent = 'The controller did not answer. Try again.';
  }
}

actionConfirm.addEventListener('click', async () => {
  if (!actionID) return;
  const id = actionID;
  actionID = '';
  actionConfirm.hidden = true;
  actionWordLabel.hidden = true;
  actionMessage.textContent = 'Working…';
  try {
    const result = await postJSON('/api/actions/apply', { id, word: actionWordInput.value });
    actionMessage.textContent = result.message || (result.state === 'completed' ? 'Done.' : '');
    showRows(actionRows, result.rows);
    showText(actionWarnings, []);
    // Computers that did not take the action stay selected for a retry.
    if (!screenMode && result.rows && result.rows.length) {
      chosen.clear();
      for (const row of result.rows) if (row.skip) chosen.add(row.name);
      updateSelection();
    }
  } catch (error) {
    actionMessage.textContent = 'The controller did not answer; the result is unknown. Check the computers before trying again.';
  }
  actionCancel.textContent = 'Close';
  actionCancel.focus();
});
actionCancel.addEventListener('click', () => actionDialog.close());
actionDialog.addEventListener('close', () => { actionID = ''; });

for (const button of toolbarButtons) {
  button.addEventListener('click', () => runAction(button.dataset.action, [...chosen].sort()));
}
for (const button of document.querySelectorAll('.actions-menu button[data-action]')) {
  button.addEventListener('click', () => {
    button.closest('details').open = false;
    if (selected) runAction(button.dataset.action, [selected]);
  });
}

// Enlarged view: frames as fast as the computer sends them (only changes).
let frameSince = 0;
let frameURL = null;
let frameLoopRunning = false;

async function frameLoop() {
  if (frameLoopRunning) return;
  frameLoopRunning = true;
  while (selected) {
    const name = selected;
    try {
      const response = await fetch('/api/frame?name=' + encodeURIComponent(name) + '&since=' + frameSince, { cache: 'no-store' });
      if (name !== selected) continue;
      if (response.status === 200) {
        const blob = await response.blob();
        const url = URL.createObjectURL(blob);
        viewerImage.src = url;
        if (frameURL) URL.revokeObjectURL(frameURL);
        frameURL = url;
      }
      frameSince = Number(response.headers.get('X-Frame') || frameSince);
    } catch (error) {
      await new Promise((resolve) => setTimeout(resolve, 1000));
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  frameLoopRunning = false;
}

// Clicking a computer again brings its existing window forward.
function openWindow(name) {
  const width = Math.round(screen.availWidth * 0.8);
  const height = Math.round(screen.availHeight * 0.8);
  window.open('/?screen=' + encodeURIComponent(name), 'nixorium-screen-' + name, 'popup,width=' + width + ',height=' + height);
}

function showScreen(name) {
  selected = name;
  document.body.classList.add('screen-mode');
  document.title = name + ' – Classroom view';
  viewerName.textContent = name;
  viewerImage.alt = 'Screen of ' + name;
  frameLoop();
}

// Remote control: mouse and keyboard in the enlarged view go to the computer.
let controlling = false;
let queue = [];
const pressedCodes = new Map();

const specialKeys = {
  Enter: 0xff0d, Backspace: 0xff08, Tab: 0xff09, Escape: 0xff1b, Delete: 0xffff,
  Insert: 0xff63, Home: 0xff50, End: 0xff57, PageUp: 0xff55, PageDown: 0xff56,
  ArrowLeft: 0xff51, ArrowUp: 0xff52, ArrowRight: 0xff53, ArrowDown: 0xff54,
  Shift: 0xffe1, Control: 0xffe3, Alt: 0xffe9, AltGraph: 0xfe03, Meta: 0xffeb,
  CapsLock: 0xffe5, ContextMenu: 0xff67, ' ': 0x20,
};

function keysym(event) {
  if (event.key in specialKeys) return specialKeys[event.key];
  const fn = /^F([1-9]|1[0-2])$/.exec(event.key);
  if (fn) return 0xffbe + Number(fn[1]) - 1;
  const chars = Array.from(event.key);
  if (chars.length !== 1) return 0;
  const point = chars[0].codePointAt(0);
  return point < 0x100 ? point : 0x01000000 + point;
}

function send(events, release) {
  if (!selected) return Promise.resolve();
  return fetch('/api/input?name=' + encodeURIComponent(selected), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ events, release: Boolean(release) }),
    keepalive: true,
  }).catch(() => {});
}

function queueEvent(event) {
  if (event.kind === 'move' && queue.length && queue[queue.length - 1].kind === 'move') {
    queue[queue.length - 1] = event;
  } else {
    queue.push(event);
  }
}

setInterval(() => {
  if (!controlling || queue.length === 0) return;
  const events = queue.splice(0, 128);
  send(events, false);
}, 40);

// The image is letterboxed (object-fit: contain): measure the drawn screen.
function position(event) {
  const box = viewerImage.getBoundingClientRect();
  const ratio = (viewerImage.naturalWidth || 16) / (viewerImage.naturalHeight || 10);
  let width = box.width;
  let height = width / ratio;
  if (height > box.height) {
    height = box.height;
    width = height * ratio;
  }
  const left = box.left + (box.width - width) / 2;
  const top = box.top + (box.height - height) / 2;
  const x = Math.min(1, Math.max(0, (event.clientX - left) / width));
  const y = Math.min(1, Math.max(0, (event.clientY - top) / height));
  return { kind: 'move', x, y };
}

const buttons = ['left', 'middle', 'right'];

viewerImage.addEventListener('pointermove', (event) => { if (controlling) queueEvent(position(event)); });
viewerImage.addEventListener('pointerdown', (event) => {
  if (!controlling || !buttons[event.button]) return;
  event.preventDefault();
  viewerImage.focus();
  queueEvent(position(event));
  queueEvent({ kind: 'button', button: buttons[event.button], pressed: true });
});
viewerImage.addEventListener('pointerup', (event) => {
  if (!controlling || !buttons[event.button]) return;
  event.preventDefault();
  queueEvent({ kind: 'button', button: buttons[event.button], pressed: false });
});
viewerImage.addEventListener('contextmenu', (event) => { if (controlling) event.preventDefault(); });
viewerImage.addEventListener('wheel', (event) => {
  if (!controlling) return;
  event.preventDefault();
  queueEvent({ kind: 'scroll', steps: event.deltaY > 0 ? 1 : -1 });
}, { passive: false });

window.addEventListener('keydown', (event) => {
  if (!controlling || event.target === controlButton || actionDialog.open) return;
  const symbol = keysym(event);
  if (!symbol) return;
  event.preventDefault();
  pressedCodes.set(event.code, symbol);
  queueEvent({ kind: 'key', keysym: symbol, pressed: true });
});
window.addEventListener('keyup', (event) => {
  if (!controlling || actionDialog.open) return;
  const symbol = pressedCodes.get(event.code) || keysym(event);
  pressedCodes.delete(event.code);
  if (!symbol) return;
  event.preventDefault();
  queueEvent({ kind: 'key', keysym: symbol, pressed: false });
});

function setControl(on) {
  if (controlling === on) return;
  controlling = on;
  controlButton.textContent = on ? 'Stop control' : 'Take control';
  controlButton.classList.toggle('primary', !on);
  controllingNotice.hidden = !on;
  viewerImage.classList.toggle('control', on);
  if (on) {
    viewerImage.focus();
  } else {
    queue = [];
    pressedCodes.clear();
    send([], true);
  }
}

controlButton.addEventListener('click', () => setControl(!controlling));

// Full screen keeps the system keys (Alt+Tab, Super, Esc) for the student's
// computer; holding Esc leaves it.
fullscreenButton.addEventListener('click', () => {
  if (document.fullscreenElement) {
    document.exitFullscreen().catch(() => {});
  } else {
    // A dialog itself cannot be full screen; its content can.
    viewerPanel.requestFullscreen().catch(() => {});
  }
});
const viewerBar = viewerPanel.querySelector('.viewer-bar');

// In full screen the bar appears when the pointer reaches the top edge.
viewerPanel.addEventListener('pointermove', (event) => {
  if (!document.fullscreenElement) return;
  const reveal = viewerPanel.classList.contains('show-bar') ? viewerBar.offsetHeight + 8 : 4;
  viewerPanel.classList.toggle('show-bar', event.clientY <= reveal);
});
document.addEventListener('fullscreenchange', () => {
  const full = Boolean(document.fullscreenElement);
  viewerPanel.classList.remove('show-bar');
  fullscreenButton.textContent = full ? 'Leave full screen' : 'Full screen';
  if (full && navigator.keyboard && navigator.keyboard.lock) {
    navigator.keyboard.lock().catch(() => {});
  } else if (!full && navigator.keyboard && navigator.keyboard.unlock) {
    navigator.keyboard.unlock();
  }
  viewerImage.focus();
});
document.getElementById('viewer-close').addEventListener('click', () => window.close());
// Closing the window gives control back and lets go of every key.
window.addEventListener('pagehide', () => setControl(false));
window.addEventListener('blur', () => { if (controlling) { pressedCodes.clear(); send([], true); } });

// A computer's window has no grid: its bar shows the notice instead.
function notice(text) {
  if (screenMode) {
    if (text) viewerDetail.textContent = text;
  } else {
    message.textContent = text;
    message.hidden = !text;
  }
}

async function refresh() {
  try {
    const response = await fetch('/api/computers', { cache: 'no-store' });
    if (response.status === 403) {
      notice('This page has expired. Open the classroom view again from Nixorium.');
      return;
    }
    if (!response.ok) throw new Error(String(response.status));
    const data = await response.json();
    notice('');
    render(data.computers);
  } catch (error) {
    notice('The controller is not answering; trying again.');
  }
  setTimeout(refresh, 2000);
}

if (screenMode) showScreen(screenName);
refresh();
