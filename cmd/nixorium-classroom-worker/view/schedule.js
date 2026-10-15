// Schedule: the dialog behind Power → Schedule…, which shows the schedule
// the controller keeps for switching the computers on and off by themselves,
// and sets or clears it. Nothing here changes a computer: the controller's
// timer does that at the set times.
'use strict';

(() => {
  const openButton = document.getElementById('schedule-open');
  const dialog = document.getElementById('schedule-dialog');
  if (!openButton || !dialog) return;
  const current = document.getElementById('schedule-current');
  const fields = document.getElementById('schedule-fields');
  const zone = document.getElementById('schedule-zone');
  const problem = document.getElementById('schedule-problem');
  const onEnabled = document.getElementById('schedule-on-enabled');
  const onTime = document.getElementById('schedule-on');
  const offEnabled = document.getElementById('schedule-off-enabled');
  const offTime = document.getElementById('schedule-off');
  const days = [...dialog.querySelectorAll('input[name="schedule-day"]')];
  const clearButton = document.getElementById('schedule-clear');
  const cancelButton = document.getElementById('schedule-cancel');
  const saveButton = document.getElementById('schedule-save');
  const weekdays = ['mon', 'tue', 'wed', 'thu', 'fri'];

  // The ready-made times of a new schedule.
  const defaults = { on: '07:50', off: '18:00' };

  function setBusy(busy) {
    for (const control of [clearButton, saveButton]) control.disabled = busy;
    for (const control of fields.querySelectorAll('input')) control.disabled = busy;
    if (!busy) syncTimes();
  }

  // A time is used only while its box is checked.
  function syncTimes() {
    onTime.disabled = !onEnabled.checked;
    offTime.disabled = !offEnabled.checked;
  }
  onEnabled.addEventListener('change', syncTimes);
  offEnabled.addEventListener('change', syncTimes);

  function showProblem(text) {
    problem.textContent = text || '';
    problem.hidden = !text;
  }

  // Fills the dialog from a schedule report; a new schedule starts with the
  // usual school day.
  function show(report) {
    const schedule = report.schedule || {};
    const set = Boolean(schedule.on || schedule.off);
    current.textContent = set ? report.summary : 'No schedule is set.';
    zone.textContent = report.zone ? 'Times are in the controller’s time zone (' + report.zone + ').' : '';
    onEnabled.checked = set ? Boolean(schedule.on) : true;
    offEnabled.checked = set ? Boolean(schedule.off) : true;
    onTime.value = schedule.on || defaults.on;
    offTime.value = schedule.off || defaults.off;
    const chosen = set ? schedule.days || [] : weekdays;
    for (const box of days) box.checked = chosen.includes(box.value);
    syncTimes();
  }

  async function call(options) {
    const response = await fetch('/api/schedule', options);
    if (!response.ok) throw new Error(await response.text());
    return response.json();
  }

  function notice(line) {
    // The page's notice at the bottom, when it is there.
    if (typeof showToast === 'function') showToast(line);
  }

  openButton.addEventListener('click', async () => {
    openButton.closest('details')?.removeAttribute('open');
    showProblem('');
    current.textContent = 'Loading…';
    zone.textContent = '';
    setBusy(true);
    if (!dialog.open) dialog.showModal();
    try {
      const report = await call({ cache: 'no-store' });
      if (report.state === 'completed') {
        show(report);
        setBusy(false);
        clearButton.disabled = !(report.schedule && (report.schedule.on || report.schedule.off));
      } else {
        current.textContent = '';
        showProblem(report.message || 'The schedule is not available.');
      }
    } catch (error) {
      current.textContent = '';
      showProblem('The controller did not answer. Try again.');
    }
  });

  async function send(schedule, doneLine) {
    showProblem('');
    setBusy(true);
    try {
      const report = await call({ method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(schedule) });
      if (report.state !== 'completed') {
        setBusy(false);
        showProblem(report.message || 'The schedule was not saved.');
        return;
      }
      dialog.close();
      notice(doneLine(report));
    } catch (error) {
      setBusy(false);
      showProblem('The controller did not answer; the schedule may not be saved. Open Schedule… again to check.');
    }
  }

  saveButton.addEventListener('click', () => {
    const chosen = days.filter((box) => box.checked).map((box) => box.value);
    if (!onEnabled.checked && !offEnabled.checked) {
      showProblem('Choose a time to switch on, a time to shut down, or both.');
      return;
    }
    if (chosen.length === 0) {
      showProblem('Choose at least one day.');
      return;
    }
    if ((onEnabled.checked && !onTime.value) || (offEnabled.checked && !offTime.value)) {
      showProblem('Enter the time as hours and minutes.');
      return;
    }
    send({ on: onEnabled.checked ? onTime.value : '', off: offEnabled.checked ? offTime.value : '', days: chosen }, (report) => 'Schedule saved. ' + report.summary);
  });
  clearButton.addEventListener('click', () => send({ on: '', off: '', days: [] }, () => 'Schedule cleared.'));
  cancelButton.addEventListener('click', () => dialog.close());
})();
