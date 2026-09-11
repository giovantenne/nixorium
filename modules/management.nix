{ config, hostName, labSettings, nixoriumPackage, lib, pkgs, ... }:
let
  isController = hostName == labSettings.masterHostName;
  cfg = config.services.nixorium;
  installSecrets = pkgs.writeShellApplication {
    name = "nixorium-install-secrets";
    runtimeInputs = [ pkgs.coreutils pkgs.diffutils pkgs.git pkgs.nix nixoriumPackage ];
    text = ''
      REPOSITORY=${lib.escapeShellArg cfg.deploymentPath}

      fail() {
        echo "Error: $*" >&2
        exit 1
      }

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
      install -d -m 0750 -o root -g veyon-master /etc/veyon/keys/private/teacher
      install -d -m 0700 -o root -g root /var/lib/nixorium/keys

      install_checked "$REPOSITORY/admin-ssh" /home/admin/.ssh/id_ed25519 0600 admin users
      install_checked "$REPOSITORY/keys/admin-ssh.pub" /home/admin/.ssh/id_ed25519.pub 0644 admin users
      install_checked "$REPOSITORY/veyon-private-key.pem" /etc/veyon/keys/private/teacher/key 0640 root veyon-master
      install_checked "$REPOSITORY/secret-key" /var/lib/nixorium/keys/harmonia-secret-key 0600 root root
    '';
  };
  applyController = pkgs.writeShellApplication {
    name = "nixorium-apply-controller";
    runtimeInputs = [
      pkgs.coreutils
      pkgs.diffutils
      pkgs.git
      pkgs.nix
      pkgs.util-linux
      nixoriumPackage
    ];
    text = ''
      REPOSITORY=${lib.escapeShellArg cfg.deploymentPath}

      fail() {
        echo "Error: $*" >&2
        exit 1
      }

      [[ -d "$REPOSITORY" && ! -L "$REPOSITORY" ]] \
        || fail "configured deployment path is not a real directory"
      [[ -f "$REPOSITORY/flake.nix" && -d "$REPOSITORY/.git" ]] \
        || fail "configured deployment path is not a Git Flake"
      [[ "$(stat -c '%U' "$REPOSITORY")" == admin ]] \
        || fail "deployment repository must be owned by admin"
      [[ -z "$(git -c safe.directory="$REPOSITORY" -C "$REPOSITORY" status --porcelain=v1 --untracked-files=normal)" ]] \
        || fail "deployment worktree must be clean before controller apply"
      [[ -z "$(git -c safe.directory="$REPOSITORY" -C "$REPOSITORY" ls-files -- \
          secret-key admin-ssh veyon-private-key.pem)" ]] \
        || fail "private key files must not be tracked by Git"

      # Use the Git fetcher so ignored private keys are never copied into the
      # immutable Nix source tree or store during evaluation and builds.
      FLAKE_URL="git+file://$REPOSITORY"
      install -d -m 0700 -o admin -g users /var/cache/nixorium/admin
      export XDG_CACHE_HOME=/var/cache/nixorium/admin
      export NIX_CONFIG="experimental-features = nix-command flakes"

      as_admin() {
        runuser -u admin -- "$@"
      }

      as_admin nixorium config validate --repo "$REPOSITORY" --json \
        || fail "deployment configuration validation failed"
      as_admin nixorium setup keys --verify-only --repo "$REPOSITORY" --json \
        || fail "deployment key correspondence verification failed"
      [[ "$(as_admin nix eval "$FLAKE_URL#deploymentStatus.ready" --json --no-write-lock-file)" == true ]] \
        || fail "deploymentStatus.ready must be true before controller apply"

      cmp -s "$REPOSITORY/admin-ssh" /home/admin/.ssh/id_ed25519 \
        || fail "installed admin SSH key is absent or differs"
      cmp -s "$REPOSITORY/veyon-private-key.pem" /etc/veyon/keys/private/teacher/key \
        || fail "installed Veyon private key is absent or differs"
      cmp -s "$REPOSITORY/secret-key" /var/lib/nixorium/keys/harmonia-secret-key \
        || fail "installed Harmonia signing key is absent or differs"

      CONTROLLER_NAME="$(as_admin nix eval "$FLAKE_URL#labMeta.controller.name" --raw --no-write-lock-file)"
      [[ "$CONTROLLER_NAME" =~ ^pc[0-9]+$ ]] \
        || fail "evaluated controller name is invalid"

      SYSTEM_PATH="$(as_admin nix build "$FLAKE_URL#nixosConfigurations.$CONTROLLER_NAME.config.system.build.toplevel" \
        --no-write-lock-file --no-link --print-out-paths)"
      [[ "$SYSTEM_PATH" == /nix/store/* && "$SYSTEM_PATH" != *[[:space:]]* \
          && -x "$SYSTEM_PATH/bin/switch-to-configuration" ]] \
        || fail "controller build did not return one valid NixOS system closure"
      exec "$SYSTEM_PATH/bin/switch-to-configuration" switch
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
    environment.systemPackages = [
      nixoriumPackage
      pkgs.colmena
    ];

    security.polkit.enable = true;
    security.polkit.extraConfig = ''
      polkit.addRule(function(action, subject) {
        var unit = action.lookup("unit");
        if (action.id == "org.freedesktop.systemd1.manage-units" &&
            (unit == "nixorium-install-secrets.service" ||
             unit == "nixorium-apply-controller.service") &&
            action.lookup("verb") == "start" &&
            subject.isInGroup("wheel")) {
          return polkit.Result.YES;
        }
      });
    '';

    systemd.tmpfiles.rules = [
      "d /home/admin/.ssh 0700 admin users -"
      "d /etc/veyon/keys/private/teacher 0750 root veyon-master -"
      "d /var/lib/nixorium/keys 0700 root root -"
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
          "-/etc/veyon/keys/private/teacher"
          "-/var/lib/nixorium/keys"
          "-/var/cache/nixorium"
        ];
        NoNewPrivileges = true;
        CapabilityBoundingSet = [ "CAP_CHOWN" "CAP_DAC_OVERRIDE" "CAP_FOWNER" ];
      };
    };

    systemd.services.nixorium-apply-controller = {
      description = "Build and activate the reviewed Nixorium controller configuration";
      after = [ "nixorium-install-secrets.service" ];
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${applyController}/bin/nixorium-apply-controller";
        User = "root";
        Group = "root";
        UMask = "0077";
        CacheDirectory = "nixorium";
        Environment = "XDG_CACHE_HOME=/var/cache/nixorium";
        PrivateTmp = true;
        ProtectHome = "read-only";
        ReadOnlyPaths = [ cfg.deploymentPath ];
        ReadWritePaths = [ "-/var/cache/nixorium" ];
        Nice = 10;
        IOSchedulingClass = "best-effort";
        NoNewPrivileges = true;
        TimeoutStartSec = "2h";
      };
    };
  };
}
