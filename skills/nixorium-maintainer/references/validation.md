# Validation matrix

Use a writable temporary cache when the normal Nix cache is unavailable:

```sh
mkdir -p /tmp/nixorium-nix-cache
export XDG_CACHE_HOME=/tmp/nixorium-nix-cache
```

Do not change `flake.lock` during validation unless updating inputs is part of
the task.

## Every change

```sh
git diff --check
bash -n setup.sh scripts/*.sh scripts/lib/*.sh
nix eval .#labMeta --json --no-write-lock-file
```

Only run the Bash command in repositories that contain those scripts.

## NixOS module or configuration change

Build every affected role. The representative full set is:

```sh
nix build .#nixosConfigurations.pc01.config.system.build.toplevel --no-write-lock-file --no-link
nix build .#nixosConfigurations.pc99.config.system.build.toplevel --no-write-lock-file --no-link
```

Derive the actual controller name from `labMeta` when it is not `pc99`.

## Netboot, API, asset plumbing, or template change

Also build:

```sh
nix build .#nixosConfigurations.netboot.config.system.build.netbootRamdisk --no-write-lock-file --no-link
nix build .#installerBundle --no-write-lock-file --no-link
```

Generate a fresh deployment from the template in a new temporary directory,
initialize Git so Flake source filtering matches real use, add the generated
files, and lock its `nixorium` input to the upstream under test. Validate one
client and build its installer bundle.

Evaluate the client derivation once through the generated deployment and once
through the real store path of the generated installer bundle using a fresh
cache and `--offline`. The two `system.build.toplevel.drvPath` strings must be
identical. Resolve an `--out-link` with `readlink -f` before using it as a
`path:` Flake reference; passing the symlink itself can make Nix reject the
source.

## Skill distribution change

Verify that the upstream and template skill directories are identical, then
check that these repository-local entries resolve to the canonical skill:

```text
.agents/skills/nixorium-maintainer/SKILL.md
.claude/skills/nixorium-maintainer/SKILL.md
.pi/skills/nixorium-maintainer/SKILL.md
```

Generate a fresh site template and repeat the same check there. When the
clients are installed, use their own skill listing or diagnostic command to
confirm discovery rather than assuming that a directory name is supported.
