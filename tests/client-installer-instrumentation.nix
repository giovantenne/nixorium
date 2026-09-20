{ labSettings, lib, modulesPath, pkgs, ... }:

let
  profileFixture = pkgs.writeText "nixorium-test-profile-fixture" "managed profile fixture\n";
in
{
  imports = [ (modulesPath + "/testing/test-instrumentation.nix") ];

  boot.initrd.availableKernelModules = [
    "virtio_blk"
    "virtio_pci"
  ];

  # Reproduce the ownership hazard that site activation used to have: GNU
  # install assigns the final file to the user but creates missing parent
  # directories as root. The production ownership activation must reconcile
  # every managed tree on the first boot of a newly installed client.
  system.activationScripts.siteHomeProfile = {
    deps = [ "createHomeTemplates" ];
    text = ''
      for user in admin ${labSettings.teacherUser}; do
        install -D -o "$user" -g users -m 0644 ${profileFixture} \
          "/home/$user/.config/Code/User/globalStorage/fixture"
        install -D -o "$user" -g users -m 0644 ${profileFixture} \
          "/home/$user/.vscode/extensions/fixture"
        install -D -o "$user" -g users -m 0644 ${profileFixture} \
          "/home/$user/.local/npm/fixture"
      done

      student_template="/var/lib/home-template/${labSettings.studentUser}"
      install -D -m 0644 ${profileFixture} \
        "$student_template/.config/Code/User/globalStorage/fixture"
      install -D -m 0644 ${profileFixture} \
        "$student_template/.vscode/extensions/fixture"
      install -D -m 0644 ${profileFixture} \
        "$student_template/.local/npm/fixture"
      chown -R ${labSettings.studentUser}:users "$student_template"
    '';
  };
  system.activationScripts.nixoriumUserHomeOwnership.deps =
    lib.mkAfter [ "siteHomeProfile" ];
}
