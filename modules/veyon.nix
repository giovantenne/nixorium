# Veyon classroom management: service, keys and base configuration.
#
# Veyon 4.11 provides a native PipeWire/XDG portal backend for Wayland.
# GNOME requires initial interactive screen-sharing consent. Every lab host
# uses native capture and keeps its authorization outside reset homes.
#
# - Public key deployed to all PCs for key-file authentication
# - Private key must be placed manually where needed (not managed by Nix)
# - Classroom/PC layout is configured via Veyon Configurator or veyon-cli
{ pkgs, lib, labSettings, ... }:

let
  laboratoryEnabled = (labSettings.deploymentMode or "laboratory") == "laboratory";
  veyonLocationName = "Lab";

  # Veyon authentication key base directories.
  # Veyon resolves keys as: BaseDir/<role>/key
  # e.g. /etc/veyon/keys/public  +  teacher/key
  publicKeyBaseDir = "/etc/veyon/keys/public";
  privateKeyBaseDir = "/etc/veyon/keys/private";
  veyonPublicKeyFile = labSettings.veyonPublicKeyFile;
  hasVeyonPublicKey = veyonPublicKeyFile != null && builtins.pathExists veyonPublicKeyFile;
  nativeUsers = lib.unique [ "admin" labSettings.teacherUser labSettings.studentUser ];
  # Separate per-user state, never part of student templates or snapshots.
  nativeStateRoot = "/var/lib/nixorium/veyon-session";
  vncServerPluginUid = "{3b8e5c1a-9f72-4d3e-b6a0-2c7f1e8d4b95}";

  padNumber = n: if n < 10 then "0${toString n}" else toString n;

  uuidFromString = value:
    let
      hash = builtins.hashString "sha256" value;
    in
    "{${builtins.substring 0 8 hash}-${builtins.substring 8 4 hash}-${builtins.substring 12 4 hash}-${builtins.substring 16 4 hash}-${builtins.substring 20 12 hash}}";

  locationUid = uuidFromString (builtins.replaceStrings [" "] ["-"] (lib.toLower veyonLocationName));

  hostNumbers = builtins.genList (n: n + 1) labSettings.pcCount;

  networkObjects = {
    a = [
      {
        Name = veyonLocationName;
        Type = 2;
        Uid = locationUid;
      }
    ] ++ map (n: {
      HostAddress = builtins.elemAt labSettings.clientIps (n - 1);
      Name = "pc${padNumber n}";
      ParentUid = locationUid;
      Type = 3;
      Uid = uuidFromString "pc${padNumber n}";
    }) hostNumbers;
  };

  networkObjectsJson = builtins.toJSON networkObjects;
  networkObjectsJsonFile = pkgs.writeText "veyon-network-objects.json" networkObjectsJson;

  # Generate the encoded network objects during the system build, not while
  # evaluating the NixOS module.
  veyonConfFile = pkgs.runCommand "Veyon.conf" {
    NETWORK_OBJECTS_JSON_FILE = networkObjectsJsonFile;
  } ''
    NETWORK_OBJECTS_BASE64="$(${pkgs.coreutils}/bin/base64 -w0 < "$NETWORK_OBJECTS_JSON_FILE")"

    ${pkgs.coreutils}/bin/cat > "$out" <<EOF
    [Authentication]
    Method=1

    [AuthenticationKeys]
    PublicKeyBaseDir=${publicKeyBaseDir}
    PrivateKeyBaseDir=${privateKeyBaseDir}

    [BuiltinDirectory]
    NetworkObjects="@@JsonValue($NETWORK_OBJECTS_BASE64)"

    [Network]
    PrimaryServicePort=11100

    [Service]
    Autostart=true
    Arguments=
    HideTrayIcon=true

    [Master]
    RemoteAccessImageQuality=0
    ComputerMonitoringImageQuality=2
    ComputerMonitoringUpdateInterval=1000

    [VncServer]
    Plugin=${vncServerPluginUid}

    [PipeWireVnc]
    PersistRestoreToken=true
    EOF
  '';
