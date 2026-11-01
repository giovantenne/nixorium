# Initial student files: validation scenarios

This ledger records the additional initial-file feature, not physical hardware
certification. `PASS` means the named check ran successfully; `FAIL` means it
ran and failed; `NOT RUN` means no execution evidence is recorded. An evaluated
seed is not a deployed or active home. Commands use synthetic repositories and
disposable VMs; no real laboratory controller or student machine was changed.

Evidence below was collected on 2026-10-13. Source commits include `4e4bd7b`
(schema/seed), `5162a8c` (capture/save), `316878c` (CLI/TUI) and `25a2ed7`
(runtime fixture).
The production-client preflight used the exact frozen core `f568aa4`; its
evidence and the runtime result are recorded separately below.

| Scenario | Status | Executed evidence |
|---|---|---|
| Inline lesson text and application configuration under `.config` | PASS | Shared Go/Nix schema corpus; `home-files-seed`; offline reset-service VM |
| Imported binary bytes remain identical to the captured source | PASS | `home-files-seed` compares a NUL-containing binary; reset-service VM compares restored bytes |
| Empty directories survive seed composition and boot restoration | PASS | `home-files-seed`; reset-service VM |
| Source executable bits do not grant executability | PASS | `TestHomeFilesCaptureAndCleanup`; imported VM fixture starts non-executable |
| Explicit executable file starts `0700`; normal file `0600`; directories `0700` | PASS | `TestAdditionalHomeFileIntegrityAndModes`; reset-service VM checks actual restored modes |
| Files belong to configured student UID and primary GID | PASS | Reset-service VM verifies UID 2000 and GID 100 on its synthetic account |
| Source extended attributes are omitted from fresh private capture files | PASS | `TestHomeFilesCaptureAndCleanup` checks source mode and a user xattr against the captured inode |
| Symbolic links, symlink ancestors, hard links and special files are refused | PASS | `TestHomeFilesCaptureRejectsUnsafeSources`; runtime no-follow link/protected-path tests |
| Traversal, duplicate/aliased JSON keys, NUL, file/directory collisions, generated paths, credentials and startup hooks are refused | PASS | `home-files-validation-cases.json` in Go and Nix; strict draft/report reader tests |
| File/text/depth limits are enforced; complete seed has reset-engine count/size guards | PASS | `TestHomeFilesBounds`; schema corpus; seed build executes the complete-tree guards |
| Changing original controller bytes after capture does not alter the reviewed copy | PASS | `TestHomeFilesCaptureAndCleanup`; `TestHomeFilesRealNixReviewedSave` |
| Update and removal require a new review and show their effects | PASS | `TestHomeFilesReplacementRemovalAndDriftDoNotMutateReviewedObjects`; reset-service VM restores updated files and removes the old declared file |
| Capture cancellation cleans its own files; an abandoned draft can discard its captures | PASS | Capture rejection/cancellation tests; TUI cancellation/state tests; private capture cleanup test |
| Draft/capture metadata changes invalidate authorization | PASS | App metadata/provenance tests; real Nix test rejects tracked-source drift and old review reuse |
| A partial save remains inspection-only and offers Git recovery | PASS | `TestHomeFilesReviewProvenanceAndPartial`; `TestHomeFilesPartialSaveRequiresGitInspection` |
| Local record preserves unrelated staged paths and commits exact manifest/payload bytes | PASS | `TestHomeFilesRealNixReviewedSave` with real Git and Nix |
| Large imported text is reviewed as metadata; add/delete/mode changes work; unchanged blobs cannot create a no-op commit | PASS | `TestHomeFilesOpaqueGitProposal` uses a payload larger than the ordinary 256 KiB patch limit |
| Student edits/chmod survive a system switch; next boot restores the new baseline | PASS | Reset-service VM modifies session files, switches the seed, verifies unchanged live content, then actually reboots |
| Boot restoration needs no controller/network reachability | PASS | Reset-service VM has no NIC and checks absent non-loopback links/routes during initial, updated and failed boots |
| Durable pending-reset evidence blocks login and manual retry across reboot | PASS | Reset-service VM retains the pending marker and verifies failed home-reset/user-session/display-manager units |
| Imported bytes and Git blob aliases stay out of direct Assistant reads/search/history; inline text remains editable | PASS | `TestAssistantImportedPayloadAliasesStayLocalAcrossAncestry`; pinned-capability and generated-text candidate tests |
| Older deployment callers adopt a present tracked manifest without adding new mkLab arguments | PASS | `home-files-adoption` builds and compares actual adopted text/binary files |
| Offline installer preserves the adopted declaration and source root exactly | PASS | `home-files-adoption` requires the real offline derivation-equivalence check |
| TUI actions remain reachable after preference failure; late reads and cancellation preserve drafts | PASS | Focused presentation state-transition tests |
| Review/result views fit 80×24, 120×30 and 180×45, including limited-color and light/dark modes | PASS | `TestHomeFilesRenderGallery`; deterministic render gallery |
| Locked static analysis and Go vulnerability checks | PASS | `scripts/security-check.sh`; no used vulnerable symbols reported |
| Representative controller and client systems build together | PASS | Batched `pc99` and `pc01` toplevel builds |
| Complete evaluation/quick gate on the final integration | PASS | `./scripts/validate.sh --eval` completed, including the quick/package/guidance checks and complete mkLab evaluation |
| Local production-client preflight on exact core `f568aa4` | PASS | Native capability discovery, candidate evaluation and ready local review completed in a synthetic fixture. Build was partial: two build targets built, five targets blocked and home preparation skipped with `needs_other_builds`; private qualification helper code is not part of this public ledger |
| Assistant qualification of the home-files tools | PASS (private) | Qualification of the service side is kept outside this repository |
| Exact frozen generated candidate restores after student edits and executable choice | PASS | Original combined generated candidate: seven files and one empty directory, verified on three boots with two actual reboots after student mutations; exact hashes, UID 2000/GID 100, private modes, explicit executable restoration and offline operation |
| Physical controller/student hardware, real GNOME interaction and real classroom data | NOT RUN | Outside this synthetic qualification |
| Power-loss or injected filesystem fsync failure during native multi-file publication | NOT RUN | Partial-result classification is covered above; this separate filesystem fault was not executed |

