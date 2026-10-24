{ gnome-console, runtimeShell, writeShellApplication }:

# GNOME Console in place of Ghostty where Ghostty cannot draw. Console keeps
# a window open, read-only, when the command it runs ends; Ghostty closes it.
# A command therefore runs in a Console of its own (its own application ID,
# so no other Console window shares the process), which is closed when the
# command ends. Without a command it is an ordinary Console.
writeShellApplication {
  name = "nixorium-console";
  runtimeInputs = [ gnome-console ];
  text = ''
    if [ "$#" -eq 0 ]; then
      exec kgx
    fi
    # shellcheck disable=SC2016 # The inner shell expands its own "$@" and $PPID.
    exec kgx --gapplication-app-id="org.nixorium.Console.p$$" -- ${runtimeShell} -c \
      '"$@"; status=$?; kill -TERM "$PPID" 2>/dev/null; exit "$status"' nixorium-console "$@"
  '';
}
