# Assistant

Assistant is an optional subscription service for preparing configuration
changes from the administrator's Nixorium menu. It works on a private candidate
workspace. You review the actual changes and choose whether to save them.
Saving records a local Git commit and a backup; applying systems is separate.

## Availability on the controller

Assistant is offered by default on new installations. Existing deployments
with an omitted `lab.assistant` setting also gain the menu after updating to
Nixorium 3.4.0 or later and applying the controller through its ordinary review.
An explicit `false` remains disabled, including across settings saves.

To disable it, set `assistant` to `false` inside the `lab` object in the
deployment's existing `lab-settings.json`, preserving the other settings:

```json
"assistant": false
```

Review and commit the configuration change, then use the ordinary controller
application review. Once that controller configuration is active, the menu
entry is removed. Set it to `true` and apply the controller to enable it again.
When enabled, **Assistant** is the fifth administrator menu entry (`m`);
it is absent on clients and from the teacher's classroom dashboard.
Menu availability does not activate an account, accept consent or send requests.

The controller installs `/etc/nixorium/assistant.json` and creates the private
state directory for `admin`. The service defaults to `https://assistant.nixorium.org/v1` and the
subscription page to `https://nixorium.org/pricing/`; overrides are for
staging and tests only. Existing deployments whose
pinned Nixorium lacks the host-class output must update before using evaluation.

## Subscription, activation and consent

Assistant always opens the chat, even before activation; the header reads
"Not activated". Slash commands that need no service (`/help`, `/code`,
`/resume`, `/share`, `/name` where available) work as usual. Pressing Enter to
send a request without an activation sends nothing: your text stays in the
input and Assistant offers a free trial, an activation code or a subscription.
The public offer supplies the price and links; if it is unavailable, the
installed links remain usable without a price. For the trial and the
subscription, the next step shows the page address and the instruction to open
it; Enter opens it in your browser when a graphical session is available (over
SSH, copy or click the link instead), then asks for the activation code you
receive by email, which you can paste. Esc goes back one step, and finally to the
chat with the draft intact.
Activation saves the laboratory token locally and opens a separate consent
screen. Once you accept it, the waiting request is sent automatically, exactly
once. If you cancel activation or consent, nothing is sent and the draft stays
in the input.

You can select text by dragging with the mouse (Shift+drag in some terminals).
HTTP and HTTPS URLs remain visible and clickable through terminal OSC 8 links.

Examples are hints and cannot be selected. Write a request, then press **Enter**
to send. The input clears once the service starts the turn; a failure before
that point preserves the draft. **Alt+Enter** and **Ctrl+J**
insert a new line, and multiline paste is supported. **Ctrl+N** starts a new chat.
The input grows from one to six rows, then scrolls while keeping the draft.
**PgUp/PgDn** and arrows when the input is empty scroll the conversation.
Terminals with alternate scroll send arrows for the mouse wheel, without
capturing mouse selection. New content preserves a scrolled position;
**End** or sending a message returns to the bottom. Type `/` for the command menu:
`/review`, `/save`, `/new`, `/resume`, `/billing`, `/report`, `/share`, `/help`,
`/name` and `/code`. Enter or Tab completes
a choice; Enter on a complete command runs it. Unknown commands stay local.
**F1** opens help. **Esc** stops a running turn without changing the active
deployment; otherwise it returns to the previous area. A stopped chat may retain
candidate edits for later review.

Subscription, quota, network and deployment-conflict notices appear inside the
conversation and preserve the draft. An invalid token is removed, the chat explains that the activation is no
longer valid, and the next send asks to activate again. `/code` opens activation for a replacement code.
`/name` gives this laboratory a name (up to 60 characters), shown after the
school name in the header, on invoices and emails and in support, so several
laboratories of one school can be told apart; an empty name removes it.

