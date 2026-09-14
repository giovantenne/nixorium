# ADR-0008: Bounded private operation records

Status: accepted

## Context

Administrators need an understandable chronology of important Nixorium actions
and a route to detailed failure output. Privileged actions already retain their
technical output in journald, while foreground deployments stream to private
files. A database would duplicate those sources and add recovery, migration,
and secret-handling risk. Raw command messages are also unsuitable for a
long-lived summary because they may contain unbounded or sensitive data.

## Decision

Record supported final action reports as fixed typed fields: operation, state,
bounded subject, timestamp, and an application-generated summary. Never copy a
raw report message, command output, environment, credential, or key material.

Store the record under the administrator's XDG state in a locked, atomically
replaced, mode-0600 JSON file beneath mode-0700 directories. Validate schema,
IDs, timestamps, ownership, modes, types, bounds, and duplicate IDs before read
or replacement. Serialize concurrent writers and fail closed on corruption or
symlinks. Retain the newest 1000 summary records; this explicit compact-record
retention does not remove detailed deployment logs or journald entries.

Expose the newest 50 records beside recognized deployment logs through one
typed `logs` report. Detailed file reads accept only IDs returned by discovery,
use directory-relative no-follow opens, return at most a 64-KiB tail, and
neutralize terminal control and format characters before presentation.

## Consequences

Routine action history remains small, private, inspectable, and usable without
a deployment checkout or database. Recording failure is visible but does not
rewrite the observed success/failure of an action that already finished.
Journald remains authoritative for privileged technical output, deployment log
files remain the detailed foreground recovery surface, and authenticated live
host state remains authoritative for convergence.

Existing actions completed before this record was introduced are not
backfilled. The record is per administrator, so actions deliberately run under
another account appear in that account's state rather than being merged across
trust boundaries.
