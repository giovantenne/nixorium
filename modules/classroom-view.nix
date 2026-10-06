{ hostName, labSettings, lib, nixoriumPackage, pkgs, ... }:
# Experimental classroom view agent (off by default). The agent runs in the
# graphical session of whoever uses a client; the controller reaches it only
# through its existing SSH access, with the fixed command below. No network
# port is opened. Capture uses Mutter's own screen cast interface, so GNOME
# shows its sharing indicator; the extension keeps it visible but not
# clickable.
let
  laboratoryEnabled = (labSettings.deploymentMode or "laboratory") == "laboratory";
  enabled = (labSettings.classroomView or true) && laboratoryEnabled
    && hostName != labSettings.masterHostName;
  extensionUuid = "nixorium-classroom@nixorium.org";
  extension = pkgs.runCommand "gnome-shell-extension-nixorium-classroom" {} ''
    install -Dm0644 ${../pkgs/gnome-shell-extension-classroom/metadata.json} \
      "$out/share/gnome-shell/extensions/${extensionUuid}/metadata.json"
    install -Dm0644 ${../pkgs/gnome-shell-extension-classroom/extension.js} \
      "$out/share/gnome-shell/extensions/${extensionUuid}/extension.js"
  '';
  gstreamerPlugins = map lib.getLib [
    pkgs.gst_all_1.gstreamer
    pkgs.gst_all_1.gst-plugins-base
    pkgs.gst_all_1.gst-plugins-good
    pkgs.pipewire
  ];
  connect = pkgs.writeShellApplication {
    name = "nixorium-classroom-connect";
    runtimeInputs = [ pkgs.systemd ];
    text = ''
      exec ${nixoriumPackage}/bin/nixorium-classroom-agent connect
    '';
  };
in
{
  config = lib.mkIf enabled {
    environment.systemPackages = [ connect extension ];

    systemd.user.services.nixorium-classroom-agent = {
      description = "Nixorium classroom view agent";
      wantedBy = [ "graphical-session.target" ];
      partOf = [ "graphical-session.target" ];
      after = [ "graphical-session.target" ];
      path = [ pkgs.gst_all_1.gstreamer.bin ];
      environment.GST_PLUGIN_SYSTEM_PATH_1_0 = lib.makeSearchPath "lib/gstreamer-1.0" gstreamerPlugins;
      serviceConfig = {
        ExecStart = "${nixoriumPackage}/bin/nixorium-classroom-agent serve";
        Restart = "on-failure";
        RestartSec = 2;
      };
    };

    # After an update replaces the running system (never at boot), record it
    # for the classroom extension, which tells the person at the computer.
    # A restart is needed when the kernel, initrd, modules or systemd change,
    # as NixOS itself checks. /run is emptied by the restart.
    systemd.tmpfiles.rules = [ "d /run/nixorium 0755 root root -" ];
    system.activationScripts.nixoriumUpdateNotice = {
      deps = [ "specialfs" ];
      text = ''
        if [ -e /run/current-system ] && [ "$(readlink -f /run/current-system)" != "$(readlink -f "$systemConfig")" ]; then
          restart=false
          for part in kernel initrd kernel-modules systemd; do
            if [ "$(readlink -f "/run/booted-system/$part")" != "$(readlink -f "$systemConfig/$part")" ]; then
              restart=true
            fi
          done
          ${pkgs.coreutils}/bin/install -d -m 0755 /run/nixorium
          printf '{"updatedAt":%s,"restart":%s}\n' "$(${pkgs.coreutils}/bin/date +%s)" "$restart" \
            > /run/nixorium/update-notice.json.new
          ${pkgs.coreutils}/bin/chmod 0644 /run/nixorium/update-notice.json.new
          ${pkgs.coreutils}/bin/mv -f /run/nixorium/update-notice.json.new /run/nixorium/update-notice.json
        fi
      '';
    };

    # Enable the indicator extension in every session; gnome-extensions keeps
    # other enabled extensions.
    systemd.user.services.nixorium-classroom-extension = {
      description = "Enable the Nixorium classroom GNOME Shell extension";
      wantedBy = [ "graphical-session.target" ];
      partOf = [ "graphical-session.target" ];
      after = [ "graphical-session.target" ];
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${pkgs.gnome-shell}/bin/gnome-extensions enable ${extensionUuid}";
        Restart = "on-failure";
        RestartSec = 3;
      };
    };
  };
}
