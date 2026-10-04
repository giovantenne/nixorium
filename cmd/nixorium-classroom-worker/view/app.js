// Classroom view: a grid of the student computers' screens, refreshed every
// two seconds. Images come only from this controller; nothing is stored.
'use strict';

const grid = document.getElementById('computers');
const summary = document.getElementById('summary');
const message = document.getElementById('message');
const viewer = document.getElementById('viewer');
const viewerName = document.getElementById('viewer-name');
const viewerDetail = document.getElementById('viewer-detail');
const viewerImage = document.getElementById('viewer-image');
const controlButton = document.getElementById('viewer-control');
const controllingNotice = document.getElementById('viewer-controlling');
const fullscreenButton = document.getElementById('viewer-fullscreen');
const viewerPanel = document.getElementById('viewer-panel');
const cards = new Map();
let selected = null;

const stateText = {
  connecting: 'Connecting…',
  viewing: '',
  'no-session': 'Nobody is signed in',
  'no-agent': 'Classroom view is not running here',
  locked: 'The screen is locked',
  unreachable: 'Switched off or not reachable',
  failed: 'The screen could not be captured',
};

function card(computer) {
  let entry = cards.get(computer.name);
  if (entry) return entry;
  const item = document.createElement('li');
  item.className = 'computer';
  const button = document.createElement('button');
  button.type = 'button';
  const screen = document.createElement('div');
  screen.className = 'screen';
  const image = document.createElement('img');
  image.alt = 'Screen of ' + computer.name;
  image.hidden = true;
  const placeholder = document.createElement('span');
  screen.append(image, placeholder);
  const label = document.createElement('div');
  label.className = 'label';
  const name = document.createElement('span');
  name.className = 'name';
  name.textContent = computer.name;
  const state = document.createElement('span');
  state.className = 'state';
  label.append(name, state);
  button.append(screen, label);
  button.addEventListener('click', () => open(computer.name));
  item.append(button);
  grid.append(item);
  entry = { image, placeholder, state, imageAt: 0 };
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
    if (computer.state === 'viewing') viewing += 1;
    if (computer.name === selected) {
      viewerDetail.textContent = computer.state === 'viewing' ? '' : text;
    }
  }
  summary.textContent = viewing + ' of ' + computers.length + ' screens visible. Student computers show a sharing notice while you watch.';
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

function open(name) {
  selected = name;
  frameSince = 0;
  viewerName.textContent = name;
  viewerDetail.textContent = '';
  const entry = cards.get(name);
  viewerImage.alt = 'Screen of ' + name;
  viewerImage.src = entry && !entry.image.hidden ? entry.image.src : '';
  viewer.showModal();
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
  if (!controlling || event.target === controlButton) return;
  const symbol = keysym(event);
  if (!symbol) return;
  event.preventDefault();
  pressedCodes.set(event.code, symbol);
  queueEvent({ kind: 'key', keysym: symbol, pressed: true });
});
window.addEventListener('keyup', (event) => {
  if (!controlling) return;
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
// Outside full screen the viewer moves by dragging its bar.
const viewerBar = viewerPanel.querySelector('.viewer-bar');
let drag = null;
viewerBar.addEventListener('pointerdown', (event) => {
  if (document.fullscreenElement || event.button !== 0 || event.target.closest('button')) return;
  const box = viewer.getBoundingClientRect();
  drag = { dx: event.clientX - box.left, dy: event.clientY - box.top, width: box.width, height: box.height };
  viewerBar.setPointerCapture(event.pointerId);
  event.preventDefault();
});
viewerBar.addEventListener('pointermove', (event) => {
  if (!drag) return;
  const left = Math.min(Math.max(0, event.clientX - drag.dx), window.innerWidth - drag.width);
  const top = Math.min(Math.max(0, event.clientY - drag.dy), window.innerHeight - drag.height);
  Object.assign(viewer.style, { margin: '0', inset: 'auto', left: left + 'px', top: top + 'px' });
});
viewerBar.addEventListener('pointerup', () => { drag = null; });
viewerBar.addEventListener('pointercancel', () => { drag = null; });

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
// While controlling, Escape belongs to the student's computer.
viewer.addEventListener('cancel', (event) => { if (controlling) event.preventDefault(); });
document.getElementById('viewer-close').addEventListener('click', () => viewer.close());
viewer.addEventListener('close', () => {
  if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
  viewer.removeAttribute('style');
  setControl(false);
  selected = null;
});
window.addEventListener('blur', () => { if (controlling) { pressedCodes.clear(); send([], true); } });

async function refresh() {
  try {
    const response = await fetch('/api/computers', { cache: 'no-store' });
    if (response.status === 403) {
      message.textContent = 'This page has expired. Open the classroom view again from Nixorium.';
      message.hidden = false;
      return;
    }
    if (!response.ok) throw new Error(String(response.status));
    const data = await response.json();
    message.hidden = true;
    render(data.computers);
  } catch (error) {
    message.textContent = 'The controller is not answering; trying again.';
    message.hidden = false;
  }
  setTimeout(refresh, 2000);
}

refresh();
