# Veyon classroom management: service, keys and base configuration.
#
# GNOME runs on Wayland (GNOME 49 dropped X11 session support).
# Veyon's built-in x11vnc cannot capture a Wayland compositor, so we use
# the "External VNC Server" plugin that delegates screen capture to
# gnome-remote-desktop (grd).  grd uses PipeWire + the Wayland screen-cast
# API, then exposes the framebuffer over VNC on port 5900.
#
# gnome-remote-desktop is patched in our overlay to allow multiple
# concurrent VNC connections (upstream limits it to one).
#
# - Public key deployed to all PCs for key-file authentication
# - Private key must be placed manually where needed (not managed by Nix)
# - Classroom/PC layout is configured via Veyon Configurator or veyon-cli
{ pkgs, lib, labSettings, ... }:

let
  # Veyon authentication key base directories.
  # Veyon resolves keys as: BaseDir/<role>/key
  # e.g. /etc/veyon/keys/public  +  teacher/key
  publicKeyBaseDir = "/etc/veyon/keys/public";
  privateKeyBaseDir = "/etc/veyon/keys/private";
  veyonPublicKeyFile = ../veyon-public-key.pem;
  hasVeyonPublicKey = builtins.pathExists veyonPublicKeyFile;

  # VNC password used between veyon-service and gnome-remote-desktop.
  # Both sides must agree on this value.  Since this is LAN-only
  # and already protected by Veyon's RSA key authentication, a simple
  # password is sufficient.
  vncPassword = "veyon";

  # The password encrypted with Veyon's hardcoded RSA-OAEP key.
  # Generated with:
  #   echo -n "veyon" | openssl pkeyutl -encrypt -pubin \
  #     -inkey <(openssl rsa -in /tmp/veyon-hardcoded-key.pem -pubout) \
  #     -pkeyopt rsa_padding_mode:oaep -pkeyopt rsa_oaep_md:sha1 | xxd -p | tr -d '\n'
  # The hardcoded key is embedded in libveyon-core.so.
  vncPasswordEncrypted = "1e44d88e4161df4d14706c39da3b14b1dba0df9ca8a6a6463663e5902bfa40a4fadd19072e5c5efd48c860e0acccffff05a684fee37aee1cedf07d90fd865cbc5ead5d44daba27260e91571e5306c2afcaab4741a781a5a030966bdb05afa1e2c1643e3b55c3b7c9024ee8ef945010879a05b252fba12100e1bb6c045e2336b6fbd9dd74cbc786a735b82eeff0b890302ed1e7117521061816b62f716de2d854c112dde6b09aa419a6c975d722c65ce6a1c988f52a7ba56c720c55fa1a6aa727bdca29dacf5196cbc7b9b3aae54cd6be0fbedb31261e44887f0cdcb22aac78c8b5c3a5e6735a3a083a535e15b12f4133131caec58ad068531f765bd01a131fe2f77c136e39d1348e551e273f85c9a04d795ce309de36b081b6b7999319360dc54e24ad48672527660d32de06ec46b2d3bb86654ea48845688b60da54644eb246b6730e75f9d6fe22f936bed036fedede388619cce640c37c15099c1330f112114cc2f21c7abb5db1e4b2229053706420ccdab2112e53f8c5056ee3d8e398c04df369429b9f1abad23c993b35f33e7894822dfc88a0ca531336fc47d4f4d48fc2c063a0e65afa97825f13485027cfa02c66e47daa2c407de5f1c1bc531b45705d17fb8d849cd47e9a24aa87938ac1fcf4e9bb20b351ae7df2440920c6a6f2fe8104759c706cd8ad19456610c515ac80dfb85cfe0517fbaf8ce1fbf300f96a7569";

  padNumber = n: if n < 10 then "0${toString n}" else toString n;

  uuidFromString = value:
    let
      hash = builtins.hashString "sha256" value;
    in
    "{${builtins.substring 0 8 hash}-${builtins.substring 8 4 hash}-${builtins.substring 12 4 hash}-${builtins.substring 16 4 hash}-${builtins.substring 20 12 hash}}";

  locationUid = uuidFromString (builtins.replaceStrings [" "] ["-"] (lib.toLower labSettings.veyonLocationName));

  hostNumbers = builtins.genList (n: n + 1) labSettings.pcCount;

  networkObjects = {
    a = [
      {
        Name = labSettings.veyonLocationName;
        Type = 2;
        Uid = locationUid;
      }
    ] ++ map (n: {
      HostAddress = "${labSettings.networkBase}.${toString n}";
      Name = "pc${padNumber n}";
      ParentUid = locationUid;
      Type = 3;
      Uid = uuidFromString "pc${padNumber n}";
    }) hostNumbers;
  };

  networkObjectsJson = builtins.toJSON networkObjects;

  networkObjectsBase64 = builtins.readFile (pkgs.runCommand "veyon-network-objects" {} ''
    printf '%s' ${lib.escapeShellArg networkObjectsJson} | ${pkgs.coreutils}/bin/base64 -w0 > $out
  '');

  # Veyon configuration (deployed as /etc/xdg/Veyon Solutions/Veyon.conf)
  # Uses the External VNC Server plugin pointing to grd on port 5900.
  veyonConf = ''
    [Authentication]
    Method=1

    [AuthenticationKeys]
    PublicKeyBaseDir=${publicKeyBaseDir}
    PrivateKeyBaseDir=${privateKeyBaseDir}

    [BuiltinDirectory]
    NetworkObjects="@@JsonValue(${networkObjectsBase64})"

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
    Plugin={67dfc1c1-8f37-4539-a298-16e74e34fd8b}

    [ExternalVncServer]
    ServerPort=5900
    Password=${vncPasswordEncrypted}
  '';
