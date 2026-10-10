# Assistant

Assistant is an optional subscription service for preparing configuration
changes from the administrator's Nixorium menu. It works on a private candidate
workspace. You review the actual changes and choose whether to save them.
Saving records a local Git commit and a backup; applying systems is separate.

## Enable it on the controller

Use a Nixorium version that provides Assistant and `nixoriumHostClasses`.
In the deployment's existing `lab-settings.json`, set `assistant` to `true`
inside the `lab` object, preserving the other settings:

```json
"assistant": true
```

Review and commit the configuration change, then use the ordinary controller
application review. Once that controller configuration is active, **Assistant**
appears as the fifth administrator menu entry (`m`). It is absent by default,
on clients and from the teacher's classroom dashboard. Setting the option to
false and applying the controller removes the menu entry.

The controller installs `/etc/nixorium/assistant.json` and creates the private
state directory for `admin`. Service URL overrides are for staging and tests;
ordinary deployments use the installed defaults. Existing deployments whose
pinned Nixorium lacks the host-class output must update before using evaluation.

## Subscription, activation and consent

Write a request, then press **Ctrl+S** to send. **Enter** inserts a new line;
**Ctrl+N** starts a new chat. **Esc** stops a running turn without changing the
active deployment. A stopped chat may retain candidate edits for later review.

Without a saved token, no HTTP request is made: the screen points to the
subscription page. After payment, press **a** and enter the activation code
shown there. Activation saves the laboratory token locally and opens a separate
consent screen. Nothing from the chat is sent until you accept that consent.
An inactive subscription returns to the renewal prompt without sending content;
an invalid token is removed and requires activation again.

The free trial includes five requests for thirty days, with one trial per email
address. Register with your email, school name and privacy consent on the
service's `/trial` page, then enter the code delivered by email. Before using
the code, you can submit the form again to replace it if delivery failed.
After activation, the screen shows **Free trial: N of 5 requests left**.
The controller checks account metadata without sending your draft or files,
and refreshes that count after a request.

When the trial expires or its requests are used, Assistant shows a signed
purchase link from the service. The link lasts seven days; reopening Assistant
fetches a fresh link. Completing the subscription converts the same account
and keeps the controller token active, so no new activation code is needed.
An annual account has no trial purchase link: after using its included requests,
wait for renewal or contact `hello@nixorium.org` for a manual agreement.
Each account enables one laboratory; a replacement code for a reinstalled
controller disables the previous token while preserving quota and period.

Each admitted turn first opens through a request containing client version and
tool version only. The subsequent steps send the conversation, visible file
names, requested file contents and tool results to the Assistant service and
its AI provider. The service does not retain prompt, file or tool-result
content. Do not enter passwords or student data in a request.

The model sees tracked files from the chat's base commit plus its own candidate
files. Untracked and ignored files, private-key paths, `keys/`, `.git`, `.ssh`,
`.env*`, credentials and agent-instruction directories are hidden. Settings,
input locks, instruction files and Git control files are read-only. Sensitive
settings fields are withheld; known token formats, password hashes and private
key material are redacted from model-facing output. The laboratory token is
kept separate from the transcript. Logs, student homes and arbitrary files
outside the workspace are not tool inputs.

## Tools and limits

The model has seven fixed tools, with no shell or system-operation tool:

| Tool | Scope and limit |
|---|---|
| `list_files` | Visible workspace paths; at most 2,000 entries |
| `read_file` | UTF-8 text; at most 64 KiB per call |
| `search` | RE2 regular expression; at most 200 matches, 300 characters per matching line |
| `write_file` | Candidate text only; 512 KiB per file, 4 MiB and 200 changed paths per conversation |
| `delete_file` | Existing, unprotected candidate files only |
| `search_packages` | Deployment's pinned package search; at most 50 results, 60-second timeout |
| `evaluate` | Isolated temporary Git clone; five minutes, bounded output, no import-from-derivation |

Paths must be relative, with no traversal, backslashes or control characters;
the limit is 12 components and 512 bytes. A file cannot replace a directory
that still contains visible files. Tools cannot build or activate a system,
manage services, deploy computers, or save a proposal into the deployment.
Evaluation errors are redacted and limited to 8 KiB. Passing evaluation proves
that Nix can evaluate the candidate; it does not prove a successful build,
activation, boot or safe behavior of arbitrary configuration code.

## How affected computers are calculated