The original generated declaration had SHA-256
`a03ffe252b47d596f079c433b795758af4f50b737a9c628eb3318d35e1077323`.
The separate runtime test produced
`2kwbrjq6kmfhmqqgb1lq4m5n41m01xha-vm-test-run-nixorium-original-generated-home-files-runtime`.
It checked exact file hashes, UID/GID and modes on all three boots, with no NIC
and no default IPv4 or IPv6 routes. The root-reviewed harmless script was run
manually on each boot and returned exit 0 with its exact expected greeting. An
initial offline fixture-only attempt failed Python type checking before boot;
that fixture error was corrected and its log retained before the successful
run.

## Reproduction commands

```sh
./scripts/test-go.sh ./internal/domain ./internal/app ./internal/adapters ./cmd/nixorium -run 'HomeFiles|GitCommit|GitReview' -count=1
nix --extra-experimental-features 'nix-command flakes' develop --file tests/source-checks.nix go-shell --command env NIXORIUM_TEST_WORKSPACE_NIX=1 go test ./internal/adapters -run TestHomeFilesRealNixReviewedSave -count=1 -v
./scripts/test-go.sh ./internal/presentation -run 'TestHomeFiles|TestWorkspace|TestSave|TestDemoRendererFits' -count=1
nix --extra-experimental-features 'nix-command flakes' build --file tests/source-checks.nix home-files-schema home-files-seed workspace-seed --no-link --option allow-import-from-derivation false
nix --extra-experimental-features 'nix-command flakes' build .#checks.x86_64-linux.home-files-adoption --no-link --option allow-import-from-derivation false --option accept-flake-config false
nix --extra-experimental-features 'nix-command flakes' build --file tests/source-checks.nix workspace-reset-service-tcg --no-link --print-out-paths
./scripts/validate.sh --eval
nix --extra-experimental-features 'nix-command flakes' develop --file tests/source-checks.nix security-shell --command ./scripts/security-check.sh
nix --extra-experimental-features 'nix-command flakes' build .#nixosConfigurations.pc99.config.system.build.toplevel .#nixosConfigurations.pc01.config.system.build.toplevel --no-link --no-write-lock-file --option accept-flake-config false
```

The final local TCG VM completed its test script in 35.79 seconds, producing
`7giymai8j06r15fi3n202kq7s0dzkamd-vm-test-run-nixorium-workspace-reset-service`.
TCG avoids requiring local `/dev/kvm`; the ordinary CI check retains KVM.
The adopted-source/offline check produced
`scx6nfqn8s3hyq8jwk5q12z3yj43vl39-nixorium-home-files-adoption-check`.
Controller/client derivations were
`g1amda1vkfnbfdy65xpp2mq2asrf069y` and
`q6s7562bfjbqz9wm8vai5r3ja2mhwkva` respectively.

The VM uses a small login consumer to exercise systemd ordering and failure
propagation. It does not certify GNOME behavior or physical firmware. Credential
marker checks and tool-facing Assistant filtering are best effort: shared
materials must be non-secret because Git, the Nix store, installer and cache
retain them. These checks do not establish an absolute isolation boundary
against arbitrary candidate Nix expressions.
