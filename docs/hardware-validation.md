# Nixorium hardware and VirtualBox validation plan

This plan validates behavior that unit and NixOS VM tests cannot prove about
real firmware, network adapters, switches, disks, and multi-machine timing. A
scenario is not passed until its evidence is recorded from the named machines.

For the first controller, client, and software change, start with the
[isolated VirtualBox recipe](evaluation-environment.md). Return here for the
broader validation matrix and physical compatibility work.

Disk installation erases the selected target. Use disposable VirtualBox disks
or dedicated test hardware with verified backups. Never run enrollment against
a disk containing data that must be retained.

## Reported deployments and compatibility evidence

| Deployment | Evidence | What it establishes |
|---|---|---|
| Original Italian school lab: 30 student workstations + 1 controller (31 machines) | Maintainer's public post and [adapted account](https://nixorium.org/case-study/original-classroom/) | The author's original operating experience; not independent validation or a current-release test |

The public account reports reinstalling the lab in **less than 20 minutes**;
the maintainer later clarified the observed time as **approximately 15 minutes**.
The exact hardware inventory, cache/build conditions, timing method, and
software chronology remain undocumented. Do not turn the observation into a
benchmark or a known-working hardware-model entry. The account does not
establish additional external deployments or duration of use.

No model-specific compatibility pass is recorded here. Add named hardware only
with the run evidence below. Keep reported deployments, virtual tests, and
physical compatibility reports distinct in the same record. Share redacted
reports through existing [Discussions](https://github.com/giovantenne/nixorium/discussions)
and reproducible failures through [Issues](https://github.com/giovantenne/nixorium/issues).

The initial VirtualBox recipe documents its own limited authoring checks; the
runtime scenarios in the result matrix remain **NOT TESTED** until executed.

## Evidence record

Create one record per run with:

- date, operator, Nixorium release/tag and private deployment Git revision;
- controller model/firmware, NIC model/driver, and laboratory interface name;
- DHCP server/switch/VLAN topology and whether clients have an Internet route;
- each client model, UEFI version, NIC, target disk type/size, and assigned name;
- exact result for every scenario below, including skipped and failed steps;
- relevant `nixorium` text/JSON reports and operation-log IDs;
- relevant bounded journal excerpts with secrets removed;
- recovery action and final observed state after every failure.

Do not attach private keys, password hashes, complete environment dumps, or raw
state directories to public reports.

## Required topologies

### VirtualBox topology

Use one controller and at least two disposable client VMs.

- Give the controller a management adapter for bootstrap access if needed.
- Put a separate controller adapter and both client adapters on one isolated
  laboratory segment. That segment needs an independent DHCP server which may
  coexist with Nixorium ProxyDHCP; it must give the controller lab adapter the
  configured `masterDhcpIp`.
- Do not provide the clients an Internet route. The controller may retain its
  separate management uplink.
- Enable EFI and network boot on both clients. Give each client a blank disk of
  at least 20 GiB. Vary the emulated NIC or storage controller where practical.
- Take hypervisor snapshots only as test-fixture recovery. Product pass/fail
  must be determined from Nixorium and guest state, not snapshot rollback.

Record the VirtualBox version and exact adapter/network modes. If the built-in
PXE firmware cannot interoperate with ProxyDHCP, record that limitation and use
a small iPXE ISO/ROM only as a separately identified compatibility scenario;
do not silently treat it as the default-firmware pass.

### Physical topology

Use a dedicated test VLAN or isolated switch with the institution's DHCP
behavior represented. Start with one disposable client, then add at least one
different firmware/NIC family and a second simultaneous client. Include both a
SATA/SCSI-style disk name and NVMe when hardware is available.

Use wired Ethernet for PXE, including when VirtualBox participates through a
physical bridged adapter. Wi-Fi and wireless bridging can prevent PXE discovery
or boot; do not use them as the baseline. The isolated VM recipe uses virtual
Ethernet on an Internal Network and does not bridge DHCP/ProxyDHCP to the LAN.

Before testing, identify the switch recovery path and confirm that stopping PXE
cannot remove ordinary DHCP service. Keep console access to the controller in
case the laboratory interface transition interrupts SSH.

### USB/SSH topology

For each VirtualBox and physical family, repeat installation from the official
NixOS 26.05 Minimal ISO for `x86_64-linux`, in UEFI mode over wired Ethernet.
Keep the physical/local console visible so the address and Ed25519 fingerprint
are independent evidence for the controller's automatic host-key observation.
Attach exactly one disposable SATA/SCSI or NVMe
target plus the boot medium; include a multi-disk case to prove that the review
does not infer a target. Clients must reach the controller's SSH and signed
Harmonia endpoints but need no Internet route. PXE must remain stopped during
this scenario.

## Baseline capture

From a clean private deployment revision:

```sh
git status --short
nixorium status --json
nixorium doctor --json
nixorium setup status --json
nixorium services --json
```

Pass when the repository is clean, configuration is ready, the signed cache is
healthy, PXE is stopped, and no unexplained recovery state is present. Resolve
baseline failures before any destructive scenario.

## Scenario 1: controller bootstrap and first run

1. Install the controller through the documented release-pinned bootstrap.
2. Reboot without the installation medium and sign in as `admin`.
3. Confirm `nixorium` and the private deployment are available.
4. Open **Installation → Install computers**, complete Laboratory settings,
   verify that time zone and keyboard are not requested again, and let the flow
   create missing keys and activate the controller.
5. Before starting PXE or rebooting, verify that the configured static
   laboratory address is present on the selected interface.
6. Reboot once more and run `nixorium setup status` and `nixorium doctor`.

Pass when setup is complete, the active controller and durable receipt match
the reviewed revision, the configured static laboratory address is present
after live activation, Harmonia is HTTP-ready, and no reboot or manual
Nix/systemd edit was needed to make that address appear. Record failures and
retry behavior rather than reinstalling first.

## Scenario 2: PXE preparation and listener lifecycle

```sh
nixorium pxe prepare
nixorium pxe start
nixorium services
nixorium doctor
```

Boot one client through its default UEFI network firmware. Confirm it downloads
the generated iPXE path, kernel and initrd and reaches guided enrollment. Then
run `nixorium pxe stop` twice.

Pass when preparation is revision/current-address bound, the client reaches the
installer, both stops succeed, ordinary DHCP remains available, and the exact
controller static address is restored without removing unrelated addresses.

## Scenario 3: interrupted PXE recovery

With PXE active, power off the controller abruptly through the hypervisor or
physical power-control procedure. Start it again without modifying its disk.

Pass when boot recovery restores normal controller addressing before routine
operation, archives the interrupted session, leaves PXE stopped, and
`nixorium pxe recover` is then an idempotent no-op/success. Record console and
network observations; do not repair addresses manually before observing boot.

## Scenario 4: guided client disk installation

On one blank client, run `/installer/setup.sh`, choose a configured unused host,
and select the disposable target disk. Exercise one incorrect confirmation
before entering the exact `ERASE <disk> INSTALL <host>` phrase.

Pass when the wrong phrase causes no disk mutation, closure/capacity checks run
without client Internet access, installation completes from prepared content,
and the client reboots from disk with the chosen hostname and mounted root.

Repeat on a different disk/NIC/firmware family for physical coverage. An
interrupted real disk installation is not blindly retry-safe: inspect and
reinitialize only the dedicated test disk before repeating.

## Scenario 4B: reviewed USB/SSH installation and recovery

Boot the supported Minimal ISO, set its temporary password, and record the live
IPv4 address, Ed25519 fingerprint, boot ID, NIC, boot medium, and all disks from
the local console. From the controller start USB/SSH installation for one
configured unused identity, enter only the address, and compare the
automatically observed fingerprint before entering the password. Exercise each
refusal in a fresh disposable run:

- present a different SSH host key after the physical comparison and prove
  password authentication and key installation do not occur;
- use a wrong password after the correct fingerprint and prove no operation key
  remains;
- make Harmonia unreachable or present a wrong cache key and prove Disko does
  not start;
- select the ISO boot medium, an ineligible disk, the wrong NIC, and an unknown
  disk path;
- interrupt the cable before apply, after apply dispatch, and after Disko has
  reported mutation started;
- restart the controller worker before apply and the controller after dispatch;
- change the DHCP lease or boot a new ISO instance before attempting recovery;
- reinstall an existing identity with a reviewed host-key rotation; and
- repeat with two eligible target disks, SATA/SCSI-style naming, and NVMe.

Pass when the physical fingerprint is checked before password use, only the
explicitly reviewed non-boot disk can mutate, closure transfer remains signed
and offline, PXE cannot start concurrently, and status survives the initiating
terminal. Pre-apply cancellation must cleanly release the reservation. Once
dispatch is uncertain, reconciliation must retain the same operation ID and
same live boot, report disk risk, and never run Disko a second time. A new boot
or lease/address mismatch must fail closed. Successful completion requires USB
removal, a separately confirmed reboot, installed-host revision verification,
ephemeral-key removal, and any approved static-address host-key rotation only
after verification.

## Scenario 5: client visibility and single-client deployment

After the installed client boots:

```sh
nixorium hosts --json
nixorium deploy plan --on pc01
nixorium deploy apply --on pc01 --expect REVISION_FROM_PLAN
nixorium hosts --json
```

Pass when the client is reachable/authenticated, the plan selects only that
host, build precedes activation, the result is durably logged, and fresh host
observation reports the reviewed revision/current system rather than trusting
only the Colmena exit status.

## Scenario 6: offline and changed-host-key diagnostics

Power off one client and run `nixorium hosts`; attempt a plan/apply that includes
it only after recording the expected diagnostic. Power it on and restore normal
state. Separately change a disposable VM's SSH host identity.

Pass when an offline host is reported unreachable/unknown without blocking
inspection of other hosts, deployment failure is honest about possible mixed
state, logs remain available, and a changed key fails authentication without
automatically deleting or accepting the stored identity.

## Scenario 7: multi-client deployment

Boot at least two installed clients, create one reviewed harmless configuration
change, and use:

```sh
nixorium deploy plan --on @lab
nixorium deploy apply --on @lab --expect REVISION_FROM_PLAN
nixorium hosts --json
```

Pass when canonical configured targets are shown before confirmation, every
target builds before apply, every client reports the reviewed revision and a
concrete active path, and the authenticated success history matches the live
result. Repeat once with one client unavailable to validate mixed-state recovery
through a fresh plan and retry. In a disposable iteration, close the initiating
terminal during apply; pass when the private log remains readable, `hosts`
reports only authenticated live state, and a fresh plan can converge the lab
without an implied automatic rollback.

## Scenario 8: DHCP coexistence and lease change

With ordinary DHCP active, prove a client still receives its normal lease while
Nixorium PXE is stopped and while ProxyDHCP is active. Then change the
controller's DHCP lease without changing the deployment.

Pass when PXE preparation captures exactly one new non-static address without a
deployment edit, the generated iPXE command passes that address to the client,
and ordinary DHCP behavior remains intact. Add a second non-static address and
repeat: preparation must refuse the ambiguity without replacing the prior valid
manifest. Remove the extra address and verify normal preparation resumes.

## Scenario 9: update and controller recovery

Run `nixorium update check` only with controller Internet access, then use an
explicit tagged release for plan/apply. Review and commit the two files
separately, make a controller plan, and apply it.

Pass when clients never need Internet, update mutation is limited to
`flake.nix`/`flake.lock`, representative builds precede acceptance, and
activation completion requires the matching durable receipt. Inject or capture
a safe activation failure where feasible; a fresh retry must not reuse stale
success evidence.

## Cleanup and final evidence

```sh
nixorium pxe stop
nixorium pxe recover
nixorium services
nixorium doctor
nixorium hosts --json
git status --short
```

Pass the run only when the controller has normal addressing, PXE listeners are
stopped, Harmonia is healthy, installed clients have explained states, and all
Git changes are intentional. Preserve operation-log IDs and the completed test
matrix. Destroy only disposable VM disks; removal of physical data or backups
is outside this plan.

## Result matrix

| Scenario | Environment | Result | Evidence / issue |
|---|---|---|---|
| Controller bootstrap / first run | VirtualBox / physical | NOT TESTED | |
| UEFI PXE boot / clean stop | VirtualBox / physical | NOT TESTED | |
| Controller power-loss recovery | VirtualBox / physical | NOT TESTED | |
| Client disk install / reboot | VirtualBox / physical | NOT TESTED | |
| USB/SSH install / interruption / recovery | VirtualBox / physical | NOT TESTED | |
| Client visibility / single deploy | VirtualBox / physical | NOT TESTED | |
| Offline and changed-key diagnostics | VirtualBox / physical | NOT TESTED | |
| Multi-client deploy / mixed-state retry | VirtualBox / physical | NOT TESTED | |
| DHCP coexistence / lease change | VirtualBox / physical | NOT TESTED | |
| Tagged update / controller recovery | VirtualBox / physical | NOT TESTED | |
