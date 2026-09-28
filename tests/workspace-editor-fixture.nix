{ pkgs, editor, extension }:
let
  runner = pkgs.writeText "workspace-editor-tests.cjs" (builtins.readFile ./workspace-editor-tests.cjs);
  harness = pkgs.runCommand "workspace-editor-test-extension" {} ''
    mkdir -p "$out"
    cp ${pkgs.writeText "package.json" (builtins.toJSON {
      name = "workspace-runtime-test";
      publisher = "nixorium-test";
      version = "1.0.0";
      engines.vscode = "*";
      main = "./extension.cjs";
    })} "$out/package.json"
    printf 'exports.activate = () => {};\n' > "$out/extension.cjs"
  '';
in
{
  module = {
    virtualisation.useBootLoader = true;
    boot.loader.grub.enable = true;
    boot.loader.timeout = 1;
    services.dbus.enable = true;
    # The test driver's serial channel still works; editor/plugin traffic is
    # confined to loopback throughout all three booted generations.
    networking.firewall.enable = false;
    networking.nftables = {
      enable = true;
      tables.editor-test = {
        family = "inet";
        content = ''
          chain output {
            type filter hook output priority 0; policy drop;
            oifname "lo" accept
          }
        '';
      };
    };
    environment.systemPackages = [ pkgs.jq pkgs.nix ];
    systemd.services.workspace-editor-check = {
      after = [ "home-reset.service" "display-manager.service" "nftables.service" ];
      requires = [ "home-reset.service" "nftables.service" ];
      serviceConfig = {
        Type = "oneshot";
        User = "student";
        Group = "users";
        RuntimeDirectory = "workspace-editor-test";
        RuntimeDirectoryMode = "0700";
        EnvironmentFile = "/run/editor-expectations";
        TimeoutStartSec = 120;
      };
      environment = {
        XDG_RUNTIME_DIR = "/run/workspace-editor-test";
        NIXORIUM_TEST_EDITOR_VERSION = editor.version;
        NIXORIUM_TEST_EXTENSION_VERSION = extension.version;
        NIXORIUM_TEST_EXTENSION_PATH = "${extension}/share/vscode/extensions/ritwickdey.liveserver";
      };
      path = [ pkgs.coreutils pkgs.dbus ];
      script = ''
        mkdir -p /home/student/editor-project
        printf '<!doctype html><html><head></head><body>nixorium-editor-proof</body></html>\n' > /home/student/editor-project/index.html
        exec ${pkgs.dbus}/bin/dbus-run-session -- ${pkgs.xvfb-run}/bin/xvfb-run -a \
          ${editor}/bin/code --wait --verbose --disable-gpu --disable-workspace-trust \
          --skip-welcome --skip-release-notes \
          --extensionDevelopmentPath=${harness} --extensionTestsPath=${runner} \
          /home/student/editor-project
      '';
    };
  };
  testScript = ''
    managed.start(allow_reboot=True)
    managed.wait_for_unit("display-manager.service")
    managed.succeed("readlink -f /run/current-system > /root/editor-base-system")

    def check_editor(size, extension):
        managed.succeed(f"printf 'NIXORIUM_TEST_FONT_SIZE={size}\\nNIXORIUM_TEST_EXPECT_EXTENSION={extension}\\n' > /run/editor-expectations")
        try:
            managed.succeed("systemctl start workspace-editor-check.service", timeout=130)
            managed.succeed("test -f /home/student/editor-proof.json")
        except Exception:
            print(managed.succeed("journalctl -u workspace-editor-check.service --no-pager"))
            raise
        proof = managed.succeed("cat /home/student/editor-proof.json")
        print("Editor runtime evidence: " + proof)
        managed.succeed("jq -e '.student and .updatesDisabled and .settingsEditable' /home/student/editor-proof.json")
        managed.succeed("nft list table inet editor-test | grep -q 'policy drop'")

    def switch_generation(name):
        managed.succeed(f"target=$(cat /root/editor-base-system)/specialisation/{name}; nix-env -p /nix/var/nix/profiles/system --set $target; $target/bin/switch-to-configuration switch", timeout=120)

    check_editor(15, 1)
    managed.succeed("systemd-run --unit=student-session-fixture --uid=student /run/current-system/sw/bin/sleep infinity")
    managed.wait_for_unit("student-session-fixture.service")
    managed.succeed("cp /home/student/.config/Code/User/settings.json /root/editor-session-settings; echo preserve-session > /home/student/session-sentinel")
    switch_generation("updated")
    managed.succeed("cmp /root/editor-session-settings /home/student/.config/Code/User/settings.json; grep -qx preserve-session /home/student/session-sentinel; systemctl is-active student-session-fixture.service")
    managed.reboot()
    managed.wait_for_unit("display-manager.service")
    managed.succeed("test ! -e /home/student/session-sentinel; test ! -e /home/student/editor-proof.json")
    check_editor(19, 1)
    switch_generation("without-extension")
    managed.succeed("test -L /home/student/.vscode/extensions/ritwickdey.liveserver")
    managed.reboot()
    managed.wait_for_unit("display-manager.service")
    managed.succeed("test ! -e /home/student/.vscode/extensions/ritwickdey.liveserver")
    check_editor(19, 0)
    managed.succeed("test ! -e /var/lib/home-snapshots/.workspace-reset/pending.json")
  '';
}
