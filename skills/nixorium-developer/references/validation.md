# Upstream validation

Use a writable temporary `XDG_CACHE_HOME` when required. Never update
`flake.lock` unless input updates are part of the task.

The canonical automated validation is:

```sh
./scripts/validate.sh
```

It checks shell syntax and Git whitespace, runs Flake checks, builds a client,
the controller, netboot ramdisk, and installer bundle, generates a fresh site
deployment, and verifies offline derivation equivalence.

GitHub Actions must use the evaluation-only mode:

```sh
./scripts/validate.sh --ci
```

This mode checks syntax and skill distribution, evaluates the schema tests and
one representative client plus the controller, netboot, apps, packages,
Colmena metadata, and deployment status. It also generates and evaluates a
fresh deployment and its installer bundle without building system closures.
It intentionally skips the other generated clients because they share the
same module graph and their address generation is covered by `mk-lab` tests.
Keep the full matrix off GitHub-hosted runners; it is a local prerequisite for
changes that affect builds and for release preparation.

Run narrower evaluations while iterating, but run the complete script after
API, template, module, installer, asset-plumbing, or netboot changes. A
successful evaluation does not prove that source patches compile, so affected
host roles require real builds.

For skill changes, validate both skill directories with the skill validator.
The upstream and template copies of `nixorium-maintainer` must be identical,
and template discovery links must resolve to that copy.
