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

## Consequences

Privileges and logs are auditable, and closing the TUI does not terminate
system work. The controller module and policy require NixOS VM tests. Although
the administrator remains a wheel user, the normal workflow has a smaller
accidental and injection surface.
