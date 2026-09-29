# Local support report format

The sharing format is separate from detailed local diagnostic reports. It uses
an explicit allowlist, not regular-expression scrubbing of arbitrary messages.
Its schema version is independent of the ordinary management report schema.

## Preview and save

From the administrator's deployment checkout:

```sh
nixorium support preview --json
nixorium support export
```

Preview prints the complete filtered JSON without creating a report file.
Export collects a new snapshot, displays its exact JSON, and asks for consent
in an interactive terminal. Both input and output must be terminals; there is
no `--yes`, `--full`, destination-path or upload option. Answering anything
other than `y` cancels. Export saves those exact bytes, not a refreshed report.

Files have random names under `$XDG_STATE_HOME/nixorium/support/`, defaulting
to `~/.local/state/nixorium/support/`. Product/export directories must be owned
by the current user with mode `0700`; files use `0600`. Symlink components,
unsafe writable ancestors and existing destination names are refused. An
unnamed file is published only after its complete content is synced; filesystems
without Linux unnamed-file support fail closed. An unconfirmed directory sync
is reported as partial, with the retained file path. Reports are never deleted
or rotated automatically. Keep this state directory on local storage, not in a
shared or automatically synchronized folder.

Collection reuses status, ordinary doctor, authenticated host observation and
the administrator's newest operation records. Operation counts cover that
user's local history, not exclusively this checkout. No raw log is read for
export. A 45-second context deadline bounds cooperative probes; unavailable
sources produce `null` sections. Existing Nix evaluation may fetch missing
pinned sources, and LAN/cache/SSH checks may contact configured computers.
Support SSH checks require an already trusted host key, disable user SSH hooks
and multiplexing, and never enroll new keys. Unknown/changed keys stay unknown;
collection does not repair trust or import credentials.
No support service, AI provider, upload or telemetry endpoint is contacted.
No controller build, service change, deploy, reset or remediation is invoked.
Observations are sequential, not an atomic fleet snapshot; a detected checkout
revision change removes the revision and host section. Counts do not certify
controller activation. In controller-only mode, lab-specific doctor findings
may be inapplicable: do not enable lab services merely to silence them.

The terminal displays the local filename and SHA-256 after saving. The filename
is not part of the shared JSON. A saved report remains an observation from the
displayed time, even if the lab changes before export. Sharing is always a
separate manual decision; ordinary diagnostic output is **not** share-safe.

## Schema 1 and data classification

| Field | Retained data and reason |
|---|---|
| `schemaVersion`, `operation` | Fixed format identity; no deployment input |
| `collectedAt` | UTC collection time, rounded to seconds; identifies an observation, not live state |
| `commandVersion`, `status.deploymentVersion` | Numeric release or alpha/beta/rc version; unsupported custom version strings become `unknown` |
| `deploymentRevision` | Full lowercase 40-character Git revision, when available; correlates the desired configuration, not proof of activation |
| `status` | Allowlisted deployment/PXE modes and readiness, Git and preparation booleans |
| `doctor` | Exact known finding IDs and severity only; no summary, evidence or remediation strings |
| `hosts` | Aggregate SSH/deployment counts; no host identity, individual revision, address or saved deployment history |
| `operations` | Counts by known operation and outcome for at most the newest 50 records; no subjects, IDs, timestamps, messages or raw logs |
| `excluded` | Fixed disclosure of intentionally excluded categories |

A `null` section means unavailable or unsupported, not healthy. Unknown finding
IDs, invalid severities, duplicate findings, unknown operation/outcome values
and entries beyond the collection limits are counted as omitted. Unknown host
states count as unknown. Host counts are derived from the observed entries,
not copied from a potentially inconsistent summary. A report is bounded to
32 KiB, 128 input findings, 1,024 input hosts and 50 input operation records.

All free text, repository/store/home paths, hostnames, IP addresses, interface
names, configuration, credentials, key material, student files, operation IDs
and raw logs are excluded. New fields on the source reports do not become
shareable automatically. Unsupported report schemas are not interpreted.

This is **data minimization, not anonymity**: the retained version, revision,
time and aggregate counts can still identify a deployment or incident. Review
the complete payload before sharing it. Detailed local doctor/status/log
output is not covered by this sharing policy and must not be attached blindly.

The snapshot is immutable after filtering. Preview and export must consume the
same bytes; an empty snapshot is not exportable. Collection and export must not
perform a controller build, upload, deploy, reset, or remediation.