The free trial includes ten requests for thirty days, with one trial per email
address. Register with your email, school name and privacy consent on the
service's `/trial` page, then enter the code delivered by email. Before using
the code, you can submit the form again to replace it if delivery failed.
After activation, the screen shows **Free trial · N of 10 left**. An ended trial
shows **Free trial ended**; exhausted annual quota shows **No requests left**.
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
its AI provider. The service does not keep your configuration files. It keeps the text of your
messages and Assistant's answers for 30 days to detect abuse, then deletes it.
Whole conversations, including files, saved history and diagnostics Assistant
read, are kept for 90 days
only if you report an answer or turn on sharing. Sharing is off by default. Do not enter passwords or student data in a request.

The model sees tracked files from the chat's base commit plus its own candidate
files. Untracked and ignored files, private-key paths, `keys/`, `.git`, `.ssh`,
`.env*`, credentials and agent-instruction directories are hidden. Settings,
input locks, instruction files and Git control files are read-only. Sensitive
settings fields are withheld; known token formats, password hashes and private
key material are redacted from model-facing output. The laboratory token is
kept separate from the transcript. Only bounded journal excerpts from fixed
core units are diagnostic inputs; arbitrary logs, student home files and other
files outside the workspace are not tool inputs.

## Subscription and payments

`/billing` shows a clickable temporary Stripe Customer Portal link in the chat.
It does not use an Assistant request or send conversation contents. Open the link
within a few minutes to manage your subscription and see payments. Trial accounts
without a subscription are told that `/billing` becomes available after
subscribing. The link stays out
of the transcript and model messages. The portal return page lets you close the
tab and return to Nixorium. The displayed invoice notice is **Official electronic
invoices are delivered through SDI.**

## Reports and conversation sharing

The activation consent offers **Share my conversations with Nixorium for 90 days
to improve Assistant** (off by default; Space toggles it). In the chat,
**/share** opens **Conversation sharing**; Enter changes the saved setting and
Esc cancels. Sharing uploads the complete redacted saved conversation after
final and error responses, replacing the previous version. Upload failures
never block a response or change its result.

After a final response, **/report** opens **Report this answer**. Choose
**Helpful** or **Wrong** with Up/Down and optionally type up to 500 characters.
The screen explains that sending includes the whole conversation: your
messages, configuration and history contents read, diagnostics and check
results. Known secret patterns are filtered, but personal or sensitive text
may remain. Enter sends; Esc cancels. A report is retained for 90 days even when
ongoing sharing is off. Successful submission shows **Report sent. Thank you.**
The laboratory token authenticates requests and is kept out of their bodies.

## Tools and limits

The model has fixed tools, with no shell or system-operation tool:

| Tool | Scope and limit |
|---|---|
| `list_files` | Visible workspace paths; at most 2,000 entries |
| `read_file` | UTF-8 text; at most 64 KiB per call |
| `search` | RE2 regular expression; at most 200 matches, 300 characters per matching line |
| `write_file` | Candidate text only; 512 KiB per file, 4 MiB and 200 changed paths per conversation |
| `delete_file` | Existing, unprotected candidate files only |
| `search_packages` | Deployment's pinned package search; at most 50 results, 60-second timeout |
| `evaluate` | Isolated temporary Git clone; five minutes, bounded output, no import-from-derivation |
| `build` | New non-cached candidate derivations after evaluation; bounded build logs, no activation |
| `lab_status` | Read-only controller observations of the lab computers |
| `prepare_npm` | Capable controllers only: prepare a public npm CLI package/tag and its tested offline dependency artifact |
| `host_diagnostics` | One exact declared computer and a fixed area; read-only state and at most 50 current-boot journal entries, 32 KiB result, 60-second total timeout |
| `git_history` | Up to 20 ancestors of this conversation's base; exact visible path filter and bounded sanitized comparison to a selected full revision; no remote access |

The wire-level step context and service messages are specified in the
[assistant protocol v2.5](assistant-protocol-v25.md); the tool schemas are in
[assistant-tools-v1.json](assistant-tools-v1.json).

Paths must be relative, with no traversal, backslashes or control characters;
the limit is 12 components and 512 bytes. A file cannot replace a directory
that still contains visible files. Tools cannot activate a system,
manage services, deploy computers, or save a proposal into the deployment.
Evaluation errors are redacted and limited to 8 KiB. Passing evaluation proves
that Nix can evaluate the candidate; it does not prove a successful build,
activation, boot or safe behavior of arbitrary configuration code.

