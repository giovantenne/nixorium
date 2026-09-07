# Release and downstream update

The upstream follows Semantic Versioning. Prereleases use identifiers such as
`2.0.0-beta.2` and Git tags use the corresponding `v2.0.0-beta.2` form.

## Prepare the upstream release

1. Start from a clean `master` synchronized with `origin/master`.
2. Choose the version from compatibility impact and the user's requested
   release channel.
3. Update `VERSION`.
4. Move the curated Unreleased notes into a dated changelog section and update
   comparison links.
5. Run the full validation matrix, including a freshly generated deployment
   and offline derivation equivalence.
6. Commit the release metadata and push `master`.
7. Run `./scripts/release.sh <version>`. It verifies metadata, creates an
   annotated tag, and pushes it. GitHub Actions publishes the release and marks
   hyphenated versions as prereleases.
8. Verify the workflow and GitHub release instead of assuming tag push implies
   publication success.

Do not create or move a release tag manually around failures unless the user
has explicitly approved the recovery operation.

## Update each private deployment

After the upstream tag exists:

1. Change `inputs.nixos-lab.url` to the released tag.
2. Update only the `nixos-lab` input in `flake.lock`.
3. Review the lock diff and release notes.
4. Evaluate `labMeta`, build the controller, one client, netboot ramdisk, and
   installer bundle.
5. Commit and push the lock update.
6. Deploy only when separately authorized.

Never merge upstream Git history into the deployment repository. A Flake input
and its lock file are the update boundary.
