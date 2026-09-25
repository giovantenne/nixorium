# Evaluate Nixorium in VirtualBox

Use one disposable controller and one client to follow the official installer,
boot the client from its own disk, and deploy one software change. This recipe
sets up the virtual environment; the [quick start](../README.md#quick-start)
and [administrator guide](../templates/site/README.md) in the exact checkout
under test remain the installation and operating references.

## Scope and verification status

Recipe target: the exact clean Nixorium checkout being evaluated, on Linux
`x86_64` with VirtualBox 7.2.16. Record `VERSION` and the full Git commit before
starting; the USB/SSH scenario requires a revision that exposes
`nixorium install usb`. This is an evaluation of the beta workflow, not the
earlier stable line or an unrecorded moving branch.

**Not tested end to end.** The virtual DHCP configuration commands were
executed in a temporary VirtualBox configuration and inspected with VirtualBox
7.2.16. Installation instructions and network settings were checked against the
source. No controller/client VM was booted, installed, or deployed:
the authoring environment did not expose `/dev/vboxdrv`.
DHCP lease delivery, UEFI/ProxyDHCP interoperability, disk installation, software
deployment, and guest shutdown still need a recorded run. Expected results
below are acceptance criteria, not observed successes.

A VM run cannot establish physical firmware, NIC, switch, or disk compatibility.
Use the existing [hardware validation plan](hardware-validation.md) for that
next step and for the evidence format.

## Host and prerequisites

- A Linux x86_64 host with working VT-x/AMD-V and VirtualBox **7.2.16**, including
  its matching kernel driver. Run `VBoxManage --version` and confirm the host can
  start disposable VMs. Do not change firmware or kernel settings as part of
  this walkthrough if that would disrupt other work.
- Budget 24 GiB host RAM, four or more CPU threads, and at least 160 GiB free
  storage for the two growing virtual disks, ISO, and build contents. These
  are recipe allocations, not measured product minimums. Large software
  closures may need more.
- Download the official **NixOS 26.05 Minimal ISO, x86_64**, from
  [NixOS downloads](https://nixos.org/download/#nixos-iso). Verify its published
  SHA-256 and record the exact filename and digest. Use the Minimal text console,
  not a terminal inside the graphical installer.
- Internet access on the host for the controller's NAT adapter. Builds and
  downloads are not assigned a completion-time estimate.
- Local access to both VM consoles. Use a new VM folder for this evaluation;
  attach no raw host disks, physical disks, shared folders, or existing VM disks.

**Ethernet for physical PXE:** when a test involves physical PCs or a
VirtualBox adapter bridged to a physical network, use wired Ethernet, not
Wi-Fi; PXE may fail over Wi-Fi or a wireless bridge. The recipe below uses
only an internal virtual provisioning network: host Wi-Fi, if used for the
controller's Internet uplink, does not carry the PXE exchange. Do not switch
this recipe to bridged networking to troubleshoot a boot failure.

## 1. Create two VMs — on the host

Use VirtualBox Manager as your normal user. Choose Linux / Other Linux (64-bit),
skip unattended installation, and create these new VMs:

| Setting | Controller | Client |
|---|---|---|
| Name | `nixorium-eval-controller` | `nixorium-eval-client` |
| RAM / CPUs | 8192 MiB / 4 | 4096 MiB / 2 |
| Firmware | EFI, Secure Boot off | EFI, Secure Boot off |
| Graphics | VMSVGA, 128 MiB, 3D off | VMSVGA, 128 MiB, 3D off |
| Disk | New dynamically allocated 96 GiB VDI | New dynamically allocated 40 GiB VDI |
| Storage | SATA/AHCI port 0, only this disk | SATA/AHCI port 0, only this disk |
| Optical drive | Minimal ISO on a separate SATA port | Empty |
| Adapter 1 | Internal Network: `nixorium-eval` | Internal Network: `nixorium-eval` |
| Adapter 1 MAC | `080027774099` | `080027774001` |
| Adapter 2 | NAT (individual NAT, no port forwarding) | Disabled |
| Other adapters | Disabled | Disabled |

For every enabled adapter, choose **Intel PRO/1000 MT Desktop (82540EM)**,
Cable Connected, and leave promiscuous mode denied. Keep both VMs powered off
until the DHCP configuration below is ready.

The network is:

```text
Host Internet → VirtualBox NAT → controller adapter 2 (uplink)
                                controller adapter 1
                                         │
                  Internal Network: nixorium-eval
                  ├─ VirtualBox DHCP (address leases only)
                  └─ client adapter 1 (no uplink)
```

No bridge or host-only adapter connects this segment to a real LAN. Do not
enable routing/NAT between the controller's guest interfaces. The controller
alone has Internet access; the client receives neither a gateway nor DNS from
the fixture. Nixorium supplies ProxyDHCP for boot information, not address
leases.

## 2. Configure isolated DHCP — on the host

First inspect existing names:

```sh
VBoxManage list vms
VBoxManage list dhcpservers
```

Do not reuse an existing `nixorium-eval` network or overwrite another DHCP
configuration. If those names already belong to another evaluation, finish its
cleanup or choose consistently different names before proceeding.

Run once, as the same host user who owns the VMs:

```sh
VBoxManage dhcpserver add --network=nixorium-eval \
  --server-ip=192.168.77.2 --netmask=255.255.255.0 \
  --lower-ip=192.168.77.100 --upper-ip=192.168.77.150 --enable
VBoxManage dhcpserver modify --network=nixorium-eval \
  --global --suppress-opt=3 --suppress-opt=6
VBoxManage dhcpserver modify --network=nixorium-eval \
  --mac-address=080027774099 --fixed-address=192.168.77.99
VBoxManage list dhcpservers
```

Expected: one enabled server for `nixorium-eval`, the stated pool and mask,
suppressed router/DNS options, and a fixed lease for the controller's **internal**
MAC. VirtualBox starts its DHCP service when the internal network is used.
Do not configure DHCP boot-file options 66/67 here; Nixorium owns boot discovery.
See the [VirtualBox DHCP reference](https://docs.oracle.com/en/virtualization/virtualbox/7.1/user/vboxmanage.html#vboxmanage-dhcpserver).

## 3. Install the controller — inside its VM

Start only the controller. Select the Minimal ISO in the EFI boot manager.
In the live console, inspect:

```sh
test -d /sys/firmware/efi && echo 'UEFI boot confirmed'
ip -br link
ip -4 -br address
ip -4 route
lsblk -o NAME,SIZE,MODEL,TYPE,MOUNTPOINTS
curl -I https://cache.nixos.org/nix-cache-info
```

Identify interfaces by MAC. Adapter 1 is normally `enp0s3` and must have
`192.168.77.99/24`; adapter 2 is normally `enp0s8` and receives NAT addressing.
The default route must use adapter 2. If names differ, record the observed
names and use them below; never select the NAT interface for provisioning.

Expected: UEFI confirmation, both adapters visible, working controller
Internet, and exactly one 96 GiB installable disk (normally `/dev/sda`), plus the
ISO. Stop if the disk list includes anything you did not create for this trial.

Now follow the linked **official quick start**, selecting the exact published
tag under evaluation in the bootstrap menu. An unreleased local checkout
cannot be installed through that public menu; build a controlled local test
artifact or defer the VM run until its release rather than silently testing an
older remote revision. Retain every account/password, keyboard, and disk
confirmation. The selected 96 GiB virtual disk will be erased. Record the
resolved commit printed by bootstrap; it must match the recipe target above.

When the installer completes, shut down or reboot as instructed, detach the
ISO in VirtualBox, and boot from the controller disk. Sign in as `admin` using
the chosen password.

Expected: a locally booted controller, the `nixorium` command, and its private
deployment in `~/nixorium-deployment`. Repeat the address/route checks; the
controller's Internet path must still use adapter 2.

## 4. Prepare one client — inside the controller VM

Open **Installation → Install computers** as described by the official guide.
Use these Laboratory settings, retaining your chosen accounts and regional
settings:

| Setting | Value for this fixture |
|---|---|
| Laboratory network interface | Adapter 1, normally `enp0s3` |
| Current controller DHCP address | `192.168.77.99` |
| Static laboratory network address / prefix | `10.77.0.0` / `24` |
| Number of client computers | `1` |
| Controller host number | `99` |

Both VMs use their first adapter for the lab, so the shared interface name fits
both. If actual interface names differ, use the documented controller/client
interface overrides under **Maintenance → Change settings → Network** before
preparing clients. Do not continue with a guessed name or an override pointing
at the controller uplink.

These two subnets share the internal virtual segment: DHCP for installation,
static `10.77.0.99` (controller) and `10.77.0.1` (`pc01`) for routine management.
They are fixture choices, not the original school's addressing or a product
requirement.

The official flow saves settings, generates missing keys, activates the
controller, and prepares client artifacts. Review the network transition and
confirm PXE only when the displayed service address is `192.168.77.99` on
adapter 1. Expected: preparation succeeds, the signed cache is available, and
PXE starts on the internal interface. During PXE the controller's static lab
address is temporarily removed; its DHCP address remains available.

## 5. Install the client — inside the client VM

Start the blank client. Use its EFI boot manager to select IPv4 network boot
on adapter 1. Expected: a DHCP lease in `192.168.77.100–150`, the iPXE/kernel/initrd
chain, then the Nixorium installer console.

If native EFI network boot does not discover ProxyDHCP, record that failure.
Do not silently substitute a different firmware or bridge onto a school
network; the hardware validation plan treats an iPXE ISO as a separate
compatibility scenario.

In the downloaded console, check `ip -4 route` (no default route), then run
the client installer specified in the official quick start. Select **pc01**.
Before confirming erasure, verify the target is the **40 GiB client VDI**
(normally `/dev/sda`), using `lsblk` as above. Retain the installer's exact local
identity and disk confirmation.

Expected: installation completes using controller-provided content. Shut down
the client when instructed; select its installed disk in the EFI boot manager
for subsequent starts. **PXE is for installation, not daily boot.**

On the controller, stop PXE through **Installation → PXE mode and network
recovery**. Expected: PXE listeners stop and `10.77.0.99/24` is restored on adapter
1. Then boot the client from disk and sign in. On the client:

```sh
hostname
findmnt /
ip -4 -br address
ip -4 route
```

Expected: `pc01`, local Btrfs root, `10.77.0.1/24` on adapter 1, and no default
route. The desktop should open; application Internet access is intentionally
absent in this fixture.

## 5B. Alternate USB/SSH installation scenario

Run this as a separate destructive scenario from a fresh blank client disk.
Either create `nixorium-eval-usb-client` with the same client settings, or
restore a snapshot taken before step 5 and verify the 40 GiB disk is blank.
Never attach the already installed disk merely to compare methods. Attach the
same verified official NixOS 26.05 Minimal ISO to the optical drive, keep EFI
enabled, and boot it. PXE must be stopped; adapter 1 remains the wired Internal
Network and the client still has no Internet route.

At the live client's local console run:

```sh
passwd
systemctl is-active sshd
ip -4 -br address
ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub
lsblk -o NAME,PATH,SIZE,TYPE,RM,RO,MOUNTPOINTS,MODEL,SERIAL
```

Record the live IPv4 address, full Ed25519 fingerprint, ISO device, and blank
target disk. On the controller choose **Installation → Install computers → USB
over SSH**, select `pc01`, and transcribe the address, fingerprint, and temporary
password while the client console remains visible. The client VM should not
have the controller's NAT adapter or any Internet route. Expected: fingerprint
verification precedes password use, the optical boot medium is excluded, only
the 40 GiB disk is eligible, the target closure arrives from the controller's
signed cache, and the review names `pc01`, the exact disk, revision, and risk.

Enter one wrong confirmation first and prove no transient install unit or disk
mutation starts. Then enter the exact displayed confirmation. Record the
operation ID and observe it independently:

```sh
nixorium install usb status --id 0123456789abcdef0123456789abcdef --json
nixorium install usb reconcile --id 0123456789abcdef0123456789abcdef
```

When status reports the install ready, detach the ISO before using the
separately confirmed reboot action. After disk boot, run `verify` and the same
hostname, mount, address, route, and active-system checks from step 5. Pass only
when the installed revision matches, PXE stayed stopped, the controller static
address never changed, the operation key was removed, and the client still had
no Internet route. Exercise a worker restart before apply in one disposable run
and a controller restart after dispatch in another; recovery must bind the same
live boot and reconcile status without running Disko twice.

## 6. Make and verify one software change

**Controller VM:** use Software to search the pinned package set for
`hello`. Confirm it is not already declared, choose the **pc01-only** scope,
and review the change. Follow the administrator guide to save/record the
declaration and explicitly deploy to **pc01**. Before confirming distribution,
check that only pc01 is selected. Saving the declaration alone is not success.

After completion, run these read-only checks on the controller:

```sh
nixorium hosts --json
nixorium doctor
nixorium services
```

Expected: fresh authenticated observation reports pc01 on the reviewed system
revision; PXE remains stopped, and the cache is healthy. Retain the operation
log ID if deployment fails and use [troubleshooting](troubleshooting.md);
do not count a process exit or a stale dashboard as proof.

**Client VM:** open a new terminal and run:

```sh
command -v hello
hello
readlink -f /run/current-system
```

Expected: the command is available, prints its greeting, and the active system
path agrees with the controller's observation. This proves one selected-client
change in this run, not a simultaneous or atomic fleet update.

For the student-home behavior, save a disposable marker in the designated
student home and reboot. Expect the clean template and inspect recent local
history through the administrator's snapshot recovery procedure. Keep real
work outside the reset cycle; local snapshots are not backups.

## 7. Stop and remove the environment

**Controller VM:** stop PXE first. Run `nixorium services` and inspect `ip -4 -br address`
to confirm stopped listeners and restored static addressing. If there is an
interrupted session, follow [PXE recovery](troubleshooting.md); do not manually
delete its state.

**Both VMs:** shut down normally from the desktop, client first, controller
second. On the host confirm both show **Powered Off**, not Saved.

**Host:** remove only this fixture's DHCP configuration:

```sh
VBoxManage dhcpserver stop --network=nixorium-eval
VBoxManage dhcpserver remove --network=nixorium-eval
VBoxManage list dhcpservers
```

An already stopped server needs no further stop; still verify removal.
In VirtualBox Manager, inspect the storage paths of `nixorium-eval-client`
and `nixorium-eval-controller` and verify that they contain only this trial's
new VDIs. Remove those two VMs using **Delete all files** only after saving
wanted evidence. This permanently deletes their virtual disks and local
snapshot history. It must not remove the downloaded ISO or any unrelated VM.

## Record the result; automation comes later

Use the evidence fields and result matrix in [hardware validation](hardware-validation.md).
Record host OS, VirtualBox build, exact Minimal ISO digest, Nixorium commit,
private deployment revision, actual adapter names, step results, logs, and final
network state. Share a redacted report in existing
[Discussions](https://github.com/giovantenne/nixorium/discussions), or a reproducible
failure in [Issues](https://github.com/giovantenne/nixorium/issues).

There is no one-command demo. A future harness still needs an observed EFI and
ProxyDHCP pass, bounded installer readiness, a real local-disk boot and
authenticated deployment check, and cleanup/failure tests. It must retain the
product's destructive confirmations. No core provisioning changes or unattended
installation shortcut are part of this recipe.
