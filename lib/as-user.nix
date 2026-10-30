{ pkgs, lib }:

# Shell text that runs a command in a staff home as that home's owner.
# Activation runs as root, and a symbolic link the user left in their home
# would point a root write at a system file. Run as the user, the same link
# leads only where the user may already write. A failure warns and does not
# stop the activation, and a time limit kept by root ends a step the user
# stalls (a FIFO in place of a file, a stopped process): the user's own files
# cannot block an update or a start.
user: command: ''
  ${pkgs.coreutils}/bin/timeout -k 5 60 ${pkgs.util-linux}/bin/setpriv --reuid=${lib.escapeShellArg user} --regid=users --init-groups -- ${command} \
    || echo "Nixorium: could not update a file in the home of ${user}" >&2''
