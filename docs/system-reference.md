# Nixorium system and extension reference

This document describes the core workstation mechanisms and supported extension
points provided by the public Nixorium framework. The generated private
deployment owns the actual workstation profile. Day-to-day commands belong in
the [deployment administrator guide](../templates/site/README.md); architecture
and privilege boundaries belong in the
[management architecture](management-architecture.md).

## Accounts

| Account | Default role |
|---|---|
| `admin` | System administrator, SSH access, sudo, and the classroom view |
| Teacher account | Persistent instructor workspace, the classroom view, and restricted classroom controls |
| Student account | Client autologin, reset home, and read-only host networking |
| `root` | Disabled password and key-only SSH access |

Teacher and student names are site settings. Accounts are declarative and
`users.mutableUsers = false`; changing credentials therefore requires a
reviewed configuration update rather than an imperative password edit.

Admin and teacher belong to the `networkmanager` group. The student does not,
and an explicit polkit rule denies every `org.freedesktop.NetworkManager.*`
action for that identity. The session can use the system-managed connection and
inspect ordinary network status, but cannot change connections, radios, DNS, or
other NetworkManager state through GNOME, `nmcli`, `nmtui`, or direct D-Bus
requests.

On the controller, `admin` owns the mode-0700 private deployment and the SSH
identity used to manage clients. The teacher instead belongs to the
`nixorium-classroom` group. Running `nixorium` without a readable deployment
connects to a group-private local worker and opens only the classroom view,
inventory, temporary Internet control, screen locking, sending files,
switching computers on and reviewed client shutdown/restart. The worker runs as
`admin`, validates every target against the fixed deployment, uses only fixed
SSH helper/systemd commands, and shares the normal client-operation lock.
Student accounts cannot traverse the runtime directory or connect to its
socket. The worker does not expose settings, software, Git, installation,
deployment, update, log, or controller-maintenance operations.

## Storage and boot

Nixorium requires UEFI boot. Disko creates an EFI System Partition mounted at
`/boot` and one Btrfs partition containing:

| Subvolume | Mount point |
|---|---|
| `@root` | `/` |
| `@home-<studentUser>` | `/home/<studentUser>` |
| `@snapshots` | `/var/lib/home-snapshots` |

The same pure Disko layout is used by controller bootstrap and guided client
installation, so destructive tooling comes from the deployment's locked inputs
instead of being downloaded at installation time.

The public controller bootstrap resolves its selected branch or tag once. Both
bootstrap stages use only the official NixOS binary cache and its published
signing key; signature checking is never disabled. The
template, installer, local Disko layout, and initial lock use that full Git
revision, while `flake.nix` retains the selected channel for later managed
updates. The initial lock pins nixpkgs to the revision that release was
validated with (`lib.packageBase.referenceRevision`), not the newest commit of
its channel; package-base updates move it forward later. Resolution failure
stops before the installer is invoked. The small shell launcher collects
account and regional input before it invokes Nix, then selects and reviews the
initial deployment-owned software profile from the same immutable template
revision. It writes only ordinary `shared` declarations for the complete
selected profile to `lab-software.json` before Git initialization, Nix
evaluation, or disk changes. It refuses releases without bootstrap capability
version 2. The supported
bootstrap environment is the official NixOS Minimal ISO (Linux text console)
or Graphical ISO (GNOME session) booted in UEFI mode. Keyboard is chosen from a numbered list and
applied first (`loadkeys` on the console, `gsettings` in GNOME), then time zone
is collected before account details, so all subsequent input uses the installed
controller layout. Installed systems and the PXE environment derive the text
console from the same XKB layout (`console.useXkbConfig`); `lab.consoleKeyMap`
is deprecated and ignored. The
destructive confirmation precedes partitioning, locking, or
controller-system transfer. Once Disko has mounted the target, the bootstrap
places its evaluation cache and temporary swap there; `nixos-install` builds
directly into the target store with one Nix job and one core per build.

## Regional settings

The guided settings forms offer time zone and keyboard selectors. Desktop
locale is configured separately with `defaultLocale` and `extraLocale` in
`lab-settings.json`, using a complete candidate and the reviewed `config plan`
and `config apply` workflow. Bootstrap starts with `en_US.UTF-8`.

## Package-base ownership

New deployments own a direct `nixpkgs` pin on the channel advertised by
`nixorium.lib.packageBase`. Nixorium's transitive consumers follow it, and the
deployment exposes `nixoriumPackageBase` with its locked revision. Framework
updates fail if their candidate lock changes that root node.

The advertised channel is reference metadata, not a hard compatibility gate.
`package-base status/plan/apply` and Maintenance → Update system and packages
manage the direct base independently, preserving every other locked node.
Channel changes require explicit unverified-compatibility acknowledgement.
`nixoriumUpdateTargets` covers the controller and distinct client variants
(scoped software, interface and explicit host modules); private
host-conditional policies declare additional `mkLab.updateValidationHosts`.
`nixoriumOfflineCheck`
compares direct and standalone-installer derivations in an isolated offline
store. Read [updates](updates.md) for migration, activation, package overrides,
runtime checks and recovery. Framework updates can still change framework-owned
patches and auxiliary inputs without changing nixpkgs.

## Workstation-profile ownership

Nixorium core supplies the GNOME session, accounts, networking, classroom
control, installation, deployment, and reset mechanisms. The private deployment
template supplies the user-facing profile: packages in `lab-software.json`,
suggestions in `software-catalog.nix`, desktop and shell policy under
`modules/`, and branding/editor defaults under `assets/`.

`mkLab` passes each host's effective managed package IDs to downstream modules
as `hostSoftwarePackages`. The template uses this list to enable Docker, npm
setup, application favorites, shortcuts, browser policy, VS Code extensions,
and the site screensaver only when their packages apply to that host. Every
built-in profile, including Essential, declares the Ghostty and TTE dependencies
required by the screensaver.
Removing a declaration therefore does not leave an upstream launcher or service
behind.

Desktop Icons NG, Dash to Dock and Tiling Assistant are baseline workstation
components rather than application-profile choices. The template enables all
three for every role, keeps a compact bottom dock visible when its area is clear,
hides it behind overlapping windows, and preserves unrelated enabled extensions. Yaru-yellow icons complement
native Adwaita decorations and a static blue vector wallpaper for persistent
staff accounts. Reset student homes choose randomly from the deployment-owned
backgrounds at boot. A one-time
appearance migration lets persistent staff preferences survive later logins.
This keeps files under the XDG Desktop directory visible for reset student
homes and persistent staff homes alike.

### Measuring profile size

Measure the current template instead of using historical profile sizes. In a
disposable deployment, keep the same lock, settings and target for each profile:

```sh
SYSTEM=$(nix build \
  path:.#nixosConfigurations.pc01.config.system.build.toplevel \
  --print-out-paths --no-write-lock-file --no-link)
nix path-info --json-format 1 --json --closure-size "$SYSTEM"
```

