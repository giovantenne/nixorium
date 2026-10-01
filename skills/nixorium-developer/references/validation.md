# Upstream validation

The contributor-facing [validation guide](../../../docs/development-validation.md)
owns the gate table, coverage, performance model and focused commands. Keep
this reference focused on the rules for choosing and reporting validation.

## Normal development

Never update `flake.lock` unless input updates are part of the task.

For repeated Go edits, use `./scripts/test-go.sh` (all packages), or pass an
affected package and ordinary `go test` flags. The runner uses the locked
compiler and persistent Go cache. A persistent shell is also available:

```sh
nix --extra-experimental-features 'nix-command flakes' \
  develop --file tests/source-checks.nix go-shell
go test ./...
```

Finish a coherent change with `./scripts/validate.sh --quick`. It checks shell
regressions, schemas, generated docs, canonical copies, agent guidance, the
packaged Go application and its isolated runtime wrapper. Package tests must
cover `./...`, not just the installed command list.

Add `--eval` for schemas, the public Nix API, host composition or software
scopes. Use only the affected VM when behavior crosses process, filesystem,
network, privilege, systemd or terminal boundaries. Presentation-only cleanup
with state-transition coverage does not require a VM. GNOME changes also need
the focused `desktop-profile` check. Security changes need the locked static
analysis/security tools documented in the validation guide.

## Complete qualification

Reserve `./scripts/validate.sh --full` for cross-cutting build changes and
milestone/release checkpoints. It builds every declared check, representative
systems, netboot/installer artifacts, and a fresh deployment, then verifies
offline derivation equivalence. Related outputs must stay batched, and the
generated deployment's command is built once and reused.

Build the controller and one representative client, not all generated clients
with the same module graph. Add another client only for materially different
host-specific modules. A successful evaluation does not prove that packages
or affected system roles build. Simulated VM tests do not replace official-ISO
or physical-hardware evidence.

Pull-request and `master` CI run Go packaging/tests plus `./scripts/validate.sh --ci`. CI splits `--ci` into parallel shards with `NIXORIUM_CI_SHARD` (`lab`, `workspace-a`, `workspace-b`, `template`); an unassigned check group falls into `lab`, so a new group is never skipped. Without the variable, `--ci` evaluates everything.
The latter evaluates all checks and grouped representative source/template
outputs with import-from-derivation disabled. The separate release-tag workflow
runs `--full` with KVM before publication, with a 360-minute hosted-job limit.
Full local preflight is optional for releases; successful full CI on the tagged
commit is mandatory. Report local and remote evidence separately.

## Cache and guidance

Validation reuses `${XDG_CACHE_HOME:-$HOME/.cache}/nixorium-validation`; override
`NIXORIUM_VALIDATION_CACHE_HOME` only when isolation is required. Build modes
use `--no-link`; the full gate's temporary installer link is cleaned up.
Never garbage-collect the shared Nix store automatically.

For guidance changes, run `bash scripts/check-agent-guidance.sh` (also part of
the quick gate). It checks real-parser CLI examples, relative links, discovery
and canonical maintainer copies. The compiled checker reads prose at runtime,
so documentation edits do not rebuild the management package. Review behavior
against the [guidance map](../../../docs/agent-guidance.md): automated checks
cannot prove prose semantics. Validate both skill directories when changing
skills; keep upstream and template maintainer copies identical.
