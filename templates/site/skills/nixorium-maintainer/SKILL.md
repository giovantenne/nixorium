---
name: nixorium-maintainer
description: Administer a private Nixorium lab deployment, including software updates, student-home and desktop customization, settings, validation, diagnostics, and reviewed installation or deployment. Use when the repository consumes nixorium.lib.mkLab, not for public core development.
license: MIT
---

# Nixorium Lab Maintainer

Translate an administrator's request into the smallest deployment-owned change.
Do not require the admin to know Nix files, package attributes, or CLI flags.

## Establish context

Read the deployment's `AGENTS.md` when present. Inspect Git status, local
modules/declarations, and `flake.lock` before edits. Check available CLI help
and evaluated metadata: an older pinned release may not support current
examples. Do not update the framework just to obtain a convenient command.

Distinguish controller-only mode from a configured laboratory. Use
`labMeta` for actual hosts/interfaces and `deploymentStatus.controller` for
controller readiness when available; client installation/deployment requires
`deploymentStatus.ready`. Missing newer capability metadata means use the
legacy contract, not assume support.

Clarify only choices that materially affect the result: which computers/users,
which tool or version when ambiguous, and whether to apply now. Explain impact
in ordinary language, especially student-home resets and controller networking.

## Choose the relevant reference

- Settings, keys, local modules, or assets:
  [configuration](references/configuration.md).
- Adding/removing/updating software, including “update OpenCode”:
  [software](references/software.md).
- Student-home defaults, VS Code extensions, dock/background, or npm content:
  [student home](references/student-home.md).
- Validation, controller/client application, PXE, diagnostics, or framework updates:
  [operations](references/operations.md).

Read only the references needed for the request. Keep installed behavior
distinct from planned product features; there is no assumed home-capture API.

## Preserve the boundary

The deployment owns site data, packages, and local policy. Extend local modules;
do not copy/edit upstream modules or merge upstream Git history. A required
core change must be reported as an upstream task, not implemented inside a lab.

Keep assets, modules, and public keys inside the deployment source tree for the
offline installer. Private signing, SSH, and Veyon keys must stay out of Git,
the Nix store, logs, and chat. Preserve unrelated work and existing key pairs.

## Finish with evidence

Use the smallest relevant validation set from the operations reference.
Separate “configured”, “validated/built”, and “activated/deployed” in the result,
including affected machines and any remaining operator action.

A request to diagnose does not authorize a fix. Configuration work does not
implicitly authorize installation, deployment, service changes, home reset,
commits, or pushes. Use authorization already given for the specific operation;
ask only when the required action exceeds it. Never bypass a failed managed
safety check or silently retry a destructive/uncertain operation.
