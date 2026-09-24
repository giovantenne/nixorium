# ADR 0021: Reviewed USB installation over verified SSH

Status: accepted; the qualification baseline is recorded in
[`docs/qualification/usb-ssh-p0.md`](../qualification/usb-ssh-p0.md).

## Context

PXE remains the efficient installation transport for a laboratory, but it is
not available on every firmware, NIC, or managed network. The existing client
installer already owns the NixOS system closure and shared Disko layout. A
second transport must reuse those artifacts without weakening disk review,
binary-cache signatures, or machine identity.

The official NixOS Minimal ISO provides SSH, password setup for its `nixos`
user, passwordless sudo, Nix, and the low-level installation tools. It does not
provide a stable application protocol, persistent operation state, or a safe
disk choice. Tools merely found in the live image are also not a sufficient
runtime contract: the P0 test reproduced a missing `jq`, `sgdisk`, and
`partprobe` as soon as the same Disko script ran outside the installer image.

`nixos-anywhere` 1.13.0 from the pinned package set proves the general ISO and
store-path workflow. Its default combined `kexec,disko,install,reboot` phases,
temporary key handling, `UserKnownHostsFile=/dev/null`, and
`StrictHostKeyChecking=no` do not implement Nixorium's physical fingerprint
check, content-bound review, separate reboot, or conservative reconciliation.

## Decision

Add `usb-ssh` as a second reviewed transport. The operator boots the official
NixOS Minimal ISO in UEFI mode, reads its IPv4 address and Ed25519 fingerprint
from the local console, and enters those values plus a temporary password in
Nixorium. The initial Go SSH client verifies the fingerprint before sending the
password. It checks the supported installer facts, installs and verifies one
ephemeral root key, and then locks the live user's password.

All later connections use a private, operation-scoped key and known-hosts file
with strict Ed25519 pinning. The password is an in-memory bootstrap value, not
part of a serializable DTO, argument, environment variable, manifest, report,
or log. Controlled buffers are cleared promptly; Go and terminal-library string
copies are minimized but are not represented as formally zeroizable memory.

The deployment exports an immutable remote-installer bundle. It contains the
precompiled shared Disko layout and its complete, store-qualified runtime. The
client imports that bundle only from the selected Harmonia endpoint with the
deployment public key, required signatures, and fallback disabled. The desktop
system is installed directly into the mounted target store; it is not staged in
the live ISO store.

Probe and plan are non-destructive. A disk is eligible only after excluding the
live medium, mounted/swap/held devices, read-only and undersized devices, and
uncertain ancestry. Review binds the operation, boot ID, deployment revision,
system and bundle paths, cache endpoint/key, host key, logical identity, NIC,
and a disk identity tuple. Apply rechecks the tuple immediately before Disko.

The controller worker and the remote systemd job own the operation, not the
TUI or an SSH process. The remote helper reserves an operation ID before
mutation, writes a bounded receipt, refuses replay, and never restarts Disko.
Transport loss after dispatch becomes `reconciliation-required`; it never
causes an automatic second apply. Reboot is a separate reviewed action.

The verified live Ed25519 host key is copied to the target. Post-boot
verification uses that pin and the deployment admin key to check the exact
hostname, configuration revision, and `/run/current-system` path. Only this
check marks the boot verified.

One controller-wide lock at
`/var/lib/nixorium/coordination/operation.lock` coordinates USB installation,
PXE network transitions, deployment, shutdown, controller activation, cache
restart, and secret installation. A persistent USB reservation survives worker
failure. Direct systemd entry points participate in the same gate; changing
`HOME` or `XDG_STATE_HOME` cannot create a second lock domain.

## Trust boundaries

- Physical console observation establishes the first SSH host-key pin.
- The temporary password authenticates only the supported live `nixos` user.
- The ephemeral root key authenticates the live session after bootstrap.
- Harmonia signatures authenticate bundle and system NARs; SSH transport alone
  is not treated as a substitute for Nix signatures.
- The Git revision and deployment lock select the immutable build inputs.
- Explicit disk review and immediate revalidation authorize destruction.
- The declared admin key plus preserved host key authenticate post-boot state.

No arbitrary username, port, SSH option, command, cache URL, host identity, or
disk path crosses from presentation into a privileged command. IPC and helper
schemas are versioned, bounded, strict, and reject unknown or duplicate fields.

## Alternatives

- Keep PXE mandatory: rejected because it excludes otherwise supportable UEFI
  clients and managed networks.
- Ship a custom installer ISO: deferred; it adds a release and trust artifact
  and is unnecessary for the first supported flow.
- Invoke `nixos-anywhere` unchanged: rejected because its safe defaults target a
  different unattended workflow and combine decisions Nixorium must separate.
- Copy unsigned local store paths over SSH: rejected. The canonical transfer is
  the signed Harmonia response.
- Run Disko directly in the SSH session: rejected because disconnects make the
  result ambiguous and encourage unsafe retries.
- Store operation state only under `/run`: rejected because controller or
  worker failure could silently release a destructive reservation.

## Consequences

USB installation requires local presence for boot, address/fingerprint
observation, and media removal. Wired networking, x86_64, UEFI, the qualified
NixOS Minimal ISO, SATA/NVMe targets, and one operation at a time are the v1
support boundary. Wi-Fi, arbitrary installers, Secure Boot certification,
parallel installation, and unattended discovery remain out of scope.

The implementation is larger than a wrapper around an installer command: it
adds a bundle protocol, worker state, operation receipts, strict SSH bootstrap,
global coordination, disk identity checks, and post-boot verification. In
return, PXE and USB converge on the same configuration, Disko layout, signed
cache, and review model.

## Qualification consequence

The P0 run proved the official ISO facts, signed offline Harmonia transfer,
wrong-key rejection, direct target-store installation, an SSH-independent and
idempotent remote unit, preserved host identity, SATA boot, NVMe Disko, and a
crash-persistent coordination marker. It also established two implementation
requirements: remote units use store-qualified executables, and the bundle
must carry the complete Disko runtime closure. Physical hardware remains a
separate beta qualification and is never inferred from the VM evidence.
