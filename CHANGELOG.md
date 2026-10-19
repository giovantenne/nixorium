# Changelog

All notable changes to Nixorium are documented in this file.

The project follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- Assistant can read system configuration files of the controller and the
  computers, such as the boot menu or the settings in `/etc`, so it can, for
  example, make Windows the default boot entry without asking you to look
  anything up. It never reads home folders, passwords, keys or Wi-Fi
  credentials. Assistant asks once more for consent because of this.
- **Update Nixorium** can now change your own deployment files when a release
  needs it, instead of asking you to edit them by hand. The review shows each
  change with a one-line reason, and a "Manual change" with the text to replace
  and the text to put instead when you have edited that file yourself. Saving
  the update writes and commits the changed files with it, you can undo it with
  Git like any update, and the new file `nixorium-migrations.json` records which
  changes were made so each is made only once. See
  [Updating](docs/updates.md#changes-to-your-own-files).

### Changed

- **Update computers** no longer uses Colmena. Nixorium builds each selected
  computer's system once (instead of twice), copies it, and activates it in a
  service on the computer that keeps running if the connection drops. A
  computer that fails or is off no longer blocks the others: only a computer
  whose result is truly unknown still needs a review, and that review closes
  itself as soon as the computer reports how its update ended. The log shows
  each computer's progress and its activation messages.
- Identical computers now share one configuration. An update, and network
  installation, builds it once instead of once for every computer. Each
  computer keeps its own name and laboratory address from a small identity
  record, `/var/lib/nixorium/identity.json`, that the update or the installer
  writes before the computer starts its new system. A computer without a
  valid record shows "This computer has no Nixorium identity"; updating it from
  the controller writes the record again, and an update never overwrites a
  record that names another computer.
- A private module that uses a computer's name or address (`hostName` or
  `hostIp`) now needs that computer listed in `hostModules` or
  `updateValidationHosts`; otherwise the evaluation stops with a message that
  says so. The flake no longer has a `colmena` output, because Nixorium has
  its own update engine.
- The `l` key for progress details appears only when there are details to
  show, instead of on every progress screen.
- The readiness check of the laboratory configuration, which takes about 20
  seconds, is done once per saved configuration and then reused: updating
  computers no longer repeats it when it reviews and again after the build.

### Fixed

- The check after an update now tries up to three times, a few seconds apart,
  before reporting a computer as unverified, because a computer that has just
  switched to its new system can take a moment to answer. When the answer is
  still unexpected, the report says what the computer replied.
- An update that changes NetworkManager's settings on computers configured
  by network card type no longer can fail to write their laboratory network
  profile: the profile writer waits for NetworkManager to answer.
- In the classroom view the **Power** menu opens towards the left, so it
  stays readable when the window is maximised.
- While an update of the student computers is still running, the Overview
  shows it as a running operation instead of also listing it as an
  interrupted update.
- When a queued update needs a new review, because the configuration changed
  after it was queued or it kept failing, the Overview now says so and points
  to **Computers → Queued updates**, instead of saying that nothing needs to
  be done. Update plans, the troubleshooting guide and the interrupted-update
  message also use plainer words and no longer mention the old deployment tool.

### Upgrade notes

- Deployments created before 3.9.0 have a `modules/workstation.nix` that
  finds the controller by comparing `hostName`. Computers that share a
  configuration have no single name, so that test now stops the evaluation
  with a message that says what to do. From this release on, Update Nixorium
  makes this change for you: the review shows it as a change to
  `modules/workstation.nix`, or as a "Manual change" if you edited that line.
  Only when you update from 3.9.0 or older to this release, which cannot apply
  it yet, do it by hand once: replace the line
  `isController = args ? hostName && args.hostName == (labSettings.masterHostName or null);`
  with `isController = args.isController or false;`, save, then update
  Nixorium again.

## [3.9.0] - 2026-10-19

### Added

- A [trademark policy](TRADEMARKS.md) says how the Nixorium name and logo can
  be used: freely to talk about and share Nixorium, with permission for forks,
  services and products that carry the name.
- The keyboard help (F1 or `?`) now shows the address for questions and
  feedback, hello@nixorium.org.

### Changed

- Reviews and updates prepare the configuration for evaluation once instead
  of several times, which shortens every update a little.
- Blocking Internet, switching computers off or restarting them, and the
  scheduled shut-down no longer wait while the administrator updates
  computers. Only the computers being updated, or whose last update was
  interrupted, are left alone, and the review says so.
- Most reviews are now confirmed with Enter on the review screen, or with a
  `Continue? [y/N]` question on the command line. A word still has to be typed
  only before something that cannot be undone: erasing a disk, resetting the
  configuration template, discarding unsaved changes, trusting a new computer
  key, removing old versions, or, on the command line, switching off or
  restarting computers someone is using. Every review still shows what will
  happen and can be cancelled with Esc, and `--yes` and `--expect` work as
  before. Reviews that no longer need a word leave `confirmation` out of their
  JSON output.
- The log of **Update computers** shows when each step started and how long it
  took, with the total time at the end, so it is clear where an update spends
  its time.
- Computers waiting for a queued update are updated together when they come
  back, instead of one after the other, and a computer already up to date
  leaves the queue without a second update. While another operation is
  running the queue waits for the next minute instead of counting a failure.
- **Update computers** selects the computers whose queued update is out of
  date, so the new review includes them; the selection can still be changed.
- Modules of a deployment can test the computer's role with the `isController`
  and `nixoriumRole` arguments instead of comparing host names.

### Fixed

- Reviewing an interrupted update of the student computers no longer waits
  forever for a program that only reused the number of the one that started
  the update.
- During a change of laboratory network, that review also checks computers
  still on their previous address.
- Preparing the review of **Update computers** no longer stops with "Loading
  took too long" right after Nixorium was updated.
- A computer switched off while **Update computers** was building is left
  unchanged and reported as not updated (or queued, if the review chose
  that), instead of ending the whole update as interrupted and blocking every
  other operation until a recovery review.
- **Computers → Queued updates** opens again; selecting it did nothing.
- Trusting a reinstalled computer's new SSH key no longer fails with "review
  changed" when the review was made just before the clock passed a
  five-minute mark. A host-key review now stays valid for four to five
  minutes, from the command line and from the computer's details.
- **Update computers** keeps each student computer on its own network
  interface. When `clientIfaceName`, `controllerIfaceName` or
  `hostIfaceNames` differed from `ifaceName`, the update configured the
  `ifaceName` interface instead: the computers lost their laboratory address,
  DHCP and Internet and could no longer be reached. Start an affected computer
  from the previous entry in its boot menu, then update the computers again.
- Student computers with the Programming web server no longer stop at boot
  with "Failed to start Restore the configured student workspace before
  login". The daily log rotation, which briefly acts as the student account
  for the web server's logs, could run during the first boot of the day and
  interrupt the home restore; the restore now finishes first.
