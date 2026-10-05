# Optional adoption statistics

Nixorium works without telemetry or a cloud account. Adoption statistics are
**off by default**, including after upgrades. Only the installed controller's
administrator can enable them. Student PCs and the classroom dashboard do not
send these reports.

In **Maintenance → Adoption statistics**, inspect the payload and explicitly
choose whether to share it. The first ordinary administrator startup offers
this choice until a decision is saved, with a benefit-first summary of the report.
Press `p` for the exact local JSON and `i` for Privacy & retention. On the overview,
`e` or Enter explicitly accepts sharing; `d` declines. Esc returns from details
but cannot skip the first choice. Exiting the application without deciding keeps
sharing off and offers the choice next time. A saved refusal is not reoffered.
The first invitation returns to the overview after saving either choice.
Adoption statistics is the last entry in Maintenance.
You can return at any time. The command-line equivalents are:

```sh
nixorium telemetry preview --json
nixorium telemetry status --json
nixorium telemetry enable
nixorium telemetry disable
```

`enable` explicitly consents to daily reports. Preview and status are local:
they do not upload data or probe clients. Before consent, the preview contains
an explicitly marked placeholder for the identity. The daily system service
uses the same payload builder as the preview. Opening the TUI is not required
for subsequent sends.

## Installer request counts

The public `https://nixorium.org/install.sh` download endpoint separately counts
GET requests as daily UTC totals, including curl, browsers, bots and retries.
It cannot establish unique visitors, script execution or successful installations.
HEAD requests and direct GitHub downloads are not counted. The counter stores
only the day and total, with no cookies, IP addresses, user agents or visitor
identifiers; Cloudflare still receives the connection IP. Daily totals are kept
for 24 calendar months and are not linked to controller reports. Counting is
best effort, so service outages or quotas can cause undercounts.

Downloading the installer never enables daily controller telemetry. The
controller administrator makes that separate choice in the TUI after installation.

## Data and purpose

Reports go over HTTPS to `https://telemetry.nixorium.org/v1/heartbeat` to measure
participating controllers, release adoption, approximate lab size, and whether
at least one installed client was previously verified. The allowlisted fields
are:

| Field | Meaning |
| --- | --- |
| `schemaVersion` | Version 1 of the reporting contract |
| `month` | UTC calendar month |
| `monthlyId` | Random-secret HMAC pseudonym that changes every UTC month |
| `version` | Installed controller generation's Nixorium release, or `unknown` |
| `deploymentMode` | Installed controller generation's mode |
| `configuredClients` | Installed generation's client count band: 0, 1–5, 6–15, 16–30, 31–60 or 61+ |
| `clientBootVerified` | `true` only after an existing authenticated installed-client observation; otherwise `null` |

Pending configuration changes are not reported as installed settings. A verified
boot is historical evidence, not current fleet health or evidence that students
are using the lab. An unknown boot does not mean a failed installation. The
collector performs no new SSH probes or Nix evaluations. Ordinary authenticated
computer checks and successful USB post-boot verification may record this one
historical boolean after consent. Saving this evidence cannot fail the operation.

The payload excludes hostnames, school names, IP addresses, usernames, machine
IDs, hardware identifiers, packages, configuration files, repository revisions, paths,
logs, credentials, files and student screens. Unsupported version strings become
`unknown`. The local support-report feature remains separate and never uploads.

## Privacy and retention

These are minimized **pseudonymous**, not guaranteed anonymous, statistics.
Cloudflare receives the source connection IP while serving the request; the
application does not store it, headers or request bodies in application logs.
Unusual combinations of version and lab size may still be recognizable.

The random secret stays in `/var/lib/nixorium/telemetry`, outside Git, the Nix
store and Nixorium controller backups. A local-only machine identity check
prevents ordinary restored state from enabling telemetry on another machine.
A full disk clone that also preserves machine-id needs `disable` followed by
an explicit `enable` on the independent controller. Do not enable telemetry on
cloned test systems unless you intend to include them in the adoption counts.

The collection is operated by Claudio Benvenuti, the Nixorium maintainer.
For privacy requests, contact [hello@nixorium.org](mailto:hello@nixorium.org).
Its purpose is to understand adoption and lab sizes and prioritize development.

Scheduled daily cleanup deletes individual observations aged 89 days or more,
normally within 90 days of server receipt, without a fixed row cap or dependence
on successful aggregation. Outages or exhausted Cloudflare quotas can delay
physical deletion; records aged 90 days are excluded from report calculations.
ID-free monthly aggregates and separate installer totals retain 24 calendar
months including the current month, with scheduled cleanup and report filtering.
If expiry preceded delayed aggregation, the monthly result stays provisional.

On the deployed Workers Free plan, Cloudflare D1 recovery history lasts
[7 days](https://developers.cloudflare.com/d1/reference/time-travel/), so deleted
records may remain recoverable for up to 7 further days. The service creates no
separate adoption-record exports or backups, and Worker request logging is disabled.
Cloudflare's infrastructure processing follows its
[privacy policy](https://www.cloudflare.com/privacypolicy/); disabling Worker
logs does not mean the provider processes no connection data. This is not a
guarantee of exclusively European processing.

`disable` stops new dispatches and removes the local secret and attempt history.
It waits for an already-running bounded request to finish; transmitted bytes
cannot be recalled. Previously received data follows server retention and is
not deleted by the local command. Re-enabling creates a new identity. Refusal
remains remembered. Missing or corrupt consent prevents sending; corrupt state
can be reset with `nixorium telemetry disable`.

## Offline operation and troubleshooting

The controller tries once per UTC day, with a randomized timer and a five-second
HTTPS deadline. It sends the current state without a backlog. Failed requests,
an unavailable endpoint or exhausted Cloudflare Free quotas do not block lab
operations. A month mismatch from an incorrect clock is rejected by the server;
check normal system time synchronization rather than changing telemetry state.

Use `nixorium telemetry status` to see the last attempt, last success and a fixed
result category. To stop participation, use the disable command. If the feature
is unavailable, confirm you are using the installed controller's administrator
account and apply the controller configuration through the normal reviewed flow.
No student-side network access is needed. The administrator's telemetry screen
requires no root privileges; the system timer runs as `admin` with restricted
filesystem access.