Use `scripts/configure-software-profile.sh software-presets.json lab-software.json`
in that disposable deployment to select another profile, then repeat. Record
the upstream revision, lock and store/cache condition. Closure size includes
the common NixOS/GNOME base; it is neither download size nor total disk usage.
Time builds separately and measure network transfer independently. A warm
build reuses existing store paths, so its duration is not an installation-time
estimate.

## Network interfaces

`lab.ifaceName` is the compatibility fallback. Deployments may
set `controllerIfaceName` and `clientIfaceName` as role defaults and use
`hostIfaceNames.<hostname>` for exceptional hardware. Resolution order is host
override, role override, then fallback. `labMeta.network.ifaceName` remains the
effective controller interface for older management consumers; controller and
client records also expose their effective interface explicitly.

A host is configured by name only when it is the controller or has a
`hostIfaceNames` entry or a `clientIfaceName`. `labMeta.clients.hosts[].ifaceExplicit`
and `labSettings.ifaceExplicit` (passed to every module; absent means `true`)
state this. Other clients ignore the fallback name: `modules/networking.nix`
declares a NetworkManager profile `nixorium-lab` for any wired Ethernet device
(DHCP plus the static lab address, no interface name), disables NetworkManager's
automatic default profiles on those clients, and `modules/client-internet.nix`
and `modules/firewall.nix` omit `iifname`/`oifname` from their rules
(`lib/client-internet-rules.nix` and `lib/client-firewall-rules.nix` take
`interface = null`). With several wired cards, only the first to come up carries
the profile; use `hostIfaceNames` for such computers. The installer and the
USB/SSH preparation skip the name check for these clients and use the card that
carries the live address. See ADR 0017.