- A student computer that stops before the login screen because its home
  restore was interrupted now repairs itself at the next boot. A refusal
  before the home changes no longer blocks later boots. After a power loss,
  the restore starts again if the old home was untouched, or completes from
  the read-only snapshot of the old home, which is kept in the teacher's
  snapshots. Computers already stopped this way, with an older Nixorium,
  recover at their first boot after the update. If that single automatic
  attempt also fails, the computer stays closed for the administrator, as
  before; see [Troubleshooting](docs/troubleshooting.md#a-student-computer-stops-before-the-login-screen).
  The journal now names the process that blocked a restore.
- Student computers no longer try `cache.nixos.org` while they are updated;
  they use only the controller's cache. On school networks that filter HTTPS
  this removes the `SSL peer certificate … was not OK` warnings during
  **Update computers** and the time spent on them. The controller still
  downloads from the public cache. A deployment that adds its own client
  caches must now set them with `lib.mkForce`.
- **Installation → USB over SSH** no longer asks again for the network,
  interface and number of computers when they are already saved: like network
  boot, it shows the saved values with **Continue** and **Edit settings**.
- A USB installation is no longer refused with "remote disk is below the
  minimum capacity" because of a small extra disk, such as a card reader, a
  second USB stick or the live USB itself. Such disks are listed as excluded;
  only the disk chosen for installation must hold at least 8 GiB.

## [3.8.2] - 2026-10-16

### Fixed

- 3.8.1 was tagged but never published: its release checks found an
  automated test still pressing the old letter for **Assistant** after the
  Overview letters were swapped. 3.8.2 is 3.8.1 with that test corrected;
  update straight from 3.8.0 to 3.8.2. The changes listed under 3.8.1 arrive
  with this release.

## [3.8.1] - 2026-10-16

### Changed

- In the classroom view, **Send/Collect files** is now **Share files**, and its
  **Collect from desktop** is now **Collect from students' desktops**.
- The classroom view toolbar is simpler: **Share files** holds **Send a
  folder…**, **Send a file…** and **Collect from students' desktops**, and the
  **Refresh** button is gone, since the page refreshes by itself.
- On the Overview, **Maintenance** now opens with `m` and **Assistant** with
  `a`; the letters were swapped so each matches its name.

### Removed

- **Exam mode** is removed from the classroom view. Use **Internet → Block
  Internet**, **Lock** and **Message** one by one.

### Fixed

- **Update computers** no longer fails after **Update Nixorium**. Until 3.8.0,
  updating Nixorium from the menu saved the new release correctly but left the
  previous release recorded inside `flake.lock`. Applying the controller still
  worked, but updating the student computers then stopped before changing any
  computer, with `flake.lock: Read-only file system` and
  `colmena build: exit status 1`. Every laboratory that updated Nixorium from
  the menu is affected. Updates now record the selected release correctly and
  refuse any candidate whose two files disagree. To repair a laboratory, run
  this release on the controller (updating from 3.8.0 leaves the problem once
  more), then open **Maintenance → Update Nixorium**, select the configured
  version, now marked "Current · select to repair", and confirm. Until then
  **Update computers** and `nixorium doctor` say so with the code
  `PIN-OUTDATED` instead of the build error. See "Updating the computers fails
  with a read-only flake.lock" in the troubleshooting guide. The same defect
  made every change of NixOS channel in **Update system and packages** fail
  its own check; that works now too.

## [3.8.0] - 2026-10-16

### Added

- **Switch on** starts student computers that are off, with Wake-on-LAN. It is
  in **Computers → Power controls** (Tab beside Shut down and Restart, for the
  administrator and the teacher), in the classroom view page (**Power →
  Switch on** for the selected computers, and in a computer's **Actions** menu
  while it does not answer) and in a terminal as
  `nixorium wake --on <computers|@lab>`. The review lists the computers that
  receive the packet and skips those already on and those whose network
  address is not known yet; in the classroom view a card shows "Starting…"
  until the computer answers, for at most three minutes. Nobody types an
  address: the controller learns each computer's network card address by
  itself while the computer is on and the classroom view is open or the
  computer inventory runs, and keeps it on the controller only, outside the
  configuration. A sent packet does not prove that a computer starts: its
  BIOS/UEFI must allow waking from the network (often called "Wake on LAN"
  or "Power on by PCI-E"), and it must stay plugged in. Addresses are learned
  from computers already installed too, as soon as the controller runs this
  version; the computers themselves accept the packet from their first
  restart after their next update.
- In the classroom view, **Save screenshot** in a computer's window keeps a
  picture of the student's screen, up to 1920 pixels wide, in
  `Nixorium/Screenshots` of your home folder, named after the computer, the
  date and the time (for example `pc03-2026-10-15-10.30.12.jpg`). A picture
  never replaces an earlier one. The button works only while the screen is
  live: a computer that is off, idle behind its own screen lock or without
  anyone signed in has no picture to save. When the page was not opened from
  Nixorium, the browser downloads the picture instead.
- **Collect from students' desktops** copies the files on the desktops of the selected
  students' computers into your home folder, in
  `Nixorium/Collected/<date and time>/<computer>/`, after the usual review
  with one confirmation button. It sits beside **Send files** in the classroom
  view toolbar and in a computer's **Actions** menu. Hidden files and links
  are left out and nothing changes on the students' computers. At most 2000
  items and 500 MB are taken from one computer; a computer with more is
  reported as "too much to collect" and nothing of it is kept. The result
  counts the computers, such as "Collect: 10 done · 2 nothing to collect ·
  1 switched off or not reachable". From a terminal, teacher or
  administrator: `nixorium collect plan --on <clients|@lab>`, then
  `nixorium collect apply` with the same options and `--expect` set to the
  review token; `--into <folder>` collects into another existing folder.
  Update the student computers first: one that is not updated is listed as
  not able to collect yet.
- **Message** in the classroom view shows a short text over the screens of the
  selected students' computers, in front of everything, a lock or your shared
  screen included. It is in the toolbar (**Message → Send a message…**, with
  **Remove message**) and in a computer's **Actions** menu. You write up to 300
  characters (plain text, up to 10 lines) and choose how long it stays: until
  the student closes it (**Close** appears after 10 seconds), 30 seconds, 2
  minutes, or until you unlock the computer. The students keep their keyboard
  and mouse, unless the computer is locked. A new message replaces the
  previous one, and signing out removes it. The review shows the text and the
  computers, and the result counts them, such as "Message: 12 shown · 1
  switched off or not reachable". Update the student computers first and have
  the students sign in again: one that is not updated is listed as unable to
  show messages yet.
- **Exam mode** in the classroom view toolbar prepares the selected computers
  for an exam with one confirmation: it blocks Internet, locks the screens
  and shows a message ("Exam in progress. Wait for the teacher.", editable),
  each step optional and each computer's result listing the three steps. The
  same button reads **End exam mode** on computers it was started on from the
  open page: it removes the message, unlocks the screens, allows Internet
  again, and then offers **Collect from students' desktops** for the same computers
  (nothing is collected unless you confirm). The classroom service remembers
  which computers are in exam mode, so a reopened page, or another user's,
  shows the same; the computers keep no exam state, and restarting the
  classroom service forgets it (then use **Remove message**, **Unlock** and
  **Allow Internet**).
- **Schedule** switches the student computers on and off by themselves, with
  at most one switch-on time and one shut-down time for a set of weekdays, for
  example on at 07:50 and off at 18:00 from Monday to Friday, in the
  controller's time zone; the two times must be more than ten minutes apart.
  Set it in the classroom view (**Power → Schedule…**, with **Save** and
  **Clear**) or in a terminal, as the teacher or the administrator:
  `nixorium schedule set --on 07:50 --off 18:00` (Monday to Friday unless
  `--days` says otherwise), `nixorium schedule show`,
  `nixorium schedule clear`. The controller checks the schedule every minute:
  the switch-on is the ordinary **Switch on** (computers without a known
  network address are skipped), and the shut-down never interrupts a person,
  since a computer that someone is using, or whose session cannot be read,
  stays on. When another operation or a network installation keeps it from
  starting, it tries again every minute for ten minutes and then gives up for
  that day. Each run leaves a line in the operation history and in the
  controller's log. The schedule lives on the controller only, outside the
  configuration and the backup: set it again after replacing the controller.
  The computers must allow waking from the network, as for **Switch on**.
- `nixorium git undo plan` and `nixorium git undo apply --expect <token>` go
  back from the last saved change (settings, software, student workspace): the
  review shows its description, date and files, and applying it (typed `UNDO`)
  saves one new "Undo: ..." change and puts the files back. In the dashboard
  press `u` on **Maintenance → Review Git changes**; Enter on the result opens
  **Apply to controller**. Nothing is pushed and no computer changes; apply to
  the controller and update the computers as usual. It refuses, and names the
  next step, unsaved changes, a first version, a merge, an earlier undo, an
  update of Nixorium or of the package base (see the update recovery),
  recorded keys, a change of the laboratory network and files that cannot go
  back safely.
- The laboratory can move to a new network without losing a computer:
  **Maintenance → Change network addressing**
  (`nixorium network migrate plan|start|status|finish|undo`) reviews the new
  address of the controller and of every computer, then continues with
  **Apply to controller** and **Update computers** while the controller and
  the computers answer on both networks. A computer still on the old network
  is updated there, computers that are off are queued, the Overview shows the
  change in progress (`NETWORK-CHANGING`) with its next step, and the change
  can be undone until it is finished.
- The settings review checks network devices before saving, without changing
  any computer. A new controller device name is compared with the controller's
  devices (listed with their addresses, the one carrying the DHCP address
  marked), and a name the controller does not have as a wired device cannot be
  saved: "The controller has no network device called enp5s0. Its devices are:
  …". A new client device name is checked on every affected computer that
  answers: a missing device blocks the save unless you choose to save anyway
  for the computers listed, and computers that do not answer are listed to
  check at their screen. The review also advises updating one computer first.
  `nixorium config plan --json` reports the same findings in a new
  `networkPreflight` object.
- The controller DHCP address in **Settings → Network** is picked from the
  controller's current DHCP addresses, with the saved value preselected;
  **Other…** still accepts a typed address, with a warning when the controller
  does not have it. The command line is unchanged.
- Student computers now get the time from the controller. The controller keeps
  its own clock right from Internet time servers (it must be able to reach
  them) and gives the time to the laboratory network only, opening UDP port
  123 on the laboratory network card and nowhere else. Each computer asks the
  controller and no Internet time server, so a computer whose clock battery is
  drained starts with the right time even though it has no Internet of its
  own, and certificates, file dates and logs stay correct, also for a computer
  that has Internet after a user signs in. If the controller cannot reach the
  Internet it keeps giving its own clock, starting five minutes after it boots
  so that a clock that may still be wrong is not given out meanwhile. The
  change reaches each computer at its next update and the controller when you
  apply it; there is nothing to configure. Controller-only installations keep
  their previous time settings.
- Assistant can now change the browser homepage, the time zone, the locale,
  the keyboard layout and the Git identities in a proposal; everything else in
  the settings stays in the menu. The change is reviewed and saved like any
  other proposal, with the same checks and the same "what happens next" lines
  as a change made in **Maintenance → Change settings**.
- Programs downloaded outside Nix now run on the controller and every client.
  NixOS does not keep the shared libraries that most downloaded programs look
  for, so they failed to start; Nixorium now enables nix-ld, which provides
  the common ones: zlib, libstdc++, glib, OpenSSL, ICU and the X11/GTK set
  that Electron and Java programs with a window need. A VS Code extension
  installed from the Marketplace that brings its own programs, such as the
  Java runtime of the Java extension, starts instead of failing with "Could
  not start dynamically linked executable". Managed extensions from the
  profile remain the recommended route, and student homes still lose their
  installs at the reset.
- When the student workspace profile selects the Java extension and the
  software includes a JDK, the Java extension finds the laboratory's JDK by
  itself, so it never depends on a runtime bundled in a Marketplace copy.
  Reviewed editor settings in the profile may still override this.

### Changed

- A student can no longer stop the classroom view from a terminal on the
  computer: the stop is refused, and when the program is killed it starts
  again by itself a second later, however many times. While it is down, the
  computer's card says "Monitoring stopped on this PC" instead of just
  dimming, and the same words explain why Lock, Send files, Share screen,
  Collect from students' desktops and Save screenshot skipped that computer; the result
  notice counts it apart. A computer waiting at the login screen still says
  "Nobody is signed in". A classroom view that is frozen rather than stopped
  is reported the same way, and a frozen one no longer keeps its connection
  open on the computer after the controller has gone. This is a classroom aid,
  not a security boundary: a determined student can still switch it off or
  freeze it, but it comes back at the next start of the computer, because the
  home folder is reset then. An update no longer restarts the classroom view
  on a computer in use; the new version takes effect at the next sign-in.
- After **Update Nixorium** or **Apply to controller** activates the
  controller, Nixorium reopens itself on the newly installed version: the
  result shows "Nixorium <version> is active. The interface reopens in 3 s
  with it (Esc keeps this one)." Enter reopens at once, and Esc keeps the
  running interface with the previous "Reopen Nixorium" reminder.
- The notification on a student computer after an update that needs no restart
  now says: "The update is in place; programs you open from now on use it. If
  something does not work as expected, restart the computer."
- Changing the laboratory network address, prefix length or controller number
  while computers are configured is refused with a clearer message and the new
  situation code `NETWORK-LOCKED`: the computers would keep their old
  addresses and lose the controller. The message points to the guided change
  of the laboratory network (**Maintenance → Change network addressing**); the
  controller number stays locked.
- The review of an Assistant proposal reuses the evaluation Assistant already
  ran when nothing changed since, instead of evaluating the laboratory a second
  time.
- When an Assistant answer ends early because the service stream was cut off,
  Assistant now says the service interrupted the answer and asks you to send
  the message again, instead of asking you to update Nixorium. The reason for
  any refused response is kept in a private per-chat record on the controller
  and is never sent to the service.
- When the Assistant service interrupts a request before it gives an answer,
  because the AI provider failed or the service reached its safety limit,
  Assistant now adds "This request was not counted." to the notice, and the
  request is not taken from the requests you have left. A request you stop
  yourself is still counted.

### Fixed

- The classroom view no longer waits forever on a slow or stuck student
  computer. When a computer does not say within 10 seconds who is signed in,
  its card says "Nobody is signed in" instead of waiting, and the program that
  captures a student's screen is stopped whenever the capture stops. On the
  controller, opening the classroom window no longer waits more than 10
  seconds for the desktop's light or dark style, and opens without the dark
  style when nothing answers. The reset of a student's home at boot stops
  waiting after 5 minutes on a step that does not finish, ends safely and
  leaves what it found for the administrator, instead of waiting without end.
- `nixorium send plan|apply` works from the teacher's terminal: like
  `nixorium collect`, it needs only the classroom service and no longer stops
  with "deployment repository not found".

### Documentation

- The "Time" section of the system reference describes how the controller
  keeps the laboratory's time and gives it to the computers.
- The administrator guide has new sections "Switch computers on", "Schedule",
  "Collect files" and "Undo the last saved change".
- The "Laboratory settings" section of the Assistant guide lists the settings
  Assistant can change, and the guide says which interrupted requests are not
  counted.
- The release checklist now asks to confirm the website release branch and to
  update the Assistant instructions before tagging.
- The development validation guide describes the ShellCheck and errcheck lint
  that the default gate now runs.

## [3.7.0] - 2026-10-15

### Added

- Assistant can download a public file once on the controller and pin its
  hash itself (`prefetch`), so programs distributed as AppImage, deb or
  tarball need no manual hash step.

### Changed

- **Update Nixorium** and **Apply to controller** now work even when an update
  of the student computers was interrupted or a USB installation was not
  finished, so a newer release can always reach the controller. The student
  computers still wait for that review, and the result says so. Only a USB
  installation that is writing its disk, or waiting for its `ERASE`
  confirmation, still holds the controller back.
- When something blocks an operation, Nixorium now says it in one plain
  sentence, why it stops there and what to do next, with the same words on
  the failure screen, the Overview, the review and in the troubleshooting
  guide. For example, **Update computers** now reports "The computers were not
  updated: a previous update of the student computers was interrupted." The
  technical message is still there behind `d`.
- On the result of **Update Nixorium**, **Apply to controller** and **Update
  computers**, Enter opens the screen that solves the problem (for example the
  review of the interrupted update); Esc closes the result as before and
  **Retry controller** stays on `a`.
- When an update of the student computers stops before Nixorium can see how
  it ended, the review of that update opens at once instead of leaving you on
  the failure screen. It is no longer refused because Nixorium itself was
  "still running".
- The review of an interrupted update, now titled **Review the interrupted
  update**, says for each computer whether it finished, is still updating or
  does not answer. Press `c` to continue without the computers that do not
  answer and Enter to mark the update as reviewed: typing `RECOVERED` is no
  longer needed, because confirming changes nothing on the computers. It also
  says when a computer that does not answer belongs to an unfinished USB
  installation, or when a network installation is still open. The command
  line asks "Mark this update as reviewed? [y/N]"; `--yes` still skips it.
- `nixorium deploy apply` prints what to do when the update needs a review.
- The review of an interrupted template reset, now titled **Finish or undo
  the reset**, says what it found in plain words: the reset had already
  finished, only the branch move is missing, nothing was replaced, or files
  from before and after the reset are mixed. In the first three cases Enter
  confirms, because no file changes; typing `RECOVERED` is no longer needed.
  `RESTORE` is still typed for mixed files, and the screen says in one
  sentence what it changes and that untracked files are not removed. The
  command line asks yes/no for the first three cases.
- An unfinished USB installation opens as **Resume or close the USB
  installation**: it says whether the computer still answers from the USB
  stick and whether its disk was touched, then lists what you can do —
  **Resume the installation**, **Check how far it got**, **Restart the
  computer**, **Cancel safely** or **Close this record**. Cancelling safely,
  and closing a record whose disk was never touched (also from the TUI now,
  when the USB stick computer is gone), ask only for Enter. Closing after the
  disk was written asks you to type `CLOSE`, in the TUI as on the command
  line. The headline is plain; the technical message is behind `d`.
  `nixorium install usb status` starts with the same findings, and `install
  usb close` asks yes/no when the disk was not touched.
- After an interrupted network installation, the network installation screen
  becomes **Recover the controller network**: what Nixorium found (the
  temporary network setup and the network boot service, in plain words),
  what you can do (recover the network, or stop the network installation)
  and Enter to recover, which changes only the controller. The result says
  whether the controller network is back to normal, and `nixorium pxe
  recover` and `pxe stop` start with the same sentence.
- On the result of a USB installation refused because of an interrupted
  network installation, Enter opens the network recovery.

### Fixed

- In a resumed Assistant chat that is read-only, the arrow keys now scroll the
  conversation; Tab chooses between continuing in a new chat and going back.

### Documentation

- The troubleshooting guide has a new section, "Computer not reachable", for a
  computer that does not answer at its laboratory address (switched off, still
  on an old version, or connected with a second network card), and its
  "Interrupted client deployment" section follows the new review. ADR 0026
  records which recoveries need a typed word and which only Enter.
- The troubleshooting sections on an interrupted USB installation and on the
  controller network, and the template reset recovery guide, follow the new
  reviews.

- The Assistant guide now states that the free trial is per laboratory: the same
  email address can register several laboratories, each under its own name.

## [3.6.1] - 2026-10-14

### Added

- Assistant has an opt-in "core override mode" for extreme cases: after you
  write "I accept core overrides" in a conversation, it may propose a private
  module that forces a NixOS option set by the Nixorium core. The proposal
  review now warns, above the save choices, when an added or changed file
  mentions `mkForce` or `mkOverride`; the save flow is unchanged. The Assistant
  guide documents the limits and the GRUB rollback.

### Changed

- **Maintenance** no longer lists **Change account passwords**, and the `p`
  shortcut on the **Change settings** screen is gone. Account names and
  passwords now live together under **Maintenance → Change settings →
  Accounts**, a small menu with **Change account names** (`n`) and **Change
  account passwords** (`p`); `Esc` goes back one level. Both keep their review
  and save; a new password still takes effect only after you apply the
  configuration to the controller and update the computers. The first-setup
  password step is unchanged.

### Fixed

- The lab screensaver service no longer depends on the executable bit of its
  scripts: both are now run through bash. Every laboratory initialised from
  the 3.6.0 template is affected, because the bootstrap's copy of the template
  (`nix flake init`) drops the executable bit; the service then fails with
  `203/EXEC` in `systemctl --user status lab-screensaver`. Refresh
  `modules/screensaver.nix` and `scripts/screensaver-monitor.sh` from the
  template, or run `chmod +x` on the two scripts and commit.
- New laboratories keep the executable bit of the template scripts: the
  bootstrap restores it on `scripts/cmd-screensaver.sh` and
  `scripts/screensaver-monitor.sh` after `nix flake init`, which writes every
  template file without it, so the first commit records them as executable.

## [3.6.0] - 2026-10-14

### Changed

- The lab screensaver no longer locks the session after 10 idle minutes. It
  starts after 5 idle minutes and stops on any activity; nothing else happens.
  The template now has two scripts instead of three (the launcher is part of
  the idle monitor), and the scripts and the logo are used straight from the
  Nix store instead of copies under `/etc/lab`. Existing deployments get this
  only when `modules/screensaver.nix`, `scripts/screensaver-monitor.sh` and
  `scripts/cmd-screensaver.sh` are refreshed from the template, and
  `scripts/launch-screensaver.sh` is removed.
- Update Nixorium no longer offers releases older than the running version when
  the deployment follows `master` or a commit. The update screen shows the
  running version, and `nixorium update plan` and `apply` refuse such releases
  without `--allow-downgrade`; the TUI never allows a downgrade.
- The NixOS Minimal ISO is no longer required. Documentation and screens now
  say to use the official NixOS 26.05 ISO, either Minimal or Graphical (GNOME),
  booted in UEFI mode, for the controller and for USB over SSH. Minimal is
  preferred for USB over SSH on computers with 4 GB of RAM or less.
- Assistant accepts the free trial size announced by the service (1 to 1,000
  requests) instead of rejecting any offer other than ten requests, and shows
  the announced size and length on the trial choice and the account's own
  allowance in the header (**Free trial · N of M left**). Controllers now send
  their version with every Assistant request, so the service can raise the
  trial without breaking older controllers. When the offer cannot be fetched,
  the trial choice no longer states a number.
- The classroom view tells computer states apart with icons over the screen.
  A student's own lock screen (after a while without use, or Super+L) dims the
  last picture under a moon with **Idle** instead of "The student's screen
  lock is on"; the teacher's lock keeps the live picture with a padlock and
  the **Locked** label; a computer that is off shows a power symbol and **Off
  or not reachable** instead of its last picture. The same applies in a
  computer's own window.
- The notice after a classroom view action is one line, such as "Lock: 8
  done · 12 switched off or not reachable · 1 failed", with each computer
  under **Details**; it never grows past a third of the window, closes with
  a click or Esc, and stays until closed only when a computer failed.
- The classroom view has a **Refresh** button that asks for every computer
  at once and draws the cards again. When the controller does not answer,
  the summary says **Reconnecting…** and the page keeps asking.

### Fixed

- "Shutting down…" and "Restarting…" no longer stay on classroom view cards
  for three minutes whatever the computer does: the label ends as soon as the
  computer stops answering after a shutdown, or comes back after a restart.
  The page also no longer stops following the computers for good after one
  "expired" answer from the controller, and gives up on a request that hangs
  after 15 seconds instead of waiting for it.
- Controllers and clients now boot as QEMU, KVM or Proxmox guests with virtio
  disks. The installed initrd lacked the virtio storage drivers, so it timed out
  waiting for the root partition and dropped to emergency mode. Physical PCs and
  VirtualBox were not affected.
- MySQL Workbench in the Programming profile now connects to the local teaching
  server. The desktop has no secret service (gnome-keyring is disabled), and
  without one Workbench refused its offered "Local instance" connection with
  "Cannot Connect to Database Server ... The name is not activatable" before
  ever contacting MySQL. Sessions now set `WB_NO_KEYRING`, so the connection
  opens as `root` with the empty classroom password. A new `mysql-workbench-vm`
  check drives the real application in GNOME to cover it.
- The lab screensaver no longer jumps up one line when an effect finishes. The
  effect fills the whole terminal, so the newline that Terminal Text Effects
  prints at the end scrolled the finished picture up a row before it was
  cleared. The template script now suppresses that newline and keeps the cursor
  hidden.

### Documentation

- Align guide menu paths, the free trial size, Assistant shortcuts, the
  teacher's classroom controls and the student snapshot path with the current
  interface and code.
- The restricted teacher dashboard in the interface gallery now lists all six
  classroom tools, as a real teacher session does. The README states what the
  student PCs need (no Internet of their own), lists the client installation
  requirements in the administrator guide, explains how Wi-Fi clients are
  configured, links the Assistant guide and its protocol note, and corrects the
  count of managed Shell extensions and the example release tags.

## [3.5.0] - 2026-10-14

### Added

- **Maintenance → Change account passwords** opens the administrator, teacher
  and student password form directly; a new password takes effect after applying
  to the controller and updating the computers.

- Controller bootstrap now runs from the official Graphical (GNOME) NixOS ISO as
  well as the Minimal ISO, which helps when a browser is needed to unlock an
  Internet portal. The keyboard is chosen from a readable numbered list and
  applied before the passwords (`gsettings` in GNOME, the closest console
  keymap otherwise).

- Additional initial student files and empty directories through strict
  `home-files.json`, content-addressed captured payloads and inline configuration
  text. The common controller/client student seed retains private ownership and
  explicit executability, takes effect only at normal boot, and is preserved in
  the offline installer.

- Assistant can read bounded diagnostics from declared computers and inspect
  saved configuration history without changing systems or Git. Updated consent
  explains the new observations and best-effort filtering before sharing them.

### Changed

- The text console now follows the graphical keyboard layout
  (`console.useXkbConfig`) on installed systems and the netboot installer, so a
  password types the same on the console, at the login screen and in GNOME.
  `lab.consoleKeyMap` is deprecated: still accepted, ignored and no longer
  written by the setup.

- Removed the unused `nixorium bootstrap configure` command; `install.sh` has
  its own bootstrap prompts and never called it.

### Fixed

- Local `systemctl` calls pass `--no-ask-password` and the command resets a
  foreign `XDG_RUNTIME_DIR` (for example after `su admin` without a dash), so no
  "Failed to add a watch for .../ask-password" message appears.

- PXE client installer checks the prepared client system in the controller's
  binary cache instead of the netboot store, which never contains it. It now
  tells apart an unreachable cache from a client the controller has not
  prepared, and tests no longer pre-seed the system in the netboot image.

## [3.4.0] - 2026-10-13

### Changed

- Offer Assistant in the administrator menu by default on the controller.
  New installations and upgraded deployments with an omitted `lab.assistant`
  setting gain the menu after controller application; an explicit `false`
  stays disabled. Account activation, consent and optional conversation sharing
  remain separate explicit choices.

### Documentation

- Move the Assistant overview into the README's administrator section and
  align the guides with the new default.

## [3.3.1] - 2026-10-13

### Added

- Assistant can prepare public npm CLI packages with exact dependency locks,
  verified sources and an offline artifact tested in isolated namespaces.
  Explicitly scoped student baselines restore after normal home reset, with
  review/save preserved and no installation at boot or login.

### Changed

- Assistant opens the chat before activation and opens the activation page,
  then asks for the code, when sending the first request.
- The administrator Overview focuses on its menu and shows all entries when
  the terminal has room.

### Fixed

- Assistant subscription progress appears once per chat; inactive annual
  subscriptions show their status instead of an available request count.
- Accept short trial upgrade links and explain how to recover an activation
  that is no longer valid.
- New Assistant clients remain compatible with services that do not yet offer
  npm preparation.

### Documentation

- Clarify that Community Edition is free and Assistant is an optional paid
  add-on; refresh the administrator menu image with Assistant enabled.

## [3.3.0] - 2026-10-12

### Added

Nixorium Assistant (optional, subscription). An administrator can ask for
help with the lab configuration from the Nixorium menu; it appears only when
`lab.assistant` is enabled on the controller and connects to the Nixorium
service at `assistant.nixorium.org`.

- Separate activation and consent precede any content sharing. Fixed tools
  prepare an isolated candidate; host-class evaluation and a full comparison
  precede an explicit local save with a recoverable Git backup. Applying
  systems remains a separately reviewed step, and settings saves preserve the
  option.
- Assistant can consult the pinned Nixorium source through the service. The
  client keeps verifiable references in the conversation and sends the
  deployment context, including the pinned core revision, at every step.
- `/resume` reopens current conversations or reads saved and outdated chats
  without using a request; 90 days of activity and up to 30 local
  conversations are kept, protecting the current chat.
- `/billing` opens a temporary Stripe Customer Portal link without using a
  request.
- Free trial requests remaining are shown, with a signed purchase link when the
  trial ends. Annual quota exhaustion directs the administrator to support.
- Answer reports and conversation sharing are explicit and off by default;
  saved history is redacted and the service keeps it for 90 days.
- `mkLab.groupModules` adds common modules for named client groups, with
  consistent Assistant, update and workspace classes and offline
  serialization. Host-specific exceptions remain separately checked.
- `nixoriumComparisonConfigurations` makes Assistant comparisons ignore the
  deployment revision.
- Host representatives are evaluated in sequential batches of at most two per
  process, within the overall time limit.

## [3.2.3] - 2026-10-12

### Fixed

- Client computers now configure DHCP and their static lab address on the wired
  Ethernet card whatever its name, unless `clientIfaceName` or a
  `hostIfaceNames` entry is set. Previously every client inherited the
  template default `enp0s3`, so real PCs (with names such as `eno1` or
  `enp3s0`) were stopped by the PXE installer with a missing interface and an
  installed client would have configured a card that does not exist. The
  temporary Internet block and the client SSH rule no longer depend on the
  name, the PXE and USB/SSH installers skip the name check for such clients,
  and first setup no longer copies the controller's card into
  `clientIfaceName`. Explicit names and the controller keep today's
  behaviour; a PC with several wired cards needs a `hostIfaceNames` entry.

### Documentation

- Add dual-boot requirements to the administrator guide: other systems on other
  disks appear in the boot menu after the next update or installation, which
  needs Fast Boot and Windows Fast Startup disabled and Secure Boot off.

## [3.2.2] - 2026-10-12

- Publish the client preparation memory fix from 3.2.1, whose GitHub release
  was withheld because a regression test needed a newer `jq` than the
  validation runners provide. Client preparation evaluates client systems in
  bounded batches and no longer runs out of memory on large laboratories.

## [3.2.1] - 2026-10-12

- Fix client preparation running out of memory on large laboratories. Shared
  artifacts and client systems are now evaluated in separate bounded batches
  (four clients per evaluator by default) instead of one evaluator for every
  client, and a failing batch names its hosts.

## [3.2.0] - 2026-10-09

- Hide older Nixorium releases in the TUI using Semantic Version precedence
  against the configured deployment tag, and refuse downgrade plans before
  confirmation. Explicit CLI recovery with `--allow-downgrade` remains available.
- Build Nixorium with Go 1.26.9 until the pinned package set provides it,
  fixing standard-library vulnerabilities in net/http, net/textproto and
  crypto/tls reported for Go 1.26.6.
- Reviewed private Git backups with passphrase-encrypted recovery keys, verified
  remote revisions and a Restore lab screen that works without an existing lab.
  Backups are optional and never block installation or updates. The TUI offers
  File / USB (a fully encrypted archive) or Remote Git; either current backup
  clears the advisory reminder.
- After the first verified Git backup, saved configuration is pushed
  automatically while Nixorium is open, without the passphrase; Back up lab asks
  for it again only when keys, account passwords or trusted computers change.
  Restore lab also restores encrypted backup files, with the same key and
  trusted-computer checks as a Git restore. Backup reminders now name the reason.
- Account hashes move to ignored `lab-credentials.json` and encrypted recovery.
  Local Nix input receives the hashes without changing PXE or installed accounts.
  New deployments use the updated template; no inline-hash migration is provided.

- Restrict the laboratory maintainer skill to supported proposals and bounded
  diagnostics. Explicitly prohibit invasive module/overlay/script changes and
  agent-executed saves or live operations; the human operator uses native
  reviews. Instructions are not a sandbox, and existing deployments need a
  separate reviewed refresh of their copied guidance.

## [3.1.1] - 2026-10-06

Version 3.1.0 was tagged but never published: its release validation failed
on outdated management-test expectations. This release contains every 3.1.0
change below, plus:

- A software profile result reads "1 package added" instead of "1 packages
  added".

## [3.1.0] - 2026-10-06

- **Update computers** can include computers that are switched off: the
  review queues them by default (F3 turns it off; CLI
  `deploy apply --queue-unreachable`). A controller timer updates each one, one
  at a time, as soon as it answers, through the same build, check and record;
  an entry is never applied once the configuration changed after queueing.
  **Computers → Queued updates** and `nixorium deploy queue status|cancel` show
  and remove them, and the Overview mentions them.
- After an update, the person at a client computer sees a GNOME notification
  that it was updated; when the kernel or systemd changed, it asks for a
  restart with a **Restart** button. Nothing restarts automatically.

- Protect configured laboratories from accidental address changes in managed
  settings: block subnet, prefix and controller host-number changes while clients
  are configured, warn about interface changes, and distinguish the PXE address
  hint from live addressing. Remove generic controller-first network advice.

- Show Classroom view first in the Computers menu when available.

- The administrator TUI is more consistent. Software uses one wording for where
  a package is installed, shows details without repeating the actions, never
  offers to remove or limit base software, and its reviews list facts in
  aligned rows. Network installation shows plain states (off, on, out of date)
  and offers each action once. The settings editor, also used when preparing
  installation, uses the same breadcrumb, action bar and margins as every other
  screen, and the settings menu lists its areas like the other menus. Lists,
  search and pagination use the theme colors; notices drop the NOTICE label and
  keep explanations in ordinary text; steps read "Step 2 of 6". Restarting or
  closing after a USB installation no longer asks for a typed word, and the
  disk review separates the facts to check from technical details. In Power
  controls, Enter confirms the reviewed restart or shutdown, as in the
  classroom view, and the review says how many sessions in use it interrupts.
  Counts read "1 computer" or "3 computers" instead of "computer(s)".

- Simplify settings forms with empty-field hints and explicit network defaults;
  move examples and extended context into help. Separate USB instructions by
  computer and make address/password entry and the next action explicit.
- Confirm USB disk erasure with `ERASE` in the TUI while preserving the
  reviewed computer, disk and worker authorization checks.

- Use plain input fields and distinct label/value colors; explain configured
  software and simplify routine TUI messages, keeping technical detail separate.

- Guide the first administrator launch through an operational disclaimer and
  separate optional telemetry choice; skipping consent repeats it next launch.
- Explain unconfigured clients and software actions, improve PXE/USB instructions
  and search layouts, and visually separate notices from menu choices.
- Notify administrators of updates on their selected Nixorium channel using a
  daily cached background check and persistent per-version dismissal.

- Organize the project README around administrators and teachers, with the
  administrator menu and the classroom grid shown in their respective sections.

## [3.0.0] - 2026-10-05

The first stable release of the 3.x line includes the classroom controls and
controller bootstrap improvements introduced in
[3.0.0-beta.1](https://github.com/giovantenne/nixorium/releases/tag/v3.0.0-beta.1), together with
the changes below.

### Upgrade notes

- Classroom view replaces Veyon. Remove `publicKeys.veyon`,
  `keys/veyon-public-key.pem` and `veyonNativeHosts` from existing deployments;
  these settings are no longer accepted.
- Student homes always reset from the workspace seed at boot. The profile-free
  reset, `nixoriumWorkspace.runtimeEnabled` field and standalone
  `run-harmonia`/`run-pxe-proxy` apps are removed. Deployments must expose
  `deploymentStatus.controller`, `labMeta.deploymentMode` and
  `nixoriumSoftware.controller`.
- Existing deployments retain their template files and software declarations.
  New template defaults require an explicitly reviewed template reset or
  corresponding deployment-owned changes.

### Changed and fixed

- Reuse the teacher and student account names saved during first setup when
  preparing PXE or USB installations. Change names through Settings → Accounts.

- Present the optional adoption-statistics invitation with a readable report,
  explicit accept/decline actions, an exact JSON preview and separate privacy
  details. The first invitation requires a choice: `e` or Enter accepts, `d`
  declines, and Esc returns only from details. Maintenance lists statistics last.

- On the controller, **Classroom view** has its own launcher, first in the
  teacher's dock, and **Nixorium** opens the dashboard, first in the
  administrator's dock. Clicking an icon again brings its open window forward
  instead of opening another classroom view or dashboard.

- Add optional controller adoption statistics, disabled until the administrator
  explicitly enables them. CLI/TUI previews show the allowlisted daily payload;
  monthly identities and consent remain outside backups. Offline reporting
  failures do not affect laboratory operations.

- **Send files** replaces Send desktop. In the classroom view page it offers
  **A file…** and **A folder…**, which open the system file chooser in the
  teacher's home; in the TUI the path starts at the user's home; from a
  terminal it is `nixorium send plan|apply --file <file-or-folder>`. A folder
  arrives on the students' desktops with its name and contents. The page's
  toolbar reads Lock, Internet, **Share screen** (formerly Show my screen),
  Send files and Power; Lock, Internet, Send files and Power are menus, and
  only one stays open.

- The classroom view follows GNOME's dark style. In a computer's own window,
  actions and **Take control** are unavailable while the computer is off or
  restarting, and its last picture is dimmed; without a student session only
  Internet and power actions remain.

- The overview no longer asks to prepare network boot files on a lab
  installed from USB, and no longer reports a saved configuration as
  unapplied after a controller review finds it already running. Controller
  reminders now follow the latest verified review, activation or setup check,
  including intervening commits and software/update follow-ups. New saves and
  observed drift remain visible; unverified results never show a success notice.

## [3.0.0-beta.1] - 2026-10-04

- A new controller now starts from the nixpkgs revision its release was
  validated with, instead of the newest commit of the channel. A channel update
  (kernel 6.18.55) broke the VirtualBox guest additions build and stopped every
  fresh installation; the channel is still declared, so package-base updates
  move the pin forward through the reviewed workflow.

- Backward-compatibility paths are removed; there are no older installations
  to carry. The student home is always restored at boot from the workspace
  profile (an empty profile when `mkLab` receives none), so the profile-free
  reset, its student template and `scripts/home-reset.sh` are gone, and
  `nixoriumWorkspace.runtimeEnabled` is dropped. Deployments must report
  `deploymentStatus.controller`, `labMeta.deploymentMode` and
  `nixoriumSoftware.controller`; there is no fallback for metadata without
  them. The installer refuses releases without bootstrap capability 2. PXE uses
  only managed preparation, and the standalone `run-harmonia` and
  `run-pxe-proxy` apps are removed. The old `deploy.lock`, the one-time
  MoreWaita, dock and desktop-icon migrations, the pre-workspace template-reset
  patch and the guides for migrating old deployment layouts are removed too.
  A workspace profile that requires no packages no longer evaluates host
  systems, which keeps evaluation memory low.

- The classroom view, on unless `"classroomView": false` in `lab-settings.json`, installs
  a classroom view agent on client computers. It runs in the graphical session
  on a private socket, opens no network port, and is reached by the controller
  only through its existing SSH access. It returns screen thumbnails through
  Mutter's own screen cast interface, without a consent dialog; GNOME's
  sharing indicator stays visible while it captures, and a bundled Shell
  extension removes the indicator's stop button. Capture stops 30 seconds
  after the last request. On the controller, **Computers → Classroom view**
  (teacher and administrator) opens a browser page with every student
  screen, refreshed every 1.5 seconds; clicking a computer shows it larger.
  The page is served on the controller's loopback only, behind a one-time
  token. Clicking a computer opens it in its own window, refreshed about ten
  times a second, which can be moved, maximized or put on full screen;
  **Take control** sends the teacher's mouse and keyboard to that student's
  session, and stopping control releases every key and button. Check boxes
  select computers for **Block/Allow Internet**, **Restart** and **Shut
  down**, also available for one computer in its window's **Actions** menu.
  They use the same reviews as the classroom dashboard and confirm with one
  explicit button; the review names computers that someone is using, and
  the outcome appears as a brief notice. Cards show each computer's real
  Internet state, read every 30 seconds while the page is open.
  **Lock** covers the selected screens with "Eyes on the teacher" and takes
  their keyboard and mouse until **Unlock**, logout or restart; a locked
  computer is still visible but cannot be controlled. The same lock is in
  **Computers → Lock screens** for the teacher and the administrator, and in
  `nixorium lock plan|apply --on <clients|@lab> --action lock|unlock`.
  **Send desktop** copies the files and folders on the desktop of whoever
  uses the controller to the desktop of the selected computers, after
  a review: at most 2000 items and 500 MB, hidden files and links left out,
  owned by the student, never executable, and a name already on the
  student's desktop is kept as "name (2)". **Computers → Send desktop**
  (teacher and administrator) and `nixorium desktop plan|apply --on
  <clients|@lab>` send the desktop of whoever runs Nixorium the same way.
  **Show my screen** (classroom view page only, up to 1920 pixels) asks GNOME to share the
  teacher's screen, a window or a tab and, after a review, shows it over the whole
  screen of the selected computers with their keyboard and mouse blocked,
  above any lock, until **Stop showing**, closing the page, or ten seconds
  without pictures; computers that sign in meanwhile get it too.

- New laboratories (site template): the desktops of the administrator and
  the teacher no longer show Home and Trash icons, so they hold only the
  files that Send desktop copies to students; the trash is in the dock.
  This applies once per account; students are unchanged. Staff docks now
  keep their own changes: the laboratory's favorites are set at the first
  login, and later only applications the laboratory adds are appended. Existing
  laboratories get it by applying the same change to their
  `modules/workstation.nix`.

- With the classroom view, the teacher's Nixorium icon opens the classroom
  view directly; `nixorium classroom-view` does the same from a terminal.
  The administrator still gets the dashboard.

- Base software: Chromium, Ghostty, Git and terminaltexteffects (the lab
  screensaver) are now always installed on every computer, whatever
  `lab-software.json` declares, because Nixorium's own features need them;
  the software review refuses to remove them or limit them to some
  computers.

- Veyon is removed: the classroom view replaces it. Gone are the Veyon
  module, package and patches, the `veyon` Flake input, the Veyon key pair
  (`setup keys` now manages the cache and SSH pairs only), port 11100 and
  the `veyon-master` group; the administrator and the teacher read student
  home snapshots through the new `nixorium-staff` group. Clients accept only
  SSH from the controller. Deployments must drop `publicKeys.veyon`,
  `keys/veyon-public-key.pem` and any `veyonNativeHosts` setting, which are
  now rejected. GNOME's remote desktop server stays disabled.

### Fixed

- USB/SSH installation now returns a durable reboot acknowledgement before the
  live ISO closes SSH, and the TUI automatically retries read-only installed
  system verification after the client starts from disk.
- `nixorium --help` now lists the `package-base`, `workspace`, `host-key`
  and `support` commands, which were accepted but missing from the usage text.

## [2.1.0] - 2026-10-01

### Upgrade notes

- The deprecated `veyonNativeHosts` string list remains accepted and ignored
  for compatibility with 2.0 deployments. Every laboratory host continues to
  use native Veyon capture; removing this field is not required for updating.
- Controller DHCP addresses inside the static laboratory subnet are now
  rejected. Existing overlapping configurations must choose a distinct static
  laboratory subnet before an update can validate.
- Existing deployments retain their software declarations and template files.
  The new template's software and student-profile defaults require a new
  deployment or an explicitly reviewed deployment template reset.
- For users tracking unreleased `master` revisions: remove the experimental
  `mkLab.workspaceRuntimeEnabled` argument from hand-written callers. It was
  introduced and removed after 2.0.0; a supplied workspace profile now selects
  the managed boot reset without a separate runtime switch.

### Highlights

- Encrypted controller backups, guided recovery of interrupted operations,
  reviewed Git discard and cleanup of old system generations.
- Guided student preferences and pinned VS Code extensions, expanded
  Programming tools, and restricted classroom controls for teachers.
- Clearer controller and USB installation, actionable diagnostics, and a
  dashboard that shows only work needing attention.

### Changed

- The Overview shows rows above the menu only when something needs action,
  under "Needs attention". The "No pending work observed" line and the
  "Clients last checked" reminder are gone; Computer inventory still shows
  when each computer was last checked.

- When the laboratory is set up for the first time, the installation form
  proposes the controller's network card for the client computers, which are
  often the same model. Laboratories with configured clients keep their saved
  value.

- USB installation no longer stops when the PC is connected through a
  differently named network card: before disk selection it shows the
  configured and the observed card and offers to save the observed one for
  that computer only, through the ordinary reviewed settings save. It then
  cancels the prepared session, prepares the computer again and returns to
  the live console step (a new `passwd` is needed). No disk is touched. The
  PXE installer, which cannot change the configuration, now names the card
  the PC started from the network through.

- USB installation screens read more easily: each interactive screen shows
  "Step N of 6", the live-ISO commands are numbered with what each one shows,
  labels are aligned and in plain words (computer, IP address of the PC,
  fingerprint), unusable disks say why (for example the USB stick you started
  from), the erase review leads with the disk and moves revision, system and
  cache to a muted technical block, and progress and waiting messages avoid
  internal terms. Confirmation words and safety checks are unchanged.

- The **Nixorium** launcher on the controller uses the Nixorium mark from
  nixorium.org, and the site template pins it first in the dock of the
  administrator and the teacher on the controller. Existing deployments get
  the dock entry after a deployment template reset or by adding
  `nixorium.desktop` to the staff favorites in `modules/workstation.nix`.

- The Overview no longer asks for a controller backup before the laboratory
  keys exist, and low disk space is shown in Maintenance beside **Free disk
  space** instead of among the Overview's pending work.

- The Pi and OpenCode coding agents are now part of the Programming software
  profile only. The other six profiles, including the default Essential, no
  longer install them, and new deployments start without them. Existing
  deployments keep their `lab-software.json` unchanged; remove the two
  packages in Software if they are not wanted.

- The controller setup command explains itself and reads more easily: an
  introduction lists the five steps and says that nothing changes before
  `ERASE`; steps are numbered; keyboard layouts are named and the time zone
  is suggested from the keyboard; the three accounts are explained; answering
  no to the review or to the applications asks again instead of cancelling;
  disks are chosen by number and shown with size and model before `ERASE`;
  installation reports its three phases, says what to do when it stops
  midway, and ends with the next steps. Legacy BIOS is refused before the
  first question, and Git and template output is no longer printed.

- The settings review (TUI and `config plan`) says what happens next: apply
  to the controller, update the computers, prepare network installation again,
  password changes, and warnings when the change renumbers the laboratory,
  removes computers or renames accounts.

- Settings that no longer validate (an obsolete field, or a value a newer rule
  rejects) open in **Change settings** with each problem listed instead of
  blocking the editor; the review shows removed fields and saving writes
  valid settings.

- Undo a mistaken uncommitted change: **Review Git changes → Discard
  changes** and `nixorium git discard plan|apply` restore the selected files
  to the last commit after the `DISCARD` confirmation, first keeping their
  content under `refs/nixorium/discard-backups/`. Untracked files, new files
  and private keys are never touched.

- Encrypted controller backups (ADR 0024): `nixorium backup create|verify|
  restore` and **Maintenance → Back up the controller** write one
  passphrase-encrypted file with the configuration and its history, the
  private keys and the trusted computer keys. The screen proposes the
  administrator's home directory, which is always writable, and reminds you
  to copy the file away from the controller. The Overview and the doctor
  remind you when no backup exists, it is older than 30 days, or keys or
  settings changed since it. The troubleshooting guide describes replacing a
  failed controller without reinstalling the computers.

- Guided recovery of interrupted operations (ADR 0023). `nixorium deploy
  recover plan|apply` (TUI: the Overview row) checks every computer of an
  interrupted client update, refuses while any is still applying, requires an
  explicit acknowledgement for computers that cannot be checked, and after
  `RECOVERED` archives the record so operations are unblocked.
  `nixorium template-reset recover plan|apply` finishes an interrupted
  template reset or, for a mixed checkout, restores the configuration from
  before it after `RESTORE`, keeping the current files under a recovery
  reference. Neither is automatic and neither declares the old operation
  successful.

- `nixorium recovery status` and the Overview list what blocks operations —
  interrupted client update, unfinished USB installation, interrupted
  template reset, controller network recovery, held lock, invalid settings,
  controller not running its last applied configuration — each with its next
  step, read from local state only. When the laboratory cannot be read, the
  dashboard opens in safe mode with these items, diagnostics, Git review and
  the support report instead of only retrying, and `nixorium doctor` still
  reports local checks with the configuration error first. The doctor also
  reports these blockers and an unsynchronized clock.

- Refusals and uncertain results name the next step with a stable code: the
  TUI adds a `Next:` line to the notice, a failed CLI command ends with
  `Next (CODE): …`, and JSON issues carry a `next` object. The codes are
  explained in the troubleshooting guide.

- A refused operation says what is running, who started it and when, for
  example "Update computers, started by admin at 10:02". The holder records
  itself in the operation lock; an unconfirmed record is ignored.

- Teachers no longer see administrative errors: classroom controls say whether
  to try again later or to ask the administrator, with a short code explained
  in the troubleshooting guide. When the classroom service is down, `nixorium`
  says so instead of reporting a missing deployment repository. Classroom
  controls read the last committed configuration, so uncommitted edits cannot
  break them.

- The Overview lists an unfinished USB installation when the dashboard opens,
  and `nixorium install usb status` without `--id` shows it.

- A **Nixorium** launcher on the controller opens the dashboard for the
  administrator and the teacher and keeps the window open after an error. The
  administrator's console and SSH logins mention waiting recovery work.

- Low disk space on the Overview opens **Free disk space**; doctor remedies
  name the screen and command to use; a dirty Git tree points to **Review Git
  changes**; unconfirmed power, Internet and cleanup results offer **Check
  computers**.

- Free disk space after review: Maintenance → Advanced → **Free disk space**
  (`nixorium cleanup plan|apply`) removes old system versions on the controller
  and selected clients. Each computer keeps its newest 10 versions plus the
  running and booted ones, the boot menu lists at most 10, and the review needs
  `CLEAN`. A computer that changed after review, is off or is busy is skipped;
  nothing runs during deployments or network installation (ADR 0022).

- The desktop uses the Yaru-yellow icons of Nixorium 1.0.0 again instead of
  MoreWaita. Accounts still set to MoreWaita switch at their next login; any
  other icon choice is kept.

- Classroom controls open faster for the teacher: the dashboard appears at
  once and loads inside, the classroom worker no longer evaluates deployment
  readiness it does not show, and it reuses the laboratory identities for an
  unchanged clean revision (evaluated in advance when the worker starts).

- Consistent TUI keys: `r` refreshes on every screen that has a refresh
  (services, logs, Git review and network boot used `f`). Keys that did
  something else with `r` moved: `n` starts a new review after a result and
  recovers the network in PXE, `s` retries or completes an interrupted local
  save, and `c` reviews the cache restart. Every acting key is in the action
  bar.

- Maintenance lists everyday tasks first (settings, diagnostics, system and
  Nixorium updates, logs) and groups controller application, services, Git
  review and template reset under a separate **Advanced** heading.

- Plainer labels: **Distribute the prepared system** is now **Update
  computers**, **Rebuild controller** is **Apply to controller**, and related
  titles and hints follow. Typed confirmation words are unchanged.

- The PXE installer starts by itself once on the client's first console,
  still asking for the identity and the typed erase confirmation (nothing runs
  unattended; `sudo /installer/setup.sh` starts it again). Both installers ask
  again after a mistyped disk choice or confirmation, and an empty answer
  cancels. The controller installer now asks for `ERASE` like the client.

- USB-over-SSH installation shows progress and plain outcomes: while the
  installer job runs the view refreshes itself with read-only status requests
  and names the current step and elapsed time; results say in one sentence
  what happened, whether the disk may have changed and the next step, with the
  raw fields behind `d`. Pending checklist steps appear as waiting, disk sizes
  are human-readable and the identity list keeps the selection visible.

- Computer lists show each computer's last known state with the time of the
  check: the update list says which computers need the update, power controls
  which are on, and Internet access whether each is on or blocked. `r` checks
  again (Internet access with a read-only check) and `n` selects the relevant
  computers; reviews still probe again before anything is sent. Internet
  reviews and results use plain words, and Inventory offers `u` to review
  updating every computer that needed it.

- Power reviews distinguish a computer someone is using from an untouched
  login. The client session helper now reports `unused` when every user
  session is a local graphical session without keyboard or mouse input for ten
  minutes or since it started (for example the student's automatic login), and
  keeps `active` whenever it cannot tell. Only sessions in use carry the
  data-loss warning; older clients still report `active` or `idle`.

- Software removal is deliberate: in Selected, Enter opens a package's details
  with explicit actions to change where it applies (current scope preselected)
  or review its removal; `r` no longer removes. Removing a package that reaches
  the controller states that the controller is rebuilt right after saving.

- The controller review states in plain words what changes since the last
  verified activation (saved settings, software selection, student preferences,
  Nixorium version, system and packages, other files), reading only Git
  objects, and moves the revision under technical details. When the
  controller already runs the saved configuration it says so and offers only
  a way back. `controller plan` reports the same `changes`.

- A supplied workspace profile configures student preferences at normal boot
  after system application. The site template ships active Essential defaults.
  Workspace review leads with readable preference changes, not JSON, and
  clearly separates saving from system application and boot-time restoration.

- Settings fields now explain their purpose and show examples and draft network
  addresses. Reviews use readable labels. Both validators reject a controller
  DHCP address inside the static lab prefix; existing overlapping deployments
  must select a distinct static subnet before an update can validate.

- Configured PXE installations offer a compact saved-settings summary with
  Continue and Edit, keeping full preparation preflight without replaying the
  whole form. Incomplete settings still open the guided form.

- Overview now lists selectable pending local work and timestamped client
  observations, including low Nix-store space. Local refresh never probes
  clients or evaluates Nix; every follow-up retains its normal review.

- Save results now separate configuration, controller and client status and
  offer the next independently reviewed action. Settings, template reset and
  workspace do not apply systems automatically. The ordinary workspace editor
  now records only its reviewed saved profile locally, preserving unrelated
  staged changes; partial writes/commits require inspection. CLI workspace
  apply remains file-only, and no save deploys clients or reboots them.

- Deployment reviews now probe only selected computers with the inventory's
  bounded SSH-port check and offer a freshly reviewed reachable-only subset.
  Results show each computer's outcome and guidance, distinguishing unavailable
  computers from update failures without weakening pending-evidence recovery,
  authenticated verification, exact targets or the required confirmation.

- The Programming software profile now prepares a complete development
  workstation: C/C++ (`gcc`, `gdb`, `gnumake`, `cmake`), Java, Python and PHP
  toolchains, MySQL 8.4 and MySQL Workbench. Where `mysql84` is selected the
  site module runs a teaching server that listens on loopback only, leaves
  `root` without a password and discards all databases at every boot; where
  `php` is selected it adds Xdebug; where `apacheHttpd` is selected a
  XAMPP-style Apache with PHP serves the student's `~/public_html` at
  `http://localhost/`, as the student and on loopback only. Student homes receive VS Code extensions
  for web/PHP, C/C++, Python and Java: bootstrap and template reset copy the new
  `workspace-profile.programming.example.json` when Programming is chosen, and
  the legacy home of profile-less deployments installs the same set. A new `programming-profile-vm`
  check activates every one of those extensions offline and exercises the
  toolchains and database.

- Student workspace profiles accept `vscode.extraSettings`: reviewed free-form
  editor defaults such as theme, telemetry and extension preferences. Typed
  settings, managed update keys, workspace-trust overrides, automatic tasks and
  integrated-terminal profile/shell/environment keys are refused. The managed
  seed also writes the fixed `.vscode/argv.json` launch defaults.

- Packaged VS Code extensions whose identity contains uppercase letters, such
  as Pylance, can now be selected by their lowercase ID. The seed build refuses
  an extension whose declared dependency is not selected.

- The workspace extension catalog is no longer an allowlist: `/` in VSCode →
  extensions searches every packaged extension of the pinned package set by
  name, and package search matches `vscode-extensions.<text>` across
  publishers. Catalog entries may set `writable` so an extension that creates
  files in its own folder (the Python and Java debuggers) is copied into the
  reset home instead of linked; the reset helper accepts either form.

- Student workspace profiles accept `vscode.marketplace` pins for extensions
  that the package set lacks: publisher, name, stable version and package
  hash (optionally `linux-x64`). The controller fetches the pinned bytes when
  it builds; clients stay offline. `nixorium workspace marketplace --extension
  <publisher.name>` and the TUI (`m` to add, `u` to check for updates) pick the
  newest stable Linux version the pinned VS Code accepts, download it into the
  Nix store and report dependencies and native programs before anything is
  added to the draft.

- The student workspace editor names its editor section VSCode and adds
  **Other settings**: add, edit or remove further editor defaults by name and
  value, or take them from a pasted settings file. Refused names are reported
  without echoing their values.

- Reopening the administrator TUI discovers surviving controller/PXE jobs from
  local systemd state and bounded progress records, including revision-bound
  controller instances. Overview offers read-only attachment; stale running
  records with no running unit are marked interrupted with journal guidance.
  Conflicting managed starts are refused, and observation never resumes a
  workflow or replaces activation verification.

- Unified TUI working states with elapsed time, visible safety/exit behavior and
  feedback for unavailable keys. Read-only loads and proposals accept Esc,
  retain editable drafts and ignore late replies; ordinary reads have a two-minute
  limit and candidate build reviews have one hour. Foreground mutations remain
  protected and PXE preparation still runs independently. Local Git/Nix helpers
  bound process groups and inherited pipes; cancelling IPC closes the client
  connection without claiming to stop a worker-owned operation.

- Added explicit single-client SSH host-key review and rotation after deliberate
  reinstall, in computer details and `host-key plan`/`apply`. Changed-key failures
  are typed, physical fingerprint verification remains required, stale reviews
  are refused, unrelated trust and private backups are preserved, and PXE or
  protected client work blocks saving. No automatic rotation or weakened SSH.
  Updated the management VM's existing update-build assertion to match the
  grouped-build contract while checking every required output is retained.
  Recheck clean Git state and revision after inventory and fingerprint reads,
  refusing concurrent edits, commits and interrupted template resets before trust
  can be saved.

- Overview distinguishes incomplete configuration, uncommitted managed files,
  controller application and installation-file preparation, without adding
  startup evaluation or treating stale artifacts as an unconfigured lab.

- Fixed empty-field handling in shell metadata, including controller-only labs;
  legacy home reset now discovers common image formats and skips empty folders.
  Setup cancellation names the correct workflow, background quit describes its
  effect, and update discovery initially selects the newest stable release.

- Corrected regional-settings guidance: guided forms select time zone and
  keyboard, while desktop locales use the reviewed configuration workflow.

- PXE enrollment refuses a missing configured client interface before the erase
  review, lists detected interfaces and explains how to prepare corrected files.
  Installation settings now edit the client override and show controller context.
  The installer VM covers refusal before disk changes and a matching interface.

- Preserved exact changed Git paths, including unstaged settings, renames and
  whitespace, so setup cannot treat uncommitted managed configuration as saved.
  Git warnings are kept separate from parsed status records.

- Made shell validation independent of the developer's boot firmware and the
  location of host executables, using an injectable UEFI probe and distinct
  PXE store fixtures built from the locked inputs.

- Reduced repeated Nix evaluation in controller preflight and grouped the complete
  update build set into one invocation. Inventory and package discovery no longer
  evaluate every workspace host; readiness and build guards remain enforced.
- PXE preparation resolves all shared artifacts and client derivations together,
  then builds the deduplicated derivation set without repeating Flake evaluation.
  Temporary roots protect outputs until the revision-bound manifest is published.
- Update validation details now expand in place with `l`, preserving the target,
  progress, elapsed time and safety notices for framework and package-base updates.

- Administrative TUI startup no longer waits for Nix evaluation of enabled
  student workspaces. Client inventory loads when needed, supports cancellation,
  and retains full validation before reviewed operations.

- Fixed controller progress tracking after framework, package-base and software
  updates: `l` now expands the managed phase details in each entry point.
- Bounded Colmena process/pipe waits and enabled SSH connection/liveness limits.
  Deployment details show a private, bounded command-output tail and its age;
  a separately confirmed stop ends local supervision without claiming remote
  cancellation. Uncertain applies retain durable fleet-wide pending evidence,
  block conflicting operations and do not record observed revisions as success.
- Added an administrator TUI deployment-template reset with pinned upstream
  preset selection, an initial active guided-home profile, explicit loss review,
  exact confirmation and a recoverable local Git backup/commit. Settings, keys,
  private files and input pins are preserved. Interrupted resets block normal
  operational preflight; no activation, deployment, reboot or push is included.
- Fixed private template output forwarding so an active guided-home profile can
  resolve flake revision metadata without infinite recursion. Reset also adapts
  the affected older template form without changing its locked framework.

### Added

- Added an internal bounded support-report schema with immutable snapshots,
  explicit data classification, fixed diagnostic/operation allowlists and
  adversarial tests excluding private fields and free-form error text.
- Added `support preview` and interactive `support export`, reusing ordinary
  diagnostic observations without builds or raw log reads. Export saves the
  exact reviewed JSON to a private create-new local file, never uploads or
  remediates, and distinguishes unavailable data and unconfirmed durability.
- Added an administrator TUI support-report preview under Diagnostics, with
  cancellable collection, bounded full-payload scrolling, exact local export
  and stale-response protection. The restricted teacher dashboard is unchanged.
- Support findings carry a versioned static route to canonical recovery
  guidance, with checked document anchors and explicit read-only-first and
  stop conditions. Routes never execute a remedy or authorize an operation.
- Added an opt-in workspace preparation API with strict preference validation,
  deployment-owned catalog/baseline resolution, pinned extension metadata and
  prerequisite checks on the controller and every client. Preparation metadata
  is preserved in the offline installer. Reading preparation metadata does
  not activate preferences; profile-free deployments retain legacy behavior.
- Added a profile-selected managed-home runtime for the configured student
  on the controller and clients. It builds an immutable shell/Git/XDG and
  preference seed, composes wallpapers, validates pinned extension payloads,
  and restores only at normal boot. Guarded snapshots and durable failure
  evidence block unsafe retries and login after an incomplete reset. Existing
  template content is not imported automatically.
- Added an active Essential workspace profile, a deployment-owned starter
  catalog and a Programming profile example. Existing deployments acquire
  these template files only through an explicitly reviewed template reset;
  framework updates alone do not replace private template files.
- Added a read-only workspace candidate-resolution hook for exact preference,
  version and destination previews without temporarily saving a profile.
- Added the internal workspace review/save application contract with complete
  proposal binding, fresh validation and distinct conflict/durability outcomes.
  Its adapter preserves Git-filtered source identity, composes both deployment
  hooks and saves only the JSON through a locked no-follow atomic replacement.
  `workspace plan`/`apply` expose JSON/text review and confirmed declaration-only
  saving without implicit commit, activation, deployment or reset.
- Recognize workspace profiles in Git review and validate their schema in the
  existing separately reviewed, exact-path commit workflow. Unrelated staged
  content remains untouched; committing does not deploy the profile.
- Added a guided student workspace editor under Maintenance → Settings.
  Desktop/dock, pinned editor extensions/settings and browser choices share the
  CLI review/save boundary and preserve inheritance and ordered favorites.
  The ordinary TUI records the reviewed profile in a local commit and offers
  a separate controller review; CLI apply only saves the profile. Saving does
  not deploy systems, capture a home or change current student preferences.
- Update reviews now compare existing workspace package/extension versions,
  dependencies and preferences across the current and proposed pins, with the
  comparison bound to the review token. Saving remains separate from system
  application and boot-time reset; builds do not certify actual plugin loading.
- Added a release-checkpoint VM test for the pinned VS Code/Live Server pair:
  real extension activation and local HTTP serving with external traffic
  blocked, editable student preferences, and boot-only profile updates and
  extension removal. The focused check is separate from the fast edit loop.

- The configured teacher can now open a restricted classroom TUI on the
  controller without access to the administrator-owned deployment. It exposes
  only authenticated computer inventory, temporary Internet control, and
  reviewed client shutdown or restart. A group-private local worker performs
  those operations as the deployment owner while student accounts remain
  excluded; configuration, deployment, installation and controller maintenance
  are not delegated.
- Added reviewed client restart to **Computers → Power controls** and the
  `restart plan` / `restart apply` CLI. It reuses inventory/session rechecks,
  expiring review tokens, the fleet operation lock and fixed SSH dispatch, and
  never treats temporary network loss as proof of a completed reboot.

### Fixed

- Adding software, saving settings and other reviewed changes no longer need
  memory proportional to the number of computers. Student-profile checks now
  evaluate one computer for each group of identical ones (same role, network
  interface and managed software; a computer with `hostModules` stands alone)
  instead of every computer: on the 20-computer template, a software review
  dropped from more than 14 GiB to about 4 GiB and stays there with 40.
  Per-computer differences must use `hostModules` or software scopes; a shared
  module that changes packages by host name alone is checked only on the first
  computer of its group.
- USB/SSH client installation no longer rejects Wi-Fi interfaces. Network
  reachability, reviewed interface/address and boot identity, signed-cache
  verification, and exact disk review remain required. Live Wi-Fi credentials
  are not automatically transferred to the installed system. Documentation
  distinguishes USB networking from hardware-dependent Wi-Fi PXE compatibility.
- Operation-summary reads now reject special files without blocking local
  diagnostics. Interrupted support collection also drops unchecked revision
  consistency, and known configuration/key outcomes remain in aggregate counts.
- Full and evaluation-only validation now use bounded groups of related checks,
  with exact coverage validation and separate base/workspace API evaluators,
  to avoid retaining every NixOS test graph in one memory-heavy process.
- Home-reset services no longer restart automatically during system rebuilds,
  including managed/legacy transitions. Returning to legacy cannot bypass an
  incomplete managed reset. Staff preferences and controller login selection
  are preserved.
- GitHub Release publication now requires successful full validation of the
  tagged commit, including VM tests, representative system builds and offline
  equivalence. It runs as parallel jobs with KVM, one per check group, and also
  nightly on `master` and on demand; pull-request and `master` push checks
  remain lightweight.
- The student login setup now preserves the random deployment-owned wallpaper
  selected during the boot-time home reset. The static blue wallpaper remains
  the default for the persistent administrator and teacher accounts.

## [2.0.0] - 2026-09-27

### Added

- Administrators can block or restore client Internet access from Computers →
  Internet access, or reviewed `internet plan` / `internet apply` commands.
  The temporary IPv4/IPv6 firewall preserves laboratory access and DHCP,
  blocks existing external connections, and resets on reboot. Authenticated
  observations, boot-bound requests and per-client verification prevent stale
  reviews or offline computers from receiving a delayed block.

- Added a reviewed USB/SSH client-installation path for systems without usable
  UEFI network boot. It pins the official Minimal ISO's physical-console host
  fingerprint before password use, installs an ephemeral operation key, builds
  an exact signed-cache closure, excludes the boot medium, and requires a
  content-bound host/disk confirmation before dispatching an independent remote
  systemd job. Durable operation IDs support fail-closed restart reconciliation,
  separate reboot and post-boot verification, and reviewed host-key rotation
  without ever replaying an uncertain disk mutation. PXE, deployment, and USB
  installation share one atomic controller coordination boundary.
- Added GitHub Sponsors metadata and a public support link so users can fund
  project infrastructure, test hardware, documentation, and maintainer time.
- Software now exposes **Add profile** in the ordinary TUI. It presents the
  deployment-owned descriptions, package exclusions and supported scopes, then
  saves one aggregated candidate while preserving existing package scopes. The
  result reuses controller recovery and the fresh affected-client deployment
  review; absent profile metadata leaves individual software management intact.
- Controller bootstrap capability version 2 now asks for and reviews the
  initial software profile after account and regional settings, but before Git
  initialization, Nix evaluation, or disk changes. Catalog, helper, and
  declarations come from the same immutable template revision; version 1
  revisions retain their settings-only flow.
- New private site templates define seven deployment-owned software profiles:
  Essential, General education, Programming, Graphics and illustration, Audio
  and video, CAD and 3D modelling, and STEM and scientific computing. Essential
  is the deterministic default for new sites; all profile package IDs are
  evaluated against the pinned package set for laboratory and controller-only
  deployments, and the offline installer preserves the profile metadata.
  Every profile includes Git, the Ghostty/TTE lab screensaver, Node/npm plus
  system-managed Pi and OpenCode CLIs; the controller and management-command
  runtime also carry Git independently of profile selection, while VS Code
  remains specific to Programming.
  Per-user npm overrides persist for
  staff, while the reset student account discards npm globals and AI-agent
  credentials/state before snapshots and restores an empty managed prefix.
- Added an optional, deployment-owned `nixoriumSoftwarePresets` contract and
  `software presets` / `software preset plan` / `software preset apply` CLI
  workflow. A profile produces one reviewed, additive and atomic
  `lab-software.json` candidate; exclusions are explicit, existing scopes are
  preserved, and catalog, package-resolution, file and token drift invalidate
  the review.
- Successful guided software saves can continue directly to a fresh client
  deployment review with exactly the affected computers preselected. The review
  states that it applies the complete current configuration, reports removed
  inventory targets without broadening the selection, and remains blocked behind
  controller recovery when activation did not verify.
- Software now offers an explicit, refreshable system-state snapshot. It shows
  the desired revision and observation time, accepts controller state only from
  a matching activation receipt and active closure, and classifies clients from
  authenticated observations without treating deployment history as current
  evidence.

### Changed

- TUI page headings use restrained violet; selected rows have a petrol background
  and high-contrast light text. Neutral bold shortcut keys and aligned menu
  highlights separate navigation from green/amber/red operation states.
- The desktop template uses intelligent dock hiding for overlapping windows,
  with bottom-edge reveal. A separate one-time migration updates only dock
  visibility for existing accounts, preserving other appearance preferences.

- Installation now opens PXE and USB/SSH directly. PXE has one state-aware
  entry for guided preparation, reviewed start, finish and network recovery;
  a failed state refresh cannot expose actions against previously ready data.
- The site desktop uses MoreWaita icons, a compact bottom dock, native Adwaita
  decoration, blue accents and a static vector wallpaper. Tiling Assistant adds
  snap assist with small gaps. A targeted one-time migration updates existing
  accounts while preserving later staff customization and unrelated settings.

- Task menus now show direct shortcuts, use consistent compact rows and return
  to their parent area. Removed the duplicate Restore route: reapply lives in
  Computers and reinstall in Installation. Settings and installation methods
  expose direct shortcuts; software tabs use F2/F3/F4. Help, focus markers and
  action-bar wrapping share one visual vocabulary across terminal sizes.
- Dashboard startup defers key, controller-closure and PXE-artifact checks to
  their operations while retaining local first-run validation and current PXE
  service state. Deferred checks are never reported as verified readiness.

- All laboratory hosts now use native PipeWire/Wayland Veyon capture. Removed
  the patched GNOME Remote Desktop bridge, embedded shared VNC credential and
  pilot selector from settings screens. Legacy selector data remains readable
  but cannot enable the removed backend; each user must approve initial GNOME
  sharing, including the controller for screen broadcasts.
- Client SSH/Veyon ports now accept only the controller's static IPv4 address
  on the laboratory interface. Nftables blocks other sources, IPv6 management,
  old unauthorized connections and external VNC while preserving loopback.

- Native Wayland pilot hosts use Veyon 4.11.3 plus upstream's opt-in portal
  restore-token persistence fix. Per-user tokens and portal permission stores
  survive student-home resets outside the template and snapshots; GNOME still
  requires initial approval. Other hosts retain the existing VNC fallback.
  The sanitized server PATH now uses trusted NixOS paths for authentication
  and input helpers; release metadata correctly identifies the 4.11.3 tag.

- Guided USB/SSH installation now asks the operator to type only the live IPv4
  address and temporary password. The controller observes the Ed25519 host key
  without credentials, displays its fingerprint for comparison with the
  physical console, and requires `MATCH` before any password authentication.
- New private deployments now install and enable Desktop Icons NG and Dash to
  Dock as baseline workstation policy, independently of the application
  profile. Files created in the Desktop directory are visible on the desktop,
  and the dock remains visible outside the GNOME overview. Login setup repairs
  these two required extensions for existing staff accounts without removing
  other enabled extensions.
- The public controller bootstrap now separates settings, input prompts,
  reviews, preparation output, and installation logs in an ASCII-only console
  interface. Time zone is requested immediately after the selected keyboard is
  activated, before account details and passwords. Its initial software-profile
  step installs the complete selected profile without exposing package IDs or
  asking for technical exclusions.

### Fixed

- PXE preparation and operation guards can lock an existing legacy deployment
  lock through a read-only home sandbox. Active legacy operations still block
  new work; no home write access or lock deletion is required.

- Veyon remote control normalizes unused RFB pixel bytes before rendering,
  preventing transparent blocks and visual corruption on Wayland while
  retaining lossless image quality.

- Client deployment now checks for unfinished USB installations before review.
  An installed computer awaiting its final check offers verification and a
  return to a fresh deployment review, preserving the selected computers.
  Failed verification offers retry, connection guidance, and technical details;
  active or uncertain installations open their existing recovery flow. A USB
  reservation appearing after review also leads to this guided recovery.
- Controller rebuilds register the reviewed system in the persistent NixOS
  system profile before activation and verify it afterward. The activated
  generation now participates in boot configuration, rollback history, and
  garbage-collection retention instead of only changing the running system.
- Controller rebuilds no longer wait for an already-installed USB client to
  come online for its first-boot check. A strict, read-only guard confirms the
  persisted completion receipt before pausing the worker, acquiring the normal
  controller lock, and resuming the worker after success or failure. The client
  remains unverified, its reservation and runtime credentials are retained,
  and active, failed, or uncertain disk installations still block activation.
  Refreshing a completed operation no longer replaces durable disk-completion
  evidence with the status of an old ISO that has already disappeared.
- USB/SSH post-boot verification can now publish host trust atomically inside
  the worker sandbox and release the installation reservation that blocks
  controller rebuilds. Controller activation preserves existing known hosts
  in a dedicated writable subdirectory, retaining the standard OpenSSH path
  through a symlink while leaving SSH keys and configuration read-only.
  Retries reuse only an identical private backup, and operation-log publishing
  no longer attempts to chmod already-private read-only parent directories.
  Verification also resumes after a controller reboot using the persisted
  reviewed identity and administrator key, without requiring old ISO credentials.
- Explicit USB/SSH session closure now releases a never-dispatched operation
  after its live ISO has rebooted or disappeared. Closure durably records the
  intent, discards only local operation credentials, and reports remote key
  revocation as unconfirmed. It never releases an uncertain/dispatched install
  through this path or reuses its disk review.
- USB/SSH CLI and TUI now require explicit bootstrap success before transferring
  the installer. A failed connection with prepared artifacts no longer advances
  into an unverified session. The TUI retains the original connection error
  across status refreshes and allows reconnecting the prepared operation with
  fresh physical host-key confirmation and password entry.
- USB/SSH live-host revalidation now compares the canonical Ed25519 key
  material rather than the non-cryptographic comment in the public-key file,
  and a confirmed remote failure before disk mutation now revokes its live key
  and releases the controller reservation automatically instead of blocking
  controller maintenance. Safe cancellation remains available when automatic
  cleanup needs operator retry.
- USB/SSH preflight now tests whether `/mnt` itself is mounted instead of
  mistaking the live ISO root filesystem for an occupied installation target.
  Status also reports remote-log retrieval or publication failures instead of
  silently advertising a log file that was never collected.
- USB/SSH temporary-password entry now treats printable `q` and `?`
  characters as masked secret input instead of global quit/help shortcuts.
- The USB/SSH worker sandbox now permits read-only netlink route queries, so
  controller interface discovery works during cache endpoint selection.
- A physically re-pinned USB/SSH session interrupted before apply now returns
  to a cancellable pre-apply state instead of permanently retaining the global
  controller-operation reservation.
- Controller activation no longer fails by starting session-free PXE recovery
  while the same reviewed controller operation holds the global coordination
  lock. Boot recovery still validates and restores every recorded interrupted
  PXE transition.

## [2.0.0-beta.5] - 2026-09-22

### Changed

- Laboratories can update their NixOS/package base independently with
  `package-base status/plan/apply` and Maintenance → Update system and packages.
  Channel advice replaces the template's hard upstream-channel assertion;
  migrations require explicit unverified-compatibility acceptance. Candidate
  updates preserve every non-nixpkgs lock node, validate configured role variants
  and offline installation equivalence, and reuse reviewed save/controller
  recovery without automatically distributing to clients. Existing private
  templates require reviewed adoption; see `docs/updates.md` and ADR 0020.

- Configured-software navigation now sizes its viewport by rendered rows, keeps
  the focused package visible, summarizes long explicit-client scopes, and
  reports the visible range. Software removals use `REMOVE` rather than `SAVE`,
  and contextual command bars now use a dedicated control color.

- Update planning now describes its safety checks in operator-facing language
  instead of exposing Nix build terminology, and hand-rendered TUI lists now
  share the same accented focus treatment across restore, software, computer,
  maintenance, update, deployment, shutdown, key, change, and log screens.

- Colmena now receives the unmodified pinned package set and lets each host's
  NixOS module graph apply the laboratory overlays exactly once. This fixes
  client builds where the GNOME Remote Desktop and Veyon patches were appended
  twice and the second application failed before deployment began.

- Interactive operational confirmations are now either a single explicit word
  or Enter on an already visible review. Deployment target lists, release names,
  controller names, review-token prefixes, and other generated phrases no
  longer need to be retyped. Content-bound review tokens, revision checks, and
  the client installer's final disk revalidation remain unchanged.

- Colmena 0.4 deployments no longer emit the unsupported
  `deployment.sshOptions` option. Nixorium supplies the supported private
  `SSH_CONFIG_FILE` policy during apply instead, retaining non-interactive SSH
  and first-connection host-key enrollment.

- Activation now repairs root-owned managed VS Code/XDG directories for the
  admin, teacher, and student accounts, including an already existing runtime
  directory, without using world-writable permissions. New site profiles
  create staff editor directories with their final owner from the start.

- Student accounts can now use system-provided connectivity but cannot alter
  NetworkManager connections, radios, DNS, or other host network state. Admin
  and teacher accounts retain network-management access.

- Reviewed client shutdown now includes reachable clients with active sessions
  after an explicit unsaved-work warning. Unknown session state remains
  protected by default, unreachable clients remain unsent, and the interactive
  confirmation is the single word `SHUTDOWN` instead of a generated phrase.
  When active sessions are present, the review states explicitly that this
  word authorizes their interruption.

- Keyboard layout is now the first controller bootstrap setting, and its Linux
  console keymap is applied before any later input. Bootstrap stops if keymap
  activation fails and refuses graphical terminals whose compositor layout
  cannot be verified portably. PXE-booted client installers use the same
  configured console keymap as the controller. The official NixOS Minimal ISO
  in UEFI mode is now explicitly documented and reported as the supported
  controller-bootstrap environment.

- Refreshed upstream and deployment agent instructions, with task-specific
  software-update and student-home guidance. Added cached parser-backed CLI
  example checks, skill distribution/link checks, and a behavior-review map
  to keep instructions aligned without rebuilding systems for prose edits.

### Removed

- Removed the legacy pilot/test-computer workflow, including its private
  installation-session state, controller-side target selection, technical
  verification, and practical-check recording. First setup and reinstall now
  use the same generic PXE screen as ordinary installation: any configured
  computer may boot, then identity and disk erasure are confirmed locally.

## [2.0.0-beta.4] - 2026-09-18

### Added

- Added one shared invalid-settings regression corpus for the public Nix
  evaluator and Go management domain, plus a dedicated GitHub CI job that
  builds the packaged management command and runs its unit tests.

- Added optional controller, client-role, and per-host network-interface
  overrides with a compatibility fallback to `ifaceName`. The detected
  controller interface no longer becomes the implicit client interface, and
  effective role/host interfaces are exposed in `labMeta`. Unknown hosts and
  invalid Linux interface names fail validation.

- Added a network-free controller-bootstrap contract test covering immutable
  revision resolution, template/installer/layout consistency, the initial lock
  override, update-channel preservation, and fail-closed split-ref handling.

- Added a deployment-owned package-base contract for new templates. Nixorium,
  Disko, Veyon, controller, and clients follow one direct `nixpkgs` pin, exposed
  through machine-readable compatibility metadata. Framework updates reject
  candidate locks that alter or remove that pin; legacy deployments remain
  supported without implicit migration.

- Added explicit `shared` and `controller` managed-software scopes, including
  controller-only deployments with no clients. Existing client scopes retain
  their meaning. Review identifies controller effects and both sides of scope
  changes, and stale target inventories invalidate the review. Controller
  candidate validation also works with older client-only template hooks.
  CLI saving remains declaration-only; the TUI now follows its transparent
  local save with verified controller activation when applicable. Removing the final managed
  package now preserves an empty JSON list instead of producing `null`, which
  the Nix schema rejects.

- Added an explicit controller-only configuration mode with zero clients,
  local networking, inactive lab services and independent controller readiness.
  Reviewed controller activation can run without lab keys in this mode;
  client operations remain blocked and existing laboratory defaults are
  unchanged. The bootstrap now selects this mode before controller installation.

- Added typed package-name search and exact package resolution against the
  deployment's locked nixpkgs input and overlays. The catalog is now a set of
  suggestions rather than an allowlist, dotted attributes resolve structurally,
  blocked/unavailable packages remain explicit, and removals do not require an
  obsolete package to remain resolvable. The TUI separates Configured, Search,
  and Suggested views and ignores stale debounced search responses.
- Added guided import for existing cache-signing, administrator SSH, and
  Veyon private keys during first setup. Imports reject symbolic links,
  oversized or broadly readable files, encrypted unattended SSH keys, and any
  existing destination; they derive and fingerprint the public key, preserve
  the source, and never perform implicit rotation.

### Changed

- Added live, typed progress to Nixorium update validation: the TUI now names
  candidate-lock generation, evaluation, each representative output build,
  review preparation, and the final unchanged-deployment check, with elapsed
  time and an output counter. Controller rebuild and Nixorium update reviews
  now use Enter after the visible review instead of requiring `REBUILD ...` or
  `UPDATE NIXORIUM TO ...` phrases; CLI confirmations remain unchanged.

- Simplified the ordinary computer-installation TUI into one continuous flow:
  Laboratory settings are validated and saved without a separate review,
  missing controller keys, controller prerequisites, and all client closures
  are prepared automatically. Existing-key import now lives under advanced
  settings, and the only confirmation is the network-impact review immediately
  before PXE starts. The former pilot-computer selection is no longer part of
  this path; PXE recovery remains available through the advanced control.
  Time zone and keyboard are inherited from the installed controller instead
  of being asked again. Switching a controller-only system to laboratory mode
  now also applies its static address during live activation, before the
  confirmed PXE transition removes it.

- Prevented first-run and routine controller activation from terminating their
  own systemd job when a Nixorium update changes the management command's store
  path. The reviewed switch now survives through active-system verification and
  durable receipt creation.

- Moved the guided software suggestions and the initial workstation package
  declarations into the private deployment template. `mkLab` now resolves an
  optional deployment-owned `softwareCatalog`; the pinned package search
  remains available independently of suggestions.
- Moved shell, Docker/npm, screensaver, application desktop policy, branding,
  MIME defaults, and VS Code settings/extensions into focused site-template
  modules. Downstream modules receive each host's effective managed package IDs
  so removing or narrowing an application declaration also removes its coupled
  policy; template validation covers both the enabled and removed states.

- Reworked validation into a fast direct-source gate, an explicit full `mkLab`
  evaluation tier, targeted VM gates, and a batched release checkpoint; added a
  contributor guide describing how to select and maintain those levels. The Go
  package now filters unrelated repository files and the fast checks expose a
  lightweight locked Go shell for incremental tests.
- Aligned public Nix settings validation with the management command for
  required non-empty regional/Git values, absolute homepage URLs, and
  configured Veyon host identities.
- Split the dashboard state machine into focused asynchronous-message,
  global-key, primary-workflow, operational, repository, and PXE handlers.
  Presentation behavior and typed application callbacks remain unchanged.
- Isolated Software, Computers, Deployment, Shutdown, Installation/PXE,
  Settings, Controller, Update/Package Base and Maintenance state behind named
  feature models, including job identity for delayed progress messages. Split
  CLI family dispatch and reviewed apply boundaries out of the composition
  root while preserving parser, text/JSON output and exit-code behavior.
- Added a deterministic real-renderer TUI gallery with non-mutating drift
  checks, explicit regeneration, and a separate code-only generator. Added an
  allowlisted canonical-copy synchronizer that detects missing/extra files and
  refuses symlinks, plus a human contributor guide for setup, ownership, gates
  and pull-request preparation.
- Recorded a reproducible full-client closure comparison for the Essential and
  Programming profiles, with one template lock and separate closure, transfer
  and elapsed-time semantics.

- Corrected the official `cache.nixos.org` public key used by both controller
  bootstrap stages. Signed substitutes are accepted again instead of being
  rejected and rebuilt locally, while signature verification remains enabled.

- Controller bootstrap now collects teacher/student usernames, time zone,
  keyboard, and three hidden passwords immediately after version resolution,
  before any Nix evaluation or build. It persists a controller-only deployment
  with US internal locales, excludes mounted live disks, and defers all client
  network/key work to the TUI. After explicit disk confirmation, the pinned
  partitioning tool is prepared before it can touch the disk; the deployment
  lock, evaluation cache, temporary 4 GiB swap, and full controller closure then
  use the mounted target disk. Serialized Nix jobs are passed explicitly across
  `sudo`, avoiding live-ISO memory exhaustion and silent terminal termination.
  `master` is the bootstrap default while older releases retain their compatible
  legacy flow.

- GitHub validation now stops after its documented evaluation-only source and
  fresh-template coverage. It no longer runs the generated deployment command,
  which built the Go package on cold runners and could exceed the 15-minute job
  limit. Full local validation builds the declared checks and one representative
  client instead of making `nix flake check` enumerate all 20 generated clients.

- Managed-software proposals now validate every declaration and pinned package
  against one representative client, plus the controller when its software is
  affected. Planning and saving no longer evaluate every configured client;
  deployment still builds each explicitly selected machine.

- Reduced the home screen to five operator tasks. Restore and Update Nixorium
  moved under Advanced tools; Install new computers owns the resumable
  laboratory/PXE setup. Controller-affecting software changes and TUI framework
  updates now save transparently, build, activate, and verify the controller in
  the same reviewed flow. Client deployment remains separate.

- The public controller bootstrap now resolves the selected branch or tag once
  and uses that full Git revision for the site template, controller installer,
  Disko layout, and initial Nixorium lock. The selected channel remains declared
  in `flake.nix` for later managed updates. A separate installer ref may no
  longer select different content.

- Made guided Nixorium update validation capability-aware. Explicit
  controller-only deployments require controller readiness and build only the
  candidate controller system; they no longer require a fabricated client or
  unrelated netboot, PXE firmware, and installer outputs. Laboratory and
  legacy deployments retain the full representative build set, while unknown
  modes, client inventory in controller mode, and missing controller readiness
  fail closed.
- Made first-run configuration one continuous sequence: all ordinary settings
  are collected in one wizard, all three passwords follow in one protected
  session, and the complete candidate is validated, reviewed, and saved once.
  Resume events after terminal password entry can no longer reach an
  uninitialized Bubble Tea password list.
- Added the configured upstream's `master` branch as an explicit Development
  target in Update Nixorium. It uses the same candidate lock, representative
  builds, reviewed two-file proposal, and transparent local save as tagged
  releases; arbitrary branches and free-form references remain unavailable.
- Simplified setup orientation: startup now shows only its non-interactive
  loading state, the setup timeline no longer resembles a selectable list,
  the next action is presented as descriptive guidance, and Esc explicitly
  pauses the resumable setup before opening Interventions.
- Simplified guided software interaction: a reviewed configuration is saved
  with Enter while its hash-bound review token remains internal, package search
  opens with the standard slash shortcut, and arrow keys move from the search
  field directly through the returned packages.
- Reduced Regional setup to the two choices an operator recognizes: time zone
  and keyboard layout. US locale defaults remain internal, and known keyboard
  choices automatically select the corresponding console keymap.
- Made early setup status local and progressive: it no longer evaluates Nix,
  derives keys, checks artifacts, or loads the full dashboard before network,
  identity, and credentials are complete. The TUI proposes the detected DHCP
  address instead of `MASTER_DHCP_IP`, collects all pending passwords in one
  protected session, validates once, reuses the reviewed candidate while
  saving, and avoids unrelated dashboard refreshes.
- Made ordinary settings and first-setup saves record their managed files
  locally without exposing Git, commit tokens, hashes, or repository identity.
  Saves use the existing isolated-path safety checks, preserve unrelated work,
  use a fixed internal identity, reject ambiguous same-file changes, and offer
  an in-place retry when writing succeeded but local recording needs recovery.
- Applied the same transparent local-save contract to guided software changes.
  The TUI no longer sends operators through Git review after adding or removing
  software, rejects ambiguous pre-existing edits to the managed file, and can
  recover a completed file write without applying the declaration twice.
- Applied the transparent local-save contract to guided Nixorium updates in the
  TUI. The reviewed `flake.nix` and `flake.lock` proposal is recorded locally
  without exposing Git or pushing anything; an interrupted recording can be
  completed only when both files still match the reviewed proposal exactly.
  Update screens distinguish the saved release from the version of the current
  TUI process and explain the rebuild-and-reopen boundary.
- Made interactive startup render immediately before repository inspection.
  The dashboard now shows an English opening activity while status and setup
  checks run once in the background, routes to first setup only after those
  checks complete, and provides an in-place retry screen when initialization
  fails. JSON and other non-interactive commands retain synchronous output.
- Unified bare `nixorium setup` and the ordinary dashboard around the same
  first-setup screens. A fresh or incomplete laboratory opens its first pending
  setup step, while every configured laboratory retains a visible Setup and
  readiness intervention. Network, identity, locale, and password work now
  opens the existing validated editors in place; missing keys can be generated,
  verified, and installed through typed setup actions without asking the
  operator to leave the TUI and run another command.
- Changed fresh deployment and standalone-example regional defaults to US
  English for language, formats, desktop keyboard, and console keyboard, with
  `America/New_York` as the visible, editable initial time zone. Existing
  deployment settings remain unchanged; the setup selector now presents the
  principal US time zones before its international suggestions.
- Made dashboard sessions start from fresh task state: reopening Settings or
  Update no longer shows a previous result, cancelling a software removal
  returns to the selected package, and setup refreshes now render their active
  wait. Every managed quit path while PXE is active opens the same consequence
  review, including `Ctrl+C` and exits outside the installation screen; the
  operator can stop and verify installation mode before exit or explicitly
  confirm that it should remain active.
- Added reviewed client-only shutdown through shared CLI/TUI plan/apply
  operations. Nixorium resolves only evaluated client identities, excludes the
  controller, checks management access and interactive sessions, serializes
  against deployments, blocks during PXE/network recovery, and rechecks before
  sending a fixed power-off request. Active sessions remain blocked; unknown
  session state requires explicit acknowledgement. Results distinguish
  accepted, not sent, and unconfirmed requests without inferring physical power
  state or retrying blindly.
- Added guided client-software management through a strict versioned
  `lab-software.json` file. The TUI and CLI share catalog/plan/apply services,
  accept only curated packages resolved from the pinned package set, support
  all-client, evaluated-group, and explicit-client configuration scopes, and
  require a content-bound review phrase before an atomic file replacement.
  Saving a declaration never commits, builds, activates, starts PXE, or deploys
  a client; Git review and distribution remain explicit later operations.
- Reorganized the TUI around first installation and explicit later
  interventions instead of a fleet-health Overview. Restore distinguishes
  reapply from disk-erasing reinstall, setup groups technical checks into five
  operator stages, and Update Nixorium selects only releases fetched through
  the typed application service. Searchable Computers remains an explicit
  advanced check. Added contextual help,
  diagnostics, compact setup/progress, symbol-and-text status, adaptive list
  layouts, and persistent confirmation controls in long reviews. Existing
  application operations, CLI contracts and exact safety confirmations remain
  shared.
- Completed the guided pilot-computer handoff in first setup. The operator now
  selects an immutable configured identity, receives local identity/disk steps,
  checks authenticated active-revision evidence separately from the practical
  desktop check, and may finish after one computer without treating powered-off
  clients as failures. The application layer validates and probes only the
  selected pilot identity. Leaving PXE active requires an explicit consequence
  review and exact confirmation.
- Extended the same explicit-identity boundary to disk-erasing restoration.
  Reinstall now selects one evaluated computer before PXE review, repeats the
  local disk warning with that identity, verifies only the selected computer,
  and supports restoring another computer or ending a partial session.
- Made first-installation and reinstallation sessions resumable across TUI
  processes. Nixorium stores private, atomic, per-repository evidence bound to
  the exact evaluated identity, Git revision and authenticated system path;
  technical and operator practical checks remain distinct. An inventory or
  revision change makes the saved session stale instead of reusing evidence,
  and a new failed observation takes precedence over historical success.

- Added one background-aware official Bubbles spinner to every dashboard wait
  state, so host checks, Git/service loading, settings validation, update
  planning, and other operations without meaningful percentages visibly remain
  active while preserving their plain-language activity label.
- Made the PXE screen stage-aware: it now recommends artifact preparation,
  reviewed PXE start, first-computer boot/install, or network recovery from
  observed state, shows only pertinent controls, and includes the exact
  `/installer/setup.sh` handoff while installation mode is active.
- Replaced appended Settings, cache-service, local-Git-commit, and Nixorium-
  update outcomes with compact success/attention screens and explicit routes to
  the dashboard, Git review, logs, retry, or further editing. First-run Git
  completion returns to the reconciled setup checklist.
- Extended the shared light/dark visual hierarchy and width-adaptive Bubbles
  key help across Computers, Deploy, Controller, Services, Logs, Git, Update,
  and PXE screens; service, Git, and PXE states now use the same semantic color
  roles while retaining explicit text.
- Added live foreground deployment feedback with elapsed time, an official
  Bubbles progress bar, typed build/apply/verification activities, and checked-
  computer counts. Raw Colmena output stays in the private log, and the compact
  result now offers dashboard, log, and fresh-review actions.
- Replaced the dashboard's flat command wall with a paginated, keyboard-
  navigable Bubbles task menu, semantic light/dark status colors, task
  descriptions, and explicit key help while preserving textual state and all
  existing one-letter shortcuts.
- Turned bare first-run setup into a resumable guided handoff: after initial
  configuration and key reconciliation it opens an observed 11-stage progress
  screen whose single primary action routes through reviewed Git commit,
  controller activation, PXE preparation, and the first network installation.
  Reopening setup skips completed configuration stages.
- Made the controller rebuild result actionable: the dashboard refreshes its
  reconciled status, collapses completed activity by default, and offers clear
  dashboard, detail, log, and new-review actions instead of leaving the
  administrator at an ambiguous terminal result.
- Made first-run network suggestions prefer the interface carrying the default
  route and its live DHCP address, explicitly excluding the controller's
  declarative static address; moved the US English locale to the first curated
  locale choice.
- Extended managed live feedback to first-run and routine controller apply:
  both fixed and revision-bound systemd units publish validation, build,
  activation, and verification progress. CLI JSON keeps stdout clean while the
  dashboard shows elapsed time, recent activity, and a Bubbles progress bar.
- Added live PXE preparation feedback to the dashboard using an official
  Bubbles progress bar, elapsed time, typed phases, counters, and five bounded
  recent activities. The systemd job atomically publishes a strict private
  progress record while verbose Nix output remains in journald.
- Introduced a task-oriented dashboard Settings area for Network, Computers,
  Accounts, Regional, Browser, Git, and Veyon edits. It reuses the typed
  Nix-backed plan/fingerprint apply boundary and provides a terminal-only,
  no-echo single-account password path whose review remains redacted.
- Reduced first-run configuration from 19 to 15 task-grouped steps by leaving
  optional Git author identity at its template defaults, and replaced raw time
  zone, locale, regional-format, desktop-keyboard, and console-keymap entry
  with offline searchable Bubbles selectors plus validated custom entry.
- Migrated the terminal frontend to the aligned Bubble Tea, Bubbles, and Lip
  Gloss v2 stack, adding reusable background-aware title, error, and
  width-adaptive key-help components as the foundation for richer setup and
  operation-progress screens.
- Made first-run credential entry recover from short, public-default, and
  mismatched passwords by retrying only the current account, preserving all
  earlier wizard input while keeping terminal and hashing failures fatal.
- Reworked the public README into a concise product overview and seven-step
  guided quick start, moving workstation defaults and `lib.mkLab` extension
  details into a dedicated system reference and linking the existing
  administrator, troubleshooting, hardware, architecture, and contributor
  guides instead of duplicating them inline. Reformatted the generated
  deployment administrator guide around a contents index, dashboard task map,
  short operation-specific sections, and a collapsible complete CLI reference.
- Bound managed PXE sessions to the unambiguous live controller address at
  preparation time and pass it to the offline installer at boot, so ordinary
  DHCP lease changes no longer require rebuilding the controller configuration.
- Made controller-apply reconciliation require an atomic, root-owned success
  receipt bound to both the reviewed Git revision and active NixOS closure, so
  late activation failures cannot be mistaken for completed setup.
- Split local validation into a fast default cycle, targeted VM modes, and an
  explicit full release matrix, with a reusable Nix evaluation cache.
- Documented the complete per-host customization workflow in both READMEs and
  in the Nixorium maintainer skill.
- Made `networkBase` a full IPv4 network address and added a configurable CIDR
  prefix, with static addresses calculated from validated host offsets.
- Split the former monolithic `common.nix` into focused desktop, package,
  power, screensaver, shell, and SSH modules.
- Made student-home reset fail closed when snapshots or cleanup are incomplete,
  and made display-manager startup require a successful reset.
- Replaced README parsing of Nix source with stable `labMeta` evaluations.
- Dedicated `nixorium-maintainer` to private laboratory operations and added a
  separate upstream-only `nixorium-developer` skill.
- Made the development-branch site template consume `master`; release
  preparation replaces it with the matching immutable tag.
- Moved Veyon network-object encoding from module evaluation into the
  `Veyon.conf` build and made CI reject import-from-derivation.

### Added

- A packaged `nixorium` Go command with a task-oriented terminal dashboard,
  human/JSON `status`, actionable `doctor` diagnostics, shared typed domain
  operations, and unit tests. Its first operational screen prepares, reviews,
  starts, stops, and recovers managed PXE installation mode; its deployment
  screen selects, reviews, confirms, and executes the same typed build-first
  client workflow as the CLI.
- Structured client hostname/IP inventory in `labMeta`, comprehensive
  read-only diagnostics for networking, Harmonia, PXE ports, SSH, Colmena and
  disk capacity, an explicit full controller-build check, and a NixOS VM test
  of the installed controller command.
- Explicit `nixorium hosts` text/JSON inventory and a dashboard Computers
  screen with bounded SSH probes and distinct reachable, unreachable, refused,
  and unknown observations, while the default status path remains probe-free.
  Managed generations now embed their private deployment revision and install
  a fixed read-only host-state helper; authenticated bounded SSH observation
  reconciles active system/revision against desired HEAD as current, outdated,
  or unknown without evaluating every client closure during inventory.
- Read-only `nixorium deploy plan --on` reports for one, selected, or all
  clients, bound to a clean Git revision and blocked by readiness, Git, or
  selector errors before any Colmena execution.
- Revision-bound `nixorium deploy apply` execution with exact target
  confirmation, repeated preflight checks, mandatory verbose Colmena build
  before apply, serialized runs, streamed mode-0600 operation logs, explicit
  partial-failure state, authenticated post-apply reconciliation, atomic
  per-host last-successful-verification history, and safe full-workflow retry.
- Routine `nixorium controller plan`/`controller apply` CLI and TUI rebuild
  workflow with exact confirmation, a narrowly revision-instanced systemd unit,
  pinned unprivileged Git builds, pre-activation drift refusal, and active-system
  verification.
- Typed `nixorium services` CLI/JSON and dashboard service management for the
  persistent signed cache and composite on-demand PXE lifecycle, plus an exact
  confirmed cache restart through a fixed capability-free systemd/polkit action
  with post-restart unit and HTTP verification.
- Typed `nixorium logs` list/detail CLI/JSON and dashboard browsing for private
  deployment logs, with basename-only selection, strict owner/mode/type and
  no-follow validation, 50-record/64-KiB bounds, terminal-control sanitization,
  and scrollable tail rendering. A locked atomic mode-0600 newest-1000 record
  adds safe typed summaries for configuration, key, controller, PXE, cache, and
  deployment outcomes without copying raw messages or deleting detailed logs.
- Read-only `nixorium git review` CLI/JSON and dashboard review of bounded
  staged/unstaged patches plus untracked paths, with managed/unexpected
  classification, disabled external diff drivers, settings-password redaction,
  terminal sanitization, and private-key-path refusal before patch capture.
- Optional `nixorium git commit plan`/`apply` CLI/JSON and TUI workflow with an
  explicit path allowlist, isolated HEAD-based proposal index, content-bound
  token, exact confirmation, generated message, file-type/secret/filter/size checks,
  atomic HEAD update, path-only index reconciliation, preserved unrelated
  changes, and no hooks, signing action, remote requirement, or implicit push.
- Reviewed `nixorium update plan`/`apply` CLI/JSON and TUI workflow for explicit
  SemVer releases, with prerelease/downgrade opt-ins, external candidate lock,
  readiness and representative no-link builds, bounded patch review, exact
  confirmation, token-bound two-file apply, and no implicit Git, activation,
  PXE, or deployment actions.
- Explicit read-only `nixorium update check` CLI/JSON release discovery against
  only the configured public GitHub upstream, with Git credentials and prompts
  disabled, strict timeout/output/result bounds, and separately sorted stable
  and prerelease tags while all routine and client paths remain offline-first.
- A task-oriented troubleshooting and recovery guide shipped both upstream and
  in every generated deployment, covering required failure symptoms, safe retry
  semantics, interrupted operations, private-key-aware backups, and restoration;
  quick validation keeps both copies identical.
- A reproducible manual VirtualBox and physical-hardware validation plan with
  isolated/offline topology, evidence records, destructive-disk warnings,
  firmware/NIC/storage coverage, PXE power-loss and DHCP coexistence recovery,
  single/multi-client deployment, and explicit pass/fail matrices.
- Immediate stderr-safe PXE preparation activity and fixed journald follow
  guidance in CLI/JSON, keeping machine stdout clean and verbose systemd-owned
  build output out of the presentation layer.
- A versioned `lab-settings.json` format for new private deployments, strict Go
  and Nix validation, deterministic atomic file writing, and the read-only
  `nixorium config validate` command. Existing `lab-config.nix` deployments
  remain compatible.
- A deterministic, read-only `nixorium setup status` reconciler that reports
  every first-run stage, selects the earliest incomplete one from observed
  settings, credentials, key files, artifacts, and deployment readiness, and
  marks the final installation offer available only with a current PXE
  preparation.
- A secure password-collection backend with terminal-only no-echo input,
  confirmation, default/length checks, best-effort memory wiping, and
  stdin-only salted SHA-512 hashing through the packaged `mkpasswd` tool.
- Idempotent `nixorium setup keys` reconciliation for Harmonia, SSH, and Veyon
  pairs, with create-new writes, private-mode enforcement, cryptographic
  correspondence checks, retry safety, and overwrite refusal.
- A non-secret `config plan` and fingerprint-bound `config apply` protocol
  that validates candidates through the deployment Flake, rejects concurrent
  edits, and atomically updates only `lab-settings.json`.
- A guided `nixorium setup` terminal form with detected network defaults,
  backward navigation, no-echo password hashing, redacted final review,
  explicit acceptance, and Git-aware classification of existing changes.
- A fixed-path, systemd-sandboxed `setup install-secrets` action with
  unit-specific wheel polkit authorization, key-pair re-verification,
  least-privilege destinations, idempotent reuse, and mismatch refusal.
- A confirmed `setup apply` workflow with clean-Git/readiness/key preflights,
  a fixed systemd/polkit action, unprivileged controller build, exact-closure
  activation, durable journald failures, and observed active-generation state.
- A declarative controller-only Harmonia lifecycle with systemd credential
  loading from the non-store installed key, restart/watchdog behavior, a
  stable `nixorium-harmonia.service` alias, health diagnostics, and VM-tested
  missing-key recovery.
- A non-disruptive `nixorium pxe prepare` workflow with clean-Git, live-DHCP,
  and Harmonia readiness gates; a capability-free administrator systemd job;
  pinned firmware and all-client builds; managed GC retention; and an atomic,
  revision-bound manifest of strictly validated immutable Nix store outputs
  used by status and PXE.
- An internal, controller-only PXE network unit with a root-owned
  session-before-mutation record, narrowly bounded `CAP_NET_ADMIN`, exact
  static-address rollback, failure-safe stop behavior, and boot/explicit
  recovery that reconciles recorded state against the live interface.
- A systemd-owned, controller-only PXE listener that strictly reconciles the
  prepared revision, active network session, and live addresses before serving
  ProxyDHCP, TFTP, and HTTP; publishes readiness only after a health check; and
  drops network children to separate unprivileged identities.
- Confirmed `nixorium pxe start`, idempotent `pxe stop`, and explicit `pxe
  recover` operations with exact polkit controls, typed lifecycle status,
  repeated readiness checks, and synchronous rollback after startup failure.
- Guided PXE client enrollment with embedded versioned host inventory,
  hardware and writable-disk display, constrained identity selection,
  best-effort duplicate detection, hostname-and-disk destructive confirmation,
  target revalidation, offline closure and capacity preflight, a precompiled
  offline Disko action, explicit progress/failure results, and confirmed reboot.
- The accepted management-system architecture and ADRs for the terminal-first
  interface, Go/Bubble Tea implementation, structured deployment settings,
  narrow privilege boundary, systemd-owned runtime services, interface-scoped
  firewall, local guided client-enrollment trust model, bounded private
  operation records, bounded read-only Git review, and reviewed local Git
  commits.
- An interactive controller-bootstrap version selector offering `master`, the
  latest GitHub prerelease, and published stable releases while preserving
  `--release` and `NIXORIUM_RELEASE` for unattended installations.
- Semantic validation for IPv4 networks, CIDR capacity, interfaces, users,
  password hashes, URLs, per-host modules, and Veyon pilot hosts.
- A `deploymentStatus` output for placeholders, missing public keys, and public
  default password hashes.
- Configuration-schema tests, targeted GitHub evaluation of one client and
  other representative outputs plus a fresh template, and a separate local
  matrix for builds, netboot, and offline installer equivalence.

### Fixed

- Serialized template-owned catalog, client-group, and home-reset policy as
  JSON files in the standalone offline installer instead of embedding JSON
  objects as invalid Nix expressions.

- Ensured a live controller activation reconciles and verifies the configured
  static laboratory address before reporting success. The first computer
  installation can now proceed directly to PXE after controller bootstrap,
  without requiring an extra reboot.

- Restored the first-run guidance in the compatibility settings command and
  aligned the management VM with the current installation flow and advanced
  key-import navigation.

- Allowed the fixed controller activation units to update declared user homes
  and `/run/user`, while retaining the reviewed private deployment as an
  explicit read-only mount. This prevents valid NixOS activation scripts from
  failing under `ProtectHome`.

### Security

- Enabled the NixOS firewall on every installed host with role-specific ports
  limited to the configured interface, including controller-only Harmonia/PXE
  rules and no implicit global OpenSSH or Avahi openings.
- Private deployments are evaluated with the local Git Flake fetcher, keeping
  ignored Harmonia, SSH, and Veyon private keys out of Nix source/store copies.
- The controller bootstrap now runs the Disko revision pinned by the generated
  deployment instead of fetching a mutable upstream revision.
- SSH now records keys on first connection and rejects later key changes;
  Colmena uses the same `accept-new` policy.

### Breaking

- Deployment configurations must change `networkBase` from three octets such
  as `10.0.0` to a full network address such as `10.0.0.0` and add
  `networkPrefixLength`. The configuration and `labMeta` schema version is 2.

## [2.0.0-beta.3] - 2026-09-08

### Added

- A Raw GitHub `install.sh` entrypoint that selects a tagged release, prepares
  the private deployment Git repository, and installs the controller from it.

### Changed

- Rewrote the deployment upgrade guide with an explicit release example,
  current Nix Flake commands, complete validation, and the Git merge workflow.
- Completed the project-wide Nixorium rebrand across repositories, Flake
  inputs, installer identifiers, command names, desktop identifiers, and the
  Agent Skill.
- Adopted `nixorium.org` as the canonical website and installer entrypoint,
  with Raw GitHub documented as the bootstrap fallback.

### Breaking

- Private deployments must rename their upstream input to `nixorium` and use
  `github:giovantenne/nixorium/<release>` when moving to this release.
- Installer environment variables now use the `NIXORIUM_` prefix.

## [2.0.0-beta.2] - 2026-09-07

### Added

- A reusable `lib.mkLab` Flake API with typed lab configuration validation.
- A `site` Flake template for private per-lab deployment repositories.
- Extension points for shared, controller, client, host-specific and netboot modules.
- Configurable logo, backgrounds, MIME defaults, VS Code settings and public key paths.
- A standalone netboot installer bundle containing the effective downstream configuration.
- A portable repository-maintainer Agent Skill, preinstalled by the site template for Codex, OpenCode, Claude Code and Pi.

### Changed

- Operational Harmonia and PXE helpers are exposed as Flake apps for downstream repositories.
- Lab-specific configuration can now update the upstream through a pinned Flake input instead of Git merges.
- The upstream screensaver logo is generic and site-specific printer drivers are delegated to deployment modules.

## [2.0.0-beta.1] - 2026-09-04

### Added

- Per-user rootless Docker daemons with declarative subordinate UID/GID ranges.
- A writable `~/.local/npm` global prefix, available in every login session for tools such as Codex and Claude Code.
- An opt-in `veyonNativeHosts` canary for Veyon 4.11's PipeWire/XDG portal Wayland backend.
- Setuid wrappers for Veyon's authentication and Wayland input helpers.

### Changed

- Updated nixpkgs from NixOS 25.11 to 26.05 and refreshed all flake inputs.
- Updated Veyon from the local 4.10.0 derivation to its official 4.11.0 flake.
- Updated the GNOME Remote Desktop reconnect patch for GNOME 50's connection throttler.
- Switched development packages to the current stable Node.js and PHP aliases.
- Migrated the controller sleep policy to NixOS 26.05's structured systemd settings.
- Disabled automatic ZFS root-pool imports because the shared Disko layout uses Btrfs.
- Excluded Docker data, the npm cache, and global npm tools from student home snapshots.

### Security

- Removed every normal user from the root-equivalent `docker` group.

### Breaking

- Docker now uses a per-user rootless socket and storage. Existing rootful images and containers are not migrated.

## [1.0.0] - 2026-09-04

### Added

- Declarative generation of controller and student workstations with Nix Flakes.
- UEFI installation and Btrfs partitioning through Disko.
- Offline client installation through ProxyDHCP, TFTP, HTTP netboot, and a local Harmonia cache.
- Multi-host deployment through Colmena.
- Parameterized lab settings and a public `labMeta` flake output for operational scripts.
- GNOME Wayland student desktop with development tools and classroom defaults.
- Boot-time student home reset with five recoverable Btrfs snapshots.
- Veyon classroom management with pre-generated lab topology and GNOME Remote Desktop integration.

### Security

- Key-only SSH access and immutable declarative users.
- Separate public and private material for SSH, Harmonia, and Veyon.

[Unreleased]: https://github.com/giovantenne/nixorium/compare/v3.9.0...HEAD
[3.9.0]: https://github.com/giovantenne/nixorium/compare/v3.8.2...v3.9.0
[3.8.2]: https://github.com/giovantenne/nixorium/compare/v3.8.1...v3.8.2
[3.8.1]: https://github.com/giovantenne/nixorium/compare/v3.8.0...v3.8.1
[3.8.0]: https://github.com/giovantenne/nixorium/compare/v3.7.0...v3.8.0
[3.7.0]: https://github.com/giovantenne/nixorium/compare/v3.6.1...v3.7.0
[3.6.1]: https://github.com/giovantenne/nixorium/compare/v3.6.0...v3.6.1
[3.6.0]: https://github.com/giovantenne/nixorium/compare/v3.5.0...v3.6.0
[3.5.0]: https://github.com/giovantenne/nixorium/compare/v3.4.0...v3.5.0
[3.4.0]: https://github.com/giovantenne/nixorium/compare/v3.3.1...v3.4.0
[3.3.1]: https://github.com/giovantenne/nixorium/compare/v3.3.0...v3.3.1
[3.3.0]: https://github.com/giovantenne/nixorium/compare/v3.2.3...v3.3.0
[3.2.3]: https://github.com/giovantenne/nixorium/compare/v3.2.2...v3.2.3
[3.2.2]: https://github.com/giovantenne/nixorium/compare/v3.2.1...v3.2.2
[3.2.1]: https://github.com/giovantenne/nixorium/compare/v3.2.0...v3.2.1
[3.2.0]: https://github.com/giovantenne/nixorium/compare/v3.1.1...v3.2.0
[3.1.1]: https://github.com/giovantenne/nixorium/compare/v3.1.0...v3.1.1
[3.1.0]: https://github.com/giovantenne/nixorium/compare/v3.0.0...v3.1.0
[3.0.0]: https://github.com/giovantenne/nixorium/compare/v3.0.0-beta.1...v3.0.0
[3.0.0-beta.1]: https://github.com/giovantenne/nixorium/compare/v2.1.0...v3.0.0-beta.1
[2.1.0]: https://github.com/giovantenne/nixorium/compare/v2.0.0...v2.1.0
[2.0.0]: https://github.com/giovantenne/nixorium/compare/v2.0.0-beta.5...v2.0.0
[2.0.0-beta.5]: https://github.com/giovantenne/nixorium/compare/v2.0.0-beta.4...v2.0.0-beta.5
[2.0.0-beta.4]: https://github.com/giovantenne/nixorium/compare/v2.0.0-beta.3...v2.0.0-beta.4
[2.0.0-beta.3]: https://github.com/giovantenne/nixorium/compare/v2.0.0-beta.2...v2.0.0-beta.3
[2.0.0-beta.2]: https://github.com/giovantenne/nixorium/compare/v2.0.0-beta.1...v2.0.0-beta.2
[2.0.0-beta.1]: https://github.com/giovantenne/nixorium/compare/v1.0.0...v2.0.0-beta.1
[1.0.0]: https://github.com/giovantenne/nixorium/releases/tag/v1.0.0
