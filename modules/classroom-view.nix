{ hostName, labSettings, lib, nixoriumPackage, pkgs, ... }:
# Experimental classroom view agent (off by default). The agent runs in the
# graphical session of whoever uses a client; the controller reaches it only
# through its existing SSH access, with the fixed command below. No network
# port is opened. Capture uses Mutter's own screen cast interface, so GNOME
# shows its sharing indicator; the extension keeps it visible but not
# clickable.
#
# The student session has a terminal, so both user units are made hard to
# stop by accident or on purpose: they refuse a manual stop, the agent
# restarts after one second however it ends, and neither runs out of start
# attempts. This is a classroom aid, not a security boundary: a student can
# still mask a unit or freeze the process, and loses that at the next boot,
# when the home directory is reset. While the agent is down the teacher's
# card says "Monitoring stopped on this PC". NixOS does not restart a unit
# that refuses a manual stop when an update changes it, so the new agent
# starts at the next sign-in.
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
      unitConfig = {
        RefuseManualStop = true;
        StartLimitIntervalSec = 0;
      };
      serviceConfig = {
        ExecStart = "${nixoriumPackage}/bin/nixorium-classroom-agent serve";
        Restart = "always";
        RestartSec = 1;
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
    # other enabled extensions. systemd rejects Restart=always for a oneshot
    # unit, so a failed enable is retried (on-failure) after one second.
    systemd.user.services.nixorium-classroom-extension = {
      description = "Enable the Nixorium classroom GNOME Shell extension";
      wantedBy = [ "graphical-session.target" ];
      partOf = [ "graphical-session.target" ];
      after = [ "graphical-session.target" ];
      unitConfig = {
        RefuseManualStop = true;
        StartLimitIntervalSec = 0;
      };
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${pkgs.gnome-shell}/bin/gnome-extensions enable ${extensionUuid}";
        Restart = "on-failure";
        RestartSec = 1;
      };
    };
  };
}
