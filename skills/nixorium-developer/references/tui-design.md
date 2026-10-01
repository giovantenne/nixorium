# Management TUI design

Read this reference before changing management navigation, interaction
behavior, visual styling, or reusable presentation components.

## Design the operator experience first

Treat information architecture, visual hierarchy, interaction rules, and code
boundaries as one design problem. Establish the intended operator workflow and
review representative rendered screens before reorganizing the state machine.
Do not preserve confusing navigation merely because existing screen constants
or key handlers make it convenient.

Keep the primary information architecture small and based on the operator's
mental model. Group related actions around stable resources such as software,
computers, installation, and maintenance. Technical implementation steps such
as Git review, controller activation, and client deployment may remain explicit
when their separate effects matter, but should appear in the context that
caused them rather than as unexplained parallel workflows.

## Use one stable screen shell

Every routine screen should use the same regions:

1. a concise breadcrumb or title and only the global state relevant now;
2. the primary list, form, review, progress, or result content;
3. a separate bounded notice area for feedback and recovery guidance;
4. a contextual action bar showing only actions available in the current state.

Do not mix status, transient feedback, instructions, and key legends into the
same paragraph. Preserve the operator's context while work is running; show
progress in place instead of replacing the whole screen with unrelated text.
The `l` progress toggle expands details in the current operation view, including
update candidate validation; retain the target, phase, elapsed time and notices.
Long content must have an explicit scroll surface while the primary action and
cancellation path remain visible.

Use the shared bounded read activity for loads and proposals: two minutes for
ordinary reads, one hour for isolated candidate build reviews. Pass its context
through typed callbacks, discard late request IDs, retain the originating view
and editable drafts, and never resume a mutation after cancellation or timeout.
Every busy screen keeps the activity, elapsed time and exit/safety policy visible;
unavailable keys produce feedback. Protected mutations and independent managed
jobs are not read-only cancellation. IPC disconnect is not worker cancellation.

## Keep visual semantics restrained

Define theme tokens centrally and use them by meaning rather than per screen.
Use restrained violet page headings, a petrol selected-row background with
light text, and teal focus markers and progress. Shortcut keys are neutral and
bold; section labels, breadcrumbs and footer descriptions use neutral text levels.
Keep the current breadcrumb stronger than its ancestors. Reserve green for
verified success, amber for attention, and red for failure or danger. A selected row must remain obvious without relying on color alone.
Status must retain a symbol and textual label for monochrome or limited-color
terminals.

Avoid decorative borders, badges, and colors that do not improve hierarchy.
Review both light and dark backgrounds and ASCII, ANSI, and ANSI-256 output.

## Make interactions predictable

Prefer a small shared vocabulary:

- every task-menu entry shows a unique direct shortcut; reserve `j`/`k` for
  list movement and keep data collections searchable or selectable with arrows;
- arrows or `j`/`k` move through a list;
- `Enter` opens or invokes the visible primary action;
- `Esc` returns or cancels without mutation;
- `/` searches when the current collection supports it;
- `Space` toggles selection in multi-select views;
- `Tab` changes a visible tab or pane;
- `r` refreshes observed state;
- `?` or `F1` opens complete contextual help.

Do not overload the same shortcut with unrelated destructive and routine
meanings. Do not expose hidden single-letter actions that are absent from the
action bar. Responsive layouts may shorten labels, but must not silently omit
the primary action, cancellation, safety state, or indication that more help is
available.

Use exact typed confirmation only when a mistake has material destructive or
operational impact. A review must distinguish desired configuration, observed
state, and the action that will happen now. Keep the existing token binding,
rechecks, privilege boundaries, and typed callbacks; visual simplification must
not weaken operational safety.

## Never leave the operator stuck

An administrator who is not a Nix or Git expert must always know the next
step. Apply these rules to every operation, refusal and recovery state:

1. Every `blocked`, `failed`, `partial` or `unconfirmed` result says what
   happened in plain words, what changed and what did not, and the next action
   as a TUI path and a CLI command, or that the operator should stop and
   collect a support report.
