# ADR-0009: Bounded read-only Git review

- Status: accepted
- Date: 2026-09-14

## Context

The private deployment is Git-backed, and administrators need to understand
changes without learning several porcelain commands. A generic raw diff can be
unbounded, execute configured diff/textconv drivers, expose password hashes, or
silently omit the important distinction between the index and worktree. Opening
untracked files automatically would also give arbitrary repository paths
content authority. Known private keys must remain outside Git altogether.

## Decision

Add a read-only typed review operation shared by CLI/JSON and the TUI. Parse
NUL-delimited Git porcelain status and report staged, unstaged, and untracked
state per path. Label the machine-managed settings and generated public-key
paths separately from unexpected edits.

Capture staged and unstaged patches with fixed argument arrays and both external
diff and textconv disabled. Bound status to 1 MiB/1000 paths and each patch to
256 KiB, sanitize terminal controls, and redact the three password fields in
`lab-settings.json`. Do not open untracked contents. If a known Harmonia, SSH,
or Veyon private-key path occurs in status, return a blocked typed report and do
not capture any patch. Exclude those paths from every diff independently, then
recheck both porcelain status and HEAD; discard the report if either changed
during review.

This operation cannot stage, discard, commit, or push. Optional commit support
is a separate operation with its own review token, explicit path allowlist,
revalidation, and confirmation boundary. A remote is never required and push
is never implicit.

## Consequences

Administrators get one safe, scriptable view while the operational logic stays
outside Bubble Tea. Very large diffs are intentionally truncated and untracked
content still requires deliberate inspection. Redaction targets the canonical
managed settings schema; unknown files remain visible in tracked patches so an
operator can detect accidentally added plaintext before any future commit.
