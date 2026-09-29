# Local support report format

The sharing format is separate from detailed local diagnostic reports. It uses
an explicit allowlist, not regular-expression scrubbing of arbitrary messages.
Its schema version is independent of the ordinary management report schema.

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
