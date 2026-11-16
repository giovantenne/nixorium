# Software

Applications, the student desktop and the files students start with. Every
change here is saved and recorded in Git, then reaches the computers through
the usual steps: apply to the controller, then update the computers.

## Add or change software

Open **Software**:

- **Configured** (`F2`) lists the applications added here. Enter on one shows
  its details: `c` changes where it is installed, `x` removes it.
- **Search** (`F3`) searches the packages of the NixOS version this laboratory uses;
  **Suggested** (`F4`) is a short list to start from. Any package of the set
  can be added.

Then choose where the application goes:

| On the screen | Installed on | `--scope` |
|---|---|---|
| This controller and all clients | The controller and every student computer, including future ones | `shared` |
| Only this controller | The controller | `controller` |
| All clients | Every student computer, including future ones, not the controller | `all-clients` |
| Group | The student computers of a group in `flake.nix` (`clientGroups`) | `group:<name>` |
| Selected clients | The computers you choose | `clients:pc01,pc04` |

The review shows every computer affected, including the old ones when you
move an application. Saving an application that reaches the controller also
applies it to the controller straight away. After a save that affects student
computers, **Update affected computers** opens the update review with them
already selected (it applies the whole saved configuration, not only that
application); **Later** leaves them for now. Student computers can all be
off while you save.

Chromium, Ghostty, Git and the screensaver's effects are always installed and
cannot be removed. Packages added by modules of your own are not shown here
and are changed in the modules.

**Check systems** in Software shows which computers already run the saved
configuration, checking them now.

```sh
nixorium software search --query libreoffice
nixorium software plan --package vlc --scope all-clients
nixorium software apply --package vlc --scope all-clients --expect <token>
nixorium software plan --package gimp --scope group:graphics
nixorium software plan --package hello --scope clients:pc01,pc04
nixorium software plan --package vlc --scope all-clients --remove
```

## Software profiles

A profile is a ready list of applications to add in one step. Press `p` in
**Software**, choose a profile, untick with Space what you do not want, choose
where it goes and review. Adding a profile only adds: existing applications
keep their place and nothing is removed. Esc at any step changes nothing.

| Profile | Adds |
|---|---|
| **Essential** (the default) | A browser, terminal, Git, fonts, the screensaver and npm |
| **General education** | Documents, spelling, web and media playback |
| **Programming** | General education plus VS Code, C/C++, Java, Python and web/PHP, a local MySQL server, MySQL Workbench, the Pi and OpenCode coding agents |
| **Graphics and illustration** | General education plus image editing, vector drawing and digital painting |
| **Audio and video** | General education plus audio editing, screen recording and video editing |
| **CAD and 3D modelling** | General education plus parametric CAD, 3D modelling, animation and rendering |
| **STEM and scientific computing** | General education plus numerical, statistical, plotting and computer-algebra tools |

Every profile includes Git, Node.js and npm, and the screensaver. The profiles
and the suggestions are files of this repository (`software-presets.json`,
`software-catalog.nix`): you can edit them without waiting for a Nixorium
release.

```sh
nixorium software presets
nixorium software preset plan --preset programming --scope shared --exclude vlc
nixorium software preset apply --preset programming --scope shared --exclude vlc --expect <token>
```

### Notes on Programming

- **MySQL** is for exercises only: it accepts connections from the same
  computer, `root` has no password, and every database is erased at each
  start, like the student home. Keep work as exported SQL files.
- **Pi and OpenCode** are installed for everyone. The administrator or the
  teacher can try a newer version for their own account, with Internet:

  ```sh
  npm install -g @mariozechner/pi-coding-agent@latest opencode-ai@latest
  hash -r
  ```

  Removing it brings back the installed version. Students start each session
  without npm globals, settings, conversations or saved logins: they sign in
  again. Never put API keys in the student home.
- A local web server for PHP pages is not part of the profile; the Assistant
  can add one.

## The desktop

Every computer has the same desktop basics, whatever the profile: icons on the
desktop, a dock at the bottom that hides when a window covers it, and windows
that snap to the edges. On the controller the administrator's dock starts with
Nixorium and the teacher's with the classroom view.

Staff accounts get the managed appearance at their first login and can change
it afterwards. Students get it again at every start, with a wallpaper chosen at
random from `assets.backgrounds`. The screensaver animates `assets/logo.txt`
after 5 idle minutes, turns the screen black after 30, and never locks the
session.

## Student workspace

