# Contributing to Nixorium

Nixorium combines a Go management application, NixOS modules, installers and a
private-deployment template. Keep a change in the smallest layer that owns the
behavior and validate the boundary it affects.

## Start a development checkout

You need Git and Nix with flakes enabled. A host Go installation is optional;
the repository provides the pinned compiler and tools.

```sh
git clone https://github.com/giovantenne/nixorium.git
cd nixorium
nix --extra-experimental-features 'nix-command flakes' \
  develop --file tests/source-checks.nix go-shell
go test ./...
```

Read [AGENTS.md](AGENTS.md) for repository invariants even when working
manually. It documents public/private ownership, validation policy and safety
boundaries. The [management architecture](docs/management-architecture.md)
explains reviewed operations and privilege separation.

## Find the owning layer

| Change | Start here |
|---|---|
| Data types, validation and safe state transitions | `internal/domain/` |
| Use cases and reusable operation coordination | `internal/app/` |
| Git, Nix, filesystem, process or systemd effects | `internal/adapters/` |
| CLI/TUI input and rendering | `cmd/nixorium/`, `internal/presentation/` |
| Public lab API and host composition | `lib/`, `modules/`, `flake.nix` |
| New private deployments and workstation policy | `templates/site/` |
| Bootstrap and client installation | `install.sh`, `setup.sh`, `scripts/` |

Site identities, network values, branding and local workstation choices belong
in a private deployment. Reusable contracts and safe mechanisms belong here.
Extend `lib.mkLab` rather than copying an upstream module into the template.

## Make one small change

For example, when clarifying a TUI hint:

1. Find the view and its semantic transition test in
   `internal/presentation/`.
2. Change the hint and update a test only when the behavior or required
   information changed. Avoid snapshots of unrelated prose.
3. Format the touched Go file inside the pinned shell:

   ```sh
   gofmt -w internal/presentation/example.go
   go test ./internal/presentation/...
   ```

4. Run the default gate from the repository root:

   ```sh
   ./scripts/validate.sh --quick
   git diff --check
   ```

Generated TUI screens come from real presentation fixtures. Edit narrative
outside the markers and use:

```sh
scripts/generate-docs.sh --write
scripts/generate-docs.sh --check
```

The maintainer skill and approved template documents have canonical upstream
sources. Check them without modifying the checkout, or synchronize them only
when intentionally updating a canonical source:

```sh
scripts/sync-canonical-copies.sh --check
scripts/sync-canonical-copies.sh --write
```

Both write modes are explicit. CI and validation use only `--check`.

## Choose the validation gate

| Boundary changed | Required starting gate |
|---|---|
| Go, shell, docs or guidance | `./scripts/validate.sh --quick` |
| Schema, `lib.mkLab`, package scope or module composition | `./scripts/validate.sh --eval` |
| Management adapter, privileged service or composed CLI/TUI operation | `./scripts/validate.sh --management-vm` |
| Enrollment, Disko or client installer runtime | `./scripts/validate.sh --client-installer-vm` |
| Cross-cutting milestone or release candidate | `./scripts/validate.sh --full` |

Use the smallest gate that proves the change while iterating. Run the complete
checkpoint once when its wider coverage is warranted. The
[development validation guide](docs/development-validation.md) describes the
full matrix, cache behavior and security tooling.

## Prepare a pull request

Before opening a pull request:

- keep the worktree free of keys, password hashes, deployment identities and
  generated `result` links;
- explain the concrete trigger and resulting behavior;
- list the exact validation commands and any gate that could not run;
- update public docs, template docs and agent guidance when their contract
  changed;
- keep generated blocks and canonical copies current with their `--check`
  commands;
- avoid drive-by formatting or unrelated cleanup.

Changes that affect releases also follow [the release process](scripts/release.sh)
and are versioned only after review. Report suspected vulnerabilities through
[SECURITY.md](SECURITY.md), not a public issue.