in
{
  # Install Veyon on all PCs
  environment.systemPackages = [ pkgs.veyon ];

  # Deploy the public key (world-readable) from the repo
  environment.etc = lib.mkIf hasVeyonPublicKey {
    "veyon/keys/public/teacher/key" = {
      source = veyonPublicKeyFile;
      mode = "0644";
    };
  };

  # Deploy Veyon configuration
  environment.etc."xdg/Veyon Solutions/Veyon.conf" = {
    text = veyonConf;
    mode = "0644";
  };

  # Veyon service: runs per user session.
  # On Wayland, veyon-service connects to grd's VNC (port 5900) rather
  # than capturing the screen directly.
  systemd.user.services.veyon-server = {
    description = "Veyon Service";
    wantedBy = [ "graphical-session.target" ];
    after = [ "graphical-session.target" "gnome-remote-desktop.service" ];
    serviceConfig = {
      ExecStart = "${pkgs.veyon}/bin/veyon-service";
      Restart = "on-failure";
      RestartSec = 5;
    };
    path = [ pkgs.veyon ];
  };

  # gnome-remote-desktop VNC configuration via dconf.
  # Enable VNC backend, set password authentication, mirror the primary screen.
  services.desktopManager.gnome.extraGSettingsOverrides = ''
    [org.gnome.desktop.remote-desktop.vnc]
    enable=true
    view-only=true
    auth-method='password'
    screen-share-mode='mirror-primary'

    [org.gnome.desktop.remote-desktop.rdp]
    enable=false

    [org.gnome.desktop.remote-desktop.rdp.headless]
    enable=false
  '';

  # Ensure the remote-desktop schemas are visible to gsettings.
  services.desktopManager.gnome.extraGSettingsOverridePackages = [ pkgs.gnome-remote-desktop ];

  # gnome-remote-desktop user service: set the VNC password via environment
  # variable (GNOME Keyring is disabled in common.nix), and ensure it's enabled
  # at session start.
  systemd.user.services.gnome-remote-desktop = {
    wantedBy = [ "gnome-session.target" ];
    serviceConfig.Environment = [
      "GNOME_REMOTE_DESKTOP_TEST_VNC_PASSWORD=${vncPassword}"
    ];
  };

  # Group for Veyon Master access (private key ownership)
  users.groups.veyon-master = {};

  # Open the Veyon and VNC ports.
  # The firewall is disabled in common.nix but we declare the ports
  # explicitly for documentation / defense-in-depth.
  networking.firewall.allowedTCPPorts = [ 11100 5900 ];
}
