{ pkgs, lib, hostSoftwarePackages, ... }:
let
  ghostty = pkgs.ghostty;
  # Ghostty draws with OpenGL 4.3. On older graphics (Intel integrated
  # graphics of 2011-2012, a virtual machine with 3D acceleration) it either
  # closes at once ("OpenGL version is too old") or keeps an empty window
  # ("create a GL context"), with exit status 0 either way. This ghostty
  # watches its first seconds and opens GNOME Console with the same command
  # instead. The screensaver calls Ghostty by its own path and is left out.
  wrapper = pkgs.writeShellApplication {
    name = "ghostty";
    runtimeInputs = [ pkgs.coreutils pkgs.gnugrep pkgs.gnome-console ];
    text = ''
      real=${ghostty}/bin/ghostty
      case "''${1:-}" in
        +*|-h|--help|--version) exec "$real" "$@" ;;
      esac
      log=$(mktemp -t ghostty-start.XXXXXX)
      "$real" --gtk-single-instance=false "$@" 2> >(tee "$log" >&2) &
      pid=$!
      cannot_draw() {
        grep -qE 'OpenGL version is too old|OpenGLOutdated|gtk-opengl-context|create a GL context' "$log"
      }
      failed=false
      for _ in $(seq 50); do
        if cannot_draw; then
          failed=true
          break
        fi
        if ! kill -0 "$pid" 2>/dev/null; then
          # Ghostty has exited: give its last messages time to reach the log.
          sleep 0.3
          if cannot_draw; then failed=true; fi
          break
        fi
        sleep 0.1
      done
      rm -f "$log"
      if [ "$failed" = false ]; then
        status=0
        wait "$pid" || status=$?
        exit "$status"
      fi
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
      echo "Ghostty cannot draw on this computer (OpenGL 4.3 is missing); opening GNOME Console." >&2
      # Keep the command after -e; Ghostty's own options mean nothing to Console.
      command=()
      found=false
      for argument in "$@"; do
        if [ "$found" = true ]; then
          command+=("$argument")
        elif [ "$argument" = -e ]; then
          found=true
        fi
      done
      if [ "''${#command[@]}" -gt 0 ]; then
        exec kgx -- "''${command[@]}"
      fi
      exec kgx
    '';
  };
  # Ghostty's own launcher starts it through D-Bus by its store path, which
  # the wrapper cannot see. This one, under the same name (the dock's
  # favorite), starts the wrapper.
  desktopEntry = pkgs.runCommand "ghostty-desktop-entry" { } ''
    mkdir -p $out/share/applications
    sed -e 's|^TryExec=.*|TryExec=${wrapper}/bin/ghostty|' \
        -e 's|^Exec=${ghostty}/bin/ghostty --gtk-single-instance=true|Exec=${wrapper}/bin/ghostty|' \
        -e '/^DBusActivatable=/d' \
      ${ghostty}/share/applications/com.mitchellh.ghostty.desktop \
      > $out/share/applications/com.mitchellh.ghostty.desktop
    if grep -q '${ghostty}/bin/ghostty' $out/share/applications/com.mitchellh.ghostty.desktop; then
      echo "Ghostty's desktop entry changed: update modules/terminal-fallback.nix" >&2
      exit 1
    fi
  '';
in
{
  config = lib.mkIf (builtins.elem "ghostty" hostSoftwarePackages) {
    environment.systemPackages = [
      (lib.hiPrio (pkgs.symlinkJoin { name = "ghostty-with-console-fallback"; paths = [ wrapper desktopEntry ]; }))
    ];
  };
}
