# Software profile closure measurements

This report compares the complete `pc01` system produced by the Essential and
Programming software profiles. It measures a whole client closure, including
the common NixOS, GNOME, firmware and classroom-management base. It is not the
sum of package archive sizes and does not predict disk usage outside the Nix
store.

## Reference measurement

The measurement was recorded on 2026-09-23 from Nixorium commit
`0167abbc2ab28b47e2a32ced4a79054ebd1c0582`. A fresh `site` template supplied
both configurations. The fixture used one generated `flake.lock`, one
`lab-settings.json`, the `pc01` target on `x86_64-linux`, and `shared` package
scope throughout.

| Input | Value |
|---|---|
| NixOS channel | `nixos-26.05` |
| Locked nixpkgs revision | `1bc55b9def8165e82073919945c3239903fe4dc2` |
| Locked nixpkgs source hash | `sha256-D2aaQetafdpHxbwA4ldAxs51UX0b11pXL+JL/5L8VrM=` |
| Nixorium source hash | `sha256-6sba5Vw51NURBGE9bU26XArsp/EELqb+rVWcXscM/Z0=` |
| System target | `nixosConfigurations.pc01.config.system.build.toplevel` |

| Profile | Exact closure bytes | GiB | Timed build |
|---|---:|---:|---:|
| Essential | 9,301,921,792 | 8.663 | 394 s |
| Programming | 14,059,022,416 | 13.093 | 179 s |
| Programming delta | 4,757,100,624 | 4.430 | not comparable |

Essential includes Node.js/npm, Pi and OpenCode, so those tools are part of the
common side of this comparison. Programming adds the general-education
applications, VS Code and the configured Java, JavaScript, Python and C
toolchains.

The timed build is elapsed wall time for evaluation, substitution and local
build work on this particular store. Essential started with a cold base and
also built the patched Veyon package. Programming reused that base. The two
times therefore describe the run conditions and must not be compared as profile
installation times.

Nix reported a pre-build estimate of 1.6 GiB download and 3.5 GiB unpacked for
Programming after Essential was present. Exact transferred bytes were not
captured, and the cold Essential transfer estimate was lost in the build log.
The transfer field is consequently **not measured** rather than inferred from
closure size. Downloads vary with the controller's existing store, the local
Harmonia cache and upstream substitute availability.

## Reproduction method

Create a disposable site and one lock. Commit or stage the template files so
Git-backed Flake evaluation includes them:

```sh
SITE_DIR=$(mktemp -d)
cd "$SITE_DIR"
nix flake init -t path:/path/to/nixorium#site
git init -q -b master
git add .
nix flake lock --override-input nixorium path:/path/to/nixorium
git add flake.lock
```

Build and measure Essential before changing the managed declarations:

```sh
ESSENTIAL_SYSTEM=$(nix build \
  path:.#nixosConfigurations.pc01.config.system.build.toplevel \
  --print-out-paths --no-write-lock-file --no-link)
nix path-info --json-format 1 --json --closure-size "$ESSENTIAL_SYSTEM"
```

Use the template's own profile helper to produce the Programming declaration
set, then build the same target with the same lock and settings:

```sh
printf 'programming\n\n\n' | bash scripts/configure-software-profile.sh \
  software-presets.json lab-software.json
git add lab-software.json
PROGRAMMING_SYSTEM=$(nix build \
  path:.#nixosConfigurations.pc01.config.system.build.toplevel \
  --print-out-paths --no-write-lock-file --no-link)
nix path-info --json-format 1 --json --closure-size "$PROGRAMMING_SYSTEM"
```

Record elapsed time around each `nix build` separately. If transfer size is
needed, capture Nix's estimate before realization or measure network traffic in
an isolated empty store. State the store/cache condition with that result; do
not substitute `closureSize` for downloaded bytes.
