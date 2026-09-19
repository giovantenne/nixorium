# Disposable evaluation environment

This document separates the evaluation path available today from a proposed
automated QEMU harness. It is not a claim that a one-command product demo
already exists.

## Available today

Use disposable virtual machines to evaluate the controller-to-client workflow
without erasing physical hardware:

1. Create one x86_64 controller VM with UEFI, an Internet-connected bootstrap
   adapter, an isolated laboratory adapter, and a disposable disk.
2. Create one x86_64 client VM with UEFI network boot, one adapter on the same
   isolated segment, and a blank disposable disk of at least 20 GiB.
3. Provide ordinary DHCP on the isolated segment. Do not give the client a
   direct Internet route. Nixorium ProxyDHCP must coexist with this service;
   it does not assign normal leases.
4. Boot the controller from the official NixOS Minimal ISO and follow the
   release-pinned quick start. The selected controller disk is erased after
   confirmation.
5. Configure one client in **Installation → Install computers**, prepare and
   start PXE, then boot that client from its UEFI network interface.
6. Run `/installer/setup.sh` on the client, choose its configured identity,
   and confirm the disposable disk locally.
7. After local-disk boot, verify one software declaration and one explicit
   client deployment. Stop PXE and verify that normal controller addressing is
   restored.

The fuller [hardware validation plan](hardware-validation.md) adds a second
client, failure injection, evidence capture, and physical-hardware scenarios.
Passing a VM evaluation does not establish compatibility with a physical
firmware, NIC, switch, or disk controller.

## Why there is no `demo.sh` yet

A useful evaluation harness must exercise the same boundaries as the product.
A script that only renders the terminal interface would demonstrate the UI,
not controller bootstrap, ProxyDHCP, client installation, cache delivery, or
deployment. A reliable harness also needs to account for large Nix closures,
UEFI/PXE behavior, virtualization acceleration, and destructive virtual-disk
selection.

Until these requirements are automated and tested, a one-command script would
hide failure modes rather than reduce evaluation risk.

## Proposed QEMU architecture

The future harness should remain development tooling, separate from the
distributed `nixorium` command:

- one controller VM with a NAT bootstrap uplink and an isolated lab NIC;
- one UEFI client VM attached only to the isolated lab network;
- one explicit DHCP fixture on that segment, independent of Nixorium
  ProxyDHCP;
- qcow2 disks created in a new temporary directory and never resolved through
  a user-supplied broad path or glob;
- release-pinned Nix inputs and cached build artifacts outside the recorded
  walkthrough;
- bounded readiness checks for controller activation, cache HTTP readiness,
  PXE listeners, client installation, local-disk boot, and authenticated
  deployment state;
- automatic cleanup limited to the harness-owned processes and temporary
  directory, with logs retained on failure.

The harness must not silently bypass the product's target review, PXE network
confirmation, or local client disk confirmation. If automation needs a
non-interactive installer, that requires a separately designed token and
private policy model; the public unattended path remains disabled.

## Concrete implementation milestones

1. Prove OVMF boot, isolated DHCP, and ProxyDHCP coexistence in a repository
   test fixture without installing a disk.
2. Build or select a release-pinned controller image while keeping the public
   Minimal ISO path authoritative.
3. Boot a client through the generated iPXE, kernel, and initrd chain and
   capture a bounded readiness signal from the real installer.
4. Complete the locally confirmed install on a disposable qcow2 disk and prove
   the next boot comes from that disk.
5. Apply one harmless reviewed change to the client and authenticate the
   resulting revision.
6. Add failure cases for interrupted PXE state, unavailable client, and mixed
   deployment results before exposing a public convenience command.

The minimum acceptance result is not a video or a successful process exit. It
is evidence that a real controller and client completed the reviewed workflow,
that the client used no Internet route, and that the controller returned to
normal network state.
