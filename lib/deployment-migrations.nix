# Changes a release makes to a deployment's own files (ADR 0034). Update
# Nixorium reads this list from the release it updates to, applies each
# migration to the reviewed candidate, shows it in the update review and
# saves it with flake.nix and flake.lock. A migration replaces one exact,
# known text: when the file was changed and the text is not there exactly
# once, nothing is touched and the review names the manual step. Entries are
# never removed, so a deployment that skips releases still receives them.
[
  {
    id = "2026-10-workstation-is-controller";
    file = "modules/workstation.nix";
    description = "The workstation profile finds the controller with isController: computers that share a configuration have no single name (ADR 0033).";
    from = "  isController = args ? hostName && args.hostName == (labSettings.masterHostName or null);\n";
    to = "  isController = args.isController or false;\n";
  }
]
