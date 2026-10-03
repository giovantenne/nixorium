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
const cards = new Map();
let selected = null;

const stateText = {
  connecting: 'Connecting…',
  viewing: '',
  'no-session': 'Nobody is signed in',
  'no-agent': 'Classroom view is not running here',
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
    if (computer.hasImage && computer.imageAt !== entry.imageAt) {
      entry.imageAt = computer.imageAt;
      entry.image.src = '/api/thumbnail?name=' + encodeURIComponent(computer.name) + '&at=' + computer.imageAt;
      entry.image.hidden = false;
      entry.placeholder.textContent = '';
    } else if (!computer.hasImage) {
      entry.placeholder.textContent = text;
    }
    if (computer.state === 'viewing') viewing += 1;
    if (computer.name === selected) {
      viewerDetail.textContent = computer.state === 'viewing' ? screenAge(computer) : text;
      if (computer.hasImage) viewerImage.src = entry.image.src;
    }
  }
  summary.textContent = viewing + ' of ' + computers.length + ' screens visible. Student computers show a sharing notice while you watch.';
}

function open(name) {
  selected = name;
  viewerName.textContent = name;
  const entry = cards.get(name);
  viewerImage.alt = 'Screen of ' + name;
  viewerImage.src = entry && !entry.image.hidden ? entry.image.src : '';
  viewer.showModal();
}

document.getElementById('viewer-close').addEventListener('click', () => viewer.close());
viewer.addEventListener('close', () => { selected = null; });

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
