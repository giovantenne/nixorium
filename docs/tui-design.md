# First-installation and intervention TUI

Implementation follows the reviewed [installation and intervention plan](ux-review/README.md),
with complete flows, layouts, delivery batches, and explicit decisions. This
document records the implementation state on the development branch.

## UX audit (2026-09-15)

The existing frontend has 22 dashboard states plus the initial settings and
configuration-review programs. Home lists nine equal-weight technical tasks.
Computers renders six columns without selection, search, pagination or a detail
route. Deploy and log lists grow beyond the terminal. Setup repeats all eleven
internal stages before showing its next step. Several results expose booleans,
revision hashes and log paths before explaining the outcome. Progress repeats
phase, fraction, gradient bar and recent activity. Help is inconsistent and
there is no application-wide help surface.

Existing flows: first-run fields → password collector → redacted config review
→ key reconciliation → secret installation → observed setup; setup → Git
review → path selection → exact commit confirmation → setup; controller
plan → confirmation → progress → verified result; preparation → PXE start
review → active installation → stop/recovery; Computers → refresh; Deploy →
one/selected/all targets → plan → exact confirmation → progress → result/logs;
Services → cache restart review/result; Logs → bounded detail; Update → tag and
policy → validated checks/diff → confirmation → result; Settings → group/editor
or password → candidate review → result → Git.

Remove uppercase status shouting, repeated summary fields, default technical
columns and gradient decoration. Disclose system paths, SSH evidence, full
revisions, completed stage history and technical progress only on request.
Promote observed problems, evidence freshness, selected scope, meaningful
progress, recovery instructions and the next explicit action.

## Information architecture

- First setup: five operator stages over the existing observed technical checks:
  laboratory settings → controller → client system → pilot computer → other computers.
- Interventions: restore, software, distribute, install/reinstall, Update
  Nixorium, and Advanced tools. Opening this screen performs no fleet scan.
- Restore: explicit choice between non-destructive reapply and disk-erasing
  reinstall; a failed reapply never escalates automatically.
- Computers: searchable, navigable inventory; selected computer detail and
  diagnostics; an explicit advanced check rather than room health.
- Distribute: selection → impact review → exact confirmation → semantic progress
  → verified outcome. Controller activation remains a separate operation.
- Install computers: preparation, reviewed network transition, local installer
  handoff, stop and recovery.
- Administration: settings, controller, services, changes/revisions, history,
  inventory and diagnostics.
- Update Nixorium: fetch bounded releases → select stable or explicitly reveal
  prerelease → validate → review → apply only the two managed Flake files.
- Software: configured/search/suggested views over pinned packages →
  configuration scope → validated proposal → exact confirmation → transparent
  local save; applying to computers remains separate.
- Shutdown: explicit client selection → reachability/session review → exact
  confirmation → immediate recheck → honest per-target request outcomes.

Guided software editing is available only through its dedicated typed service;
private-module customisation remains an explicit Advanced-tools path.

## Presentation and evidence

Use a single muted accent, neutral foreground, semantic green/amber/red and
symbol-plus-text statuses. No dashboard borders or nested cards. Bound reading
width on large terminals. Use two columns only where list/detail benefits;
compact terminals open details separately. Lists follow the cursor and reserve
space for contextual help. Full-page overflow remains scrollable.

120×30 is the recommended everyday working size, not a hard minimum. Layout
regressions also cover 80×24 and 180×45. Larger terminals retain a bounded
reading width instead of spreading unrelated metrics across the screen.

The intervention entry renders no fleet-health synthesis. It may promote an
already observed controller PXE recovery condition because that affects normal
controller networking. Computer state is loaded explicitly inside inventory or
an operation that needs targets. Missing observation and powered-off clients do
not become either success or a global warning.

Keyboard: arrows/j/k move, Enter opens/reviews, Esc returns/cancels, / searches
lists, ? opens contextual help, q quits outside text entry. Show at most five
footer bindings. Text entry owns its keys. Help must not consume confirmations
or change operations. Foreground mutation keeps its existing quit protection.

## Safety map

Preserve domain/application preflights and typed callbacks for every flow:

| Operation | Retained review / safety boundary |
|---|---|
| Deploy | evaluated clients, clean revision, exact DEPLOY selector, mandatory build, recheck, lock, authenticated verification and private logs |
| Controller | exact REBUILD identity, reviewed revision, fixed service, owner build, root activation receipt |
| PXE start | exact START PXE, interface/address impact, readiness recheck, transactional cleanup and recovery |
| Cache restart | exact RESTART CACHE, fixed action unit, post-action verification |
| Git commit | selected safe paths, exact phrase, content token, secret checks, no push |
| Update Nixorium | service-discovered release, candidate checks, exact phrase/token, two-file scope, no implicit activation |
| Settings/password | full validation, redacted review, source fingerprint, atomic writer, no-echo password collector |
| Software declaration | pinned search plus suggestions, structured nested attributes, policy state, evaluated scope, all-client candidate validation, fingerprint/token recheck, one-file atomic writer and transparent local record, no implicit build/deploy |
| Client shutdown | evaluated clients only, controller exclusion, active-session block, explicit unknown-session acknowledgement, expiring review, immediate recheck, shared deployment lock, fixed SSH commands |
| Client disk install | local immutable inventory and exact disk/identity confirmation; not a controller-side shortcut |

