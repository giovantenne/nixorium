# ADR-0033: One system per class of identical client computers

- Status: accepted
- Date: 2026-10-19
- Amends: [ADR 0031](0031-client-deployment-engine.md) (what the engine builds
  and what it writes before an activation)
- Related: [ADR 0012](0012-preparation-bound-pxe-address.md),
  [ADR 0021](0021-reviewed-usb-ssh-installation.md),
  [ADR 0028](0028-guided-network-change.md)

## Context

Every client computer was a different system, because its name and its
laboratory address were evaluated into it. A laboratory of twenty identical
computers therefore meant twenty evaluations, twenty builds of nearly the same
system and, for network installation, twenty evaluations before the first
computer could boot. Nothing else differed: software, interface name and
module groups were already the same for most of them. `mkLab` already grouped
the clients that match in those respects into classes (`nixoriumHostClasses`),
but only to decide which computers an Assistant proposal affects.

## Decision

Computers of one class share one system, and a computer takes its identity
from a small record on its own disk.

- **Classes.** A class is a set of clients with the same module groups,
  managed software, interface name and explicitness. The controller, every
  host in `hostModules` and every host in `updateValidationHosts` is a class
  of one. A class of one keeps the per-host system of earlier releases and is
  named after its computer (`pc03`). A class with several members is
  `shared-<n>`, numbered by its first member, so a software change does not
  rename every class.
- **Outputs.** `nixosConfigurations.pcNN` stays: for a member of a shared
  class it is an alias of the class system, evaluated and built once.
  `labMeta.clients.hosts[].class` and `labMeta.clients.classes` list the
  classes. `labSettings.clientIdentities` lists number, name, address and
  previous address of every client, so the class system carries the whole
  inventory. Modules receive `nixoriumClass`
  (`{ id, shared, members, representative }`).
- **No name in a shared system.** In a shared class system `hostName` and
  `hostIp` throw an error that names the class members and says to give the
  computer its own configuration with `hostModules` or
  `updateValidationHosts`. A private module that needs a computer's name or
  address therefore stops the evaluation instead of silently using the
  representative's.
- **Identity record.** `/var/lib/nixorium/identity.json` holds only
  `{"schemaVersion":1,"number":N}`. Name and addresses follow from the number
  and the inventory in the system, so a change of network changes no record.
  A per-host system writes the record at activation when it is missing. A
  shared system reads it at activation, writes `/run/nixorium/identity.env`
  (`NIXORIUM_HOST_NAME`, `NIXORIUM_HOST_NUMBER`, `NIXORIUM_LAB_ADDRESS`,
  `NIXORIUM_PREVIOUS_ADDRESS`) and sets the running host name.
  `nixorium-identity.service` fails with a clear message when the record is
  missing or invalid, and the computer then gets no laboratory address.
- **Network.** A shared system uses one NetworkManager profile,
  `nixorium-lab`, for every client: bound to the card name when one is
  configured, with the address from the environment file, `hostname-mode=none`
  (DHCP never renames the computer), `dad-timeout=3000` and
  `autoconnect-retries=3`. A duplicated number is refused by address conflict
  detection and retried only a few times. Per-host systems keep their profile.
  The profile writer waits for NetworkManager.
- **Engine.** The engine builds each class once. Before it switches a
  computer to a shared system it writes the identity record over SSH. A record
  that names another computer is never overwritten: the computer stays as it
  is and the result says "identity record names another computer". Verifying a
  shared system requires `nixorium-host-state` to report the computer's
  number (a third line, printed only for a valid record). `deploy plan` and
  the review show "Builds N configurations for M computers".
- **Installers.** The PXE `setup.sh` and the USB/SSH installer bind a computer
  to the system of its class and write the identity record before its first
  boot. PXE preparation evaluates one computer per class.
- **Migration.** The first update after the upgrade writes the record before
  switching a computer to a shared system, so no computer needs a visit. A
  computer that keeps a per-host system writes its own record at activation.
- **`colmena` output removed.** Nixorium has had its own engine since ADR
  0031; the flake no longer has a `colmena` output.

## Consequences

- An update of a laboratory of identical computers evaluates and builds one
  system per class instead of one per computer; network installation prepares
  one evaluation per class.
- A mistake in a class system now reaches every computer of the class. The
  engine still builds before it applies, and results are per computer, so a
  failure on the first computers is visible before the rest of the update;
  classes of one remain available for computers that must differ.
- A private module that uses `hostName` or `hostIp` must be listed under
  `hostModules.<pcNN>` or `updateValidationHosts`; otherwise evaluation stops
  with the error above. Such a module in `sharedModules` or `clientModules`
  now needs that change once.
- A computer without a valid record has no laboratory address and a failed
  `nixorium-identity.service`. The update from the controller writes the
  record again; a reinstall does too.
- Rollback uses the boot menu. The previous per-host generation does not read
  the record, so it starts with its own name and address and leaves the
  record alone.
- Scripts that read the `colmena` flake output must stop; they were never
  part of Nixorium's own tools after ADR 0031.

## Alternatives considered

- **One system for all clients.** Rejected: laboratories declare software per
  computer or group, and the interface name can differ; the classes follow
  those differences.
- **Names assigned by DHCP.** Rejected: DHCP may rename a computer, and the
  laboratory depends on fixed addresses for the controller's checks, the
  classroom controls and the network-change flow.
- **Name derived from the network card address.** Rejected: a cloned disk
  would carry another computer's number to a different card, and a replaced
  card would change a computer's identity.
