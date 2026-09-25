# Operational disclaimer

Nixorium is provided under the [MIT License](LICENSE), including its warranty
and liability limitations. This document highlights operational risks; it does
not replace the license or constitute legal, security, compliance, or
professional systems-administration advice.

Nixorium coordinates privileged operations on physical computers. Depending
on the selected workflow, it can partition and erase disks, install operating
systems, replace student home directories, deploy configuration over SSH,
interrupt active sessions, and request workstation shutdown. A configuration,
automation result, or successful test in one environment does not guarantee the
same result on different hardware, firmware, networks, or institutional policy.

The operator is responsible for:

- verifying every target, scope, disk, network interface, and destructive
  confirmation before proceeding, including physically comparing a live USB
  client's address and SSH fingerprint and excluding its boot medium;
- maintaining tested backups for data that must survive reset, reinstall, disk
  failure, or operator error;
- testing changes on a representative non-critical system before wider
  deployment and verifying the result on each selected machine;
- obtaining authorization to administer the computers and network, and
  complying with applicable organizational policy and law;
- protecting private repositories, password hashes, SSH keys, cache-signing
  keys, Veyon credentials, and other deployment secrets, including prompt
  rotation after suspected exposure;
- evaluating hardware, firmware, network, software, privacy, accessibility,
  and security requirements for the actual laboratory; and
- monitoring supported releases and applying relevant fixes and mitigations.

Local snapshots are not backups. Student-home reset can remove files, and a
reinstallation or failed disk can remove both current data and snapshots.
Nixorium does not provide centralized identity management, durable student-file
storage, regulatory compliance, or unattended authorization for destructive
installation.

USB/SSH installation does not make an interrupted disk mutation reversible.
The operator must keep local console access, remove the installation medium at
the documented point, and treat an uncertain post-dispatch result as possible
partial disk erasure until the same operation is reconciled or the dedicated
target is deliberately reinitialized.

Documentation, demonstrations, sample configuration, and observations from the
original classroom describe specific tested or historical conditions. They are
not promises of performance, compatibility, security, or fitness for another
deployment.
