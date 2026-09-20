# ADR-0010: Reviewed local configuration history

- Status: accepted, amended for transparent application saves
- Date: 2026-09-14

## Context

Nixorium should record reviewed deployment changes without absorbing unrelated
administrator work. Git is an implementation detail in ordinary setup,
settings, and guided software flows; the update flow must migrate to the same
contract. Its terminology remains available only in Advanced and automation
interfaces.
A broad `git add`/`git commit`
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
Apply repeats the entire plan and requires that token. Advanced interactive Git
commands additionally require the exact one-word `COMMIT` confirmation (or
explicit automation-only `--yes`). An application-level `Save configuration`
operation may consume its own just-created token without exposing Git or a
second confirmation after the user has reviewed and accepted the configuration
change itself.

After one final proposal/tree check, use `git commit-tree` and compare-and-swap
`git update-ref` to advance HEAD only from the reviewed parent. Then reset only
the selected index paths to the new HEAD. This preserves unrelated staged,
unstaged, and untracked state while supporting selected new files. No repository
hook, signing helper, remote, or push is invoked. Commits created by Nixorium use
a fixed internal author and committer identity, independent of user-level Git
configuration. Report a partial non-retryable result if HEAD advanced but
selected-index reconciliation failed.

## Consequences

The history mechanism is local; normal Git remains available to experts.
Conservative secret and Git-attribute checks can reject a legitimate unusual
file, which can still be committed manually after deliberate review. The
operation does not validate or deploy the full laboratory configuration; normal
configuration, build, and deployment gates remain separate.
