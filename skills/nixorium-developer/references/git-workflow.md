# Upstream Git workflow

Keep `master` releasable and its history linear. Inspect the worktree, current
branch, remotes, and divergence before staging or changing history. Fetch before
integrating remote work; update `master` only with a fast-forward.

## Commits

Create focused commits whose diff represents one coherent change. Do not stage
unrelated user changes, generated build results, private deployment data, or
secrets. Review the staged diff and run the validation appropriate to the
change before committing.

Use the repository's Conventional Commit-style subjects:

```text
feat: add deployment readiness output
fix: preserve CIDR host calculation
docs: document downstream migration
refactor: split shared desktop modules
test: cover invalid lab networks
ci: validate offline installer equivalence
chore: prepare v2.0.0-beta.4
feat!: change the lab network schema
```

Write a concise, imperative, lowercase subject without a trailing period.
Choose the prefix for the user-visible purpose, not merely the files touched.
Use `!` for a breaking change and document its migration in `CHANGELOG.md`.
Add a body when the reason, compatibility impact, migration, or validation is
not obvious from the diff. Do not amend, squash, or rewrite commits belonging
to someone else unless explicitly authorized.

## Branch integration and merges

Before integration, fetch `origin` and compare the branch with
`origin/master`. Resolve conflicts from an understanding of both changes; do
not accept one side wholesale when that can discard unrelated work. Re-run the
full affected validation after conflict resolution.

Prefer a linear result:

- Fast-forward when the branch already forms the desired commit sequence.
- Squash a branch when all of its commits implement one logical change.
- Rebase a curated local branch when preserving its separate commits adds
  value and rewriting it will not disrupt collaborators.
- Create a merge commit only when preserving branch topology is intentional.

Never force-push `master`. Do not force-push a shared branch without explicit
authorization; if rewriting an authorized private branch, use
`--force-with-lease`. Do not merge or push while required validation is failing.

## GitHub CI

`.github/workflows/validate.yml` runs for pull requests and pushes to `master`.
After pushing or merging, verify that the intended commit is reachable from
the remote target branch and monitor its GitHub Actions run to completion. A
green local validation does not replace the required remote result. If CI
fails, inspect the failing job and logs, fix forward in a new commit, validate,
and push again; do not rewrite `master` to hide the failed commit.

`.github/workflows/release.yml` is separate and runs only for release tags. Its
success is part of release verification, not ordinary integration validation.
Report checks that cannot be observed or are still running. Deleting a branch
is a separate cleanup action, not an implicit part of the merge.

Commits, pushes, rebases of published work, merges, force-pushes, and branch
deletion require authorization appropriate to their external or destructive
effect. Authorization for one of these actions does not imply the others.
