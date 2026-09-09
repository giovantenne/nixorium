# Upstream architecture

## Repository roles

The public repository is a versioned framework. It exports `lib.mkLab`, a site
template, standalone example hosts, Colmena outputs, netboot outputs, helper
apps, readiness metadata, and an offline installer bundle.

A private deployment owns site configuration and consumes a pinned upstream
release. Never solve ordinary customization by copying an upstream module into
the deployment.

## API and extension points

`mkLab` accepts `deploymentSelf`, typed `labConfig`, public-key and asset
paths, and shared/controller/client/host/netboot module lists. Filesystem
references must remain inside the upstream or deployment source trees.

Generated host names and addresses come from the validated IPv4 network,
prefix, client count, and host numbers. Reject unknown per-host modules and
Veyon pilot names so configuration typos cannot be ignored.

`deploymentStatus` reports whether placeholders, missing public keys, or
public default passwords remain. Keep the standalone example evaluable even
when it is intentionally not deployment-ready.

## Offline invariant

The installer bundle contains a standalone Flake with the effective typed
configuration, local input sources, public cache key, downstream modules, and
assets. Preserve path types as well as values when serializing it.

For an unchanged lock and configuration, a client evaluated through the
deployment and through the offline bundle must have the same
`system.build.toplevel.drvPath`.

All destructive bootstrap tooling, including Disko, must come from the
deployment's locked inputs. Do not fetch a mutable tool revision at install
time. Keep the exported Disko app and package buildable in automated validation.

## Built-in module boundaries

Keep the small `common.nix` module as the composition point. Desktop policy,
packages, power behavior, screensaver, shell configuration, and SSH policy
belong in separate modules. Site-specific removal or policy should use
downstream overrides or the smallest new generic extension point.
