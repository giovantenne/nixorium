# Maintenance

The administrator's tasks, in **Maintenance** and **Computers**. They follow
one pattern: **save** a change (recorded in Git, no computer changes), **apply
to the controller**, then **update the computers**. Each step has its own
review, confirmed with Enter. Free disk space, Apply to controller, Change
network addressing, Controller services, Review Git changes and Reset
deployment template are at the bottom of Maintenance, under **Advanced**.

## Change settings

**Maintenance → Change settings**:

| Entry | Changes |
|---|---|
| **Network** | Network cards of the controller and the student computers, laboratory network, controller DHCP address |
| **Computers** | Number of student computers, controller number |
| **Accounts** | Teacher and student names (`n`); passwords (`p`), one account at a time |
| **Regional** | Time zone and keyboard |
| **Browser** | Classroom homepage |
| **Classroom view** | On or off |
| **Git** | Name and email used in commits |
| **Student snapshots** | How many past sessions each student computer keeps (0–30) |
| **Student workspace** | See [Software](SOFTWARE.md#student-workspace) |
| `y` **Controller keys** | Import existing keys |

Each change ends in a review and is saved with Git history. Saving changes no
computer: apply to the controller, then update the computers. A new password
works only after both.

Before saving a network card name, the review checks it:

- **Controller card:** it must be a wired card the controller has.
- **Student computers' card:** every affected computer that answers is asked
  whether it has it. A missing card blocks the save until you press `c` to save
  anyway for the computers listed. Computers that do not answer are listed so
  you can check them at their screen.
 After a network change, apply to the
controller, then update **one** computer and check that it answers before the
others. The rules for card names are in
[Installation](INSTALLATION.md#before-installing-student-computers).

The controller DHCP address is only used to prepare network installation;
choose it from the controller's current addresses, or type another.

The desktop language (`defaultLocale`, `extraLocale` in `lab-settings.json`)
and a network card for a single computer (`hostIfaceNames`) have no screen:
change them with the commands below.

```sh
nixorium config validate
nixorium config plan --file candidate.json
nixorium config apply --file candidate.json --expect <fingerprint>
```

## Apply to the controller

**Maintenance → Apply to controller** makes the controller run the saved
configuration. The review shows what changes and whether a restart is needed.
The work runs as a system service: closing Nixorium does not stop it. A
software change that reaches the controller, and an update of Nixorium, apply
to the controller by themselves after saving.

```sh
nixorium controller plan
nixorium controller apply --expect <revision>
```

## Update computers

**Computers → Update computers** brings the student computers to the saved
configuration.

1. Select computers. The list shows each one's state at the last check; `r`
   checks again, `n` selects those that need the update.
2. Review and press Enter. Nixorium builds the systems first, then updates the
   computers, several at a time.
3. The result says, for each computer, whether it was updated and checked.
   `l` shows the details during the update; the full output is in the log.
   `s` stops waiting for the result without stopping the update on the
   computers.

After a failed or partial update, look at the result and at **Computer
inventory**, then make a new review: some computers may already be updated.
If the outcome of a computer is unknown (for example the controller lost power
during the update), Nixorium keeps a record and asks for a
[reviewed recovery](TROUBLESHOOTING.md#interrupted-client-deployment) before
the next update.

```sh
nixorium deploy plan --on @lab
nixorium deploy apply --on @lab --expect <revision>
```

### Computers that are off

When selected computers do not answer, the review offers to update them when
they are switched on (on by default; `F3` turns it off). The others are
updated now. The controller checks every minute and updates the waiting
computers that answer. **Computers → Queued updates** lists them and removes
entries. A waiting update is never applied once the configuration has changed;
the next **Update computers** selects those computers for you.

```sh
nixorium deploy apply --on @lab --expect <revision> --queue-unreachable
nixorium deploy queue status
nixorium deploy queue cancel --on pc02
```

### What the students see

Once the controller has checked an updated computer, the person at it sees a
notification. Usually it says the update is in place and that programs opened
from now on use it. When the kernel or systemd changed, it asks for a restart,
with a **Restart** button. A computer never restarts on its own. With the
classroom view turned off there is no notification.

### Computer inventory

**Computers → Computer inventory** checks every computer now: `current` (runs
the saved configuration), `outdated` (runs another one) or `unknown` (could not
be checked). It also shows the last successful update. `/` searches, Enter
shows a computer, `i` its diagnostics, `u` updates the computers that need it.

```sh
nixorium hosts
```

## Change the laboratory network

**Maintenance → Change network addressing** moves the controller and every
computer to a new laboratory network, for example from `10.0.0.0/24` to
`10.1.0.0/24`. Every computer keeps its number. Until you finish, the
controller answers on both networks and each computer keeps its old address
next to the new one, so nothing loses contact.

Before you start: finish or stop network installation, resolve what the
Overview lists as blocking, save pending changes, and switch the computers on.

1. **Review and save.** Type the new network. The review lists every address
   before and after and checks that every computer answers. A computer that is
   off blocks it until you switch it on or press `c` to continue without it
   (it moves at its next update). Saving changes no computer.
2. **Apply to controller.** The controller now has both addresses.
3. **Update computers.** Each computer is reached at its old address, moves
   and is checked at the new one. Computers that are off can wait in the queue.

While the change is in progress, the Overview shows **The laboratory network
is being changed**; open it to see each computer and the next step.

- **Finish** (`f`) appears once every computer runs the saved configuration at
  its new address. Save it, apply to the controller (which drops the old
  address) and update the computers. Then prepare network installation again
  before installing by PXE.
- **Undo** (`u`) is possible until you finish: it saves the old network again.
  Update the computers first, then apply to the controller.
- A computer **still on the previous network**: update it. If it already runs
  the saved configuration, restart it. A computer that answers on neither
  address is off or unplugged.

```sh
nixorium network migrate plan --to 10.1.0.0/24
nixorium network migrate start --to 10.1.0.0/24 --expect <token>
nixorium network migrate status
nixorium network migrate finish   # prints the review; repeat with --expect <token> to save
nixorium network migrate undo     # likewise
```

## Undo the last saved change

When the last change you saved (settings, software, the student workspace)
causes trouble, **Maintenance → Review Git changes**, then `u`. The review
shows the change and what the undo does. Enter saves a new change named
`Undo: …` that puts the files back; the old change stays in the history. Then
apply to the controller and update the computers, as after any save: Enter on
the result opens the controller review.

Only the last change can be undone, once. Save or discard other unsaved
changes first. Nixorium refuses, and says what to do instead, when the last
change is:

- the first version, a merge or already an undo;
- an update of Nixorium or of the package base (see
  [Updates](UPDATES.md#recovery-and-long-term-autonomy));
- keys or the recovery file;
- part of a network change.

```sh
nixorium git undo plan
nixorium git undo apply --expect <token>
```

## Review Git changes

**Maintenance → Review Git changes** shows what changed in this repository and
is not yet saved, without ever showing private keys or password hashes. You can
save selected files as a commit. Nixorium never pushes, except to the Git
backup below.

```sh
nixorium git review
nixorium git commit plan --paths lab-settings.json
nixorium git commit apply --paths lab-settings.json --expect <token>
```

## Back up and restore

Backup is optional and never blocks installation or updates. **Maintenance →
Back up lab**:

- **Encrypted file.** Choose a folder (for a USB stick, mount it and type its
  folder), enter a passphrase twice, review. Everything is encrypted: the
  configuration, its history, the private keys and the password hashes.
- **Private Git repository.** Create an empty **private** repository on GitHub,
  GitLab or another Git server over SSH, and add the backup key Nixorium shows
  with write access. Enter its address and branch and a passphrase. For a
  server other than GitHub or GitLab.com, compare and trust its fingerprint
  when asked. The configuration and its history stay readable there; keys and
  password hashes are encrypted. From then on every saved change is sent
  automatically while Nixorium is open; the passphrase is asked again only
  when keys, passwords or trusted computers change.

Use a passphrase of at least 12 characters and keep it away from the
controller. Without it, the keys cannot be recovered.

**Maintenance → Restore lab** restores a file or a Git backup into a new
folder, checks the keys and the trusted computers, and never overwrites the
current laboratory or applies anything. For a new or replaced controller,
**Use this lab on this controller** then puts it in place (the old folder is
kept), and you apply to the controller. Without an existing laboratory,
`nixorium backup clone` opens the same screen. See
[Backups and restoration](TROUBLESHOOTING.md#backups-and-restoration).

```sh
nixorium backup key                 # shows the key to add to the Git server
nixorium backup clone
```

## Free disk space

Every apply and every update adds a system version, and nothing removes old
ones by itself. Maintenance warns when the controller's disk is low.
**Maintenance → Free disk space**: select the computers and review. Each keeps
its newest 10 versions, the one it runs and the one it started with; older
versions disappear from the boot menu. Type `CLEAN` to confirm. Homes, settings and
installation files are never touched.

```sh
nixorium cleanup plan --on controller,@lab
nixorium cleanup apply --on controller,@lab --expect <token>
```

## Controller services

**Maintenance → Controller services** shows the software cache that feeds the
student computers and network installation. Restart the cache only when
troubleshooting asks for it.

```sh
nixorium services
nixorium services restart cache
```

## Logs and support report

- **Maintenance → View operation logs** lists recent operations and the end of
  each update log (`nixorium logs`, `nixorium logs show <id>`).
- **Maintenance → Diagnostics** checks the laboratory and shows what to do
  (`nixorium doctor`; `nixorium recovery status` for what blocks operations).
- **Maintenance → Diagnostics → Support report** prepares a short report to
  share with support. It is only saved locally: read it before sending it.
  Detailed logs are not safe to share as they are.

```sh
nixorium support preview --json
nixorium support export
```

## Update Nixorium

**Maintenance → Update Nixorium** and **Update system and packages**: see
[Updates](UPDATES.md). Nixorium checks for new releases at most once a day
while the administrator has it open and shows a notice on the Overview.

<details>
<summary>Manual update, when the guided update refuses the input</summary>

Only for a `flake.nix` whose Nixorium input the guided update cannot read.
Replace `v3.5.0` with the release you want:

```sh
git switch -c upgrade/nixorium-v3.5.0
# in flake.nix: inputs.nixorium.url = "github:giovantenne/nixorium/v3.5.0";
nix flake update nixorium
git diff -- flake.nix flake.lock
nix build .#nixosConfigurations.pc01.config.system.build.toplevel --no-link
nix build ".#nixosConfigurations.$(nix eval .#labMeta.controller.name --raw --no-write-lock-file).config.system.build.toplevel" --no-link
nix build .#nixosConfigurations.netboot.config.system.build.netbootRamdisk --no-link
nix build .#pxeFirmware --no-link
nix build .#installerBundle --no-link
git add flake.nix flake.lock
git commit -m "chore: update nixorium to v3.5.0"
git switch master
git merge --ff-only upgrade/nixorium-v3.5.0
```

Then apply to the controller and update the computers.

</details>

## Template reset

Updating Nixorium does not refresh the files this repository received from
the template (modules, profiles, guides, the agent skill).
**Maintenance → Reset deployment template** replaces them from the version in
use: your modules, the software list, the student workspace and the assets go
back to the template. Settings, keys and the Nixorium version are kept, and a
backup is made first. You confirm by typing `RESET DEPLOYMENT`. Read
[Template reset](DEPLOYMENT-RESET.md) first.

## Adoption statistics

**Maintenance → Adoption statistics** shows the small daily report and lets you
share it or not. It is off until you choose, never needed to use the
laboratory, and sent only by the controller. It contains a monthly pseudonym,
the Nixorium version and mode, a range of the number of computers and whether
a student computer has ever started; no names, users, files, configuration or
logs. The receiving service sees the connection address, so the statistics are
pseudonymous, not anonymous. Reports are kept for 90 days, monthly totals for
24 months. Turning it off stops sending and removes the local identity; it
does not erase what was already received.

```sh
nixorium telemetry preview --json
nixorium telemetry enable
nixorium telemetry disable
```
