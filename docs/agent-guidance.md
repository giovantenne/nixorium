# Keeping agent guidance aligned with implementation

Agent instructions are maintained interfaces, not independent specifications.
A behavior change is incomplete until its affected instructions and tests have
been reviewed. Do not “fix” a mismatch by weakening a safety invariant without
an explicit product decision.

## Ownership and review map

| Changed contract | Source of truth | Review with the change |
|---|---|---|
| CLI commands/options | `cmd/nixorium/main.go` argument parser, `cmd/nixorium/commands_*.go` family handlers, `cmd/nixorium/command_apply.go`, and CLI tests | Maintainer command examples; administrator guide |
| TUI navigation, confirmations, automatic follow-up | `internal/presentation/` workflow tests and typed callbacks in `cmd/nixorium/` | Maintainer operations/software guidance; TUI tour; README if entry flow changes |
| Software scopes and package ownership | `internal/domain/software.go`, `lib/eval-lab-software.nix`, template modules | Maintainer software guide; both AGENTS files |
| Software profile schema and batch apply | `internal/domain/software_presets.go`, `lib/eval-software-presets.nix`, application tests | System reference; deployment README; maintainer software/configuration guidance; both AGENTS files |
| Controller-only readiness, inventory, interfaces | `lib/mk-lab.nix`, domain/app readiness tests | Deployment AGENTS; maintainer context/validation |
| Home reset, extensions, npm, desktop defaults | `modules/home-reset.nix`, home scripts, template modules/assets | Maintainer student-home guide; system reference |
| Account roles and NetworkManager authorization | `modules/users.nix`, mkLab tests | Both AGENTS files; maintainer configuration guidance; deployment README; system/architecture docs; changelog |
| Privilege, keys, PXE or USB/SSH recovery | Management/PXE modules, remote worker/helper, adapters, integration tests | Core invariants; maintainer configuration/operations; troubleshooting |
| Input ownership and updates | Update adapters/tests, Flake/template contract | Maintainer software/framework-update guidance; deployment AGENTS |
| Bootstrap prompts, sequencing, and netboot keyboard | `install.sh`, bootstrap CLI, `lib/mk-lab.nix`, controller-bootstrap and mkLab tests | Root README; deployment README; system reference; controller-first ADR; changelog |
| Validation or skill distribution | `scripts/validate.sh`, `tests/source-checks.nix`, package source fileset | Both AGENTS files; developer validation reference |
| Generated TUI gallery | `cmd/nixorium-docs`, real fixtures in `internal/presentation/demo.go`, `scripts/generate-docs.sh` | TUI tour, README and contributor guide |
| Canonical upstream/template copies | `scripts/sync-canonical-copies.sh` allowlist and `tests/canonical-copy-sync.sh` | Source documents, template copies and contributor guide |

Keep root `AGENTS.md` focused on upstream work. The template's `AGENTS.md`
orients a private-deployment agent; the maintainer skill routes to task-specific
references. Prefer referring to a live review report over copying transient
confirmation phrases or menu names into many places.

## Automated checks and their limits

The separately compiled Go checker reads guidance files and passes fenced
Nixorium CLI examples through the real argument parser without executing operations.
It also checks relative Markdown targets and skill discovery links, and
compares the complete upstream/template maintainer trees. Missing required
files fail rather than skip. Documentation is read from the checkout at runtime,
so prose edits do not invalidate the application or checker compilation cache.

These checks run in the normal quick gate and the management-command CI job.
They catch broken syntax/examples, missing references, broken distribution, and
discovery drift. They cannot prove that prose describes runtime effects or
that an accepted CLI argument has the intended impact. Existing application,
TUI, and integration tests remain the behavioral evidence.

For each relevant change, review explicitly:

- CLI save-only behavior versus TUI commit/activation follow-up;
- configuration versus validation/build versus activation/deployment;
- controller-only versus fleet readiness;
- authorization, private data, offline operation, and destructive boundaries;
- examples against the pinned deployment's capabilities, not just newest core.

The pull-request checklist records this review, including when guidance is
unaffected. Do not add tests that freeze prose wording as a substitute for
checking its meaning.

## Skill delivery

Edit `skills/nixorium-maintainer/` and its identical copy under
`templates/site/skills/` together, including added or removed references.
Only the maintainer skill ships in a private deployment. Developer discovery
stays upstream. Run the skill frontmatter validator when editing skills.

Existing private deployments contain their own copied instructions: updating
the Nixorium input does not automatically replace those files. Refresh them
only as an explicit, reviewed deployment change, preserving local additions
and matching the deployment's pinned capabilities.

## Behavioral spot checks

When changing task routing, walk through at least these requests using a
disposable fixture or read-only review, never an actual unapproved deployment:

- Add one application for all computers without deploying.
- Update only OpenCode, preserving the framework and unrelated packages.
- Add Python editor extensions to the reset student home.
- Rebuild a controller-only deployment with zero clients.
- Diagnose a failed PXE transition without changing network state.
- Diagnose an interrupted USB/SSH install by operation ID without replaying
  disk mutation or accepting a new physical identity.

Check chosen files/scopes, questions, validation cost, authorization boundaries,
and whether the result honestly separates saved from applied state.
