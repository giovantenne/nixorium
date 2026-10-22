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
When enabled, **Assistant** is the fifth administrator menu entry (`a`);
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
school name in the header and on invoices and emails, so several
laboratories of one school can be told apart; an empty name removes it.

The service sets the size and length of the free trial and shows them when you
activate: on the trial choice in Assistant, on the `/trial` page and in the
email with your code (currently ten requests for thirty days). There is one
trial per laboratory: the same email address can register several laboratories,
each under its own name, and a trial keeps the requests and end date it started
with. Register with your email, school name and privacy consent on the
service's `/trial` page, then enter the code delivered by email. Before using
the code, you can submit the form again to replace it if delivery failed.
After activation, the screen shows **Free trial · N of M left**, where M is the
trial allowance the service reports for your account. An ended trial
shows **Free trial ended**; exhausted annual quota shows **No requests left**.
The controller checks account metadata without sending your draft or files,
and refreshes that count after a request. A request that the service interrupts
before an answer (a service error or its safety limit) is not counted; a request
you stop yourself is.

When the trial expires or its requests are used, Assistant shows a signed
purchase link from the service. The link lasts seven days; reopening Assistant
fetches a fresh link. Completing the subscription converts the same account
and keeps the controller token active, so no new activation code is needed.
An annual account has no trial purchase link: after using its included requests,
wait for renewal or contact `hello@nixorium.org` for a manual agreement.
Each account enables one laboratory; a replacement code for a reinstalled
controller disables the previous token while preserving quota and period.