**Maintenance → Change settings → Student workspace** changes what students
find at each start, on the controller and every student computer:

- **Desktop**, **Dock** and **Browser** settings: change each, then review the
  whole proposal and save. "Inherit" keeps the default; "Clear" empties a list.
- **Initial files** (below).

Saving records only the workspace in Git, then offers the controller review.
Students see the change at their next start; a session in progress is not
reset.

The screen does not edit VS Code settings or extensions, because VS Code may
not be installed. Choosing **Programming** when installing (or in a template
reset) sets VS Code and MySQL Workbench in the dock, editor defaults and
extensions for web/PHP, C/C++, Python and Java. Adding Programming later from
Software does not change the workspace. To change the editor's extensions,
including one pinned from the Marketplace (the controller needs Internet at
that moment), ask the Assistant; test a new extension on one computer first.
Without the Assistant, review and save the Programming workspace that ships
with this repository (it replaces the current workspace):

```sh
nixorium workspace plan --file workspace-profile.programming.example.json
```

Extension versions come from the package base: **Maintenance → Update system
and packages** updates them, together with the editor and the system. Staff can
install other extensions themselves; programs that bundle generic Linux files
(such as the Java runtime of a Marketplace extension) start normally. Anything
a student installs in their home is lost at the next start: add it to the
profile or to the software list instead.

```sh
nixorium workspace plan --file <candidate.json>
nixorium workspace apply --file <candidate.json> --expect <token>
```

### Initial files

**Student workspace → Initial files** adds files or folders that every student
finds in a fresh home, on every computer, from the next start: worksheets, a
template project, a shortcut.

- Choose a file or folder on the controller and where it goes in the home.
  Nixorium copies it into this repository (`home-files/`), so later changes to
  the original are not followed; update it from the same screen.
- Students can change their copies; the original comes back at the next start.
- The files go into Git, the installation files and every computer: use
  nothing private.

```sh
nixorium home-files import plan --source <file-or-folder> --destination Documents/Lessons
nixorium home-files plan --file <candidate.json>
nixorium home-files apply --file <candidate.json> --expect <token>
```

### The student home

The student home is restored at every start from the workspace, the initial
files and the defaults (shell, Git, standard folders). Earlier sessions are
kept as read-only copies that the teacher can
[recover files from](CLASSROOM.md#recover-student-files); they are not
backups. If a computer reports that restoring the home did not finish, it
stops before the login screen and keeps the evidence: see
[A student computer stops before the login screen](TROUBLESHOOTING.md#a-student-computer-stops-before-the-login-screen).

## Modules of your own

For behaviour the screens do not offer, write NixOS modules in this repository:

| File | Applies to |
|---|---|
| `modules/shared.nix` | Every computer |
| `modules/controller.nix` | The controller |
| `modules/clients.nix` | Every student computer |
| `modules/workstation.nix` | Desktop icons, dock, application policy |
| `modules/development.nix` | Shell, npm, MySQL and rootless Docker policy |
| `modules/home-profile.nix` | Default applications and editor settings |

For a group of student computers, add a group and its modules in `flake.nix`:

```nix
clientGroups.course = [ "pc01" "pc02" ];
groupModules.course = [ ./modules/course.nix ];
```

For one computer, create `modules/pc05.nix`:

```nix
{ pkgs, ... }:
{
  environment.systemPackages = [ pkgs.vlc ];
}
```

and list it in `flake.nix`:

```nix
hostModules = {
  pc05 = [ ./modules/pc05.nix ];
};
```

Then check that it builds, commit it, apply to the controller and update that
computer:

```sh
git add flake.nix modules/pc05.nix
nix build .#nixosConfigurations.pc05.config.system.build.toplevel --no-link
git commit -m "pc05: add VLC"
nixorium deploy plan --on pc05
```

Things to know:

- Student computers with the same groups, software and network card share one
  system, built once. A module that uses `hostName` or `hostIp` must have its
  computer in `hostModules` (or in `updateValidationHosts` in `flake.nix`);
  otherwise the build stops with a message saying so.
- Make an application's policy depend on the application being installed, so
  that moving or removing it in Software leaves nothing behind.
- Keep every file a module uses inside this repository: the network installer
  carries this repository and installs without Internet.
- More wallpapers go in `flake.nix`:

  ```nix
  assets = {
    logo = ./assets/logo.txt;
    backgrounds = [ ./assets/backgrounds/one.jpg ./assets/backgrounds/two.jpg ];
  };
  ```
