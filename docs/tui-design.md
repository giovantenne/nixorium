# First-installation and intervention TUI

Implementation follows the reviewed [installation and intervention plan](ux-review/README.md),
with complete flows, layouts, delivery batches, and explicit decisions. This
document records the implementation state; no commit is implied.

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

Software package editing and computer shutdown are absent from the current
application and CLI. Do not represent them as working actions or implement
shell execution in presentation. Explain the existing private-module software
workflow and the limitation explicitly; dedicated typed plan/apply services
are required before integrated package and power management can be enabled.

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
- Added coverage for intervention entry without implicit scans, release
  discovery failure without manual fallback, restore-path separation, search
  and target identity, setup navigation, back
  navigation, help isolation and six disruptive confirmation/cancel flows.
- Added a post-build progress regression test so revalidation cannot visually
  jump back before the completed build.
- Layout checks: PASS at 80×24, 120×30 and 180×45 with 200 computers;
  focused rows and exact-confirmation controls remain visible.
- Color capability checks: PASS for no color, ANSI and ANSI256; meaning survives
  through symbols, labels and selection markers.

The management VM exercised the packaged TUI and shared application operations
for setup continuation, settings, framework updates, PXE, controller activation,
cache restart, deployment, log browsing and local Git review/commit. Existing
checks for stale reviews, secret redaction, permissions, fixed privileged units,
deployment build-before-apply and recovery remain in place. Earlier attempts
exposed outdated PTY assertions: Settings now checks stable body text instead
of complete titles in incremental terminal output, and log browsing explicitly
scrolls to the final result. No application safety check was removed.

Final management VM derivation:
`/nix/store/jhsyp9116k545wgs9h74h3297sdmz3ya-vm-test-run-nixorium-management.drv`.

No physical laboratory tests, full release matrix or client-installer VM ran
for this frontend change. No live laboratory operations, repository commit,
push or release were performed. Integrated package editing and managed power
operations remain planned application work, not implemented TUI actions.

### Changed files

- Presentation: `internal/presentation/tui.go`, `dashboard_home.go`,
  `experience.go`, `components.go`, `settings_dashboard.go`, `setup_wizard.go`.
- Read model: `internal/domain/computer_condition.go`.
- Application wiring: `cmd/nixorium/main.go` adds existing Doctor and
  UpdateManager release discovery to the typed TUI callbacks; existing CLI
  command handlers are unchanged.
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
