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
7. Run `./scripts/release.sh <version>` only with explicit authorization.
8. Verify the GitHub workflow and published release.

The release workflow first checks metadata, then runs
`./scripts/validate.sh --full` on the exact tagged commit, including VM tests,
representative system builds and offline equivalence. A separate publication
job depends on successful full validation and consumes its validated release
notes. Failure, cancellation or timeout must never publish a GitHub Release.
The tag already exists while validation runs; it is not proof of qualification.

Full CI runs only for release tags, including prereleases, never on ordinary
pushes to `master`. The hosted runner enables KVM, runs one Nix build at a time,
and retains the validation log as an artifact. Its 360-minute job limit is
GitHub's maximum, not an unlimited execution guarantee. Nix has no additional
build/silence timeout; individual test deadlines remain intact.
Use only the annotated tag created by `scripts/release.sh`; never create, move,
replace, or push tags as an implicit part of implementation work. If publishing
fails after the tag is pushed, inspect the tag and workflow before retrying.
Do not retag a different commit under an existing version; fix forward with a
new version unless the user explicitly chooses another recovery procedure.
