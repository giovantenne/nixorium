# Deployment configuration: inspect and hand off

Inherit the proposal-only boundary in [the skill](../SKILL.md).

## Ownership is not edit permission

A deployment owns its settings, software, workspace, catalogs, assets and local
modules; upstream supplies mechanisms through `nixorium.lib.mkLab`.
Do not turn this ownership into permission to edit those files.

Inspect only relevant non-secret fields. Do not load `lab-settings.json`
wholesale. Current templates keep account hashes in ignored `lab-credentials.json`
and encrypted recovery; never read those files or a prepared Nix source. Older
pins may still keep hashes in settings. For a requested settings diagnosis,
on a known trusted deployment and after checking installed help:

```sh
nixorium config validate
```

Use safe field/error summaries, not raw sensitive values. Explain the desired
settings change for the operator's native form; do not generate a complete
settings candidate or migrate legacy Nix configuration.

## Network and identity

Network addresses, interfaces, host counts, accounts, permissions, boot, disks,
reset policy and SSH trust are operator-controlled. Do not change them here.

A controller DHCP hint must not overlap the static laboratory prefix. On
supporting pins, saved clients prevent managed changes to subnet, prefix and
controller host number. Never remove clients or edit JSON to bypass this.
A `lab.networkTransition` record belongs to the guided change of the
laboratory network; never propose adding, editing or removing it.
The refusal code is `NETWORK-LOCKED`. On supporting pins the operator moves the
laboratory to another network with the guided change (Maintenance → Change
network addressing, `nixorium network migrate status`); refer to it instead of
proposing settings edits. A change in progress shows `NETWORK-CHANGING`.

Interface changes can disconnect computers; do not prescribe a universal
controller-first order. On supporting pins the settings review checks the named
devices read-only (`networkPreflight` in `config plan --json`); point the
administrator to that review instead of guessing device names. The laboratory
network is wired: every computer needs an Ethernet cable, and on supporting pins the
settings refuse a Wi-Fi card name (`wl…`) for any interface. Do not
propose a Wi-Fi laboratory, Wi-Fi interface names or Wi-Fi credentials in the
deployment. Wi-Fi in the USB live system serves only the installation itself;
the installed computer needs its cable before the restart. PXE and Switch on
(Wake-on-LAN) need the cable too.

Never relax student network-administration restrictions, share the deployment
with teachers/students or copy credentials to bypass a refusal.

## Unsupported customization and setup

Modules, assets, catalogs, scripts, per-host code, services and overrides are
outside this skill. Report the missing capability and a separate development
task; do not provide a runnable bypass recipe.

This explicitly forbids invasive changes through new or existing NixOS/Home
Manager modules, overlays, mkForce/package overrides, custom derivations,
activation/login scripts, services/timers and shell startup hooks. Do not wire
them into Flake imports, module lists or specialArgs, and do not alter upstream
mechanisms to bypass a managed refusal. The prohibition includes proposing
ready-to-run bypass code for a human to paste, not only executing it yourself.

Key generation, secret installation and first setup belong to the human-run
managed workflow. Do not read/replace existing keys. Private keys, passwords
and provider tokens must never enter Git, the Nix store or model context.

Controller-only mode does not need invented clients or lab keys. Controller
readiness is distinct from fleet readiness. Passing validation does not permit
activation, installation or deployment. Use [operations](operations.md).