2. The dashboard always opens. When the laboratory cannot be read, offer
   local diagnostics, recovery guidance, Git review and the support report
   instead of only retrying.
3. Every blocking condition that survives a restart (pending deployment, USB
   reservation, interrupted template reset, PXE recovery, held operation lock)
   is visible on the Overview when the dashboard opens, not only when an
   operation is refused.
4. No supported recovery requires editing, moving or deleting coordination
   files, markers or JSON by hand. Add a reviewed command; keep the manual
   procedure only as a documented fallback.
5. A refused operation that is busy names the running operation, who started
   it and when.
6. Teachers never see administrative detail: they learn whether to try again
   later or to ask the administrator, with a short code.
7. Every loss of access (password, keys, controller disk) has a recovery
   procedure exercised at least in a VM.
8. Recovery adds a review; it never removes review tokens, rechecks,
   privilege boundaries or typed confirmations.

Before finishing a change, answer: what does the operator see if this stops
halfway; does the residual state block other work and appear on the
Overview; which command leads out of it; what does the teacher read; and
which troubleshooting section explains it?

## Keep presentation modular

The root Bubble Tea model should coordinate global window state, navigation,
and cross-screen messages. Feature models should own their local state,
updates, and rendering. Reusable header, notice, list, detail, review, progress,
result, and action-bar components should replace screen-specific approximations
of the same concept.

Presentation receives typed callbacks from the command composition root. It
must not execute shell commands, choose privileged units, reproduce domain
validation, or infer success from visual progress.

Keep startup free of Nix evaluation: check saved first-run fields, local Git and
current service state only. On older pins, labMeta can resolve the full workspace graph.
Load evaluated inventory before client selection, with cancellation and rejection
of late results. Load key reconciliation, controller closures and PXE artifact
readiness only when opening the relevant task. Never
turn deferred checks into a claim of readiness; operation planning retains its
full validation. Keep each operation in one canonical area, with contextual
follow-ups returning to their parent.

Grouped update builds show the required output count and elapsed time, not a
fabricated per-output percentage. Keep their check details expandable in place.

Managed-job attachment combines adapter-owned unit state with bounded progress,
including revision-bound controller units. It is a separate read-only view,
not a synthetic apply result: never trigger verification or installation
follow-ups from progress alone. Keep timestamp filtering for jobs started by
this TUI, accept earlier matching progress when attaching, report abandoned
running records as interrupted and refuse conflicting starts. Observation is
bounded, local-only and unavailable to the restricted teacher dashboard.

Deployment review shows bounded selected-target availability and can request a
fresh reachable-only plan through a typed callback. Keep subset computation in
the application; clear confirmation and never reuse the wider authorization.
Cancel/late-result handling must preserve the original review, not a half-edited
selector. Per-computer results use typed evidence and remain scrollable with
logs/recovery actions visible. A TCP probe is not authentication or power state;
an aggregate error alone cannot identify a failed client activation.

## Validate behavior and rendering

Save results share separate configuration/controller/client rows. A successful
controller result must match the saved revision before it is presented as up
to date. Settings, workspace and reset results offer a fresh controller review
without applying automatically, return to their result after completion, and
then offer ordinary client selection with fresh inventory. Never infer live
state from an unchanged declaration or weaken recovery gating. TUI workspace
confirmation includes its confined local record; CLI apply remains file-only.

Put interaction and state-transition coverage in deterministic Go tests. Keep
VM scenarios only for real terminal, process, filesystem, privilege, network,
or systemd boundaries. Maintain a render gallery of representative states at
least at 80x24, 120x30, and one wide layout, including loading, empty, warning,
failure, review, progress, partial result, and recovery states.

Tests should verify meaning and reachable actions rather than freezing every
word or ANSI sequence. Check that layouts fit, focused items stay visible,
limited-color output preserves meaning, help cannot trigger mutations, and
every destructive review retains an explicit cancellation path.
