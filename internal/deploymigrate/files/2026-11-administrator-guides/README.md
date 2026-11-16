# Nixorium deployment

This private repository holds the configuration of one laboratory: its
settings, software, public keys and any customisation of your own. On the
controller it is `~/nixorium-deployment` in the administrator's home. The
Nixorium version it uses is recorded here too and changed from the menu.

> [!IMPORTANT]
> Keep this repository private. It contains internal network details and an
> encrypted recovery file. Keep `secret-key`, `admin-ssh` and
> `lab-credentials.json` outside Git (the `.gitignore` already does).

## Guides

| Guide | What it covers |
|---|---|
| [Classroom](CLASSROOM.md) | During a lesson: classroom view, Internet, locked screens, messages, sending, collecting and recovering files, switching computers on and off |
| [Installation](INSTALLATION.md) | Requirements, installing student computers by network boot or USB, dual boot |
| [Software](SOFTWARE.md) | Applications, software profiles, the student desktop, initial files, modules of your own |
| [Maintenance](MAINTENANCE.md) | Settings and passwords, updating computers, the controller, a new network, undo, backup, disk space, logs |
| [Updates](UPDATES.md) | Updating Nixorium, NixOS and applications |
| [Troubleshooting](TROUBLESHOOTING.md) | When something stops: messages, codes and recovery |
| [Template reset](DEPLOYMENT-RESET.md) | Replacing your customisation with the current template |

## Open Nixorium

On the controller, the administrator and the teacher open **Nixorium** from
the application grid, or type `nixorium` in a terminal.

