# Classroom

What the teacher and the administrator do during a lesson. Everything here
is in **Computers** (and in the classroom view page) and never changes the
laboratory configuration. The first time after an update that adds a feature,
update the student computers and have the students sign in again: a computer
that is not updated is listed as unable to do it yet.

Computers that are off, or where nobody is signed in, are listed and skipped.
Each action shows a review of the computers it reaches before you confirm.

## Classroom view

**Computers → Classroom view** (the teacher's dock icon) opens a page with
the students' screens. It is on unless you turn it off in **Maintenance →
Change settings → Classroom view**. With it off, the dock icon opens the
Nixorium menu, without the view, locking, messages, screen sharing, sending or
collecting files. Internet, power and Recover student files keep working; the
schedule keeps running and is changed with `nixorium schedule`.

- **Watch and control.** Open a computer's window to see it large and take
  keyboard and mouse. Its screen shows "Teacher in control" until you stop.
- **Save screenshot** keeps a picture in `Nixorium/Screenshots` of your home
  folder, such as `pc03-2026-10-15-10.30.12.jpg`.
- **Message** shows a short text over the selected screens. You choose how
  long: until the student closes it, 30 seconds, 2 minutes, or until the
  computer is unlocked.
- **Share screen** shows your screen, or a window or tab you choose, on the
  selected computers, with their keyboard and mouse blocked, until **Stop
  sharing** or until you close the page.
- **Share files** sends and collects files (below).
- **Lock**, **Internet** and **Power** do what the sections below describe.

For a test, use **Internet → Block Internet**, **Lock** and **Message** one
by one.

What the students see: GNOME shows its screen-sharing notice while the page is
open and visible (a small "Classroom view open" label when a window is full
screen). Saving a screenshot or collecting files shows a short notice on that
computer. A student who stops the view from a terminal sees "Monitoring
stopped on this PC" and the view starts again within seconds.

The view reaches each computer only over the controller's existing SSH access;
no other port is open. After an update, students get the new version of the
view when they next sign in.

## Internet access

**Computers → Internet access**: select computers, choose **block** or
**unblock** with Tab, review, Enter.

- Blocked computers still reach the laboratory network, so the classroom
  view and updates keep working.
- Internet returns when a computer restarts.
- Computers that are off are not queued.

```sh
nixorium internet plan --on @lab --action block
nixorium internet apply --on @lab --action block --expect <token>
```

## Lock screens

**Computers → Lock screens**: select computers, choose **lock** or **unlock**
with Tab, review, Enter. A locked screen shows "Eyes on the teacher" and
ignores keyboard and mouse. Signing out or restarting always unlocks it. Needs
the classroom view.

```sh
nixorium lock plan --on @lab --action lock
nixorium lock apply --on @lab --action lock --expect <token>
```

## Send files

**Computers → Send files** copies a file or a folder to the desktop of the
selected computers. Type the path, which starts at your home folder. In the
classroom view page, **Share files → Send a file…** or **Send a folder…** opens
the file chooser instead.

- A folder arrives with its name and contents, without hidden files and links.
- At most 2000 items and 500 MB per sending.
- The copies belong to the student and are never executable. A name already on
  the desktop is kept, and the new copy becomes "name (2)".
- A restart empties the student's home, as always.

```sh
nixorium send plan --file <file-or-folder> --on @lab
nixorium send apply --file <file-or-folder> --on @lab --expect <token>
```

## Collect files

In the classroom view page, **Share files → Collect from students' desktops**
(also in a computer's **Actions** menu) copies the files on the selected
desktops into your home folder, in
`Nixorium/Collected/<date and time>/<computer>/`. Nothing changes on the
students' computers.

- Hidden files and links are left out.
- At most 2000 items and 500 MB per computer: a computer with more is reported
  as "too much to collect" and nothing of it is kept.
- An empty desktop is "nothing to collect".

```sh
nixorium collect plan --on @lab
nixorium collect apply --on @lab --expect <token>   # add --into <folder> for another folder
```

## Recover student files

Each student computer keeps its last sessions as read-only copies: five,
unless you choose 0 to 30 in **Maintenance → Change settings → Student
snapshots**. A computer has saved sessions from its second start on.

1. **Computers → Recover student files**, choose the computer the student
   used, then the session. "Last change" is when one of its files last
   changed; "saved at startup" is when the computer was next switched on.
2. Enter opens a folder, Backspace goes up, Space chooses a file or folder,
   `r` reviews.
3. Enter copies them into **Recovered files** on that computer's desktop, with
   their original dates.

Nothing on the desktop is replaced and the student's files are not changed.
Hidden files and links are left out; at most 2000 items and 500 MB at once.
The copies disappear at the next restart like everything in the student
account, so the student should save them where they keep their work. Works
with or without the classroom view.

## Shut down or restart

**Computers → Power controls**: choose **Shut down** or **Restart** with Tab,
select computers, review, Enter. The controller is never in the list.

- A computer someone is using is included, with a warning that unsaved work
  may be lost. One where the student has not touched keyboard or mouse for ten
  minutes (or since signing in) shows "Logged in, not in use", without the
  warning.
- A computer whose session cannot be read is left out unless you press `u` to
  include it.
- Computers that are off or do not answer are listed as not sent. Nothing is
  queued.
- "Accepted" means the computer received the request, not that it is already
  off. An "unconfirmed" result may have worked: check the computer before
  trying again.

```sh
nixorium shutdown plan --on @lab
nixorium shutdown apply --on @lab --expect <token>
nixorium restart plan --on pc01,pc02
nixorium restart apply --on pc01,pc02 --expect <token>
```

In the terminal, when someone is using a computer in the review, `apply` asks
you to type `SHUTDOWN` or `RESTART`; otherwise it asks `Continue? [y/N]`.
`--acknowledge-unknown-sessions` (on both plan and apply) includes computers
whose session cannot be read.

## Switch computers on

**Computers → Power controls → Switch on** (Tab), or **Power → Switch on** in
the classroom view page, wakes the selected computers that are off over the
network. Their card shows "Starting…" until they answer. You never type an
address: the controller learns each computer's network address by itself while
the computer is on and the classroom view or **Computer inventory** sees it.

```sh
nixorium wake --on pc03,pc04
nixorium wake --on @lab
```

It prints, for each computer, sent, already on, or address unknown. A sent
request does not prove that a computer starts. For that, each computer must:

- allow waking from the network in its firmware ("Wake on LAN" or "Power on by
  PCI-E"; some also need "ErP" or "Deep sleep" turned off);
- have restarted once after the update that enabled waking on its laboratory
  network card;
- stay plugged in: a switched-off power strip or a power cut disables waking
  until the computer is started by hand once.

When a computer stays off, check those points, start it by hand and let the
classroom view or the inventory see it once.

## Schedule

A schedule switches the student computers on and shuts them down at set times:
one switch-on time and one shut-down time (either may be left out) on the
weekdays you choose, for every student computer, in the controller's time
zone.

In the classroom view page open **Power → Schedule…**, set the times and the
weekdays, and **Save**; **Clear** removes it.

```sh
nixorium schedule show
nixorium schedule set --on 07:50 --off 18:00
nixorium schedule set --off 17:30 --days mon,wed,fri
nixorium schedule clear
```

`--days` takes `mon-fri` (the default), `all` or a list. `set` replaces the
whole schedule; the two times must be more than ten minutes apart.

- The switch-on works like **Switch on** above, so each computer must allow
  waking from the network.
- The shut-down never interrupts a person: a computer someone is using, or
  whose session cannot be read, stays on. One logged in but not touched for
  ten minutes shuts down.
- A computer being updated, or whose last update was interrupted, stays on.
- When an installation keeps the controller busy at the set time, it tries
  every minute for ten minutes, then gives up for that day. A time at which the
  controller was off is not made up later.

Each run appears in `nixorium logs` and in
`journalctl -u nixorium-classroom-schedule`. The schedule is kept only on the
controller, not in Git or in the backup: **set it again after replacing the
controller**.
