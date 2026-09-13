# ADR-0007: Local guided client enrollment

Status: accepted

## Context

A PXE-booted client currently receives a public offline installer bundle and
is identified only when an operator passes a numeric argument to `setup.sh`.
The bundle has no per-client secret, and the installation network cannot prove
that an unauthenticated request came from a particular physical computer. A
controller reservation API built on those inputs could be spoofed and could
let any machine on the interface reserve identities indefinitely.

The installer nevertheless needs to reduce operator mistakes, make disk
destruction unmistakable, preserve offline installation, and report its
result clearly.

## Decision

Implement enrollment locally in the netboot environment. Include the
versioned, non-secret `labMeta` document in the immutable installer bundle and
allow selection only from its configured client inventory. Show firmware,
system, CPU, memory, interfaces, and all candidate disks before selection.

Before wiping anything, probe the selected host address when a usable route is
available. Refuse an identity that responds, but describe a silent address as
unverified rather than reserved. Require a final typed phrase containing both
the exact hostname and canonical disk path. Revalidate the disk immediately
before Disko, show explicit partition/install/verification progress, report a
clear terminal result, and make reboot a separate confirmed action.

Do not add a controller enrollment protocol in this milestone. Do not support
unattended installation until a private deployment explicitly enables it and
provides an invocation token with a documented lifetime and trust model.

## Consequences

The common interactive path becomes much harder to invoke with the wrong host
or disk and remains usable without client internet. The immutable inventory
cannot coordinate simultaneous installers and reachability cannot prove that a
silent identity is free, so the UI must never claim a reservation. Operators
must still coordinate concurrent installations. Reliable automatic assignment
remains a possible later feature only if authenticated controller and client
identities can be established without embedding reusable secrets in the public
netboot closure.