## Validation

Run Go formatting, all package tests and vet, quick repository validation and
the management VM for changed frontend integration. Add domain evidence tests,
navigation/search/back/help regressions, wrong/exact/cancel confirmation tests,
large-inventory layout tests, no-color rendering, and deterministic captures at
compact, standard and wide sizes. Record physical testing separately.

### Results — 2026-09-15

- Go formatting: PASS (`gofmt`, followed by a clean `gofmt -l` check).
- `go test ./...`: PASS for CLI, adapters, application, domain and presentation.
- `go vet ./...`: PASS.
- `git diff --check`: PASS.
- `./scripts/validate.sh --management-vm`: PASS, including the quick matrix
  (shell syntax, installer shell tests, schema/mkLab checks, skill-copy
  consistency and packaged Go tests).
- `./scripts/validate.sh --full`: PASS, including 43 Flake checks, explicit
  client/controller/netboot and installer builds, both VM suites, a fresh
  private template, and direct/offline installer client equivalence.
- Added coverage for intervention entry without implicit scans, release
  discovery failure without manual fallback, restore-path separation, search
  and target identity, setup navigation, back
  navigation, help isolation and six disruptive confirmation/cancel flows.
- Added a post-build progress regression test so revalidation cannot visually
  jump back before the completed build.
- Added focused-pilot tests proving that an identity outside the evaluated
  inventory is rejected before network access and that only the selected
  client's SSH and active-system probes run. Presentation tests cover pilot
  selection, honest no-telemetry handoff, separate technical/practical checks,
  partial-session summary, and exact confirmation before leaving PXE active.
- Added restoration-path tests that keep reapply non-destructive and require an
  evaluated identity before disk-erasing reinstall, with verification scoped to
  that selected computer.
- Added strict domain, application, adapter and presentation coverage for
  durable installation sessions. Records are atomic, private and isolated per
  repository; evidence is bound to identity, revision and installed system
  path. Tests cover cross-process resume, stale revision/inventory rejection,
  unknown identities, corrupt or unsafe state files, symlinks, clock rollback,
  technical-before-practical ordering and failed current observations taking
  precedence over stored evidence.
- Layout checks: PASS at 80×24, 120×30 and 180×45 with 200 computers;
  focused rows and exact-confirmation controls remain visible.
- Color capability checks: PASS for no color, ANSI and ANSI256; meaning survives
  through symbols, labels and selection markers.

The management VM exercised the packaged TUI and shared application operations
for setup continuation, guided software changes, settings, framework updates,
PXE, controller activation, cache restart, deployment, log browsing and local
Git review/commit. Existing
checks for stale reviews, secret redaction, permissions, fixed privileged units,
deployment build-before-apply and recovery remain in place. Earlier attempts
exposed outdated PTY assertions: Settings now checks stable body text instead
of complete titles in incremental terminal output, and log browsing explicitly
scrolls to the final result. No application safety check was removed.

Validated VM derivations:

- management: `/nix/store/4l8mzpqxypmqvibxq0930afmi6aswakd-vm-test-run-nixorium-management.drv`;
- client installer: `/nix/store/d402p7rgwiq1z8119bj3dnx6dc14qdp3-vm-test-run-nixorium-client-installer.drv`.

No physical laboratory operations, push or release were performed. Managed
power remains planned application work, not an implemented TUI action.

### Changed files

- Presentation: `internal/presentation/tui.go`, `dashboard_home.go`,
  `software_dashboard.go`, `experience.go`, `components.go`,
  `settings_dashboard.go`, `setup_wizard.go`.
- Read model: `internal/domain/computer_condition.go`.
- Application wiring: `cmd/nixorium/main.go` adds existing Doctor and
  UpdateManager release discovery plus the single-inventory-identity observer
  and installation-session manager to the typed TUI callbacks; existing CLI
  command handlers are unchanged.
- Installation session: `internal/domain/installation_session.go`,
  `internal/app/installation_session.go`, and
  `internal/adapters/installation_session.go` own the versioned record,
  reconciliation rules, focused observation and private atomic storage.
- Guided software: `internal/domain/software.go`, `internal/app/software.go`,
  `internal/adapters/software.go`, `lib/eval-lab-software.nix`,
  `lib/software-catalog.nix`, and `templates/site/lab-software.json` implement
  the allowlisted schema, typed plan/apply boundary, candidate evaluation and
  atomic managed-file writer.
- Tests: `internal/domain/computer_condition_test.go`,
  `internal/presentation/experience_test.go`, `tui_test.go`,
  `components_test.go`, and `tests/management-vm.nix`.
- Dependency classification: `go.mod` marks the already-pinned colorprofile
  module direct because the new color-capability tests import it; no version
  or lock-file updates.
- Documentation: `README.md`, `templates/site/README.md`, `CHANGELOG.md`,
  `docs/adr/0002-go-bubble-tea.md`, this audit and `docs/tui-renders.md`.
- External project journal: `../nixorium-agent/IMPLEMENTATION_STATUS.md` remains
  outside the Git repository.