Every Assistant request carries the Nixorium version in the
`X-Nixorium-Client-Version` header, so the service can announce trial terms
that older controllers would not accept. Each admitted turn first opens through
a request containing client version and tool version only. The subsequent steps send the conversation, visible file
names, requested file contents and tool results to the Assistant service and
its AI provider. The service does not keep your configuration files. It keeps the text of your
messages and Assistant's answers for 30 days to detect abuse, then deletes it.
Conversations (your questions, the answers and the tool results Assistant
used, after the controller's best-effort redaction) are saved on the service
for 90 days to find and fix Assistant's mistakes. Saving is on by default and the
administrator can switch it off. Do not enter passwords or student data in a request.

The model sees tracked files from the chat's base commit plus its own candidate
files. Untracked and ignored files, private-key paths, `keys/`, `.git`, `.ssh`,
`.env*`, credentials and agent-instruction directories are hidden. Input
locks, instruction files and Git control files are read-only, and so are the
settings except the few listed under [Laboratory settings](#laboratory-settings).
Sensitive settings fields are withheld; known token formats, password hashes and
private key material are redacted from model-facing output. The laboratory token is
kept separate from the transcript. Only bounded journal excerpts from fixed
core units are diagnostic inputs; arbitrary logs, student home files and other
files outside the workspace are not tool inputs.
Assistant sees only the lab configuration folder on the controller (normally
`/home/admin/nixorium-deployment`; the exact path is recorded in
`/etc/nixorium/deployment-path`), never your home folder or `Downloads`. For a
program available from an official download, such as an AppImage, it proposes
a package that the controller downloads with a pinned hash when it builds the
update; a private file must first be copied into that folder and committed to
its Git before a new chat can use it.

## Subscription and payments

`/billing` shows a clickable temporary Stripe Customer Portal link in the chat.
It does not use an Assistant request or send conversation contents. Open the link
within a few minutes to manage your subscription and see payments. Trial accounts
without a subscription are told that `/billing` becomes available after
subscribing. The link stays out
of the transcript and model messages. The portal return page lets you close the
tab and return to Nixorium. The displayed invoice notice is **Official electronic
invoices are delivered through SDI.**

## Reports and saved conversations

The activation consent shows **Save conversations on the service: On**,
pre-selected (Space switches it off). The consent text says that conversations
are saved on the Nixorium Assistant service for 90 days to find and fix
Assistant's mistakes. In the chat, **/share** opens **Assistant settings** with the same
switch; Enter changes the saved setting and Esc cancels. Saving uploads the
complete redacted saved conversation after final and error responses, replacing
the previous version; nothing is uploaded when it is Off. Upload failures never
block a response or change its result. A controller that accepted an earlier
consent text sees the updated consent once, with saving pre-selected On, and
can refuse it.

After a final response, **/report** opens **Report this answer**. Choose
**Helpful** or **Wrong** with Up/Down and optionally type up to 500 characters.
The screen explains that sending includes the whole conversation: your
messages, configuration and history contents read, diagnostics and check
results. Known secret patterns are filtered, but personal or sensitive text
may remain. Enter sends; Esc cancels. A report is retained for 90 days even when
saving conversations is off. Successful submission shows **Report sent. Thank you.**
The laboratory token authenticates requests and is kept out of their bodies.

## Tools and limits

The model has fixed tools, with no shell or system-operation tool:

| Tool | Scope and limit |
|---|---|
| `list_files` | Visible workspace paths; at most 2,000 entries |
| `read_file` | UTF-8 text; at most 64 KiB per call |
| `search` | RE2 regular expression; at most 200 matches, 300 characters per matching line |
| `write_file` | Candidate text only; 512 KiB per file, 4 MiB and 200 changed paths per conversation; `lab-settings.json` only for the settings under [Laboratory settings](#laboratory-settings) |
| `delete_file` | Existing, unprotected candidate files only |
| `search_packages` | Deployment's pinned package search; at most 50 results, 60-second timeout |
| `evaluate` | Isolated temporary Git clone; five minutes, bounded output, no import-from-derivation |
| `build` | New non-cached candidate derivations after evaluation; bounded build logs, no activation |
| `lab_status` | Read-only controller observations of the lab computers |
| `prepare_npm` | Capable controllers only: prepare a public npm CLI package/tag and its tested offline dependency artifact |
| `host_diagnostics` | One exact declared computer and a fixed area; read-only state and at most 50 current-boot journal entries, 32 KiB result, 60-second total timeout |
| `git_history` | Up to 20 ancestors of this conversation's base; exact visible path filter and bounded sanitized comparison to a selected full revision; no remote access |
| `read_system_file` | Capable controllers only, after the updated consent: one allowlisted system file or directory of one declared computer (see [System files](#system-files)); 256 KiB of text, 500 listing entries, 60-second timeout |
| `prefetch` | Capable controllers only, after the updated consent: one public `https` file of at most 1 GiB downloaded once into the controller's Nix store; returns its pinned hash, size and store path; 15-minute timeout |
| `vscode_marketplace` | Capable controllers only, after the updated consent: one VS Code extension (`publisher.name`) resolved in the public Marketplace for the deployment's pinned editor and downloaded into the controller's Nix store; returns the pin for `vscode.marketplace` in `workspace-profile.json`, its dependencies and whether it ships native programs |

Assistant never changes the laboratory network; for a new network address or
prefix it refers the administrator to the guided change (Maintenance → Change
network addressing, `nixorium network migrate`).

A program downloaded by hand runs on these computers thanks to nix-ld, but
that is a convenience for one user; the `prefetch` route remains the one that
puts a program on every computer.

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

### System files

Controllers that advertise `system_files_v1` offer `read_system_file`: one
path of one computer from fresh declared inventory, after the version-3
disclosure. Only `/etc`, `/boot/grub`, `/boot/loader`, `/proc/cmdline`,
`/proc/cpuinfo`, `/proc/meminfo`, `/proc/mounts`, `/proc/partitions`,
`/proc/swaps`, `/proc/version`, `/sys/class/net`, `/sys/class/dmi/id`,
`/var/lib/nixorium` and `/run/nixorium` are readable. Home folders, `/root`,
shadow files, private SSH keys, Wi-Fi connections, Nixorium keys, reset
snapshots, installation state and device serial numbers are refused, by path
and by name (`secret`, `private`, `token`, `password`, key files), and so is
any link leading there; links are followed into the Nix store and, for the
paths that need them, into `/sys/devices`, `/proc/PID/mounts`, `/var/lib/cups`
and `/run/systemd/resolve`. `/etc/machine-id` and `/etc/passwd` are refused on
purpose. A directory is listed with refused entries counted, not named. Text is
returned up to 256 KiB, binary files only as such; lines that set passwords,
passphrases, pre-shared keys, tokens or keys keep only their first word, and
credentials in URIs are withheld. The fixed helper
`nixorium-host-diagnostics file PATH` reads clients as root through the
existing strict SSH trust; on the controller it runs as the administrator, so
root-only files there (such as the boot menu on `/boot`) are reported as not
readable. A computer whose helper is older answers `computer_needs_update`.
The controller checks every answer again before it reaches the model. See
ADR 0032.

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

### VS Code extensions for students

Students' VS Code extensions are listed in `vscode.extensions` of
`workspace-profile.json`; the TUI no longer edits them. Assistant adds or
removes them on request. Extensions packaged in the deployment's package
collection are found with `search_packages` (`vscode-extensions.<text>`). For
one that exists only in the public Marketplace, controllers advertise
`vscode_marketplace_v1` and the `vscode_marketplace` tool resolves the newest
stable Linux version that the pinned VS Code accepts, downloads it into the
controller's Nix store and returns its pin (publisher, name, version, hash and
platform when needed), its dependencies and extension pack members, and whether
it ships native programs. Assistant writes the pin in `vscode.marketplace` and
the lowercase identifier in `vscode.extensions` of its candidate, then
evaluates. Pins do not follow package-base updates: after an editor update,
ask Assistant to check them again. The tool writes no deployment file, uses the
controller's Internet access only when called, and requires the updated
consent; failures are reported as `not_resolved` with a one-line reason,
`consent_required` or `marketplace_unavailable`.

### Prefetched downloads

Controllers advertise `prefetch_v1` through the same optional header. For a
program its publisher distributes as an AppImage, `.deb` package or tarball at
a fixed-version address, `prefetch` downloads that one file on the controller
and returns its `sha256-…` hash, its size in bytes and its Nix store path. The
Assistant puts the hash in the module it writes, so the administrator never
computes it. The tool does not read the file or send it to the service, and it
installs or activates nothing. Like computer diagnostics, it requires the
updated consent.

The address must be an absolute `https` URL of at most 2,048 characters, with
no user name, password, fragment, spaces or control characters. Hosts written
as IP addresses, `localhost` and names ending in `.localhost`, `.local`,
`.internal` or `.lan` are refused, and so is a host with any loopback, private
(10/8, 172.16/12, 192.168/16, IPv6 unique-local), link-local, carrier-grade NAT
(100.64/10) or unspecified address. A first HEAD request (10 seconds, at most
five redirects, each redirect checked by the same rules) refuses a file whose
reported size exceeds 1 GiB; a missing size is allowed, and a download that
turns out larger is refused afterwards. These checks cover the address and the
redirects seen by the HEAD request; Nix then performs the download itself with
`nix store prefetch-file`, one at a time on the controller, within 15 minutes.
Refusals and failures are reported as `invalid_url`, `host_refused`,
`too_large`, `busy`, `consent_required`, `download_failed` (with a one-line
reason) or `timeout`.

The file stays in the controller's Nix store without a garbage-collection
root. Building the candidate finds it there through `fetchurl` with the same
hash and file name (by default the last component of the address), so it is
not downloaded twice. The computers never download it: at update time they
receive the built program from the controller cache. If the store copy was
collected in the meantime, the controller downloads it again during the build
and Nix checks the same hash.

### Laboratory settings

Controllers advertise `settings_v1` through the same optional header. The
Assistant can then propose a change to these settings in `lab-settings.json`,
reviewed and saved like any other proposal:

| Setting | Field |
|---|---|
| Browser homepage | `lab.homepageUrl` |
| Time zone | `lab.timeZone` |
| Default and additional language | `lab.defaultLocale`, `lab.extraLocale` |
| Keyboard layout | `lab.keyboardLayout` |
| Student Git name and email | `lab.studentGitName`, `lab.studentGitEmail` |
| Administrator Git name and email | `lab.adminGitName`, `lab.adminGitEmail` |

Everything else stays in the menu (Maintenance → Change settings): the
network and its addresses, the number of computers, the controller number,
network devices, the DHCP address hint, account names and passwords, the
credentials version, the classroom view, the `assistant` switch and the schema
version. Assistant cannot change them, and it cannot delete the file.

The model reads a copy of the file with sensitive fields withheld and writes it
back whole. Nixorium never takes that copy as it is: it keeps the saved file,
takes only the writable values the model changed, checks the result with the
decoder and validation of the settings menu, and stores it in the form
Nixorium itself writes. A value the read tool masked is never written back, and
the file keeps the mode it has in the base commit. A refused write is a tool
error and leaves the candidate as it was:

| Error | Meaning |
|---|---|
| `settings_field_read_only: lab.pcCount` | The copy differs from what the model read in a setting it may not change, or adds or removes one. The message names every such field, even when a writable setting changed too. |
| `settings_invalid: …` | The content is not a settings file or has an unknown field, a value is not text, a value fails the settings validation (for example `lab.timeZone: must not be empty` or `lab.homepageUrl: must be an absolute http:// or https:// URL`), a value is longer than 128 bytes (256 for a Git name or email, 2,048 for the homepage), has spaces at either end, spans several lines or carries a `[REDACTED]` or `[WITHHELD]` stand-in, or the saved settings no longer validate and must be repaired in the menu first. |

Deleting `lab-settings.json` and writing a file of that name in a subfolder
remain `protected_path`.

When a proposal changes `lab-settings.json`, the review applies the rules of a
native settings save to the saved and the proposed file again, whatever wrote
the candidate. Changing the network or the controller number while computers
are configured, a file that does not validate or is not in the form Nixorium
writes, or a change outside the writable settings blocks the save with a
message. Otherwise **Checked by Nixorium** adds the same lines a native
settings change shows: apply the settings to the controller, update the
computers (offline computers keep the old values until updated), and prepare
network installation again before installing more computers. A change of the
administrator Git identity reaches the controller only. The save, the backup
and the follow-up reviews are those of any proposal; the candidate evaluation
runs the settings through the same Nix validation as the menu, and nothing is
applied by saving.

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
representative cannot establish their individual behavior. The same classes
share one system (ADR 0033), where `hostName` and `hostIp` stop the evaluation
with an error naming the hosts that need their own configuration. Class/base results
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
When nothing changed since the last successful check in this chat, usually
the assistant's own `evaluate` (same proposal files, base commit, `flake.lock`
and Nixorium version), the review reuses that result instead of evaluating the
laboratory again and says how long ago it was verified; any edit evaluates
afresh.
Read the full comparison with arrows, PgUp/PgDn and Home/End.

Choose **Save and update the controller** or **Save only** and press **Enter** to record the reviewed proposal (typing `save` in the chat only opens this review). The complete
diff must fit within 2 MiB and the evaluation must pass. A review expires after
ten minutes; changes to the candidate or deployment require another review.
The save repeats evaluation, under the same reuse rule, and checks the
proposal before taking the managed operation gate and replacing files. An
active PXE operation blocks checkout.

The new commit is `Save reviewed Assistant proposal`. Its parent and a durable
`refs/nixorium/assistant-backups/…` ref preserve the previous committed tree.
The result displays the backup. Nothing is built or activated by saving.

The next action opens the ordinary **Apply to this controller** review. After
verified application at the saved revision, **Update computers** opens client
selection with fresh inventory and another review. Neither action runs
automatically. Use a new chat for further proposals after saving, since the
previous conversation remains tied to its original base commit.

### Core override mode

By default the Assistant only extends Nixorium's supported mechanisms. In
extreme cases it can propose overriding a NixOS option that the Nixorium core
sets, from a private module in your deployment (`lib.mkForce` or
`lib.mkOverride` in `sharedModules`, `clientModules`, `controllerModules`,
`groupModules` or `hostModules`). This mode is off by default and applies to one
conversation only. You enable it by writing "I accept core overrides" (Italian:
"Accetto override del core"); it ends with "I no longer accept core overrides"
or a new chat.

Each such proposal includes a **Core override** section listing the forced
options and the affected computers, why the change is unsupported (it is outside
Nixorium's tests, and a Nixorium update can change the option and silently void
the override) and the rollback path. The review also shows a warning, with the
files that mention `mkForce` or `mkOverride`, above the save choices. The check
is a plain text match, so a mention in a comment or a string counts too; the
warning is informational and the save flow is unchanged.

The Assistant never edits or patches Nixorium itself, and never overrides the
controller's management channel (SSH access, keys, firewall protection, cache
signing), credentials, or the student home reset in a way that could block
login.

Evaluation and build prove that the configuration builds, not that it boots or
works: update one computer first. If a PC does not boot or loses the network
after applying, restart it and choose the previous system generation in the GRUB
boot menu, PC by PC, the controller included, then fix the configuration before
applying again. A client that has lost its network cannot be repaired remotely
from the controller.

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
the token (0600), `consent.json`, `conversations/<uuid>/`, and an empty
`prefetch/` directory that serves only as the one-download-at-a-time lock. Conversations
hold metadata with the base commit, creation and last activity dates, title,
save date and last final answer (up to 2,000 characters), a private transcript,
`overlay/`, `deleted.json`, `modes.json`, `base-classes.json`, `base-drv-v25.json`
and, after a successful evaluation, `evaluation-receipt-v25.json` (0600).
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
return to the list. The arrows, PgUp/PgDn, Home and End scroll the conversation,
and Tab chooses between the two. Continuing fills the input with the previous title and up
to 600 characters of the last final answer, leaving a blank line for your
request. It does not send the text. Older conversations use the last nonempty
assistant message without tool calls; when absent, only the title is filled.
