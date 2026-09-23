{ installerBundle, clientSystem, diskoPackage, diskoRuntimePackages }:
{
  name = "nixorium-client-installer";

  nodes = {
    installer = { pkgs, ... }: {
      virtualisation = {
        additionalPaths = [ installerBundle clientSystem ];
        memorySize = 4096;
        emptyDiskImages = [ 20480 ];
        useBootLoader = true;
        useEFIBoot = true;
      };

      boot.loader.systemd-boot.enable = true;
      boot.loader.efi.canTouchEfiVariables = true;
      environment.systemPackages = [
        diskoPackage
        pkgs.iputils
        pkgs.jq
        pkgs.nix
        pkgs.nixos-install-tools
        pkgs.sudo
        pkgs.util-linux
      ] ++ diskoRuntimePackages;
      nix.settings.experimental-features = [ "nix-command" "flakes" ];
      security.sudo.wheelNeedsPassword = false;

      system.activationScripts.installClientFixture.text = ''
        install -d -m 0755 /installer
        cp -a ${installerBundle}/. /installer/
        printf '%s\n' \
          'cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16Zx7i9I7rP6rbD8=' \
          > /installer/public-key
        chmod 0644 /installer/public-key
      '';

      environment.etc."nixorium-test/run-client-installer" = {
        mode = "0755";
        text = ''
          #!${pkgs.runtimeShell}
          set -euo pipefail
          source /installer/setup.sh
          require_deployment_ready() { return 0; }
          prompt_input() {
            local _prompt="$1"
            local target_var="$2"
            IFS= read -r "$target_var"
          }
          probe_host_identity() { return 2; }
          main pc01 /dev/vdb <<'EOF'
          ERASE

          EOF
        '';
      };

      system.stateVersion = "25.11";
    };

    target = {
      virtualisation = {
        diskImage = "./empty0.qcow2";
        memorySize = 4096;
        useBootLoader = true;
        useEFIBoot = true;
      };
      system.stateVersion = "25.11";
    };
  };

  testScript = ''
    import os

    installer.start()
    installer.wait_for_unit("multi-user.target")
    installer.succeed("test -d /sys/firmware/efi")
    installer.succeed("lsblk -dnro PATH,TYPE | grep -Fx '/dev/vdb disk'")
    installer.succeed("test -e ${clientSystem}")
    installer.succeed("set -o pipefail; /etc/nixorium-test/run-client-installer 2>&1 | tee /tmp/client-installer.log")
    installer.succeed("grep -F 'DESTRUCTIVE REVIEW' /tmp/client-installer.log")
    installer.succeed("grep -F '[1/3] Partitioning /dev/vdb' /tmp/client-installer.log")
    installer.succeed("grep -F '[2/3] Installing pc01' /tmp/client-installer.log")
    installer.succeed("grep -F '[3/3] Verifying' /tmp/client-installer.log")
    installer.succeed("grep -F 'SUCCESS: pc01 is installed on /dev/vdb' /tmp/client-installer.log")
    installer.succeed("test -e /mnt/nix/var/nix/profiles/system")
    installer.succeed("lsblk -nrpo PATH,FSTYPE /dev/vdb | grep -E '/dev/vdb[12] '")
    installer.succeed("umount -R /mnt")
    installer.succeed("sync")
    installer.shutdown()

    target.state_dir = installer.state_dir
    os.environ["NIX_EFI_VARS"] = str(installer.state_dir / "installer-efi-vars.fd")
    target.start()
    target.wait_for_unit("multi-user.target")
    target.succeed("test \"$(hostname)\" = pc01")
    target.succeed("findmnt -n -o SOURCE / | grep -E '/dev/vda2|/dev/disk/by-label/nixos'")
    target.succeed("for user in admin teacher student; do home=$(getent passwd $user | cut -d: -f6); test \"$(stat -c %U:%G $home)\" = $user:users; for path in .config .config/Code/User/globalStorage .vscode .vscode/extensions .local .local/npm; do test \"$(stat -c %U:%G $home/$path)\" = $user:users; su -s /bin/sh $user -c \"test -w $home/$path\"; done; test -z \"$(find $home/.config/Code $home/.vscode/extensions $home/.local/npm -xdev ! -user $user -print -quit)\"; done")
    target.succeed("for user in admin teacher student; do uid=$(id -u $user); systemctl start user-runtime-dir@$uid.service; test \"$(stat -c %U:%G:%a /run/user/$uid)\" = $user:users:700; su -s /bin/sh $user -c \"test -w /run/user/$uid\"; done")
    target.succeed("install -d -o student -g users /home/student/.pi /home/student/.config/opencode /home/student/.local/share/opencode /home/student/.npm; for path in .pi/session.json .config/opencode/credentials.json .local/share/opencode/history.json .npm/cache; do install -o student -g users -m 0600 /dev/null /home/student/$path; done; echo keep > /home/student/lesson.txt; chown student:users /home/student/lesson.txt")
    target.succeed("systemctl restart home-reset.service")
    target.succeed("test -f /var/lib/home-snapshots/snapshot-1/lesson.txt; for path in .pi .config/opencode .local/share/opencode .local/npm .npm .opencode; do test ! -e /var/lib/home-snapshots/snapshot-1/$path; done")
    target.succeed("test ! -e /home/student/lesson.txt; test -f /home/student/.local/npm/fixture; test ! -e /home/student/.pi; test ! -e /home/student/.config/opencode; test ! -e /home/student/.local/share/opencode; test ! -e /home/student/.npm; test ! -e /home/student/.opencode")
  '';
}