`mkLab` exports `nixoriumHostClasses`: a list of records with `representative`
and `members`, sorted by representative. Every configured host belongs to
exactly one class. The controller is alone; each host in `hostModules` or
`updateValidationHosts` is alone. Other clients share a class when their
applicable software, network interface and non-empty module groups match. The first member in inventory
order represents that class.

Assistant evaluates one representative per candidate class, comparing its
system derivation with the same host at the base commit. All members are marked
changed when that derivation changes. Added or removed hosts and hosts whose
assigned representative changes are also included. Membership changes alone
do not mark the other members changed.

For example, if `pc02` leaves `{pc01, pc02}`, only `pc02` is included when the
representative `pc01` and its system stay unchanged. If `pc01` leaves instead,
`pc02` receives a new representative and is included too, even if its own
system is unchanged. This deliberate conservative overestimate can report an
extra computer rather than miss a potentially affected one.

For common behavior on selected clients, `groupModules.<group>` assigns module
paths to an existing `clientGroups.<group>`. Prefer this explicit shared scope
over repeating the same module in `hostModules`. Group membership is combined
with software and interface differences; overlapping groups form distinct
combinations. Empty module lists do not create extra classes. The controller
cannot belong to a client group.

Group modules declare common behavior within each class; this is a configuration
contract, not automatic proof that arbitrary Nix code is equivalent. A private
module that branches on `networking.hostName` must declare the
exceptional hosts in `hostModules` or `updateValidationHosts`. Otherwise the
representative cannot establish their individual behavior. Class/base results
are cached only within the conversation; the candidate is evaluated afresh.
System derivations are evaluated in sequential batches of at most two
representatives per Nix process, on both sides. This limits memory accumulation
when many hosts require separate classes. All batches share the same five-minute
deadline; a failed or canceled batch stops evaluation without a valid partial
result.

## Review and save

Start from a clean, committed deployment. If HEAD changes during a chat,
start a new chat. Once files change, **Ctrl+R** opens **Assistant / Proposal**.
The assistant's summary is its opinion; **Checked by Nixorium** reports the
locally computed file counts, affected computers and final evaluation result.
Read the full comparison with arrows, PgUp/PgDn and Home/End.

Type **SAVE**, then **Enter**, to record the reviewed proposal. The complete
diff must fit within 2 MiB and the evaluation must pass. A review expires after
ten minutes; changes to the candidate or deployment require another review.
The save repeats evaluation and checks the proposal before taking the managed
operation gate and replacing files. An active PXE operation blocks checkout.

The new commit is `Save reviewed Assistant proposal`. Its parent and a durable
`refs/nixorium/assistant-backups/…` ref preserve the previous committed tree.
The result displays the backup. Nothing is built or activated by saving.

The next action opens the ordinary **Apply to this controller** review. After
verified application at the saved revision, **Update computers** opens client
selection with fresh inventory and another review. Neither action runs
automatically. Use a new chat for further proposals after saving, since the
previous conversation remains tied to its original base commit.

## Interrupted save

Keep the backup and `.git/nixorium-template-reset.json`. An Assistant save
uses this shared recovery journal with `kind: "assistant-save"`; it blocks
other configuration operations until recovery. Choose **r / Recover** from
the interrupted-save result, or open the pending condition in **Overview**.
The Overview condition currently retains the title **A deployment template
reset was interrupted**; the recovery report identifies the Assistant save.

The existing [reviewed recovery workflow](deployment-template-reset.md#backup-and-recovery)
checks Git and decides whether to finish the commit, clear an untouched intent,
or preserve diverging files and restore the prior configuration. Follow that
review and its confirmation. Do not remove the marker or replay the save to
bypass it. Recovery never applies the system.

## Local data and retention

`/var/lib/nixorium/assistant/` is owned by `admin` with mode 0700. It contains
the token (0600), `consent.json`, and `conversations/<uuid>/`. Conversations
hold metadata with the base commit and creation date, a private transcript,
`overlay/`, `deleted.json`, `modes.json`, `base-classes.json` and `base-drv.json`.
Temporary evaluation clones live inside the conversation and are removed when
evaluation finishes. Local transcripts and candidates may contain private
configuration; they are separate from the deployment and its Git history.

Opening Assistant removes conversations created more than 30 days ago. This
cleanup does not revoke the subscription token, remove consent, or delete Git
backup refs. Starting a new chat also leaves the previous local conversation
until that cleanup. Disabling the controller option does not erase this data.
