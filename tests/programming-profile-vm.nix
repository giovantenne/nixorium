{ pkgs }:
# Qualifies the template's Programming profile in a disposable VM: the managed
# reset restores its editor extensions, each one activates in the pinned
# VS Code without network access, the toolchains run as the student, and the
# teaching MySQL server stays on loopback and is emptied at boot only. The
# local Apache server runs PHP from the student's public_html as the student.
# It does not exercise GNOME, MySQL Workbench or a physical laboratory.
let
  editorPkgs = import pkgs.path { inherit (pkgs.stdenv.hostPlatform) system; config.allowUnfree = true; };
  inherit (editorPkgs) lib;
  presets = (builtins.fromJSON (builtins.readFile ../templates/site/software-presets.json)).presets;
  programming = builtins.head (builtins.filter (preset: preset.id == "programming") presets);
  # Graphical and unrelated applications stay out of the small VM closure.
  vmPackages = builtins.filter (package: !(builtins.elem package [
    "chromium" "libreoffice-qt" "vlc" "ghostty" "mysql-workbench" "opencode" "pi-coding-agent"
    "hunspell" "hunspellDicts.en_US" "hunspellDicts.it_IT" "liberation_ttf"
    "python3Packages.terminaltexteffects"
  ])) programming.packages;
  resolution = import ../lib/resolve-workspace-profile.nix { inherit lib; pkgs = editorPkgs; } {
    profileJSON = builtins.readFile ../templates/site/workspace-profile.programming.example.json;
    catalog = import ../templates/site/workspace-catalog.nix;
    controllerName = "pc99";
    clientNames = [];
    hostPackages.pc99 = programming.packages ++ [ "gnome-shell" "gnomeExtensions.dash-to-dock" "nautilus" "gnome-text-editor" ];
  };
  seed = import ../lib/build-workspace-home.nix { inherit lib; pkgs = editorPkgs; } {
    inherit resolution;
    labSettings = { studentGitName = "Student"; studentGitEmail = "student@example.invalid"; };
  };
  nixoriumPackage = pkgs.callPackage ../pkgs/nixorium.nix {};
  runner = pkgs.writeText "programming-profile-tests.cjs" (builtins.readFile ./programming-profile-tests.cjs);
  harness = pkgs.runCommand "programming-profile-test-extension" {} ''
    mkdir -p "$out"
    cp ${pkgs.writeText "package.json" (builtins.toJSON {
      name = "programming-profile-test";
      publisher = "nixorium-test";
      version = "1.0.0";
      engines.vscode = "*";
      main = "./extension.cjs";
    })} "$out/package.json"
    printf 'exports.activate = () => {};\n' > "$out/extension.cjs"
  '';
  extensions = resolution.effective.vscode.extensions;
