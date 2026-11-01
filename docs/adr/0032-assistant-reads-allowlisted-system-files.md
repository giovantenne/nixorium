# ADR-0032: Assistant reads allowlisted system files

- Status: accepted
- Date: 2026-10-19
- Related: [ADR 0031](0031-client-deployment-engine.md); the Assistant
  diagnostics described in `docs/assistant.md`

## Context

Administrators ask Assistant for changes that depend on a computer's own
files: which entry of the boot menu is Windows, how a printer or a network
card is configured, what an installed service wrote in `/etc`. Assistant's
diagnostics read fixed service state and journals only, so it had to ask the
operator to run commands and paste the output. The owner asked for Assistant
to read such files itself, and considered letting it read any file of the
disk.

Reading any file was rejected. The disks hold students' homes (often of
minors), secrets (account password hashes, SSH host keys, the cache signing
key, Wi-Fi passwords) and text written by students that could carry
instructions to the model. Everything Assistant reads goes to the service and
its AI provider.

## Decision

A new tool, `read_system_file`, reads one path of one computer declared in
fresh inventory, after a new disclosure (consent text version 3) and only on
controllers that advertise `system_files_v1`:

- Readable locations (`internal/systemfiles`): `/etc`, `/boot/grub`,
  `/boot/loader`, `/proc/cmdline`, `/proc/cpuinfo`, `/proc/meminfo`,
  `/proc/mounts`, `/proc/partitions`, `/proc/swaps`, `/proc/version`,
  `/sys/class/net`, `/sys/class/dmi/id`, `/var/lib/nixorium`, `/run/nixorium`.
- Refused even there: shadow files, private SSH keys, NetworkManager and
  wpa_supplicant connections, PPP and IPsec secrets, `sudoers.d`, Nixorium
  keys, Assistant, telemetry and backup state, remote installation state,
  device serial numbers, and any name containing `secret`, `private`, `token`,
  `password`, `credential`, `shadow`, `psk`, `snapshot`, `recovery` or a key
  file suffix. Public `.pub` keys stay readable.
- The requested path must be a plain absolute path of common characters,
  without `.` or `..` parts. Links are resolved: the target must itself be
  readable, or be in the Nix store (where NixOS keeps what `/etc` links to),
  or be one of the few system locations a readable path legitimately links
  to: `/sys/devices` for `/sys/class/net` and `/sys/class/dmi/id`,
  `/proc/PID/mounts` for `/proc/mounts` and `/etc/mtab`, `/var/lib/cups` for
  `/etc/cups`, `/run/systemd/resolve` for `/etc/resolv.conf`. The refusals
  apply to the target too, and the `/etc` refusals also hold through
  `/etc/static` and its store copy. `/etc/machine-id`, garbage-collector
  roots of prepared systems and device serial numbers are refused. Names
  containing `passw` are refused on purpose, so `/etc/passwd` is not readable.
- A file returns at most 256 KiB of text; a binary file returns only that
  fact. Lines that set a password, passphrase, pre-shared key, token or key
  keep their first word and are otherwise withheld, and the `user:password@`
  part of URIs (printer queues) is withheld. A directory returns up to 500 readable entries and the
  number of refused ones, never their names.
- The fixed helper `nixorium-host-diagnostics file PATH` runs on a client as
  root through the existing strict SSH trust (no key enrolment or update), and
  on the controller as the administrator. The controller re-validates every
  answer: the same path, a readable resolution, bounded content, readable
  listing entries only, no unknown fields.

## Consequences

- Assistant can resolve boot, network, printer and service questions without
  the operator copying files.
- Everyone who uses Assistant is asked to consent again once.
- On the controller, root-only files (for example the boot menu on the EFI
  partition) are reported as not readable.
- Widening the allowlist is a privacy change: it needs a new disclosure
  version and an amendment of this record.
