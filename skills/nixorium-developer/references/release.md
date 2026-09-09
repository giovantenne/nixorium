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
5. Run `./scripts/validate.sh`.
6. Commit the metadata as `chore: prepare v<version>` and push it to `master`.
7. Run `./scripts/release.sh <version>` only with explicit authorization.
8. Verify the GitHub workflow and published release.

The release workflow repeats the full validation matrix and rejects a tag when
`VERSION`, `DEFAULT_RELEASE`, or the changelog section does not match.
Use only the annotated tag created by `scripts/release.sh`; never create, move,
replace, or push tags as an implicit part of implementation work. If publishing
fails after the tag is pushed, inspect the tag and workflow before retrying.
Do not retag a different commit under an existing version; fix forward with a
new version unless the user explicitly chooses another recovery procedure.
