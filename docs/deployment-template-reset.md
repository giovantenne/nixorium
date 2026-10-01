# Reset a private deployment template

Updating the Nixorium input changes the framework, not the files copied into
your private deployment. An older `flake.nix` can therefore lack the workspace
catalog and candidate hooks even after an update. A template reset is an
explicit replacement of local customizations, **not** a framework update or a
reinstallation. Do not use it merely to refresh an application.

## Operator workflow

1. Resolve and commit tracked local edits first. Stop PXE and finish other
   installation/deployment operations. Nothing is automatically stashed.
2. Open **Maintenance → Reset deployment template** in the administrator TUI.
   The teacher dashboard does not expose this action.
3. Select a replacement software preset from the template at the exact
   framework revision already recorded in your `flake.lock`.
4. Review the complete scrollable list of preserved, added, replaced and
   removed paths. The software selection replaces the old selection; it is
   not the additive software-profile workflow. Local modules may contain
   important policies; their removal is intentional only after this review.
5. Type `RESET DEPLOYMENT` exactly and press Enter. The operation creates a
   durable local backup ref before replacing tracked files and creating a new
   local commit. Escape cancels selection/review without writing the deployment.
6. Keep the backup ref shown in the result. The three status lines distinguish
   the saved configuration, controller application and unobserved client state.
   The primary action offers a separate controller review; nothing activates
   just by finishing the reset. After verified application at the saved revision,
   the next action opens fresh client selection and normal deployment review.
   Esc leaves either follow-up without starting it. Reboot normally after system
   application to seed student homes; this is never done automatically.

The reset never pushes, changes the framework/package-base pins, activates a
system, deploys clients, reboots or resets a live home. Review evaluates the
candidate hooks and representative system derivations; it does not claim a
successful system build. The later application/deployment performs its normal
build and readiness checks.

## What is preserved or replaced

Preserved byte-for-byte: tracked `lab-settings.json`, `flake.lock`, files under
`keys/`, and existing `.gitignore` files. All untracked and ignored files,
including private keys, remain in place and are not read into the candidate or
backup. Git history, remotes and repository configuration remain in place.
The candidate retains the current framework URL and NixOS channel declaration.
For older workspace-capable templates, it also makes output forwarding lazy so
the active profile can validate without a circular dependency on flake metadata.
This compatibility adjustment does not bypass validation or change the pin.

Other tracked files are reset to the pinned template: software/catalogs,
workspace preferences, assets, local modules, documentation and copied skills.
The selected preset is shared by the controller and clients. It also enables
the guided-home runtime with the template's initial profile; VS Code joins the
favorites when included in that preset. The existing student home and arbitrary
editor/plugin state are not captured or imported.

This changes the student's next-boot home behavior **after system application**,
on the controller as well as clients. Preserve needed student work through the
normal home/snapshot policy before rebooting. Staff homes are not reset by this
action. A deployment-template backup is not a backup of student home data.

## Safety boundaries

The first version supports ordinary, owned Git repositories with clean tracked
files and recognized direct GitHub framework/nixpkgs inputs. It refuses linked
worktrees, submodules, sparse/assume-unchanged entries, unsafe paths or file
ownership, hard links, Git content-conversion attributes, tracked private-key
material and collisions with any untracked/ignored path. Unsupported layouts
need a manual, reviewed migration; the action never uses `git clean` or a hard
reset to force them through. Tracked public keys should use the template's
`keys/` layout; effective key bindings are checked as well as preserved bytes.

The pinned upstream must support the guided-home catalog and runtime. Update
Nixorium separately if necessary. Effective host/network identity, disk devices,
mounts, account identities, public key bindings and `system.stateVersion` must
remain unchanged. A custom module that changes these values blocks the reset;
removing it requires a separate explicit migration.

## Backup and recovery

The result includes a ref such as
`refs/nixorium/template-backups/20260929T120000.123456789Z-0123456789ab`.
It points to the complete previous tracked commit and is not automatically
deleted or pushed. The new reset commit also has the old commit as its parent.
Ignored and untracked files are preserved in place, not copied into that ref.

To inspect the old tree, substitute the exact displayed ref:

```sh
git show --stat refs/nixorium/template-backups/EXACT-BACKUP
git diff refs/nixorium/template-backups/EXACT-BACKUP HEAD --
```

After a successful reset, a reviewed `git revert` of the reset commit can restore
the previous tracked configuration as a new commit. Inspect conflicts and newer
edits first; do not force an overwrite. Applying/rebooting that reverted system
is again a separate decision, not a student-data restore.

**Guided recovery.** The dashboard opens in safe mode with **A deployment
template reset was interrupted**; open it, or run:

```sh
nixorium template-reset recover plan
nixorium template-reset recover apply --expect REVIEW_TOKEN
```

The review decides from Git state only: when the reset already completed or
every file already matches it, typing `RECOVERED` finishes it (moving the
branch with a compare-and-swap when needed); when no file was replaced, it
keeps the original configuration. When the files are a mix of both, typing
`RESTORE` saves the current tracked files under a new
`refs/nixorium/template-backups/…-recovery` reference and restores the
configuration from before the reset as a new commit. Untracked and ignored
files are kept; an untracked file that the restore would overwrite stops the
review. The marker is archived under `.git/nixorium-recovered/`.

A multi-file checkout cannot be atomic. An interruption retains
`.git/nixorium-template-reset.json`, recording the original/candidate revisions,
branch, review token and backup ref. Normal apply/deploy preflight refuses this
marker even if Git otherwise appears clean. Do not remove it merely to bypass
the block, retry the reset, or run `git reset --hard`/`git clean`.

Keep the marker and backup; inspect Git status, the index and both recorded
commits locally. Preserve any additional edits/private files before recovery.
If every tracked file **and the index** match the candidate, the current branch
still matches the recorded branch, and HEAD is the recorded original commit,
a maintainer can finish the interrupted branch update with Git's compare-and-
swap `update-ref` using those exact recorded values. If HEAD already equals
the candidate, no branch update is needed. Verify the final tree and durability
before archiving the marker outside `.git` and clearing the block. Partial or
divergent checkouts require individual reviewed file recovery from the backup;
there is intentionally no blind automatic retry. Retain evidence if any step
has an uncertain result.