### Computer diagnostics and saved history

New controllers advertise `host_diagnostics_v1` and `git_history_v1` through the
optional capabilities header; older services continue ordinary chats. Before
these observations can leave the controller, existing users must accept the
updated disclosure. Known secret patterns and common private identifiers are
filtered, but filtering cannot guarantee removal of every personal or sensitive
detail in logs or historical configuration. Student home files are not read.

`host_diagnostics` takes an exact name from fresh evaluated inventory and one of
`system`, `home_reset`, `updates`, `network`, `cache`, or `pxe`. Cache and PXE are
controller-only. The native helper accepts only that area, using immutable
systemd executable paths and fixed core unit lists. It reports service loading,
state, result, exit status and timestamps; boot ID and deployment revision;
numeric free space for fixed mounts; and available reset/update markers. It
does not read reset snapshots or student home content. Recent journal excerpts
are limited to 240 characters each and the observed boot. Credential,
environment and command-line messages are omitted; common paths, URLs, email
addresses and quoted identifiers are scrubbed. Remote reads use existing
strict SSH trust without enrolling or updating keys. Unknown keys, unsupported
roles, missing helpers, insufficient journal permissions and incomplete evidence
remain explicit; an empty journal is not proof of service health. Apply an
updated configuration through ordinary reviews to install the helper on older
computers. This tool never restarts a service, changes a system or runs a queue.

`git_history` reads only committed ancestors of the conversation's original
base, with a default of ten and a maximum of twenty. An optional `path` is an
exact visible relative file path. A selected revision must be a complete
40- or 64-character lowercase ancestor object ID. The comparison is from that
revision to the conversation base, with modes and sanitized unified diffs for
at most twenty paths, bounded per-file inputs and an explicit truncation flag.
Known private paths and unsafe historical content are refused or omitted before
capture. Hidden, untracked and student home files stay outside this tool; it
never changes the index, worktree, refs or a remote.

### Native initial home files

On supported pinned upstreams, Assistant can author ordinary UTF-8 files and
empty directories in `home-files.json` using its existing candidate file tools.
The `home_files_v1` advisory capability uses the optional HTTP header; discovery
reads only the input's pure `lib.homeFilesVersion` marker, with offline,
no-lock-update and no-IFD evaluation, once per immutable conversation baseline.
It never runs during TUI startup. Absent or unavailable metadata omits the
capability, and the service gives clear Update Nixorium guidance rather than
proposing declarations or ordinary-file copy services on older controllers.
The existing evaluate, build and reviewed SAVE boundaries still apply.

Local imports are opaque to model-facing file tools: `home-files/imports/` and
identical blob aliases are withheld from read, list, search and Git history.
The controller identifies import blobs throughout the baseline's ancestry, so
renamed/copied payloads stay withheld even when neither compared endpoint
contains the original import path. A bounded, unavailable or excessive history
check fails closed instead of silently exposing bytes. Evaluation keeps the
complete tracked source, including imports; inline text authored in the visible
JSON remains readable. Import selection and byte inspection are local workflows.

This is a file-tool privacy boundary, not a secret storage facility or an
absolute sandbox against arbitrary Nix expressions. Imports are non-secret
teaching materials committed to Git, copied into the Nix store and distributed
to the applicable computers. The administrator reviews source metadata,
destinations and executable choices locally; no original permissions, ACLs or
extended attributes are restored. The native seed supplies student-owned files
at 0600, directories at 0700 and explicitly executable files at 0700, at the
ordinary boot reset. It never writes into a live student session or adds an
autostart entry.

### Prepared npm baselines

