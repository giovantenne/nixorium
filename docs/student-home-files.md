# Initial student files

`home-files.json` adds non-secret files and empty directories to the existing
student workspace seed. The same declaration applies to the student account on
the controller and every client. Staff homes remain persistent. Changing a
declaration prepares a new seed; controller activation and client deployment
have their own reviews. A student home receives the new seed at the next normal
boot. Saving or switching a system never resets a running session.

The private repository owns this declaration and captured payloads. Local
imports are copies: changing the original controller file does not update the
seed. Updating an imported file requires a new capture and review. Removing a
declaration removes it from the next seed; previous Git revisions may retain its
payload. Do not put passwords, tokens, private keys or other confidential data
in this configuration: payloads enter Git, the Nix store, installer and cache.
Credential marker checks are best effort, not a confidentiality guarantee.

## Controller workflow

Open **Maintenance → Change settings → Student workspace → Initial files**. Add a
local file or folder, choose a relative destination in the student home, and
review the captured tree. Tab completes a typed source path; it never imports
an example hint. Imported files start non-executable regardless of their source
mode. Explicitly select any files that should be executable. Update captures a
new source for the selected destination; Remove changes the declaration.
Neither action deletes a student's current session files.

The save review lists additions, replacements, removals and permission changes,
then the complete resulting declaration. Confirming saves the manifest and
captured copies and records exactly those paths in local Git. Existing edits
to the managed files require Git inspection first; unrelated index paths remain
unchanged. Controller application and client distribution are separate reviews.

The CLI can capture and review without changing the deployment:

```sh
nixorium home-files import plan --repo . --source /PRIVATE_SOURCE_DIRECTORY/course --destination Documents/Course --json
```

Save that JSON report into a private file outside the deployment. The report
contains a candidate and a content-bound review token. `--replace` explicitly
replaces an already declared destination tree. A raw `home-files.json` candidate
or a draft/report can also be reviewed:

```sh
nixorium home-files plan --repo . --file /PRIVATE_TEMP_DIRECTORY/home-files-review.json --json
```

After reviewing the complete result, the operator may save with its current
token:

```sh
nixorium home-files apply --repo . --file /PRIVATE_TEMP_DIRECTORY/home-files-review.json --expect REVIEW_TOKEN
```

Interactive apply asks for confirmation; non-interactive apply needs explicit
`--yes`. CLI apply writes the manifest and required copies only; it does not
commit. Review and commit these exact paths before system application. The
ordinary TUI performs that local record as part of its visible save.

Capture IDs refer to private controller state, work across CLI processes, and
expire after 24 hours. At most 16 captures are retained. Failed or cancelled
captures clean their own files; abandoning a TUI draft discards its captures,
and a verified TUI commit discards its used captures. Expired or unavailable
captures require choosing the source again. Partial saves or unconfirmed commits
require **Maintenance → Review Git changes** (`nixorium git review`), followed by a new
review; never replay the earlier token. Publication places verified immutable
copies first and replaces the manifest last, without claiming an atomic
multi-file transaction.

## Declaration

```json
{
  "schemaVersion": 1,
  "directories": ["Documents/Exercises/empty"],
  "files": [
    {"destination": "Documents/Exercises/README.txt", "text": "Start here.\n"},
    {"destination": ".config/ghostty/config", "text": "theme = dark\n"},
    {"destination": "bin/example", "text": "#!/bin/sh\nexit 0\n", "executable": true}
  ]
}
```

A file has exactly one of `text` or `source`. Text is UTF-8, limited to 64 KiB
per entry. An imported source has the form `imports/<64 lowercase SHA-256 hex
digits>` and names a regular file under `home-files/`. The source digest must
match its name. Sources do not retain the original controller pathname.
The declaration is limited to 1 MiB and 512 explicit entries; imported files
are limited to 16 MiB each and the complete additional file payload to 64 MiB.
Destinations are plain relative paths, at most 512 bytes and 12 components.
Duplicate destinations, file/directory conflicts, traversal, symbolic links,
credential locations, startup hooks and generated workspace paths are refused.

Files belong to the configured student account and its primary group. Their
initial mode is `0600`; directories are `0700`. An explicit `executable: true`
sets that file to `0700`. Source ownership, permission bits, setuid/setgid,
ACLs, capabilities and other extended attributes are not copied. Students can
edit, remove or chmod their session files; ownership does not provide a
read-only policy. The next normal boot restores the declared baseline.

## Public API and compatibility

`mkLab.homeFilesJSON` accepts the raw JSON declaration, preserving duplicate-key
validation. When omitted, it reads a present deployment-owned `home-files.json`,
otherwise an empty declaration. `mkLab.homeFilesRoot` is an optional path inside
the deployment or upstream source tree, defaulting to a present deployment
`home-files` directory. This adopts the feature on older deployment flakes
without requiring a flake edit. Explicit arguments remain authoritative. The site
template
passes its `home-files.json` and `./home-files`. Existing hand-written callers
receive the same preference/scaffold behavior when both arguments are omitted.

`nixoriumHomeFiles` exposes preparation metadata and the common destinations;
its seed path is not evidence of a built or active home.
`nixoriumResolveHomeFilesCandidate` and `nixoriumValidateHomeFilesCandidate`
review raw candidate JSON without changing deployment files. Local callers must
also inspect captured payload sizes and compose downstream validation before
authorizing a save. Both inputs are preserved in the offline installer.

Imported payload bytes are omitted from direct Assistant file reads, search
and Git history. Inline text is visible configuration and can be prepared with
the existing candidate file tools. Tool filtering does not isolate imported
materials from arbitrary Nix expressions evaluated in a candidate: the files
remain build inputs. Shared materials must therefore be non-secret.

See [the scenario matrix](student-home-validation.md) for executed checks,
commands and qualification limits.
