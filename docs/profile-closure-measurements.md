# Software profile closure measurements

This report compares the complete `pc01` system produced by the Essential and
Programming software profiles. It measures a whole client closure, including
the common NixOS, GNOME, firmware and classroom-management base. It is not the
sum of package archive sizes and does not predict disk usage outside the Nix
store.

These figures are a reproducible snapshot of the source revision below. Git and
the baseline GNOME desktop-icon/dock extensions became part of every built-in
workstation after that revision, so repeat the documented procedure before
using the table to size a current deployment.

## Reference measurement

The measurement was recorded on 2026-09-23 from Nixorium commit
`8d273124614d4d2fe15e8fedec45f852b7623fe2`. A fresh `site` template supplied
both configurations. The fixture used one generated `flake.lock`, one
`lab-settings.json`, the `pc01` target on `x86_64-linux`, and `shared` package
scope throughout.

| Input | Value |
|---|---|
| NixOS channel | `nixos-26.05` |
| Locked nixpkgs revision | `1bc55b9def8165e82073919945c3239903fe4dc2` |
| Locked nixpkgs source hash | `sha256-D2aaQetafdpHxbwA4ldAxs51UX0b11pXL+JL/5L8VrM=` |
| Nixorium source hash | `sha256-pahFaPivCS14d9xo72gM8Y0RAeMH99PYxmNR1ByeOBg=` |
| System target | `nixosConfigurations.pc01.config.system.build.toplevel` |

| Profile | Exact closure bytes | GiB | Timed build |
|---|---:|---:|---:|
| Essential | 9,304,907,232 | 8.666 | 16 s |
| Programming | 14,062,007,856 | 13.096 | 15 s |
| Programming delta | 4,757,100,624 | 4.430 | not comparable |

Essential includes the Ghostty/TTE lab screensaver, Node.js/npm, Pi and
OpenCode, so those tools are part of the common side of this comparison.
Programming adds the general-education applications, VS Code and the configured
Java, JavaScript, Python and C toolchains.

The timed build is elapsed wall time for evaluation, substitution and local
build work on this particular warm store. Programming reused the Essential base
and packages realized by an earlier comparison. The two times therefore
describe the run conditions and must not be compared as profile installation
times.

Nix reported one 400.0 KiB substitute (2.8 MiB unpacked) for TTE while building
Essential and no additional paths to fetch for Programming in this warm-store
run. Exact network traffic was not captured, so transferred bytes remain **not
measured** rather than inferred from closure size or the pre-build estimate.
Downloads vary with the controller's existing store, the local Harmonia cache
and upstream substitute availability.

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
printf 'programming\n\n' | bash scripts/configure-software-profile.sh \
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