`prepare_npm` accepts `package`, `id`, and an optional exact `version` or tag
(default `latest`). It resolves the tag once using the deployment's pinned
Node/npm, and writes five bounded text files under `npm/<id>/`: package and
lock declarations, a manifest, a Nix package, and a deployment-owned module.
Import the module explicitly for the requested hosts before ordinary evaluation,
build and SAVE review. Preparing alone changes no system or home. To update a
baseline, prepare a new `id`, replace the scoped import, and remove the old
generated files in the reviewed proposal.
Controllers advertise support through the optional `X-Nixorium-Capabilities`
HTTP header. Request/share JSON keeps its original schema: older services ignore
the header and continue ordinary chats; newer services offer this tool only to
capable controllers.

The resolver has an empty home and no external network: a fixed Unix broker
supplies sanitized metadata from `registry.npmjs.org`. Git, file, alias and
foreign-URL dependencies, redirects, bundled dependencies and native addons
are refused. Every required tarball has SHA-512 integrity and confined archive
validation. Install hooks and CLI version/help tests run in separate Bubblewrap
namespaces with no controller home, credentials or external network. Hooks
requiring downloads or unavailable build tools fail. Optional dependencies are
omitted and listed, which can remove features such as Pi's native clipboard
support. CLI smoke tests do not certify every feature or package safety; additional
runtime programs may be needed. Pi's normal file tools need pinned `fd` and
`ripgrep` in the same host scope, so they do not try to download binaries.

Preparation allows 512 lock entries, 512 KiB of lock text, 8 MiB of metadata per
package/64 MiB total, 128 MiB per tarball/512 MiB total, and 50,000 archive
entries/1 GiB expanded data. The operation takes at most ten minutes;
installation and smoke tests have wall, CPU, process, file and per-process
virtual/data limits. There is no aggregate resident-memory guarantee. Missing
namespace support fails closed. Changing the recorded Node runtime requires
preparing again.

The artifact retains verified tarballs and Node in its Nix closure, distributed
to offline clients by the normal controller cache. Up to 32 artifacts and 4 GiB
of conservatively counted closures are rooted in private Assistant state.
Cleanup retains artifacts referenced by the current deployment commit or a
retained chat, including after SAVE. It removes unused roots and never
garbage-collects the shared Nix store. Committed locks/hashes can refetch identical
sources on the controller after old artifacts are removed.

One baseline module may apply to a host. Its student service runs after the
normal boot-time home reset and before login. It copies only its package/bin
entries into writable `~/.local/npm`, refuses collisions, and performs no
network or installation at boot. A switch does not reset or seed a live home.
Student updates disappear at the next reset; npm and credential snapshot
exclusions remain intact.

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
start a new chat. Once files change, **Ctrl+R** opens **Review the proposal**.
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
hold metadata with the base commit, creation and last activity dates, title,
save date and last final answer (up to 2,000 characters), a private transcript,
`overlay/`, `deleted.json`, `modes.json`, `base-classes.json` and `base-drv-v25.json`.
Temporary evaluation clones live inside the conversation and are removed when
evaluation finishes. Local transcripts and candidates may contain private
configuration; they are separate from the deployment and its Git history.

Opening Assistant or `/resume` removes conversations inactive for more than
90 days and keeps at most 30 conversations, ordered by last activity. The current
chat is protected. Older metadata uses the creation date for last activity. This
cleanup does not revoke the subscription token, remove consent, or delete Git
backup refs. Starting a new chat also leaves the previous local conversation
until that cleanup. Disabling the controller option does not erase this data.


## Resume a conversation

`/resume` opens a list of previous conversations, most recently active first.
The current chat is excluded. Use the cursor and **Enter** to open a chat, or
**Esc** to return. Opening a conversation does not use a request. The list shows
the first line of the first user message as its title, the activity date, and
`saved`, `proposal not saved`, or `outdated` where applicable.

A conversation based on the current configuration continues normally; use
**Ctrl+R** to review its unsaved proposal. Saved conversations and those based
on an older configuration open as read-only history. Choose **Continue in a
new chat** to prepare a new chat on the current configuration, or **Back** to
return to the list. Continuing fills the input with the previous title and up
to 600 characters of the last final answer, leaving a blank line for your
request. It does not send the text. Older conversations use the last nonempty
assistant message without tool calls; when absent, only the title is filled.
