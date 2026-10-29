{ pkgs, lib }:

# Shell text that runs a command in a staff home as that home's owner.
# Activation runs as root, and a symbolic link the user left in their home
# would point a root write at a system file. Run as the user, the same link
# leads only where the user may already write. A failure warns and does not
# stop the activation: the user's own files cannot block an update.
user: command: ''
  ${pkgs.util-linux}/bin/setpriv --reuid=${lib.escapeShellArg user} --regid=users --init-groups -- ${command} \
    || echo "Nixorium: could not update a file in the home of ${user}" >&2''
