{ config, isController, labSettings, nixoriumPackage, lib, pkgs, ... }:
let
  cfg = config.services.nixorium;
  migrateKnownHosts = pkgs.writeShellApplication {
    name = "nixorium-migrate-known-hosts";
    runtimeInputs = [ pkgs.coreutils pkgs.diffutils pkgs.util-linux ];
    text = builtins.readFile ../scripts/migrate-known-hosts.sh;
  };
  # controllerApply: only the controller apply job may proceed while a client
  # update record exists (it changes only the controller, owner decision
  # 2026-10-15, ADR 0023 amendment) and may cross a USB reservation whose disk
  # work completed or never started. Every other job keeps both refusals.
  operationGate = controllerApply: ''
    COORDINATION_DIRECTORY=/var/lib/nixorium/coordination
    COORDINATION_LOCK="$COORDINATION_DIRECTORY/operation.lock"
    USB_RESERVATION="$COORDINATION_DIRECTORY/usb-reservation.json"
    [[ -d "$COORDINATION_DIRECTORY" && ! -L "$COORDINATION_DIRECTORY" \
        && "$(stat -c '%U:%G:%a' "$COORDINATION_DIRECTORY")" == root:nixorium-operations:770 ]] \
      || fail "managed operation coordination directory is unsafe"
    [[ -f "$COORDINATION_LOCK" && ! -L "$COORDINATION_LOCK" \
        && "$(stat -c '%U:%G:%a' "$COORDINATION_LOCK")" == root:nixorium-operations:660 ]] \
      || fail "managed operation lock is unsafe"
    check_usb_reservation() {
      ${lib.optionalString (!controllerApply) ''
        [[ ! -e "$COORDINATION_DIRECTORY/deployment-pending.json" && ! -L "$COORDINATION_DIRECTORY/deployment-pending.json" ]] \
          || fail "an unfinished client deployment requires reviewed recovery before another operation"
      ''}
      if [[ -e "$USB_RESERVATION" || -L "$USB_RESERVATION" ]]; then
        ${if controllerApply then ''
          runuser -u admin -- ${nixoriumPackage}/bin/nixorium-remote-worker --controller-rebuild-check \
            || fail "a USB disk installation is in progress or unresolved; controller apply cannot interrupt it"
        '' else ''
          fail "a USB installation remains reserved; reconcile it before starting another operation"
        ''}
      fi
    }
    check_usb_reservation
    ${lib.optionalString controllerApply ''
      if [[ -e "$USB_RESERVATION" || -L "$USB_RESERVATION" ]]; then
        # A completed client no longer depends on the controller cache/worker,
        # and an installation that never started has not touched its disk.
        # Keep the durable reservation and resume the worker after this job.
        USB_WORKER_PAUSED=true
        systemctl --no-ask-password stop nixorium-remote-install.service \
          || fail "could not pause the USB installation worker"
      fi
    ''}
    exec 9<>"$COORDINATION_LOCK"
    flock -n 9 \
      || fail "another Nixorium controller or client operation is already running"
    # Describe the holder for refused operations; the flock stays the lock.
    read -r -a OPERATION_OWNER_STAT < "/proc/$$/stat" || OPERATION_OWNER_STAT=()
    if [[ ''${#OPERATION_OWNER_STAT[@]} -ge 22 ]]; then
      TZ=UTC printf '{"operation":"%s","user":"%s","pid":%d,"pidStart":"%s","startedAt":"%(%Y-%m-%dT%H:%M:%SZ)T"}\n' \
        "''${0##*/}" "''${USER:-root}" "$$" "''${OPERATION_OWNER_STAT[21]}" -1 > "$COORDINATION_LOCK" || true
    fi
    check_usb_reservation
  '';
  installSecrets = pkgs.writeShellApplication {
    name = "nixorium-install-secrets";
    runtimeInputs = [ pkgs.coreutils pkgs.diffutils pkgs.git pkgs.nix pkgs.util-linux nixoriumPackage ];
    text = ''
      REPOSITORY=${lib.escapeShellArg cfg.deploymentPath}

      fail() {
        echo "Error: $*" >&2
        exit 1
      }

      ${operationGate false}

      [[ -d "$REPOSITORY" && ! -L "$REPOSITORY" ]] \
        || fail "configured deployment path is not a real directory"
      [[ -f "$REPOSITORY/flake.nix" && -d "$REPOSITORY/.git" ]] \
        || fail "configured deployment path is not a Git Flake"
      [[ "$(stat -c '%U' "$REPOSITORY")" == admin ]] \
        || fail "deployment repository must be owned by admin"

      nixorium setup keys --verify-only --repo "$REPOSITORY" --json >/dev/null \
        || fail "deployment key correspondence verification failed"

      install_checked() {
        local source_path="$1"
        local target_path="$2"
        local mode="$3"
        local owner="$4"
        local group="$5"

        [[ -f "$source_path" && ! -L "$source_path" ]] \
          || fail "key source is not a regular non-symlink file: $source_path"
        if [[ -L "$target_path" ]]; then
          fail "refusing symlink destination: $target_path"
        fi
        if [[ -e "$target_path" ]]; then
          [[ -f "$target_path" ]] || fail "key destination is not a regular file: $target_path"
          cmp -s "$source_path" "$target_path" \
            || fail "refusing to replace different key material at $target_path"
        fi
        install -m "$mode" -o "$owner" -g "$group" "$source_path" "$target_path"
      }

      install -d -m 0700 -o admin -g users /home/admin/.ssh
      install -d -m 0700 -o root -g root /var/lib/nixorium/keys

      install_checked "$REPOSITORY/admin-ssh" /home/admin/.ssh/id_ed25519 0600 admin users
      install_checked "$REPOSITORY/keys/admin-ssh.pub" /home/admin/.ssh/id_ed25519.pub 0644 admin users
      install_checked "$REPOSITORY/secret-key" /var/lib/nixorium/keys/harmonia-secret-key 0600 root root
    '';
  };
  applyController = pkgs.writeShellApplication {
    name = "nixorium-apply-controller";
    runtimeInputs = [
      pkgs.coreutils
      pkgs.diffutils
      pkgs.gawk
      pkgs.git
      pkgs.iproute2
      pkgs.jq
      pkgs.networkmanager
      pkgs.nix
      pkgs.systemd
      pkgs.util-linux
      nixoriumPackage
    ];
    # The script lives in scripts/apply-controller.sh; Nix fills in the deployment path and
    # the operation gate.
    text = builtins.replaceStrings
      [ "@REPOSITORY@" "@OPERATION_GATE@" ]
      [ (lib.escapeShellArg cfg.deploymentPath) (operationGate true) ]
      (builtins.readFile ../scripts/apply-controller.sh);
  };
  preparePxe = pkgs.writeShellApplication {
    name = "nixorium-prepare-pxe";
    runtimeInputs = [
      pkgs.coreutils
      pkgs.findutils
      pkgs.curl
      pkgs.gawk
      pkgs.git
      pkgs.gnugrep
      pkgs.iproute2
      pkgs.jq
      pkgs.nix
      pkgs.util-linux
      nixoriumPackage
    ];
    # The script lives in scripts/prepare-pxe.sh; Nix fills in the deployment path and
    # the operation gate.
    text = builtins.replaceStrings
      [ "@REPOSITORY@" "@OPERATION_GATE@" "@PXE_BUILD@" ]
      [ (lib.escapeShellArg cfg.deploymentPath) (operationGate false) (builtins.readFile ../scripts/lib/pxe-build.sh) ]
      (builtins.readFile ../scripts/prepare-pxe.sh);
  };
  restartCache = pkgs.writeShellApplication {
    name = "nixorium-restart-cache";
    runtimeInputs = [ pkgs.coreutils pkgs.systemd pkgs.util-linux ];
    text = ''
      fail() {
        echo "Error: $*" >&2
        exit 1
      }

      ${operationGate false}
      systemctl --no-ask-password restart harmonia.service
    '';
  };

  # Opened from the desktop: runs Nixorium in a terminal and keeps the window
  # open after an error so the message can be read.
  nixoriumSession = pkgs.writeShellApplication {
    name = "nixorium-session";
    runtimeInputs = [ pkgs.coreutils nixoriumPackage ];
    text = ''
      GROUPS_OF_USER=" $(id -nG) "
      if [[ "$GROUPS_OF_USER" != *" nixorium-operations "* && "$GROUPS_OF_USER" != *" nixorium-classroom "* ]]; then
        echo "Nixorium is for the laboratory administrator and the teacher."
        read -r -p "Press Enter to close. " _ || true
        exit 0
      fi
      if ! nixorium "$@"; then
        echo
        read -r -p "Nixorium stopped with an error. Press Enter to close. " _ || true
      fi
    '';
  };
  # The dashboard's terminal window carries this class, so the Nixorium icon
  # brings an open dashboard forward instead of starting another.
  dashboardWindowClass = "org.nixorium.Nixorium";
  nixoriumOpen = pkgs.writeShellApplication {
    name = "nixorium-open";
    # GNOME Console is the launcher's own fallback, so it is there even where
    # the desktop leaves it out.
    runtimeInputs = [ pkgs.coreutils pkgs.gnugrep pkgs.gnome-console ];
    text = ''
      SESSION=${nixoriumSession}/bin/nixorium-session
      if command -v ghostty >/dev/null; then
        # Maximized: below 32 rows the dashboard draws its next-step buttons
        # without their border, and a default-size window is about 26.
        # Ghostty needs OpenGL 4.3. Without it (an old graphics card, a virtual
        # machine with 3D acceleration) it either closes at once ("OpenGL
        # version is too old") or keeps an empty window ("create a GL
        # context"), and it exits with status 0 either way. Watch its first
        # seconds and open another terminal instead.
        log=$(mktemp -t nixorium-open.XXXXXX)
        ghostty --class=${dashboardWindowClass} --title=Nixorium --maximize=true -e "$SESSION" 2> >(tee "$log" >&2) &
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
      fi
      if command -v kgx >/dev/null; then exec kgx -- "$SESSION"; fi
      if command -v gnome-terminal >/dev/null; then exec gnome-terminal --title=Nixorium -- "$SESSION"; fi
      if command -v xterm >/dev/null; then exec xterm -T Nixorium -e "$SESSION"; fi
      echo "No terminal application is installed; open a terminal and run: nixorium" >&2
      exit 1
    '';
  };
  nixoriumLauncher = pkgs.makeDesktopItem {
    name = "nixorium";
    desktopName = "Nixorium";
    comment = "Manage the laboratory and the classroom computers";
    exec = "${nixoriumOpen}/bin/nixorium-open";
    # The mark used on nixorium.org, so the dock shows the product.
    icon = "${../assets/nixorium-icon.svg}";
    categories = [ "System" "Settings" ];
    startupWMClass = dashboardWindowClass;
  };
  # The teacher's icon. The classroom view's windows carry this class
  # (cmd/nixorium/classroom_view.go), so it brings an open view forward; the
  # dashboard opens if the view cannot.
  classroomOpen = pkgs.writeShellApplication {
    name = "nixorium-classroom-open";
    text = ''
      ${nixoriumPackage}/bin/nixorium classroom-view >/dev/null || exec ${nixoriumOpen}/bin/nixorium-open
    '';
  };
  classroomLauncher = pkgs.makeDesktopItem {
    name = "nixorium-classroom";
    desktopName = "Classroom view";
    comment = "See and manage the student computers of this classroom";
    exec = "${classroomOpen}/bin/nixorium-classroom-open";
    icon = "${../assets/nixorium-icon.svg}";
    categories = [ "Education" ];
    startupWMClass = "nixorium-classroom";
  };

  # Remove this controller's old system generations reviewed under the
  # digest in the unit instance; the helper refuses when the set changed.
  cleanControllerGenerations = pkgs.writeShellApplication {
    name = "nixorium-clean-controller-generations";
    runtimeInputs = [ pkgs.coreutils pkgs.systemd pkgs.util-linux ];
    text = ''
      fail() {
        echo "Error: $*" >&2
        exit 1
      }

      [[ $# -eq 1 && "$1" =~ ^[0-9a-f]{16}$ ]] || fail "reviewed removal digest is invalid"
      ${operationGate false}
      exec /run/current-system/sw/bin/nixorium-clean-generations --apply "$1"
    '';
  };
in
{
  options.services.nixorium.deploymentPath = lib.mkOption {
    type = lib.types.strMatching "/.*";
    default = "/home/admin/nixorium-deployment";
    description = "Fixed administrator-owned private deployment used by privileged Nixorium actions";
  };

  config = lib.mkIf isController {
    system.activationScripts.nixoriumKnownHosts = {
      deps = [ "users" ];
      text = ''
        ${migrateKnownHosts}/bin/nixorium-migrate-known-hosts /home/admin/.ssh "$(id -u admin)" "$(id -g admin)"
      '';
    };
    users.groups.nixorium-operations = { };
    users.groups.nixorium-classroom = { };
    users.users.admin.extraGroups = [ "nixorium-operations" "nixorium-classroom" ];
    users.users.${labSettings.teacherUser}.extraGroups = [ "nixorium-classroom" ];

    environment.etc."nixorium/deployment-path" = {
      text = "${cfg.deploymentPath}\n";
      mode = "0444";
      user = "root";
      group = "root";
    };

    environment.systemPackages = [
      nixoriumPackage
      nixoriumLauncher
      pkgs.git
    ] ++ lib.optionals (labSettings.classroomView or true) [
      classroomLauncher
      # The classroom view's Send files opens it to choose a file or folder.
      pkgs.zenity
    ];

    # Console and SSH logins of the administrator say how to start and what
    # waits. Only file presence is checked: no evaluation, no network.
    programs.bash.loginShellInit = ''
      if [[ $- == *i* && "''${USER:-}" == admin && -t 1 ]]; then
        echo "Nixorium: manage the laboratory with 'nixorium' (or the Nixorium icon)."
        [[ -e /var/lib/nixorium/coordination/deployment-pending.json ]] \
          && echo "  Attention: a previous update of the student computers was interrupted. Open nixorium to review it."
        [[ -e /var/lib/nixorium/coordination/usb-reservation.json ]] \
          && echo "  Attention: a USB installation was not finished. Open nixorium → Installation."
        [[ -e ${lib.escapeShellArg cfg.deploymentPath}/.git/nixorium-template-reset.json ]] \
          && echo "  Attention: a reset of the configuration template was interrupted. Open nixorium to finish or undo it."
        true
      fi
    '';

    security.polkit.enable = true;
    security.polkit.extraConfig = ''
      polkit.addRule(function(action, subject) {
        var unit = action.lookup("unit");
        var verb = action.lookup("verb");
        if (action.id == "org.freedesktop.systemd1.manage-units" &&
            ((verb == "start" &&
              (unit == "nixorium-install-secrets.service" ||
               unit == "nixorium-apply-controller.service" ||
               /^nixorium-apply-controller@[0-9a-f]{40}\.service$/.test(unit) ||
               unit == "nixorium-prepare-pxe.service" ||
               unit == "nixorium-remote-install.service" ||
               unit == "nixorium-restart-cache.service" ||
               /^nixorium-clean-generations@[0-9a-f]{16}\.service$/.test(unit) ||
               unit == "nixorium-pxe-recover.service")) ||
             (unit == "nixorium-pxe.service" &&
              (verb == "start" || verb == "stop" || verb == "reset-failed")) ||
             (unit == "nixorium-pxe-network.service" &&
              (verb == "stop" || verb == "reset-failed"))) &&
            subject.isInGroup("wheel")) {
          return polkit.Result.YES;
        }
      });
    '';

    systemd.tmpfiles.rules = [
      "d /home/admin/.ssh 0700 admin users -"
      "d /var/lib/nixorium/keys 0700 root root -"
      "d /var/cache/nixorium/admin 0700 admin users -"
      "d /var/lib/nixorium/coordination 0770 root nixorium-operations -"
      "f /var/lib/nixorium/coordination/operation.lock 0660 root nixorium-operations -"
      "f /var/lib/nixorium/coordination/classroom.lock 0660 root nixorium-operations -"
      "d /var/lib/nixorium/remote-install 0700 admin users -"
      "d /var/lib/nixorium/remote-install/logs 0700 admin users -"
      "d /home/admin/.local 0700 admin users -"
      "d /home/admin/.local/state 0700 admin users -"
      "d /home/admin/.local/state/nixorium 0700 admin users -"
      "d /home/admin/.local/state/nixorium/operations 0700 admin users -"
    ];

    systemd.services.nixorium-install-secrets = {
      description = "Install verified Nixorium controller key material";
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${installSecrets}/bin/nixorium-install-secrets";
        User = "root";
        Group = "root";
        UMask = "0077";
        CacheDirectory = "nixorium";
        Environment = "XDG_CACHE_HOME=/var/cache/nixorium";
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = "read-only";
        ReadOnlyPaths = [ cfg.deploymentPath ];
        ReadWritePaths = [
          "-/home/admin/.ssh"
          "-/var/lib/nixorium/keys"
          "-/var/cache/nixorium"
          "/var/lib/nixorium/coordination"
        ];
        NoNewPrivileges = true;
        CapabilityBoundingSet = [ "CAP_CHOWN" "CAP_DAC_OVERRIDE" "CAP_FOWNER" ];
      };
    };

    systemd.services.nixorium-apply-controller = {
      description = "Build and activate the reviewed Nixorium controller configuration";
      after = [ "nixorium-install-secrets.service" ];
      # This unit executes switch-to-configuration itself. Never let that
      # switch terminate the running job before it can verify and record the
      # newly active controller system.
      restartIfChanged = false;
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${applyController}/bin/nixorium-apply-controller";
        User = "root";
        Group = "root";
        UMask = "0077";
        CacheDirectory = "nixorium";
        StateDirectory = "nixorium/controller";
        StateDirectoryMode = "0755";
        Environment = "XDG_CACHE_HOME=/var/cache/nixorium";
        PrivateTmp = true;
        # NixOS activation legitimately updates declared user homes and
        # /run/user. Keep the reviewed deployment itself read-only instead of
        # placing the entire switch-to-configuration process behind
        # ProtectHome, which also makes /run/user read-only.
        ProtectHome = false;
        ReadOnlyPaths = [ cfg.deploymentPath ];
        ReadWritePaths = [ "-/var/cache/nixorium" "/var/lib/nixorium/coordination" ];
        Nice = 10;
        IOSchedulingClass = "best-effort";
        NoNewPrivileges = true;
        TimeoutStartSec = "2h";
      };
    };

    systemd.services."nixorium-apply-controller@" = {
      description = "Build and activate reviewed Nixorium controller revision %i";
      after = [ "nixorium-install-secrets.service" ];
      # The target configuration may contain a newer management command and
      # therefore a different ExecStart store path. The active reviewed job
      # must survive that unit-file change and finish its receipt.
      restartIfChanged = false;
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${applyController}/bin/nixorium-apply-controller %i";
        User = "root";
        Group = "root";
        UMask = "0077";
        CacheDirectory = "nixorium";
        StateDirectory = "nixorium/controller";
        StateDirectoryMode = "0755";
        Environment = "XDG_CACHE_HOME=/var/cache/nixorium";
        PrivateTmp = true;
        # See the parameterless first-run unit above. Revision binding and the
        # explicit read-only deployment mount remain the security boundary.
        ProtectHome = false;
        ReadOnlyPaths = [ cfg.deploymentPath ];
        ReadWritePaths = [ "-/var/cache/nixorium" "/var/lib/nixorium/coordination" ];
        Nice = 10;
        IOSchedulingClass = "best-effort";
        NoNewPrivileges = true;
        TimeoutStartSec = "2h";
      };
    };

    systemd.services.nixorium-restart-cache = {
      description = "Restart the Nixorium binary cache";
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${restartCache}/bin/nixorium-restart-cache";
        User = "root";
        Group = "root";
        CapabilityBoundingSet = "";
        NoNewPrivileges = true;
        PrivateTmp = true;
        ProtectHome = true;
        ProtectSystem = "strict";
        ReadWritePaths = [ "/var/lib/nixorium/coordination" ];
      };
    };

    systemd.services."nixorium-clean-generations@" = {
      description = "Remove reviewed old system generations of this controller";
      restartIfChanged = false;
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${cleanControllerGenerations}/bin/nixorium-clean-controller-generations %i";
        User = "root";
        Group = "root";
        UMask = "0022";
        NoNewPrivileges = true;
        PrivateTmp = true;
        ProtectHome = true;
        Nice = 10;
        IOSchedulingClass = "idle";
        TimeoutStartSec = "1h";
      };
    };

    systemd.services.nixorium-prepare-pxe = {
      description = "Build and record Nixorium PXE artifacts and client closures";
      wants = [ "harmonia.service" "network-online.target" ];
      after = [ "harmonia.service" "network-online.target" ];
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${preparePxe}/bin/nixorium-prepare-pxe";
        User = "admin";
        Group = "users";
        UMask = "0022";
        StateDirectory = "nixorium/prepared";
        StateDirectoryMode = "0755";
        Environment = "XDG_CACHE_HOME=/var/cache/nixorium/admin";
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = "read-only";
        ReadOnlyPaths = [ cfg.deploymentPath ];
        ReadWritePaths = [
          "-/var/cache/nixorium"
          "-/var/lib/nixorium/prepared"
          "/var/lib/nixorium/coordination"
        ];
        Nice = 10;
        IOSchedulingClass = "best-effort";
        NoNewPrivileges = true;
        CapabilityBoundingSet = "";
        RestrictAddressFamilies = [ "AF_INET" "AF_INET6" "AF_NETLINK" "AF_UNIX" ];
        TimeoutStartSec = "4h";
      };
    };

    systemd.services.nixorium-remote-install = {
      description = "Coordinate reviewed Nixorium USB SSH client installations";
      path = [ pkgs.git pkgs.nix pkgs.openssh ];
      serviceConfig = {
        Type = "simple";
        ExecStart = "${nixoriumPackage}/bin/nixorium-remote-worker";
        User = "admin";
        Group = "users";
        SupplementaryGroups = [ "nixorium-operations" ];
        UMask = "0077";
        RuntimeDirectory = "nixorium/remote-install";
        RuntimeDirectoryMode = "0700";
        # Controller repair may pause a completed installation worker. Its
        # operation credentials still expire at reboot or explicit cleanup.
        RuntimeDirectoryPreserve = "yes";
        StateDirectory = "nixorium/remote-install";
        StateDirectoryMode = "0700";
        CacheDirectory = "nixorium/admin";
        CacheDirectoryMode = "0700";
        Environment = [
          "XDG_CACHE_HOME=/var/cache/nixorium/admin"
          "XDG_STATE_HOME=/home/admin/.local/state"
        ];
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = "read-only";
        ReadOnlyPaths = [ cfg.deploymentPath "/etc/nixorium/deployment-path" "/home/admin/.ssh/id_ed25519" "/home/admin/.ssh/id_ed25519.pub" ];
        ReadWritePaths = [
          "/run/nixorium/remote-install"
          "/var/lib/nixorium/remote-install"
          # Save only the optional historical verified-boot telemetry flag.
          "-/var/lib/nixorium/telemetry"
          "/var/lib/nixorium/coordination"
          # Only host trust data needs atomic replacement, never SSH keys/config.
          "/home/admin/.ssh/nixorium-known-hosts"
          "-/home/admin/.local/state/nixorium/operations"
          "-/var/cache/nixorium/admin"
        ];
        NoNewPrivileges = true;
        CapabilityBoundingSet = "";
        RestrictAddressFamilies = [ "AF_INET" "AF_INET6" "AF_NETLINK" "AF_UNIX" ];
        LimitCORE = 0;
        Restart = "on-failure";
        RestartSec = "2s";
      };
    };

    # Client updates queued for computers that were off: each pass updates
    # those that answer again together, with one ordinary update under the
    # operation gate (nixorium deploy queue run). Without a queue nothing
    # starts.
    systemd.services.nixorium-deferred-updates = {
      description = "Apply queued Nixorium client updates to computers that are back";
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];
      # It also runs while an interrupted client update waits, so that the
      # record closes itself once every computer's result is known.
      unitConfig.ConditionPathExists = [
        "|/var/lib/nixorium/coordination/deferred-updates.json"
        "|/var/lib/nixorium/coordination/deployment-pending.json"
      ];
      path = [ pkgs.git pkgs.nix pkgs.openssh pkgs.util-linux ];
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${nixoriumPackage}/bin/nixorium --repo ${lib.escapeShellArg cfg.deploymentPath} deploy queue run";
        User = "admin";
        Group = "users";
        SupplementaryGroups = [ "nixorium-operations" ];
        UMask = "0077";
        # One build (at most 6 h) and one apply (at most 2 h) per pass.
        TimeoutStartSec = "9h";
        Environment = [
          "XDG_CACHE_HOME=/var/cache/nixorium/admin"
          "XDG_STATE_HOME=/home/admin/.local/state"
        ];
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = "read-only";
        ReadOnlyPaths = [ cfg.deploymentPath "/etc/nixorium/deployment-path" "/home/admin/.ssh/id_ed25519" "/home/admin/.ssh/id_ed25519.pub" ];
        ReadWritePaths = [
          "/var/lib/nixorium/coordination"
          "/home/admin/.ssh/nixorium-known-hosts"
          "-/home/admin/.local/state/nixorium"
          "-/var/cache/nixorium/admin"
        ];
        NoNewPrivileges = true;
        CapabilityBoundingSet = "";
        RestrictAddressFamilies = [ "AF_INET" "AF_INET6" "AF_NETLINK" "AF_UNIX" ];
        LimitCORE = 0;
      };
    };
    systemd.timers.nixorium-deferred-updates = {
      wantedBy = [ "timers.target" ];
      timerConfig = {
        OnBootSec = "2min";
        OnUnitInactiveSec = "1min";
        AccuracySec = "10s";
      };
    };

    systemd.services.nixorium-classroom = {
      description = "Restricted Nixorium classroom operations worker";
      wantedBy = [ "multi-user.target" ];
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];
      path = [ pkgs.git pkgs.iputils pkgs.nix pkgs.openssh pkgs.systemd pkgs.util-linux ];
      serviceConfig = {
        Type = "simple";
        ExecStart = "${nixoriumPackage}/bin/nixorium-classroom-worker";
        User = "admin";
        Group = "nixorium-classroom";
        SupplementaryGroups = [ "nixorium-operations" ];
        UMask = "0007";
        RuntimeDirectory = "nixorium-classroom";
        RuntimeDirectoryMode = "0770";
        # Learned network card addresses for Switch on (Wake-on-LAN), kept
        # across reboots outside Git and the store; the administrator's own
        # Nixorium shares them as the same user.
        StateDirectory = "nixorium/classroom";
        StateDirectoryMode = "0700";
        Environment = [
          "XDG_CACHE_HOME=/var/cache/nixorium/admin"
          "XDG_STATE_HOME=/home/admin/.local/state"
        ];
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = "read-only";
        ReadOnlyPaths = [ cfg.deploymentPath "/etc/nixorium/deployment-path" "/home/admin/.ssh/id_ed25519" "/home/admin/.ssh/id_ed25519.pub" ];
        ReadWritePaths = [
          "/run/nixorium-classroom"
          "/var/lib/nixorium/coordination"
          "/home/admin/.ssh/nixorium-known-hosts"
          "-/home/admin/.local/state/nixorium/operations"
          "-/var/cache/nixorium/admin"
        ];
        NoNewPrivileges = true;
        CapabilityBoundingSet = "";
        RestrictAddressFamilies = [ "AF_INET" "AF_INET6" "AF_NETLINK" "AF_UNIX" ];
        LimitCORE = 0;
        Restart = "on-failure";
        RestartSec = "2s";
      };
    };

    # Scheduled switch-on and shut-down of the student computers. The
    # schedule is set from the classroom view or `nixorium schedule set` and
    # kept in the classroom service's state directory, outside Git and the
    # backup. Every minute the timer starts `nixorium schedule run` as the
    # service user; it acts only in a minute the schedule names (Wake-on-LAN,
    # or the reviewed shut-down that leaves computers in use on) and runs no
    # minute twice. Without a schedule file the service does not start.
    systemd.services.nixorium-classroom-schedule = {
      description = "Switch the student computers on and off at the scheduled times";
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];
      unitConfig.ConditionPathExists = "/var/lib/nixorium/classroom/schedule.json";
      path = [ pkgs.git pkgs.nix pkgs.openssh pkgs.systemd pkgs.util-linux ];
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${nixoriumPackage}/bin/nixorium schedule run --repo ${lib.escapeShellArg cfg.deploymentPath}";
        User = "admin";
        Group = "users";
        SupplementaryGroups = [ "nixorium-operations" ];
        UMask = "0077";
        StateDirectory = "nixorium/classroom";
        StateDirectoryMode = "0700";
        Environment = [
          "XDG_CACHE_HOME=/var/cache/nixorium/admin"
          "XDG_STATE_HOME=/home/admin/.local/state"
        ];
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = "read-only";
        ReadOnlyPaths = [ cfg.deploymentPath "/etc/nixorium/deployment-path" "/home/admin/.ssh/id_ed25519" "/home/admin/.ssh/id_ed25519.pub" ];
        ReadWritePaths = [
          "/var/lib/nixorium/coordination"
          "/home/admin/.ssh/nixorium-known-hosts"
          "-/home/admin/.local/state/nixorium/operations"
          "-/var/cache/nixorium/admin"
        ];
        NoNewPrivileges = true;
        CapabilityBoundingSet = "";
        RestrictAddressFamilies = [ "AF_INET" "AF_INET6" "AF_NETLINK" "AF_UNIX" ];
        LimitCORE = 0;
        # A run that hangs on an unreachable computer must not pile up
        # behind the next minute's.
        TimeoutStartSec = "10min";
      };
    };
    systemd.timers.nixorium-classroom-schedule = {
      wantedBy = [ "timers.target" ];
      timerConfig = {
        OnCalendar = "minutely";
        AccuracySec = "1s";
        # A minute the controller was off is not made up afterwards.
        Persistent = false;
      };
    };
  };
}
