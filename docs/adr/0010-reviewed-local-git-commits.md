# ADR-0010: Reviewed local Git commits

- Status: accepted
- Date: 2026-09-14

## Context

Nixorium should optionally record reviewed deployment changes without hiding
Git or absorbing unrelated administrator work. A broad `git add`/`git commit`
can silently include existing index entries, invoke configured filters, hooks,
or signing programs, race with a prior review, and encourage an implicit remote
workflow. New generated public-key files also need support without first
mutating the real index merely to construct a plan.

## Decision

Use a separate plan/apply workflow with an explicit comma-separated allowlist
of clean repository-relative paths. Planning starts a private temporary Git
index from HEAD and stages only those paths there. Before accepting a proposal,
reject conflicts, known private paths, unknown or unchanged selections,
directories, symbolic links or linked-parent traversal, rename/copy changes,
invalid managed settings, active filter/working-tree-encoding/ident attributes,
patches larger than 256 KiB, and recognizable private-key, access-token, or
plaintext secret additions.

The proposal contains the exact tree ID, redacted diff, fixed generated commit
message, and a SHA-256 token bound to parent revision, tree, paths, and message.
Apply repeats the entire plan and requires both that token and an exact
`COMMIT <token-prefix>` confirmation (or explicit automation-only `--yes`).

After one final proposal/tree check, use `git commit-tree` and compare-and-swap
`git update-ref` to advance HEAD only from the reviewed parent. Then reset only
the selected index paths to the new HEAD. This preserves unrelated staged,
unstaged, and untracked state while supporting selected new files. No repository
hook, signing helper, remote, or push is invoked. Report a partial non-retryable
result if HEAD advanced but selected-index reconciliation failed.

## Consequences

The workflow is local and optional; normal Git remains available to experts.
Conservative secret and Git-attribute checks can reject a legitimate unusual
file, which can still be committed manually after deliberate review. The
operation does not validate or deploy the full laboratory configuration; normal
configuration, build, and deployment gates remain separate.