in
{
  # Install Veyon on all PCs
  environment.systemPackages = [ pkgs.veyon ];

  # Deploy the public key (world-readable) from the repo
  environment.etc."veyon/keys/public/teacher/key" = lib.mkIf hasVeyonPublicKey {
    source = veyonPublicKeyFile;
    mode = "0644";
  };

  # Deploy Veyon configuration
  environment.etc."xdg/Veyon Solutions/Veyon.conf" = {
    source = veyonConfFile;
    mode = "0644";
  };

  # The user session supplies the Wayland, PipeWire and portal environment.
  systemd.user.services.veyon-server = lib.mkIf laboratoryEnabled {
    description = "Veyon Service";
    wantedBy = [ "graphical-session.target" ];
    partOf = [ "graphical-session.target" ];
    after = [ "graphical-session.target" "xdg-permission-store.service" ];
    requires = [ "xdg-permission-store.service" ];
    environment.PATH = lib.mkForce "/run/wrappers/bin:${lib.makeBinPath [
      pkgs.veyon
      pkgs.coreutils
      pkgs.findutils
      pkgs.gnugrep
      pkgs.gnused
      pkgs.systemd
    ]}";
    # veyon-service reconstructs the child's environment from the login
    # session, so a service-only XDG_STATE_HOME would not reach veyon-server.
    # Link only Veyon's state directory as the unprivileged desktop user.
    preStart = lib.optionalString laboratoryEnabled ''
      STATE_HOME="''${XDG_STATE_HOME:-$HOME/.local/state}"
      TARGET="${nativeStateRoot}/$(${pkgs.coreutils}/bin/id -un)/state/veyon"
      mkdir -p "$STATE_HOME"
      if [ -L "$STATE_HOME/veyon" ]; then
        test "$(readlink "$STATE_HOME/veyon")" = "$TARGET"
      else
        if [ -d "$STATE_HOME/veyon" ]; then
          # Never discard or overwrite a previously granted authorization.
          rmdir "$STATE_HOME/veyon"
        fi
        ln -sT "$TARGET" "$STATE_HOME/veyon"
      fi
    '';
    serviceConfig = {
      ExecStart = "${pkgs.veyon}/bin/veyon-service";
      Restart = "on-failure";
      RestartSec = 5;
      UMask = "0077";
    };
  };

  # The portal holds the other half of the restore token in PermissionStore.
  # Redirect only that service, not applications' XDG_DATA_HOME. Its portal
  # grants persist together across home reset; initial approval stays explicit.
  systemd.user.services.xdg-permission-store = lib.mkIf laboratoryEnabled {
    overrideStrategy = "asDropin";
    serviceConfig = {
      Environment = [ "XDG_DATA_HOME=${nativeStateRoot}/%u/data" ];
      UMask = "0077";
    };
  };

  systemd.tmpfiles.rules = lib.optionals laboratoryEnabled (
    [ "d ${nativeStateRoot} 0711 root root - -" ]
    ++ lib.concatMap (user: [
      "d ${nativeStateRoot}/${user} 0700 ${user} users - -"
      "d ${nativeStateRoot}/${user}/state 0700 ${user} users - -"
      "d ${nativeStateRoot}/${user}/state/veyon 0700 ${user} users - -"
      "d ${nativeStateRoot}/${user}/data 0700 ${user} users - -"
    ]) nativeUsers
  );

  # Stop and mask the obsolete bridge on upgraded installations too.
  systemd.user.services.gnome-remote-desktop.enable = lib.mkIf laboratoryEnabled false;

  # Group for Veyon Master access (private key ownership)
  users.groups.veyon-master = {};

  # Nix store permissions cannot carry setuid bits. Veyon uses these narrowly
  # scoped helpers for PAM authentication and Wayland input locking.
  security.wrappers.veyon-auth-helper = {
    source = "${pkgs.veyon}/bin/veyon-auth-helper";
    owner = "root";
    group = "root";
    setuid = true;
  };

  security.wrappers.veyon-input-helper = {
    source = "${pkgs.veyon}/bin/veyon-input-helper";
    owner = "root";
    group = "root";
    setuid = true;
  };
}