- The **administrator** (`admin`) sees the whole menu.
- The **teacher** sees only the classroom controls and never reads this
  repository. When something blocks them, the screen says whether to try again
  later or to ask the administrator, with a short code explained in
  [Troubleshooting](TROUBLESHOOTING.md#codes-shown-to-the-teacher).

The first time, the administrator acknowledges the operational notice (Enter)
and is asked about the optional [adoption statistics](MAINTENANCE.md#adoption-statistics):
`e` shares them, `n` declines, Esc decides later.

The administrator's console and SSH logins say when something is waiting, such
as an interrupted update or an unfinished USB installation. The window stays
open after an error so you can read it.

## After installing the controller

1. Sign in as `admin` and open Nixorium. The controller is already usable.
2. Open **Installation** and choose **Network boot (PXE)** or **USB over SSH**
   to install the student computers. See [Installation](INSTALLATION.md).
3. Add applications in **Software** ([Software](SOFTWARE.md)).
4. After any saved change, apply it to the controller and update the
   computers ([Maintenance](MAINTENANCE.md#update-computers)).
5. Back up the laboratory: **Maintenance → Back up lab**
   ([Maintenance](MAINTENANCE.md#back-up-and-restore)).

Use a released Nixorium version for daily work, not the development branch:
**Maintenance → Update Nixorium** shows which one is in use.

## The menu

| Area | What you do there |
|---|---|
| **Computers** | See the computers, update them, Internet, locked screens, files, power |
| **Installation** | Install computers by network boot (PXE) or USB, recover an interrupted installation |
| **Software** | Add, move or remove applications, add a software profile |
| **Maintenance** | Settings, accounts and passwords, update Nixorium, backup, diagnostics, logs; less frequent tasks under **Advanced** at the bottom |
| **Assistant** | Optional: describe a change in words and review it before saving |

Nixorium opens on the **Overview**, which lists only what needs attention. The
screens call the student computers *clients*.

Keys:

- `Up`/`Down` and Enter choose; `Esc` goes back; `q` quits when nothing needs
  attention.
- `?` opens help, `F1` also while typing; `r` refreshes what the screen shows.
- Each screen lists the keys that act at the bottom.
- During a long operation, `l` shows its details.

Opening Nixorium never contacts the student computers and never treats a
switched-off computer as a problem. **Computers → Computer inventory** checks
them when you ask.

## Where to find what

| I want to… | Menu | Guide |
|---|---|---|
| Watch or control the students' screens | Computers → Classroom view | [Classroom](CLASSROOM.md#classroom-view) |
| Block the Internet during a test | Computers → Internet access | [Classroom](CLASSROOM.md#internet-access) |
| Give a student back a lost file | Computers → Recover student files | [Classroom](CLASSROOM.md#recover-student-files) |
| Switch computers on in the morning | Classroom view → Power → Schedule… | [Classroom](CLASSROOM.md#schedule) |
| Install a new computer | Installation | [Installation](INSTALLATION.md) |
| Add an application | Software | [Software](SOFTWARE.md#add-or-change-software) |
| Change the students' dock or browser | Maintenance → Change settings → Student workspace | [Software](SOFTWARE.md#student-workspace) |
| Change a password | Maintenance → Change settings → Accounts | [Maintenance](MAINTENANCE.md#change-settings) |
| Bring the computers to the saved configuration | Computers → Update computers | [Maintenance](MAINTENANCE.md#update-computers) |
| Go back after a change that caused trouble | Maintenance → Review Git changes, then `u` | [Maintenance](MAINTENANCE.md#undo-the-last-saved-change) |
| Move the laboratory to another network | Maintenance → Change network addressing | [Maintenance](MAINTENANCE.md#change-the-laboratory-network) |
| Update Nixorium | Maintenance → Update Nixorium | [Updates](UPDATES.md) |
| Prepare a report for support | Maintenance → Diagnostics → Support report | [Maintenance](MAINTENANCE.md#logs-and-support-report) |

## Command line

Every task in the menu also exists as a command, for scripts and remote work.
Commands that change something work in two steps:

```sh
nixorium deploy plan --on @lab                     # shows the review and its token
nixorium deploy apply --on @lab --expect <token>   # does exactly what was reviewed
```

`plan` changes nothing and prints the exact `apply` command to copy. `apply`
repeats the checks, refuses when anything changed since the review and asks
`Continue? [y/N]`; `--yes` skips the question but still checks the token. Add `--json` for output a
script can read. `--on` takes computer names (`pc01,pc02`) or `@lab` for all
student computers. Run `nixorium --help` for the list. Before the controller's
first apply, run the same commands as `nix run .#nixorium -- …` from this
repository. Each guide shows the commands of its tasks.

## Files in this repository

| File | What it holds |
|---|---|
| `lab-settings.json` | Laboratory settings; change them in **Maintenance → Change settings** |
| `lab-software.json` | Applications and where they are installed; change them in **Software** |
| `software-presets.json`, `software-catalog.nix` | The software profiles and the suggestions shown before a search |
| `workspace-profile.json`, `workspace-catalog.nix` | The student desktop, dock, browser and editor |
| `home-files.json`, `home-files/` | Files every student finds in a fresh home |
| `modules/` | NixOS modules of your own: `shared.nix` (every computer), `controller.nix`, `clients.nix`, and the workstation policy |
| `flake.nix`, `flake.lock` | The pinned Nixorium version and package base, client groups and per-computer modules |
| `assets/` | The screensaver logo, wallpapers and editor defaults |
| `keys/` | Public keys only |
| `nixorium-recovery.age` | Private keys and password hashes, encrypted with the backup passphrase |

Settings, software and the student workspace are saved through their screens,
which check the change and record it in Git. Editing modules is described in
[Software](SOFTWARE.md#modules-of-your-own).

## Assistant

The optional Assistant service turns a request written in words into a
proposed change, which you review and save like any other; it never applies or
updates computers by itself. It needs a subscription or the free trial and a
separate consent. Read the
[Assistant guide](https://github.com/giovantenne/nixorium/blob/master/docs/assistant.md)
for privacy, what it can read, retention and recovery. To hide the menu, set
`lab.assistant` to `false` in `lab-settings.json` and apply to the controller.

## AI agents

This repository includes the `nixorium-maintainer` skill, with discovery links
for Codex, OpenCode, Claude Code and Pi. With it, an agent can look at the
laboratory and propose software or student-desktop changes, but it does not
edit this repository, save proposals or operate the computers: you make every
change through the reviews above. These instructions are not a sandbox: do not
give an agent root or unrestricted access and expect them to prevent harm. The
procedures in these guides are for the administrator, not permission for an
agent. Updating Nixorium does not refresh the copied skill; a
[template reset](DEPLOYMENT-RESET.md) does.
