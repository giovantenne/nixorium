# USB over SSH P0 qualification

Date: 2026-09-24

Product revision: `06d1ae558dbbe962c2b61018049950dfe0b58473`

Result: passed for implementation entry. This is VM evidence, not physical
hardware certification.

## Qualified inputs

- Official ISO: `nixos-minimal-26.05.10478.1bc55b9def81-x86_64-linux.iso`
- SHA-256: `fc01aaaf63437949988ce7e7264e5e72fcb201b4d405952e5275a475ca643f2d`
- Size: 1,753,006,080 bytes
- Installer build: `26.05.10478.1bc55b9def81`, `VARIANT_ID=installer`
- Architecture/firmware: x86_64, UEFI
- Nix: 2.34.8
- nixpkgs revision: `a5cc6f2c37bf518436dc8d1c288ccd0c43c2f4c4`
- Harmonia: 3.1.0
- nixos-anywhere: 1.13.0

The ISO had active SSH, the `nixos` user (uid 1000, wheel), non-interactive
sudo, and `nix`, `nixos-install`, `lsblk`, `findmnt`, `systemd-run`, `jq`, and
`curl`. The observed live Nix store capacity was 1,522,139,136 bytes.

## Recorded tests

| Boundary | Evidence | Result |
|---|---|---|
| First trust | Ed25519 fingerprint read on the VM console and placed in a dedicated known-hosts file; all later OpenSSH calls used strict checking and a single explicit identity | Pass |
| Credential transition | Root key login worked after `passwd -l nixos`; a new password login was refused; the existing local console retained passwordless sudo | Pass |
| Signed cache | With client Internet blocked, Harmonia served the bundle using `nixorium-p0-1:v5IITg4lBHYhnvhBoOpixAD+ge6cAzVX8EVff7LupsI=`; Nix signature verification succeeded | Pass |
| Wrong cache key | Import failed and the requested store path remained absent | Pass |
| Bundle sizing | Missing bundle closure: 9 paths, about 260.9 MiB; live-store use after import was about 398 MiB (28%) | Pass |
| Direct target install | The 7,779,324,720-byte desktop closure was absent from the live store and installed into `/mnt/nix/store` from Harmonia | Pass |
| Remote job | A transient unit continued after the dispatching SSH connection was killed, wrote a `completed` receipt, and rejected replay of the same 128-bit ID | Pass |
| Runtime independence | The first unit failed on `/usr/bin/env bash`/ambient `PATH`; the corrected unit with store-qualified shell and commands passed | Pass; binding requirement recorded |
| SATA | Disko produced GPT + 512 MiB FAT ESP + Btrfs subvolumes; installed system booted from `/dev/sda2[/@root]` | Pass |
| NVMe/multi-disk | Disko targeted `/dev/nvme0n1` (serial `NIXORIUM-P0-NVME`); the SATA partition-table digest was unchanged | Pass |
| Live-media exclusion | Official ISO was `/dev/sr0`, read-only/removable and mounted at `/iso`; the only selected writable disk was explicitly distinct | Pass |
| Host identity | Live and installed fingerprint both equalled `SHA256:5CHvAygvjxcoBUD2Xs1RNMijKI/YnTO0fPXpO24yrkM` | Pass |
| Installed identity | Strict SSH with the deployment admin key returned hostname `pc01`, the prepared system path, and revision `06d1ae558dbbe962c2b61018049950dfe0b58473` | Pass |
| Ephemeral-key cleanup | The live bootstrap key was rejected by the installed target while the deployment admin key succeeded | Pass |
| USB/PXE gate | Direct transient systemd starts in both orders admitted one lock owner only; SIGKILL released the flock but the persistent USB marker continued blocking new work until explicit reconciliation | Pass |

The installed network exposed both the declared static address `10.0.0.1/24`
and a DHCP address because the current client module intentionally enables DHCP
alongside the static address. USB preflight must reject a live DHCP address that
equals a selected static identity or controller address and must bind the
declared NIC; it must not interpret the second address as a new identity.

## nixos-anywhere comparison

The package built from the pinned package set. Its own help reports default
phases `kexec,disko,install,reboot`, direct `--store-paths`, ISO-oriented
`--no-disko-deps`, and optional host-key copying. The wrapped 1.13.0 program
initializes SSH with `IdentitiesOnly=yes` but also
`UserKnownHostsFile=/dev/null` and `StrictHostKeyChecking=no`. It is useful
feasibility evidence, but those defaults and combined phases do not satisfy the
Nixorium trust/review/reboot contract. Nixorium therefore implements the small
reviewed helper described by ADR 0021.

## Important observations

1. A negative Nix binary-cache lookup is cached locally. Test order must
   register controller store paths before the first client query, or clear only
   the isolated test cache before retrying; production preparation must verify
   every closure reference before publishing review readiness.
2. A script file is not a runtime bundle. On the installed desktop the same
   precompiled Disko script lacked `jq`, `sgdisk`, and `partprobe`; adding the
   pinned runtime closure made the NVMe run pass.
3. A VM installed while the disk used virtio could not boot that same image as
   virtio because the generic client initrd did not include virtio-blk. SATA
   boot and NVMe target operations passed and define the v1 VM qualification.
4. A QEMU direct `-kernel/-initrd` launch cannot prove post-install boot; the
   post-boot proof used disk-only OVMF/GRUB.
5. The controller cache secret was delivered at runtime under `/run`, mode
   0600. It was never embedded in a derivation or Nix store path.

## Reproduction boundary

[`tests/usb-ssh-p0.sh`](../../tests/usb-ssh-p0.sh) verifies the immutable ISO,
pinned nixos-anywhere behavior, strict post-boot identity, and the operation
gate using caller-supplied local VM endpoints. Password bootstrap remains an
interactive console/TTY step by design and the script never accepts a password
argument or environment variable. The destructive SATA/NVMe cases must use
disposable images and explicit `NIXORIUM_P0_ALLOW_DISPOSABLE_VM=1`; the script
does not accept host block devices.

No physical disk, lab deployment, push, tag, release, or website publication
was performed.
