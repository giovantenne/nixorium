# ADR-0004: Narrow privileged actions

Status: accepted

## Context

Most management work can run as the administrator, while controller rebuilds,
private-key installation, service control, and network transitions need root.
Running the entire TUI as root would expose terminal parsing, Git operations,
and configuration input to unnecessary privilege.

## Decision

Run the management application unprivileged. Expose root work as a fixed set of
declaratively installed systemd actions controlled through narrowly scoped
polkit policy. Privileged code accepts validated typed parameters and fixed
deployment/service locations, invokes programs with argument arrays, and never
offers a generic shell or arbitrary command endpoint.

Controller apply is split further: Git evaluation and the Nix build run as the
administrator who owns the private deployment. The root action activates only
the exact returned NixOS store closure. Local deployments use the Git Flake
fetcher, not `path:`, so ignored private keys are excluded from copied Nix
sources and never become build inputs.

PXE preparation needs authorization to start a fixed long-running unit but no
root capability. That unit runs entirely as `admin`, reads the deployment
read-only, builds content-addressed outputs, and can write only its Nix cache
and preparation state directories. Later privileged PXE consumers must treat
the administrator-owned manifest as untrusted structured input and accept
only canonical `/nix/store/<hash>-<name>` roots and fixed artifact names.

The address transition is a separate root unit bounded to `CAP_NET_ADMIN`.
It accepts only the compiled start/stop/recover verb, writes a root-owned
session record before mutation, removes only the exact configured static CIDR,
and restores only an address proven present in that record. Git inspection
stays outside this unit so access to the administrator's private home does not
require adding filesystem-bypass capabilities.

The listener is a separate systemd-owned boundary. Its supervisor retains
only the capabilities needed to bind the DHCP/TFTP ports, configure dnsmasq,
and drop child identities. The HTTP child immediately runs as `nobody`, while
dnsmasq drops to the dedicated `nixorium-pxe-dnsmasq` system user. Both consume
only fixed runtime paths derived from strictly validated preparation and
root-owned session records.

The public PXE lifecycle uses an exact verb/unit allowlist. Administrators may
start or stop `nixorium-pxe.service`, stop the internal network unit, and start
the fixed recovery unit; direct public start of the network unit is denied.
The application performs readiness and Git checks unprivileged, starts the
listener whose systemd dependencies enter installation mode, and synchronously
stops both units if startup or post-start verification fails.

## Consequences

Privileges and logs are auditable, and closing the TUI does not terminate
system work. The controller module and policy require NixOS VM tests. Although
the administrator remains a wheel user, the normal workflow has a smaller
accidental and injection surface.
