# Upstream releases

Nixorium follows Semantic Versioning. Prereleases use identifiers such as
`2.0.0-beta.3`; tags add the `v` prefix.

Choose the increment from compatibility impact: patch for compatible fixes,
minor for compatible functionality, and major for incompatible public API or
deployment-schema changes. While the project is on a prerelease line, advance
the prerelease identifier unless the user explicitly approves a stable release.

Before release:

1. Start from a clean `master` synchronized with `origin/master`.
2. Select the version from its compatibility impact.
3. Update `VERSION`, `DEFAULT_RELEASE` in `install.sh`, and the released tag in
   `templates/site/flake.nix`.
4. Move Unreleased notes into a dated changelog section and update links.
5. Run the focused local checks; use `./scripts/validate.sh --full` locally
   when full preflight is practical. GitHub repeats the complete gate regardless.
6. Commit the metadata as `chore: prepare v<version>` and push it to `master`.
7. Website release branch: confirm that `release/<version>` exists in
   `nixorium-website`, is built against this commit and is green on `staging`
   (see [Website release branch](#website-release-branch)). Do not tag without it.
8. Run `./scripts/release.sh <version>` only with explicit authorization.
9. Verify the GitHub workflow and published release.

The release workflow first checks metadata, then calls the full-validation
workflow, which runs `./scripts/validate.sh --full` on the exact tagged commit
as parallel shards, including VM tests, representative system builds and
offline equivalence. A separate publication
job depends on successful full validation and consumes its validated release
notes. Failure, cancellation or timeout must never publish a GitHub Release.
The tag already exists while validation runs; it is not proof of qualification.

Full CI runs for release tags, including prereleases, nightly on `master` and
on demand, never on ordinary pushes. Before tagging, check that the latest
nightly or a manual run passed on the commit to release. Each hosted shard
enables KVM, runs one Nix build at a time, and retains its validation log as an
artifact. Its 240-minute job limit is not an unlimited execution guarantee. Nix
has no additional build/silence timeout; individual test deadlines remain
intact.
Use only the annotated tag created by `scripts/release.sh`; never create, move,
replace, or push tags as an implicit part of implementation work. If publishing
fails after the tag is pushed, inspect the tag and workflow before retrying.
Do not retag a different commit under an existing version; fix forward with a
new version unless the user explicitly chooses another recovery procedure.

## Website release branch

The website for a release is prepared before the release and published at the
moment of the release. While the features are developed, `nixorium-website` gets
a branch `release/<next version>` (for example `release/3.8.0`) from its
`master`; its copy describes the new features from the product documentation at
the commit it is built against, with
`NIXORIUM_PRODUCT_ROOT=<clean product checkout> NIXORIUM_PRODUCT_REV=<commit> npm run build`
(and `npm run demos:generate` when a demo changes), and it is pushed to
`staging` for review. A preview branch may pin an unreleased commit.

After the tag, once the release workflow has published the release, finish the
site in the same session, on that branch:

1. Run `npm run demos:generate` at the tag (or update the pin) and commit
   "Pin the site to Nixorium v<version> (<sha>)".
2. Review the technician guide against the released changelog and documentation.
3. Check with `SITE_RELEASE_CHECK=strict npm run build`, merge into `master`, and
   push `master` and `master:staging`.

Production therefore changes in the same moment as the release. The website's
production build fails when its pinned commit is not a published release, so
`master` cannot move ahead of the tag. The website README, section "Release
branches", holds the same process.