Clients answer Wake-on-LAN magic packets on their lab card, so **Switch on**
can start them (see [Classroom view](#classroom-view)); the controller does
not. A client configured by type sets `ethernet.wake-on-lan=64` (magic packet)
in the `nixorium-lab` profile, which NetworkManager applies to the card each
time it activates the profile. A client configured by name gets a udev rule
that runs `ethtool -s <interface> wol g` when that card appears, matching the
final interface name, or the kernel name when nothing renames it. The NixOS
`networking.interfaces.<name>.wakeOnLan` option is not used: its systemd
`.link` file matches only the kernel's original name (`OriginalName=`), so it
misses cards renamed to predictable names such as `enp3s0`. Either way the
setting takes effect from the first restart after the client update. The
computer's firmware must also allow it (BIOS/UEFI options often called "Wake
on LAN" or "Power on by PCI-E"), and the computer must keep standby power: a
switched-off power strip or a power cut disarms it.

### Laboratory network transition

While computers move from one laboratory network to another, the settings
hold a `lab.networkTransition` record
(`{ "previousBase": "10.0.0.0", "previousPrefixLength": 24, "startedAt": "<RFC 3339>" }`)
next to the new `networkBase` and `networkPrefixLength`, and the configuration
is dual. Every host keeps its host number on both networks. The controller
carries its new and its previous address on the laboratory interface; its
firewall is interface-scoped, so SSH, the cache and PXE ports are open on both,
and the cache listens on every address. Clients take the new address and keep
the previous one next to it (a second address on a named interface,
`address2` of the `nixorium-lab` profile otherwise, which is applied to the
running card with `nmcli device reapply` when the profile changes), so the
update that moves a client keeps its connection and an undo finds every client
on the restored network. They accept SSH from both controller addresses (two `ip saddr` rules, in the
firewall allow list and in the `nixorium-client-access` guard), list the new
controller cache first and the previous one second, and keep both networks
reachable under the temporary Internet block. PXE preparation and network
installation keep using the new controller address only: prepare network
installation again after the change. `labSettings.networkTransition` gives
modules `{ base, prefixLength, masterIp, hostIps }` of the previous network
(`hostIps` maps each client name to its previous address), or `null`;
`labMeta.network.transition` (present only during a change) lists the previous
controller address and each client's previous address for management tools.
The previous network must be a valid IPv4 network different from and not
overlapping the current one, hold every configured host number and exclude the
controller DHCP address; without client computers the record is ignored (the
evaluation warns and the management command refuses to save it). Settings
changes accept a new network base or prefix while computers are configured
only together with a record naming the saved network, and during a change only
a return to its previous network, or an undo that restores the previous network
and removes the record. Without the record the configuration is exactly the
single-network one; an address a client no longer needs after the record is
removed disappears at its next start. The guided change (Maintenance → Change
network addressing, `nixorium network migrate`) writes and removes the record;
see ADR 0028 and the administrator guide.

## Time

The controller keeps the laboratory's time and the client computers take it
from it, so a computer with a drained clock battery shows the right time
although it has no Internet of its own (certificates, file dates and logs stay
correct). `modules/time.nix`, imported by `modules/common.nix`, declares it.

- **Controller.** It runs `chrony`, which replaces `systemd-timesyncd` there.
  The upstream is the default NixOS Internet pools; the controller needs
  outbound UDP 123 to them. It answers only the laboratory network
  (`allow <networkBase>/<networkPrefixLength>`), listens on the controller's
  laboratory address, and `modules/firewall.nix` opens UDP 123 on the
  laboratory interface only. When the pools are unreachable it keeps serving
  its own clock at stratum 10, starting five minutes after boot so that a
  clock that may still be wrong is not served while the upstream is being
  reached.
- **Clients.** They keep `systemd-timesyncd`, with the controller's laboratory
  address as the only server (`services.timesyncd.servers` and
  `networking.timeServers`) and no fallback, so no Internet pool is listed and
  a client never waits for hosts it cannot reach. A client that has Internet
  after a user signs in still takes the time from the controller. Clients open
  no port. The defaults are `lib.mkDefault`, so a site module may name another
  server.
- **Controller-only mode** has no laboratory network: nothing is served and
  the controller keeps the NixOS defaults (`systemd-timesyncd` with the
  Internet pools).
- **During a change of laboratory network** the controller serves both
  networks (chrony binds one address only, so it is not bound to the
  laboratory address meanwhile) and clients list the new controller address
  first and the previous one second, like the cache.

The change reaches a client at its next update and the controller when the
controller is applied; until then a computer keeps its previous clock
configuration. `nixorium doctor` still reports an unsynchronized controller
clock.

## USB/SSH client installation boundary

The supported remote-install environment is the official NixOS 26.05 Minimal
or Graphical (GNOME) ISO for `x86_64-linux`, booted with UEFI and Ethernet or
Wi-Fi connectivity. Prefer Minimal on computers with 4 GB of RAM or less.
The controller must be a configured laboratory controller with a healthy signed Harmonia
cache; controller-only deployments and arbitrary rescue environments are not
accepted. This workflow does not depend on PXE services or alter the
controller's static address.

The declared client interface must carry the reviewed live IPv4 address.
Connect Wi-Fi in the live ISO before the SSH workflow and arrange persistent
system connectivity separately; live profiles and credentials are not copied.
An installed client uses a wired Ethernet card unless a `clientIfaceName` or
`hostIfaceNames` entry (see [Network interfaces](#network-interfaces)) names
its interface; with the name of a wireless card, the lab address is applied to
that card.
After reboot, the configured static client address must be reachable for
verification. Keep Wi-Fi secrets out of Git and the Nix store. PXE may be
incompatible with Wi-Fi depending on hardware, firmware, and network setup;
that restriction is not part of USB/SSH transport.

`packages.x86_64-linux.remoteInstallerBundle` contains the target-independent
remote helper, metadata, and Disko programs. It deliberately excludes a client
system closure and can be prepared before the live address is known. After a
credential-free SSH host-key observation has been compared with the
physical-console fingerprint and explicitly confirmed, the controller builds the exact
selected client's closure and the live helper copies only signed paths from
Harmonia with substituter fallback disabled. The helper excludes the ISO boot
medium and validates the chosen disk, NIC, cache public key, deployment
revision, and closure immediately before mutation.

The public CLI surface is `install usb prepare/start/status/reconcile/reboot/
verify/cancel/close`. `prepare` and `start` take `--host`; later actions take
the returned `--id`. `start`, `reboot`, and `close` require `/dev/tty`, have no
JSON or unattended form, and collect secrets only through the terminal.
`status`, `reconcile`, `verify`, and `cancel` support `--json`. The installed
controller owns `nixorium-remote-install.service`; its private socket is
`/run/nixorium/remote-install/control.sock`, durable state is under
`/var/lib/nixorium/remote-install`, and volatile keys/password material remains
under `/run`.

An interrupted post-dispatch operation is never replayed. Reconciliation
observes the exact operation receipt and reports whether disk mutation may have
started. A definitive failed receipt with no mutation remains safely
cancellable and normally triggers automatic live-key, preparation-root, and
reservation cleanup; uncertain or post-mutation failures do not. A controller
restart may reattach only after the controller re-observes the key and the
operator physically reconfirms the same live boot; reboot and known-host
rotation remain separately confirmed. Ordinary changes to an installed client
use `deploy`, not this destructive installation API.

The remote helper records and returns the reboot acknowledgement before a
short delayed transient unit closes the live SSH session. Once the TUI receives
that acknowledgement, it retries only the read-only installed-host verification
at the configured static address. It never replays Disko or the reboot request;
the CLI keeps reboot and verification as separate explicit operations.

## Student home reset

At boot, the previous student home becomes one of five rotating snapshots and
a clean home is restored from the activation-time template. Core creates the
configured Git identity and standard XDG directories; deployment modules add
editor and MIME defaults when those applications are selected.

Activation also repairs ownership and user-write access on the managed VS Code,
npm, and XDG roots for admin, teacher, and student accounts. If a runtime
directory already exists, only its top-level ownership and mode are reconciled;
logind remains responsible for creating `/run/user/<uid>` and its contents.

Administrators and the teacher can recover a file from the snapshot store:

```sh
ls /var/lib/home-snapshots/workspace/snapshot-1/
cp /var/lib/home-snapshots/workspace/snapshot-1/file.txt /home/<studentUser>/
```

Docker images, npm's download cache, and user-installed global npm packages are
runtime data inside the student home. They are intentionally removed on the
next reset; source files remain recoverable through the snapshots.

## Programs downloaded outside Nix

`modules/nix-ld.nix`, imported by `modules/common.nix`, enables `programs.nix-ld`
on the controller and every client with a plain list of free, common libraries
(zlib, libstdc++, glib, OpenSSL, libxml2, ICU and the X11, GTK 3, NSS, ALSA,
GL and similar libraries that Electron and Java GUI programs usually need).
A program that bundles generic Linux binaries, such as the JRE inside a VS Code
extension installed from the Marketplace, therefore starts instead of stopping
with "Could not start dynamically linked executable"; another library is a
deployment change to `programs.nix-ld.libraries` in a shared module. This is a
convenience for staff, who keep their homes: student homes are restored at each
boot and the managed profile extensions remain the supported route. When the
workspace profile selects `redhat.java` and the resolved packages include a
JDK, the seed's editor settings also set `java.jdt.ls.java.home` and
`java.configuration.runtimes` to that JDK, which reviewed `vscode.extraSettings`
may override.

## Rootless Docker

The default site profile gives each normal account its own rootless Docker user service and subordinate
UID/GID range. No account belongs to the root-equivalent `docker` group. Docker
and Compose use the current user's socket automatically.

```sh
docker run --rm hello-world
docker-compose up
```

Rootless containers cannot bind privileged ports below 1024 without additional
site policy. Prefer ports such as 3000, 8000, or 8080 for student projects.

## User-managed npm tools

When Node.js is selected by the site profile, global npm installs use
`~/.local/npm`, which is already in `PATH`. They never
need `sudo` and do not write into the Nix store.

```sh
npm install -g <package>@latest
npm outdated -g
npm update -g
```

Downloading packages requires Internet access for that user session. Packages
installed by the reset student account are intentionally ephemeral.

## Classroom view

`lab.classroomView` in `lab-settings.json` (on unless set to `false`) installs the
classroom view agent on client computers only. It is a systemd user service
started with the graphical session; it listens on
`$XDG_RUNTIME_DIR/nixorium-classroom.sock` (mode 0600) and opens no network
port. The controller reaches it with its existing SSH access and the fixed
command `nixorium-classroom-connect`, which runs as root, finds the user of
the active graphical session on seat0, checks the socket without following
links and relays standard input and output. The protocol is versioned and
bounded; it answers `hello`, `thumbnail.request` (a JPEG of the screen,
64–1920 pixels wide, never wider than the screen), `input` and `lock.set`.

On the controller, **Computers → Classroom view** (teacher and administrator)
asks the classroom service for a one-time address and opens it in a browser
window. The service keeps one SSH channel per client while the page is open,
refreshes the thumbnails every 1.5 seconds and closes every channel 15
seconds after the page stops asking. The page is served on the controller's
loopback address only; the one-time token becomes an HttpOnly, same-site
session cookie, foreign `Host` headers are refused, and a strict content
security policy allows only the page's own files.

Each card shows its computer's state over the picture. GNOME refuses to
capture a session behind its own lock screen (the student pressed Super+L, or
the session was left unused long enough to lock, for example by the site
template's screensaver): the agent answers `screen-locked`, and the card dims
the last picture under a moon with **Idle**. The teacher's lock (below) keeps
the live picture, with a padlock in its corner and a **Locked** label. A
computer that is switched off or not reachable shows a power symbol and **Off
or not reachable** instead of its last picture; other pictures that are no
longer live (nobody signed in, for example) are dimmed. The page asks for the
cards every two seconds and keeps asking when the controller does not answer,
giving up on a request after 15 seconds; after two failed requests in a row
the summary says **Reconnecting…**.

A computer that answers over SSH with someone signed in, but whose agent does
not answer (the socket is missing, or the agent is frozen and gives no answer
to the relay's own hello within 3 seconds), shows a warning sign over its
dimmed last picture and **Monitoring stopped on this PC**. The login screen does not count as someone signed in
(only a session of class `user` does), so a computer waiting at it says
**Nobody is signed in**. The same words explain the computer wherever an
action skips it (Lock, Share files, Share screen, Collect from students' desktops and
Save screenshot), and a result notice counts such computers apart, as
"monitoring stopped on this PC". The agent restarts by itself: its user unit
is `Restart=always` with a one-second delay and no start limit, and both
classroom user units refuse a manual stop (`RefuseManualStop`), so
`systemctl --user stop` fails in the student's terminal and a killed agent is
back within seconds; the card returns to the live picture. The Shell
extension helper is a one-shot unit, which systemd restarts only after a
failure. This is a classroom aid, not a security boundary: a student can still
mask a unit or freeze the process (which the card reports, but nothing
revives), and loses that at the next boot, when the home is reset. The relay
never outlives the controller's channel by more than 5 seconds, even when the
agent is frozen. Because the units refuse a manual stop, an update does not
restart them either: the new agent starts at the next sign-in.

Clicking a computer opens it in its own browser window, refreshed about ten
times a second at up to 1920 pixels; the agent sends a frame only when the
screen changed, and the overview and the large view share one capture. The
window can be moved, maximized or put on full screen, where its button bar
slides in at the top edge. **Take
control** sends the teacher's mouse (move, buttons, wheel) and keyboard (as
X keysyms) to the student's session through the same Mutter remote desktop
session; while controlling, Escape also goes to the student. **Stop control**,
closing the view, or leaving the browser window releases every key and
button still pressed. Input is accepted only as JSON from the page itself
(same Origin, session cookie), at most 128 events per message, and only for
a computer currently open in the large view.

Check boxes on the overview select computers for **Block/Allow Internet**,
**Lock/Unlock**, **Restart** and **Shut down**; a computer's window offers the
same in its **Actions** menu. The page calls the classroom service's own
review and apply handling, as the classroom dashboard does: the review token
stays in the service and each review applies once. The review names
computers whose session is active or unknown. While the page is open, the
service reads every computer's real Internet state every 30 seconds with the
same read-only check as the Internet review, so "Internet off" also follows
changes made from the TUI or by a restart. One explicit button (such as
"Restart 3 computers") confirms. The outcome appears as a notice with one
line that counts the computers done, switched off or not reachable, skipped
(nobody signed in, for example) and failed, such as "Lock: 8 done · 12
switched off or not reachable · 1 failed", with each computer under
**Details**. A click on the notice, outside Details, or Esc closes it; it
closes by itself unless a computer failed. After **Restart** or **Shut
down**, a card shows "Restarting…" or "Shutting down…" until the computer
comes back from the restart or stops answering after the shutdown, and for
at most three minutes.

**Switch on** sends a Wake-on-LAN magic packet to the selected computers
that are off: **Power → Switch on** for the selected computers, and **Switch
on** in a computer's **Actions** menu while it does not answer. The review
lists the computers that receive the packet and those skipped because they
are already on (they answered on the lab network a moment ago) or because
their address is not known yet. After the click a card shows "Starting…"
until the computer answers, for at most three minutes, and the notice counts
packets, such as "Switch on: 3 sent · 2 already on · 1 address unknown". The
same review is in **Computers → Power controls** (Tab chooses Switch on), and
`nixorium wake --on <clients|@lab> [--json]` sends without a review token,
printing one line per computer (sent, already on, address unknown or invalid
target). Like the lock, it takes neither a review token nor the
administrative operation lock, since it changes no system state; the
teacher's command and dashboard go through the classroom service, which
validates the computer names against the inventory like every classroom
operation.

The controller learns each client's network card address without anyone
typing it: right after a client answered on the lab network (the classroom
view's connection, the computer inventory, or a Switch on review that found
it on), it reads the kernel's IPv4 neighbor table (`/proc/net/arp`) for that
client's lab address and keeps complete Ethernet entries only. The record is
`/var/lib/nixorium/classroom/wake-addresses.json`: the classroom service's
own state directory (systemd `StateDirectory`, mode 0700, owned by `admin`),
so it survives reboots and stays outside Git, the deployment and the Nix
store; it is never written to `lab-settings.json`. It is replaced atomically
with mode 0600 under a lock, a damaged or unsafe record reads as empty and is
replaced by the next address learned, and an address moves to the last
computer seen with it. Applying a switch-on review always reads the
addresses from this record, never from the review. The packet (six `0xFF`
bytes, then the address sixteen times) goes as UDP to port 9 of the lab
subnet's broadcast address and of `255.255.255.255`, three times over about
one second, from an ordinary unprivileged socket bound to the controller's
lab address and, where the kernel allows it, to its lab interface. No port is
opened and no service is added. A sent packet is not proof that a computer
starts: see [Network interfaces](#network-interfaces) for what the client
and its firmware need.

**Power → Schedule…** switches the student computers on and off by
themselves, so nobody has to be there at 07:50 or 18:00. The dialog sets at
most one switch-on time and one shut-down time (either may be left out) for a
set of weekdays, in the controller's time zone, for every client computer;
**Save** replaces the schedule and **Clear** removes it. It is an operational
aid like blocking the Internet, not laboratory configuration: it takes no
review token and changes no system, and `nixorium schedule show`, `set [--on
HH:MM] [--off HH:MM] [--days mon-fri|all|<list>]` (Monday to Friday when
`--days` is left out), `clear` and `--json` do the same for the teacher and
the administrator, through the classroom service's typed `schedule-get` and
`schedule-set` operations; the two times must be more than ten minutes apart. The schedule is
`/var/lib/nixorium/classroom/schedule.json` (`{"schemaVersion":1,"on":"07:50",
"off":"18:00","days":["mon",…,"fri"],"lastRun":"2026-10-15T07:50"}`, with a
`retry` minute while an action waits for its next try), beside
the learned addresses: the service's own state directory, replaced
atomically with mode 0600 under a lock, and a missing or damaged file reads as
"no schedule". It is not in Git or in the backup, so set it again after
replacing the controller. `nixorium-classroom-schedule.timer` starts
`nixorium schedule run` every minute as `admin`, the classroom service's user
(the service does not start while the file is missing). In a minute the
schedule names, the run sends the ordinary Switch on to every client, which
skips the computers already on and those with no known address yet, or the
ordinary reviewed shut-down with the `skip-active` session policy: a computer
someone is using, or whose session state cannot be read, is left on, both in
the review and in the check just before each request, and the usual client
operation lock and installation checks apply. The set minute is recorded as
`lastRun` under the lock before anything is sent, so no minute runs twice.
An action that cannot start, because the inventory cannot be read, another
client operation holds the lock or an installation check refuses, has sent
nothing and is not counted as run: its minute goes to `retry`, the next runs
report "waiting" and try again every minute until ten minutes after the set
time, and the last try that still cannot start gives it up for that day with
one `gave-up` record in the history (a changed schedule drops it too). An
action that started, completed or partial, is never repeated, and a minute in
which the controller was off is not made up afterwards. A run prints one line
per computer and a summary to the journal of
`nixorium-classroom-schedule.service`, adds a `schedule-run` record with counts
only to the operation history (not for a waiting minute), and prints nothing
when nothing is due. The
computers must allow waking from the network, as for Switch on.

**Lock** asks the agent to have the bundled Shell extension cover the whole
screen with "Eyes on the teacher" and take the keyboard and mouse (a modal
grab); the extension offers this only on the session bus. A locked computer
stays visible but cannot be controlled. Logout and restart always unlock. The
lock is a classroom aid, not a security boundary. Locking also works from
**Computers → Lock screens** (teacher and administrator) and with
`nixorium lock plan|apply --on <clients|@lab> --action lock|unlock`; it does
not take the administrative operation lock, since it changes no system
state.

The page's **Share files** menu (toolbar) holds **Send a folder…**,
**Send a file…** and **Collect from students' desktops**, in this order, and so does a
computer's **Actions** menu.

Sending a file, or a folder with its name and contents, is chosen by whoever
uses the controller. The classroom service runs as `admin` and cannot read that
user's home, so Nixorium starts the page's browser through a small helper
running as the user; while the classroom browser is open, the helper waits
on the classroom socket. When the page asks, the service gives that user's
helper a job (users are told apart by the socket's kernel peer credentials,
recorded with the one-time view token), and the helper opens the system file
chooser (zenity) in the user's home for a file or a folder, then reads the
choice (hidden files and links inside a folder are left out) and prepares its files
in the service's private temporary folder in 512 KB pieces. The page then
reviews the selected computers. The review binds each file's path,
size and SHA-256, so the same files prepared again keep the review token.
The service streams the files to at most four computers at a time with
`files.begin`, `files.chunk` (1 MB pieces, each acknowledged) and
`files.end`. The agent writes them as the student into a private staging
folder of the home with explicit 0755/0644 modes and moves the top-level
items to the XDG desktop folder only when everything arrived; an existing
name becomes "name (2)". At most 2000 items and 500 MB are sent at once, and
at most three prepared sendings exist, each removed after 30 unused minutes
or once delivered everywhere. **Computers → Send files** (teacher and
administrator, with a path that starts at the user's home) and
`nixorium send plan|apply --file <file-or-folder> --on <clients|@lab>` read
the chosen file or folder as whoever runs Nixorium and send it through the
classroom service the same way; `send apply` prepares the files again and
requires the token of the review. Like `collect`, the command needs no
deployment repository, so the teacher can run it from a terminal.

**Save screenshot**, in the window of one computer whose screen is live,
asks that computer for a picture at up to 1920 pixels through the
overview's channel (`thumbnail.request`); the button is off while the
computer is switched off, behind GNOME's own lock (Idle) or without anybody
signed in, and a computer that changed state meanwhile is reported in the
card's words. The service hands the picture to the desktop helper of the
page's user with a save job through `desktop-wait`; the helper copies it
over the classroom socket (`save-items`, `save-read` in 512 KB pieces,
`save-done`) and writes
`~/Nixorium/Screenshots/<computer>-<YYYY-MM-DD>-<HH.MM.SS>.jpg` itself, as
that user (folders 0755, files 0644), never replacing a file: a taken name
becomes "name (2)". The notice names the file, such as "Screenshot saved:
Nixorium/Screenshots/pc03-2026-10-15-10.30.12.jpg". A page opened without
the helper downloads the picture once through the browser instead
(`Content-Disposition: attachment`, same name); the service keeps it for two
minutes. Only the save's own user (the socket's kernel peer credentials) may
list, read or end a save, and the service itself never writes into a home.

**Collect from students' desktops** (in **Share files** in the toolbar, and in a
computer's Actions menu) is the reverse of sending files. The review lists each
selected computer's desktop; a computer whose agent predates collecting is
left out and named. After the one confirmation, the service reads at most
four computers at a time with `collect.list` (the regular files and folders
of the student's XDG desktop folder with their sizes; hidden names, links
and special files are left out, and names with control characters or
backslashes and folders nested more than 32 levels are left out and
counted), `collect.chunk` answered by `collect.data` (1 MB pieces, asked for
by position in that list, never by name) and `collect.end`. The agent only
reads: it opens each file below the desktop folder it listed, never through
a link, and refuses a file whose size changed. The service treats the list
as untrusted: relative paths only, no `..`, absolute paths, control
characters or backslashes, at most 32 levels and 255 bytes per name, and at
most 2000 items, 500 MB and 512 KB of names per computer. Beyond the limits
a computer is "too much to collect"; then, as after any refusal or
interruption, nothing of that computer is kept. Files wait in the service's
private temporary folder under position-based names, only while the
controller keeps 1 GB free beyond twice their size, and are removed once
saved or after 30 unused minutes. The helper of the page's user saves them
like screenshots into `~/Nixorium/Collected/<YYYY-MM-DD HH.MM>/<computer>/…`
(folders 0755, files 0644, never executable; a folder of the same minute
already there is kept and the new one becomes "… (2)"). A page opened
without the helper does not collect. The result line counts the computers,
such as "Collect: 10 done · 2 nothing to collect · 1 switched off or not
reachable"; an empty desktop is not a failure. `nixorium collect plan|apply
--on <clients|@lab> [--into <folder>]` (teacher and administrator) collects
through the classroom service the same way and saves as whoever runs it,
into `~/Nixorium/Collected` or the existing folder given; the review token
binds the computers and the folder, and each review applies once. Only the
desktop folder is collected, and the students are not notified.

**Share screen** exists only in the page. The browser asks GNOME what to
share: the entire screen is offered first, and a window or another tab works
too (never the classroom view page itself). After the review,
the page draws the screen at up to 1920 pixels (JPEG quality 0.85) and sends a JPEG at most
five times a second when it changed, and at least every four seconds. The
classroom service keeps one channel per selected computer and always sends
it the latest picture (`broadcast.frame`), so a slow computer skips
pictures; a computer that is off or has nobody signed in is tried again
every few seconds. The agent shows each picture through the Shell extension
in a cover above everything, the lock included, with keyboard and mouse
blocked. **Stop sharing**, closing the page, the end of GNOME's sharing,
ten seconds without pictures at the service or fifteen at the agent end
the showing (`broadcast.stop`); a lock stays in place.

**Message** shows a short text over the screens of the selected computers:
**Message → Send a message…** in the toolbar and in a computer's **Actions**
menu, with **Remove message** beside it. A dialog asks for the text (at most
300 characters and 10 lines, plain text, no control characters) and how long
it stays: until the student closes it (**Close** appears after 10 seconds),
30 seconds, 2 minutes, or until the computer is unlocked (a computer that is
not locked lets the student close it). The review shows the text and the
computers, one button such as "Send the message to 12 computers" confirms,
and the notice counts them, such as "Message: 12 shown · 1 switched off or not
reachable"; computers that are off, have nobody signed in or show "Monitoring
stopped on this PC" are listed and skipped, like for the lock. Like the lock it
takes no administrative operation lock, and its
review token binds the text and the duration. The agent answers `message.show`
(`text`, and `seconds` or `untilUnlock`; checked again against the same
limits, 600 seconds at most) with `message.shown`, and `message.hide` with
`message.hidden`; a session whose agent or extension cannot do it answers
`unsupported` or `message-unavailable`, and the review calls the computer
unable to show messages until it is updated and the student signs in again
(the extension loads at login). The agent asks the Shell extension on the
session bus (`ShowMessage`, `HideMessage`, property `MessageShown`), which
draws a centered panel above everything, the lock and the teacher's screen
included. The text is a plain label, never markup. The panel takes neither
keyboard nor mouse: the student keeps both, unless the computer is locked or
shows the teacher's screen, and then Close is not offered. A computer has one
message at a time (a new one replaces the previous); it belongs to the
session, not to the controller's connection, and logging out clears it.

Capture uses Mutter's own `ScreenCast` and `RemoteDesktop` D-Bus interfaces,
so no consent dialog appears. It starts only when the controller asks for a
frame and stops after 30 seconds without requests. While it runs, GNOME shows
its screen sharing indicator in the top bar; the
`nixorium-classroom@nixorium.org` Shell extension, enabled in every client
session, keeps that indicator visible but removes its stop button, so a
student cannot end the teacher's view. Mutter sends frames only when the
screen changes; the agent keeps the last one.

## Temporary client Internet access

The administrator and teacher Computers → Internet access action uses authenticated SSH
and the root-only `nixorium-internet` helper. Requests bind to the observed
client boot ID; an old request cannot reapply a block after reboot. The
`nixorium-internet-block.service` unit is never enabled for boot. It owns only
the `inet nixorium_internet` table, which is retained across ordinary firewall
reloads and removed on unblock. The controller itself is never a target.

Output and forwarded traffic outside the configured lab IPv4 subnet is
blocked, including established connections and IPv6 Internet traffic.
Loopback, IPv4 DHCP, IPv6 DHCP and neighbor discovery remain available.
This is destination-based network control: allowed laboratory services remain
reachable, including any proxy deliberately hosted there. No gateway or
NetworkManager connection is rewritten. Reboot restores normal connectivity.

## Private deployment customization

Do not fork or edit upstream modules for site policy. New installations create
a private deployment with these extension points:

- `modules/shared.nix` for every machine;
- `modules/controller.nix` for the controller only;
- `modules/clients.nix` for all client PCs;
- `hostModules` in `flake.nix` for a generated individual host;
- `lab-software.json`, `software-catalog.nix`, and optional
  `software-presets.json` for package and initial-profile choices;
- the focused modules and assets already shipped in the private template.

For example, add VLC only to `pc05` with `modules/pc05.nix`:

```nix
{ pkgs, ... }:
{
  environment.systemPackages = [ pkgs.vlc ];
}
```

Then reference it from the deployment Flake:

```nix
hostModules = {
  pc05 = [ ./modules/pc05.nix ];
};
```

The name must already be generated by the configured client count. Track both
files, validate the host, and deploy only that target:

```sh
git add flake.nix modules/pc05.nix
nix build .#nixosConfigurations.pc05.config.system.build.toplevel --no-link
nix run .#nixorium -- deploy plan --on pc05
```

Additional backgrounds can be passed without changing upstream:

```nix
assets = {
  logo = ./assets/logo.txt;
  backgrounds = [
    ./assets/backgrounds/one.jpg
    ./assets/backgrounds/two.jpg
  ];
};
```

Keys, assets, and downstream modules referenced by the deployment must remain
inside its source tree. `lib.mkLab` packages that tree into the offline
installer so client installation needs no second checkout.

## `lib.mkLab` extension points

| Argument | Purpose |
|---|---|
| `deploymentSelf` | Downstream Flake `self`, used when packaging deployment files for offline installation |
| `labConfig` | Required typed site settings, normally decoded from `lab-settings.json` |
| `labSoftware` | Strict versioned supported client-package declarations, normally decoded from `lab-software.json` |
| `softwareCatalog` | Deployment-owned suggested packages shown before free search; each entry has `id`, `label`, and `summary` |
| `softwarePresets` | Optional versioned deployment-owned software-profile catalog; each profile has an ID, label, description, and package IDs |
| `workspaceProfileJSON` | Raw JSON text of the student preferences; defaults to `{"schemaVersion":1}` |
| `homeFilesJSON` | Raw, strict JSON for additional initial student files; defaults to a present deployment `home-files.json`, otherwise an empty declaration |
| `homeFilesRoot` | Optional source path for captured payloads; defaults to a present deployment `home-files` directory, otherwise `null` |
| `workspaceCatalog` | Deployment-owned baseline/application/extension catalog; defaults to an empty catalog; no implicit package installation |
| `clientGroups` | Named sets of evaluated client identities for software scopes and group modules |
| `publicKeys` | Harmonia and SSH public-key paths |
| `assets` | Logo, wallpapers, MIME defaults, and editor settings |
| `homeResetEphemeralPaths` | Deployment-owned relative cache/tool paths excluded before student snapshots |
| `sharedModules` | NixOS modules applied to every generated host |
| `controllerModules` | Modules applied only to the controller |
| `clientModules` | Modules applied to all client PCs |
| `groupModules` | Module-path lists keyed by existing client group names, applied only to their members |
| `hostModules` | Attribute set of modules keyed by generated host name |
| `updateValidationHosts` | Extra generated hosts with private host-conditional policy; included in base-update builds and offline equivalence |
| `netbootModules` | Additional modules applied only to the PXE environment |

`groupModules` defaults to `{}` and refers to existing `clientGroups`:

```nix
clientGroups.course = [ "pc01" "pc02" ];
groupModules.course = [ ./modules/course.nix ];
```

Groups may overlap: their module lists are composed in group-name order, after
`clientModules` and before `hostModules`. The controller is excluded. Unknown
groups, invalid members and non-list/non-path module declarations are rejected.
Module paths retain the existing deployment/upstream ownership rules and are
serialized into the offline installer. Empty lists do not split host classes.

A group module declares common behavior for its members with the same software
and interface. It must not hide per-host differences: declare those exceptions
through `hostModules` or `updateValidationHosts`. This contract is shared by
Assistant, update targets and workspace prerequisite checks; it is not automatic
analysis of Nix program equivalence.

Unknown settings, host names, and asset names fail evaluation. Referenced
filesystem paths must belong to either the upstream or private deployment
source tree. `deploymentStatus` reports placeholders, public default password
hashes, and missing public keys separately from successful Flake evaluation.
The public template's seven profiles are fully expanded deployment data.
Essential is the default for new repositories and its six `shared`
declarations match the initial `lab-software.json`; existing repositories are
never rewritten when they update Nixorium. Profile metadata is serialized into
the offline installer source alongside the effective package declarations.
All seven profiles include `git` and `nodejs`; only Programming includes
`pi-coding-agent`, `opencode`, `vscode` and its toolchain-coupled extension payload.
Programming also carries the C/C++ (`gcc`, `gdb`, `gnumake`, `cmake`), Java,
Python and PHP toolchains, `mysql84` and `mysql-workbench`. Where `mysql84` is
selected, the site development module runs a teaching MySQL server that
listens on loopback only, leaves `root` without a password and discards every
database at boot; where `php` is selected it adds a PHP build with Xdebug.
Where `apacheHttpd` is selected it runs Apache with that PHP as the student
account, on loopback only, serving the student's `~/public_html` (recreated
empty after the boot reset) with `.htaccess` overrides and folder listings.
The controller management module also installs Git independently of profile
selection, and the packaged Nixorium command carries it in its runtime PATH.
Global npm uses each user's `~/.local/npm`, which precedes the system PATH.
Staff overrides persist. The reset student profile recreates an empty prefix
and excludes npm globals plus Pi/OpenCode credentials and state from snapshots,
so no shared template or historical snapshot captures agent authentication.

The Software TUI loads the optional catalog only when the operator chooses
**Add profile**. Package inclusion, scope and the full additive candidate are
separate stages; one review distinguishes additions from existing declarations
whose scopes remain unchanged. The TUI saves and records the batch through the
typed preset boundary, then reuses the ordinary controller recovery and fresh
client-deployment review. Missing metadata leaves individual software
management available, and controller-only results never offer a client
distribution action when no client is affected.

### Workspace preparation

The template ships a catalog, an active `workspace-profile.json` with Essential
defaults and two proposals that are never read as the declaration:
`workspace-profile.example.json` (the same Essential defaults) and
`workspace-profile.programming.example.json`, which bootstrap and template
reset copy to `workspace-profile.json` when the Programming software profile
is selected. Hand-written callers that pass no profile get an empty one, so
the student home still resets to the system defaults. `nixoriumWorkspace`
reports `state = "prepared"`, the configured `studentUser`, all target hosts,
declared/effective preferences, normalized catalog and pinned
package/extension versions. `seed` is the immutable build-output path; it does
not report live installed or active state.

`workspaceProfileJSON` must be raw JSON text, limited to 64 KiB, with integer
`schemaVersion: 1`. Unknown fields, duplicate keys (including escaped aliases),
nulls and wrong types are rejected. Optional sections are:

- `desktop`: ordered unique `favorites` (at most 32 desktop IDs), `colorScheme`
  (`light`/`dark`), boolean `enableAnimations`, and `dock`. Dock supports
  `position` (`top`/`bottom`/`left`/`right`), integer `iconSize` (16–128) and
  booleans `autoHide`, `extendHeight`, `showTrash`, `showMounts`.
- `vscode`: unique `extensions` (at most 64 lowercase `publisher.extension`
  IDs, normalized in sorted order) and allowlisted `settings`: `editor.fontSize`
  (8–40), `editor.tabSize` (1–8), booleans `editor.insertSpaces`,
  `editor.formatOnSave`, `editor.minimap.enabled`, `editor.wordWrap`
  (`off`/`on`/`wordWrapColumn`/`bounded`), and `files.autoSave`
  (`off`/`onFocusChange`/`onWindowChange`). Optional `extraSettings` holds up
  to 256 further editor settings as reviewed free-form JSON values. It refuses
  the typed settings above, the managed update keys, workspace-trust overrides,
  automatic tasks and integrated-terminal profile, shell and environment keys.
  Optional `marketplace` pins up to 32 Marketplace extensions that the package
  set lacks: `publisher`, `name`, a stable `version`, the SRI `hash` of its
  package and optional `platform` (`linux-x64`). The builder derives the
  download URL; every pin must also be selected in `extensions`.
- `browser`: `defaultApplication`, a desktop ID catalogued as a browser.

Identifiers have a 128-byte ASCII limit. Desktop IDs end in `.desktop` and
cannot contain paths, whitespace or control characters. There are no arbitrary
files, commands, account selectors, download URLs, or whole-home imports.
Extra settings are initial editor preferences only; they are not policy and
the student can change them until the next reset.

The catalog is a Nix attribute set containing `schemaVersion = 1`, `baseline`
(a profile attribute set using the same schema), `applications` and `extensions`.
Both lists are limited to 128 unique entries. Applications contain `id`
(desktop ID), `package` (pinned package attribute) and optional boolean `browser`.
Extensions contain `id`, matching `package = "vscode-extensions.<id>"`, and
optional `requiredPackages` and `requiredExtensions` lists and boolean
`writable`. IDs are lowercase; the packaged identity is compared without
regard to case. The extension catalog is not an allowlist: a profile may select
any extension of the pinned package set, which then has no prerequisites.
Dependencies must be present explicitly; they are never installed or selected
automatically, and the seed build fails when a declared one is missing.
Extensions are linked to their store payload; `writable` ones are copied into
the home instead, because some debug adapters create files in their own folder
and the editor identifies an extension by the real path of its running code.
Fields omitted from the profile inherit the baseline; explicit lists replace
baseline lists, including `[]` to clear them. An empty section does not erase
baseline fields. Catalog declarations do not prove plugin compatibility or
complete manifest dependencies.

Every selected application/runtime must be in each generated host's declarative
`environment.systemPackages`, including the controller in controller-only mode.
Selected package identities must match the shared pin; different downstream
package overrides are not silently treated as equivalent. VS Code preferences
require `vscode`, desktop preferences require `gnome-shell`, and dock preferences
require `gnomeExtensions.dash-to-dock`. Broken, insecure, unsupported-platform
or unresolved required packages fail preparation. No live hosts are contacted.

Hosts that share their role, network interface, managed software and module groups,
and have no `hostModules` or `updateValidationHosts` exception, evaluate the same
modules; the check evaluates one of them per such class and applies the result to
the whole class, so its memory does not grow with the number of computers. A host
with either exception is always checked on its own. Express per-computer
differences through `hostModules` or software scopes: a shared module that changes packages by host name alone is not seen
for the other members of its class.

`nixoriumValidateWorkspaceCandidate` accepts raw candidate JSON text and checks
the same schema, catalog and all-host prerequisites without saving or deploying.
The candidate hook does not qualify extension loading or mutate sessions.
Pins exposing `lib.workspaceCandidateVersion` also provide
`nixoriumResolveWorkspaceCandidate`, with the same raw JSON argument and core
validation. It returns the candidate's complete preparation metadata, including
declared/effective preferences, catalog, resolved versions, targets and boot
behavior, without saving the candidate or changing current metadata. Consumers
must still invoke `nixoriumValidateWorkspaceCandidate` to preserve any additional
validation supplied by the deployment. Resolved metadata alone is not a review
token or proof that the deployment stayed unchanged while evaluating; a save
workflow must bind and recheck its source, pin, base file and exact candidate.
Both profile text and catalog data travel through the offline installer. The
template includes an active Essential `workspace-profile.json` and its catalog.
It reads that saved file, never `workspace-profile.example.json` as a fallback.
Leave the profile unchanged for the defaults; customizations use the same boot
path without a separate enablement or migration step.

#### Managed-home runtime

Every deployment uses the managed student home; there is no other student
reset path and no runtime switch. Runtime capability version 2 identifies this
contract. The profile applies to the student on every client and the
controller, including controller-only mode, without changing controller autologin
or staff preferences. Profile/catalog and seed metadata survive offline
reconstruction. Evaluation and saving do not activate a system or reset a home.

A Marketplace pin replaces a packaged extension with the same identifier. The
controller fetches the pinned bytes when it builds the seed; clients receive
the result like any package. `nixorium workspace marketplace --extension
<publisher.name>` (and `m`/`u` in the TUI) asks the public gallery for the
newest stable Linux version the pinned VS Code accepts, downloads it into the
controller's Nix store under the fetcher's own name so later builds work
offline, and reports its dependencies and any native programs. That command
and the gallery request use the controller's Internet access only when invoked;
nothing is contacted at build review, reset or login. Pins do not change with
package-base updates: check them with `u` after an editor update.

Package search accepts `vscode-extensions.<text>` and then matches
`publisher.name` across all publishers, listing only extensions selectable by
their lowercase ID. The TUI uses it for extension search.

The immutable seed combines supported preferences with a neutral shell/Git/XDG
scaffold. XDG folders use stable English names. It does not copy a live home,
a writable template, or arbitrary application assets. Extension links
point to identity-checked pinned store payloads, and editor update checks start
disabled as editable session defaults. The seed build fails when a selected
extension declares a dependency that is not selected. With a `vscode` section
the seed also writes `.vscode/argv.json` with the basic password store and no
crash reporter. This is not proof that every plugin
loads correctly or works offline; qualify each supported plugin separately.
Wallpapers are composed with the profile dconf source before restoration.

The template makes no student template writes, ownership repairs or login
preference changes, while retaining staff behavior and desktop extension
enablement.
Keep desired preferences explicit in the profile/baseline. Missing settings use
system/application defaults, not an inferred translation of old Nix modules.

Build and deploy through the existing reviewed controller/client flows. The
new home becomes available at the next normal boot, not on save or rebuild.
The reset never restarts during a configuration switch. Students may change their
initial preferences during the session; the next reset restores the seed.

The managed reset requires the declared Btrfs home/snapshot mounts, exact
account ownership, a closed login barrier, no student processes or lingering,
and a fully checked seed/home tree. Ephemeral paths must be canonical relative
paths, non-overlapping and unique, at most 128 entries and 4096 bytes each.
Nested mounts/subvolumes and intermediate symlinks fail closed. Unsupported
kernel confinement APIs do not fall back to unsafe removal.

Before changing the original home, it creates a private snapshot, removes the
configured ephemeral data from that copy and makes it read-only. Five managed
snapshots are published under `/var/lib/home-snapshots/workspace`. The teacher reads them through the `nixorium-staff` group.
Reset success records bind the seed, user and boot ID under the root-only
`/var/lib/home-snapshots/.workspace-reset` directory. They are not fleet status
and are not currently consumed by the TUI.

An incomplete attempt leaves `pending.json` and recovery data, blocks normal
login and refuses subsequent attempts, including after reboot. The root-only helper is not an interactive reset or
recovery command. Inspect the `home-reset.service` journal and preserve the
private evidence before administrator-led recovery; never delete the marker
or rerun the reset just to clear an error. Snapshot recovery, external backups
and reverting the declared profile are distinct operations.

## Optional Assistant

Capable controllers offer read-only diagnostics for declared computers and
bounded saved configuration history after renewed disclosure. Diagnostics use
fixed core units and strict SSH trust; journal and historical-text filtering
is best effort. See the [Assistant tool limits](assistant.md#tools-and-limits).

Capable controllers offer bounded public npm CLI preparation. Its generated
deployment module restores a reviewed offline baseline after home reset, without
boot/login downloads; see the [tool limits](assistant.md#prepared-npm-baselines).

`lab.assistant` is on unless explicitly false. It enables the administrator
Assistant menu only on the controller, after applying its configuration. New
installations and upgraded deployments with an omitted setting use this default;
saved explicit choices are preserved. Account activation and consent remain
separate. See [Assistant](assistant.md) for the
subscription, consent, confined tools and reviewed local-save workflow.

`nixoriumHostClasses` is a cheap `mkLab` output: sorted records with
`representative` and `members`, covering every host exactly once. The controller,
`hostModules` hosts and `updateValidationHosts` are singleton classes; remaining
clients group by applicable software, interface and non-empty module groups. Private modules branching
on `networking.hostName` must declare exceptional hosts through those inputs.
Assistant evaluates representatives and conservatively includes hosts whose
assigned representative changes, even when their own system is unchanged.

## Public Flake outputs

The upstream keeps its standalone example evaluable while private deployments
consume `lib.mkLab`. Important generated outputs include:

- `nixosConfigurations.<host>` for the controller, clients, and netboot system;
- Colmena deployment metadata and target groups;
- `labMeta` for non-sensitive operational identity and network data;
- `deploymentStatus` for readiness blockers;
- `nixoriumSoftware` for the supported pinned catalog, evaluated scopes, and managed declarations;
- `nixoriumSoftwarePresets` for the normalized optional profile catalog, or `null` when a deployment does not provide one;
- `nixoriumSearchSoftwarePackages`, `nixoriumResolveSoftwarePackage` and `nixoriumValidateControllerSoftwareCandidate` for pinned package search, resolution and controller candidate checks;
- `nixoriumWorkspace`, `nixoriumValidateWorkspaceCandidate` and `nixoriumResolveWorkspaceCandidate` for optional preparation metadata, prerequisite validation and candidate resolution, not activation;
- `nixoriumUpdateTargets` and `nixoriumOfflineCheck` for base-update validation;
- `nixorium` and supporting Flake applications;
- `pxeFirmware`, `installerBundle`, and the target-independent
  `remoteInstallerBundle` for USB/SSH live-ISO installation;
- schema, compatibility, package, and VM checks.

For an unchanged lock and site configuration, a client evaluated through the
private deployment and through the offline installer bundle must produce the
same `system.build.toplevel.drvPath`. Repository validation enforces this
invariant.

## Agent skills

The public repository contains two different Agent Skills:

- [`nixorium-developer`](../skills/nixorium-developer/SKILL.md) governs public
  API, built-in module, installer, CI, and release work.
- [`nixorium-maintainer`](../skills/nixorium-maintainer/SKILL.md) governs one
  private laboratory's configuration and operation.

Only the maintainer skill is copied into generated deployment repositories.

## Optional controller telemetry

The built-in telemetry module installs only on the controller. It exposes the
installed generation’s version, deployment mode and client-count band in
`/etc/nixorium/telemetry.json`. Its daily systemd service runs as admin and sends
nothing until runtime consent is explicitly enabled. It opens no listening ports
and adds no client-side Internet requirement. See [telemetry](telemetry.md).