in
editorPkgs.testers.runNixOSTest {
  name = "nixorium-programming-profile";
  nodes.managed = { lib, ... }: {
    imports = [ ../modules/workspace-reset.nix ../templates/site/modules/development.nix ];
    _module.args = {
      labSettings = { studentUser = "student"; teacherUser = "teacher"; };
      hostSoftwarePackages = vmPackages;
      homeResetEphemeralPaths = [ ".local/npm" ];
      workspaceSeed = seed;
      workspaceWallpapers = [];
      inherit nixoriumPackage;
    };
    virtualisation = {
      memorySize = 6144;
      cores = 4;
      diskSize = 8192;
      emptyDiskImages = [ 512 ];
      useBootLoader = true;
    };
    boot.loader.grub.enable = true;
    boot.loader.timeout = 1;
    users.users.student = { isNormalUser = true; uid = 2000; group = "users"; };
    users.users.teacher = { isNormalUser = true; uid = 2001; group = "users"; };
    users.groups.veyon-master = {};
    environment.systemPackages = [ editorPkgs.btrfs-progs editorPkgs.jq editorPkgs.iproute2 editorPkgs.curl ]
      ++ map (package: lib.attrByPath (lib.splitString "." package) null editorPkgs) vmPackages;
    # Editor and extensions must work offline, as on a lab client.
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
    # Only this disposable VM disk is formatted, and only on the first boot.
    systemd.services.prepare-reset-fixture = {
      requiredBy = [ "home-reset.service" ];
      before = [ "home-reset.service" ];
      after = [ "local-fs.target" ];
      path = [ editorPkgs.btrfs-progs editorPkgs.util-linux editorPkgs.coreutils ];
      serviceConfig = { Type = "oneshot"; RemainAfterExit = true; };
      script = ''
        if ! blkid /dev/vdb; then
          mkfs.btrfs -f /dev/vdb
          mkdir -p /mnt/reset-fixture
          mount /dev/vdb /mnt/reset-fixture
          btrfs subvolume create /mnt/reset-fixture/@home-student
          btrfs subvolume create /mnt/reset-fixture/@snapshots
          umount /mnt/reset-fixture
        fi
        mkdir -p /var/lib/home-snapshots
        mount -o subvol=@home-student /dev/vdb /home/student
        mount -o subvol=@snapshots /dev/vdb /var/lib/home-snapshots
        chown student:users /home/student
        chmod 0700 /home/student
      '';
      restartIfChanged = false;
    };
    systemd.services.display-manager = {
      wantedBy = [ "multi-user.target" ];
      after = [ "systemd-user-sessions.service" ];
      serviceConfig = { Type = "oneshot"; RemainAfterExit = true; };
      script = "test ! -e /run/nologin";
      restartIfChanged = false;
    };
    systemd.services.programming-editor-check = {
      after = [ "home-reset.service" "display-manager.service" "nftables.service" ];
      requires = [ "home-reset.service" "nftables.service" ];
      serviceConfig = {
        Type = "oneshot";
        User = "student";
        Group = "users";
        RuntimeDirectory = "programming-editor-test";
        RuntimeDirectoryMode = "0700";
        TimeoutStartSec = 1500;
      };
      environment = {
        XDG_RUNTIME_DIR = "/run/programming-editor-test";
        NIXORIUM_TEST_EXTENSIONS = builtins.toJSON extensions;
      };
      path = [ editorPkgs.coreutils editorPkgs.dbus "/run/current-system/sw" ];
      script = ''
        mkdir -p /home/student/editor-project
        printf 'print("hi")\n' > /home/student/editor-project/a.py
        printf 'public class A { public static void main(String[] a) {} }\n' > /home/student/editor-project/A.java
        printf '#include <iostream>\nint main(){std::cout<<1;}\n' > /home/student/editor-project/a.cpp
        printf '<?php echo 1;\n' > /home/student/editor-project/a.php
        exec ${editorPkgs.dbus}/bin/dbus-run-session -- ${editorPkgs.xvfb-run}/bin/xvfb-run -a \
          ${editorPkgs.vscode}/bin/code --wait --verbose --disable-gpu --disable-workspace-trust \
          --skip-welcome --skip-release-notes \
          --extensionDevelopmentPath=${harness} --extensionTestsPath=${runner} \
          /home/student/editor-project
      '';
    };
    system.stateVersion = "26.05";
  };
  testScript = ''
    import base64
    import json

    managed.start(allow_reboot=True)
    managed.wait_for_unit("display-manager.service")
    managed.succeed("test -f /var/lib/home-snapshots/.workspace-reset/success.json")
    # Linked payloads, except debuggers that write into their own folder.
    managed.succeed("test -L /home/student/.vscode/extensions/ms-python.vscode-pylance")
    managed.succeed("test -d /home/student/.vscode/extensions/ms-python.debugpy; test ! -L /home/student/.vscode/extensions/ms-python.debugpy")
    managed.succeed("test \"$(stat -c %U /home/student/.vscode/extensions/ms-python.debugpy)\" = student")
    managed.succeed("jq -e '.\"password-store\" == \"basic\"' /home/student/.vscode/argv.json")

    # Toolchains as the student.
    managed.succeed("runuser -u student -- bash -lc 'cd /tmp && printf \"#include <iostream>\\nint main(){std::cout<<42;}\\n\" > t.cpp && g++ -g t.cpp -o t && test \"$(./t)\" = 42'")
    managed.succeed("runuser -u student -- bash -lc 'gdb --version && make --version && cmake --version && javac -version && mvn -v && python3 --version && node --version'")
    managed.succeed("runuser -u student -- php -m | grep -cx xdebug")
    managed.succeed("runuser -u student -- php -m | grep -cx mysqli")

    # Teaching database: loopback only, course-material credentials.
    managed.wait_for_unit("mysql.service")
    managed.succeed("test \"$(ss -ltn | grep -E ':(3306|33060) ' | grep -vc 127.0.0.1)\" = 0")
    managed.succeed("runuser -u student -- php -r '$c = new mysqli(\"localhost\", \"root\", \"\"); $c->query(\"CREATE DATABASE scuola\"); $c->query(\"CREATE TABLE scuola.t (id INT)\"); $c->query(\"INSERT INTO scuola.t VALUES (7)\"); exit($c->query(\"SELECT id FROM scuola.t\")->fetch_row()[0] == 7 ? 0 : 1);'")
    managed.succeed("runuser -u student -- php -r '$p = new PDO(\"mysql:host=127.0.0.1;port=3306\", \"root\", \"\"); exit($p->query(\"SELECT COUNT(*) FROM scuola.t\")->fetchColumn() == 1 ? 0 : 1);'")
    # A rebuild-time tmpfiles pass must not empty the running server.
    managed.succeed("systemd-tmpfiles --create --remove")
    managed.succeed("runuser -u student -- mysql -u root -e 'SHOW DATABASES' | grep -cx scuola")

    # Local web server: the student's folder, PHP with MySQL, .htaccess
    # rewrites and listings, on loopback only and running as the student.
    managed.wait_for_unit("httpd.service")
    managed.succeed("test \"$(stat -c %U:%a /home/student/public_html)\" = student:755")
    managed.succeed("test \"$(ps -o user= -C httpd | sort -u)\" = student")
    managed.succeed("test \"$(ss -ltn | grep -E ':80 ' | grep -vcE '127.0.0.1:80|\\[::1\\]:80')\" = 0")
    managed.succeed("runuser -u student -- mkdir /home/student/public_html/sito")
    php_page = '<?php $c = new mysqli("localhost", "root", ""); echo $c->query("SELECT id FROM scuola.t")->fetch_row()[0]; file_put_contents(__DIR__ . "/scritto.txt", "ok");'
    htaccess = "RewriteEngine On\nRewriteRule ^bello$ index.php [L]\n"
    for name, content in (("index.php", php_page), (".htaccess", htaccess)):
        encoded = base64.b64encode(content.encode()).decode()
        managed.succeed(f"echo {encoded} | base64 -d | runuser -u student -- tee /home/student/public_html/sito/{name} >/dev/null")
    managed.succeed("test \"$(curl -fsS http://localhost/sito/)\" = 7")
    managed.succeed("test \"$(curl -fsS http://127.0.0.1/sito/bello)\" = 7")
    managed.succeed("test \"$(stat -c %U /home/student/public_html/sito/scritto.txt)\" = student")
    managed.succeed("curl -fsS http://localhost/ | grep -q sito/")

    try:
        managed.succeed("systemctl start programming-editor-check.service", timeout=1600)
    except Exception:
        print(managed.succeed("journalctl -u programming-editor-check.service --no-pager | tail -150"))
        raise
    proof = json.loads(managed.succeed("cat /home/student/editor-proof.json"))
    print("Editor runtime evidence: " + json.dumps(proof))
    inactive = {name: state for name, state in proof["result"].items() if not state.startswith("active")}
    assert not inactive, f"extensions did not activate: {inactive}"
    assert len(proof["result"]) == ${toString (builtins.length extensions)}
    assert proof["telemetry"] == "off" and proof["updateMode"] == "none" and proof["autoUpdate"] is False

    # Boot empties the databases and restores a clean home.
    managed.reboot()
    managed.wait_for_unit("display-manager.service")
    managed.wait_for_unit("mysql.service")
    managed.fail("runuser -u student -- mysql -u root -e 'SHOW DATABASES' | grep -cx scuola")
    managed.succeed("runuser -u student -- mysql -u root -e 'SELECT 1'")
    managed.wait_for_unit("httpd.service")
    managed.succeed("test -d /home/student/public_html; test -z \"$(ls -A /home/student/public_html)\"")
    managed.succeed("test ! -e /home/student/editor-proof.json")
  '';
}
