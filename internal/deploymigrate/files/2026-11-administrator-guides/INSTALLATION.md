# Installation

How to install the controller and the student computers. Install and check
**one** student computer before the rest of the room.

## Requirements

- **Wired network.** The controller and the student computers share the
  laboratory network, with an Ethernet cable on every computer. Wi-Fi is not a
  laboratory network: no network boot, no Switch on, and an installed computer
  never takes its laboratory address on a Wi-Fi card.
- **Internet on the controller** for preparation and updates. The student
  computers need no Internet: the controller gives them software and the time.
  The school firewall must let the controller reach Internet time servers
  (NTP).
- **Laboratory addresses.** The laboratory uses its own range of addresses on
  the same cable, beside the addresses the school's DHCP server hands out.
  Choose a range that the school network does not use.
- **Network boot (PXE)** also needs UEFI network boot on the student computers
  and permission to run ProxyDHCP: the controller answers network-boot
  requests without handing out addresses. Ask whoever runs the school network.
  **USB over SSH** needs neither.
- **UEFI and disks.** Every computer starts in UEFI mode. The disk you confirm
  is erased completely. The network installer offers only disks with room for
  the system plus 2 GiB.
- **A screen and keyboard** on each computer during installation. On a
  computer with 4 GB of RAM or less, use the Minimal ISO.
