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
Long content must have an explicit scroll surface while the primary action and
cancellation path remain visible.

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

## Keep presentation modular

The root Bubble Tea model should coordinate global window state, navigation,
and cross-screen messages. Feature models should own their local state,
updates, and rendering. Reusable header, notice, list, detail, review, progress,
result, and action-bar components should replace screen-specific approximations
of the same concept.

Presentation receives typed callbacks from the command composition root. It
must not execute shell commands, choose privileged units, reproduce domain
validation, or infer success from visual progress.

Keep startup observational and small: check saved first-run fields, evaluated
inventory and current service state. Load key reconciliation, controller
closures and PXE artifact readiness only when opening the relevant task. Never
turn deferred checks into a claim of readiness; operation planning retains its
full validation. Keep each operation in one canonical area, with contextual
follow-ups returning to their parent.

## Validate behavior and rendering

Put interaction and state-transition coverage in deterministic Go tests. Keep
VM scenarios only for real terminal, process, filesystem, privilege, network,
or systemd boundaries. Maintain a render gallery of representative states at
least at 80x24, 120x30, and one wide layout, including loading, empty, warning,
failure, review, progress, partial result, and recovery states.

Tests should verify meaning and reachable actions rather than freezing every
word or ANSI sequence. Check that layouts fit, focused items stay visible,
limited-color output preserves meaning, help cannot trigger mutations, and
every destructive review retains an explicit cancellation path.
