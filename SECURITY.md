# Security policy

Nixorium manages privileged installation and maintenance operations for NixOS
workstations. Reports involving privilege boundaries, SSH, PXE, cache signing,
credential handling, destructive confirmation, or generated deployment
configuration are especially important.

## Supported versions

Security fixes target the current `master` branch and the most recent release.
Older releases are not maintained. Include the affected release or commit in a
report so the maintainer can confirm whether the current code is still exposed.

## Report a vulnerability

Do not open a public issue for a suspected vulnerability. Use GitHub's
[private vulnerability report](https://github.com/giovantenne/nixorium/security/advisories/new)
so the report, discussion, and any draft advisory remain private. If that form
is unavailable, email [hello@nixorium.org](mailto:hello@nixorium.org) with the
subject `[Nixorium security]`.

Include:

- the affected component, release, or commit;
- the prerequisites and expected impact;
- a minimal reproduction or proof of concept, if it is safe to share;
- any known mitigations or evidence that the issue is already being exploited;
- your preferred attribution and disclosure constraints.

Do not include live private keys, password hashes, student data, or other
secrets. If a credential has been exposed, revoke or rotate it first and report
only the information needed to identify the affected path.

The maintainer aims to acknowledge reports within seven days and provide an
initial assessment within fourteen days. These are best-effort targets, not a
guaranteed service level. The reporter and maintainer should coordinate public
disclosure after a fix or mitigation is available. There is currently no paid
bug bounty program.

## Safe research

Use systems and credentials you own or are authorized to test. Do not access a
school network, interrupt a class, install to a workstation, erase a disk, or
retrieve another person's data without explicit permission. Stop if testing
could cross one of those boundaries.

Good-faith research that follows this policy is welcome. Automated scanner
output is useful when it identifies an affected dependency or reachable code
path; a tool name or unverified alert alone may not be enough to assess impact.

## Response process

The maintainer will reproduce and scope the report, identify supported
mitigations, prepare a fix and regression coverage, and coordinate release and
disclosure. A public advisory may be published after affected users have a
reasonable opportunity to update.

This process complements automated checks. Passing CodeQL, `govulncheck`, or
`staticcheck` does not establish that privileged operations or a deployed lab
are secure.
