// Message of the classroom view page. This script follows app.js and uses
// its dialog, review and notice handling (runAction and the like); app.js
// calls it only through classroomTools.
//
// A message is one more reviewed action.
'use strict';

const classroomTools = (() => {
  const messageDialog = document.getElementById('message-dialog');
  const messageText = document.getElementById('message-text');
  const messageCount = document.getElementById('message-count');
  const messageDuration = document.getElementById('message-duration');
  const messageProblem = document.getElementById('message-problem');

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

  return {
    message: openMessage,
  };
})();