- **Free space on the controller**: every update adds a system version (see
  [Free disk space](MAINTENANCE.md#free-disk-space)).

## Install the controller

Start the official [NixOS ISO](https://nixos.org/download/#nixos-iso) in UEFI
mode, Minimal (text console) or Graphical (GNOME), with Internet, and run:

```sh
curl -fsSL https://nixorium.org/install.sh | bash
```

It first asks which Nixorium version to install (Enter keeps the latest
release), then:

1. **Keyboard**, chosen from a list and applied at once, before any password.
   The installed system uses the same layout on its console and login screen.
2. **Time zone**, then the teacher and student names and the three passwords.
3. **Software profile** to start with (see
   [Software](SOFTWARE.md#software-profiles)).
4. **Disk**, erased only after you type `ERASE`.

Use the Graphical ISO when a web browser is needed to unlock a captive
Internet portal: connect, unlock it in Firefox, then open Terminal and run the
command. Ignore the "Install NixOS" icon.

The installer creates this private repository and pins Nixorium and the
package base to one exact version. After the restart, sign in as `admin` and
open Nixorium: the controller is already usable on its own. The first
preparation in **Installation** asks for the laboratory settings.

### Before installing student computers

- **Controller number.** The controller is `pc99` by default, with the
  matching address in the laboratory network. To change it (for example to
  `pc42`), use **Maintenance → Change settings → Computers → Controller host
  number** now, then apply to the controller. The number must be greater than
  the number of student computers. Once student computers are configured, this
  number, the laboratory network and its prefix are locked (code
  `NETWORK-LOCKED`): a new network goes through
  [Change the laboratory network](MAINTENANCE.md#change-the-laboratory-network).
  Do not lower the number of computers to get around the lock.
- **Network cards.** Leave the client interface name empty (the default): each
  student computer then uses its wired card whatever its name, so different
  models work without any setting ("Client computers' network interface" in
  the settings). Give a name only when every student computer has the same
  card name. A computer with more than one wired card needs its own card name
  (`hostIfaceNames` in `lab-settings.json`, see
  [Change settings](MAINTENANCE.md#change-settings)); a USB installation
  offers to save it. Wi-Fi card names (`wl…`) are refused.

## Network boot (PXE)

**Installation → Network boot (PXE)** does everything on one screen. Opening
it only checks; Enter offers the next step:

1. **Prepare.** The first time, it asks for the laboratory settings, creates
   the keys, applies the controller and builds the installation files. Later
   it skips what is already current. Preparing changes nothing on the network
   and can be repeated.
2. **Start.** After a review, the controller answers network boot requests.
   This changes the controller's network for the duration.
3. **Install each computer** (below).
4. **Stop** when you have finished. This restores the controller's normal
   network. `q` while PXE runs asks whether to leave it running.

If installation was interrupted, the screen opens as **Recover the controller
network**: Enter repairs it, changing only the controller. The controller also
repairs it by itself when it restarts.

Prepare again after any saved change or update before installing more
computers: the installation files carry the configuration they were built
from.

### On each student computer

Start the computer from the network. The installer opens on its screen:

1. It shows the hardware and the disks, and asks which configured computer
   this is. A name that already answers on the network is refused.
2. It shows the computer and the disk and erases the disk only after you type
   `ERASE`. A wrong answer is asked again; an empty answer cancels without
   touching the disk.
3. It installs, verifies, and offers a restart.

To start the installer again on that computer: `sudo /installer/setup.sh`.

```sh
nixorium pxe prepare
nixorium pxe start
nixorium pxe stop
nixorium pxe recover
journalctl -u nixorium-prepare-pxe.service   # full build output, if needed
```

## USB over SSH

Use this when a computer cannot start from the network. You need the official
**NixOS 26.05 ISO** (Minimal or Graphical, `x86_64-linux`) on a USB stick.

1. Start the computer from the USB stick in UEFI mode. Prefer the laboratory
   cable. Wi-Fi (`nmtui`) works only during the installation, and only for a
   computer without its own card name. Connect the cable before the restart:
   the check afterwards needs it.
2. At its console, set a temporary password and read the address and the key
   fingerprint:

   ```sh
   passwd
   systemctl is-active sshd
   ip -4 -br address
   ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub
   ```

   Keep this screen visible.
3. On the controller, **Installation → USB over SSH**: choose which configured
   computer this is and type the address.
4. Compare the `SHA256:` fingerprint Nixorium shows with the one on the
   computer's screen, type `MATCH`, then the temporary password.
5. Choose the disk; it is erased only after you type `ERASE`. Do not remove
   the USB stick or restart while it installs; the screen shows the current
   step.
6. When it says so, remove the USB stick and confirm the restart. Nixorium
   then checks the installed computer by itself (**Verify now** checks again).

If the computer is connected through a card with a different name from the
configured one, Nixorium shows both names and offers to save the observed card
for that computer only. It then starts again from step 2: set a new temporary
password, because the previous one was locked. No disk is touched.

The password is used once, kept only in memory and never stored. Wi-Fi
settings are not copied into the installed system.

```sh
nixorium install usb prepare --host pc01   # optional, prepares in advance
nixorium install usb start --host pc01
nixorium install usb status --id 0123456789abcdef0123456789abcdef
nixorium install usb reconcile --id 0123456789abcdef0123456789abcdef
nixorium install usb reboot --id 0123456789abcdef0123456789abcdef
nixorium install usb verify --id 0123456789abcdef0123456789abcdef
nixorium install usb cancel --id 0123456789abcdef0123456789abcdef
nixorium install usb close --id 0123456789abcdef0123456789abcdef
```

The `--id` is the one `start` prints for that installation. `start`, `reboot`
and `close` need an interactive terminal. When an
installation stops halfway, `status` says what Nixorium found (does the
computer answer, was the disk touched) and the command for each way out;
see [A USB installation was interrupted](TROUBLESHOOTING.md#a-usb-installation-was-interrupted).
Nixorium never repeats a disk erase or a restart whose outcome is uncertain.

## Reinstalling a computer

A reinstalled computer has a new SSH key, and the controller refuses it until
you confirm the change. The USB installation shows it and asks you to type
`ROTATE HOST KEY` after checking the computer, the fingerprint and the disk.
After a network installation, use the computer's details in **Computer
inventory**, or `nixorium host-key plan` and `host-key apply`; see
[A client was deliberately reinstalled](TROUBLESHOOTING.md#a-client-was-deliberately-reinstalled-and-its-ssh-key-changed).

## Dual boot

Installation erases the whole disk chosen for Nixorium, including a Windows
installation on that same disk. A system on **another** disk, such as Windows,
is added to the boot menu so students can start it from there. For it to be
found:

- turn off the firmware's **Fast Boot**, which can hide the other disks;
- turn off Windows **Fast Startup** and shut Windows down fully;
- turn **Secure Boot** off.

The entry appears after the next update or installation of that computer.
