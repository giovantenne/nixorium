{
  description = "Nixorium - reproducible NixOS lab infrastructure";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
    disko = {
      url = "github:nix-community/disko";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    veyon = {
      url = "git+https://github.com/veyon/veyon.git?ref=refs/tags/v4.11.0&submodules=1";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, disko, veyon }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs {
        inherit system;
      };
      mkLab = import ./lib/mk-lab.nix {
        upstreamSelf = self;
        inherit nixpkgs;
        inherit disko;
        inherit veyon;
      };
      defaultLab = mkLab {
        deploymentSelf = self;
        labConfig = import ./lab-config.nix;
        publicKeys = {
          cache = ./public-key;
          ssh = ./id_ed25519.pub;
          veyon = ./veyon-public-key.pem;
        };
      };
      configSchemaTest = import ./tests/eval-lab-config.nix {
        inherit (nixpkgs) lib;
      };
      settingsSchemaTest = import ./tests/eval-lab-settings.nix {
        inherit (nixpkgs) lib;
      };
      softwareSchemaTest = import ./tests/eval-lab-software.nix {
        inherit (nixpkgs) lib;
        inherit pkgs;
      };
      softwarePresetSchemaTest = import ./tests/eval-software-presets.nix {
        inherit (nixpkgs) lib;
      };
      mkLabTest = import ./tests/mk-lab.nix {
        inherit mkLab;
        deploymentSelf = self;
        labConfig = import ./lab-config.nix;
      };
      managementVmTest = pkgs.testers.runNixOSTest (import ./tests/management-vm.nix {
        nixoriumPackage = defaultLab.packages.${system}.nixorium;
      });
      installerTestLab = mkLab {
        deploymentSelf = self;
        labConfig = import ./lab-config.nix;
        homeResetEphemeralPaths = [
          ".cache/opencode"
          ".config/opencode"
          ".local/share/opencode"
          ".local/npm"
          ".npm"
          ".opencode"
          ".pi"
        ];
        publicKeys = {
          cache = ./public-key;
          ssh = ./id_ed25519.pub;
          veyon = ./veyon-public-key.pem;
        };
        hostModules.pc01 = [ ./tests/client-installer-instrumentation.nix ];
      };
      clientInstallerVmTest = pkgs.testers.runNixOSTest (import ./tests/client-installer-vm.nix {
        installerBundle = installerTestLab.packages.${system}.installerBundle;
        clientSystem = installerTestLab.nixosConfigurations.pc01.config.system.build.toplevel;
        diskoPackage = disko.packages.${system}.default;
        diskoRuntimePackages = disko.lib.packages {
          disko.devices = import ./lib/disko-layout.nix {
            device = "/dev/nixorium-install-target";
            studentUser = "student";
          };
        } pkgs;
      });
      remoteInstallerDeploymentRevision = "0123456789abcdef0123456789abcdef01234567";
      remoteInstallerTestLab = mkLab {
        deploymentSelf = self;
        deploymentRevision = remoteInstallerDeploymentRevision;
        labConfig = import ./lab-config.nix;
        publicKeys = {
          cache = ./tests/fixtures/remote-vm-cache-public-key;
          ssh = ./tests/fixtures/remote-vm-admin.pub;
        };
        hostModules.pc01 = [ ./tests/client-installer-instrumentation.nix ];
      };
      remoteClientInstallerVmTest = pkgs.testers.runNixOSTest (import ./tests/remote-client-installer-vm.nix {
        remoteInstallerBundle = remoteInstallerTestLab.packages.${system}.remoteInstallerBundle;
        clientSystem = remoteInstallerTestLab.nixosConfigurations.pc01.config.system.build.toplevel;
        deploymentRevision = remoteInstallerDeploymentRevision;
      });
    in
    defaultLab // {
      lib = {
        inherit mkLab;
        controllerBootstrapVersion = 2;
        configSchemaVersion = 2;
        settingsSchemaVersion = 1;
        softwareSchemaVersion = 1;
        softwarePresetSchemaVersion = 1;
        packageBase = {
          schemaVersion = 2;
          source = "github:NixOS/nixpkgs";
          # Recommendation, not permission to evaluate a deployment-owned pin.
          channel = "nixos-26.05";
          # Read upstream's own lock: a downstream follows override must not
          # masquerade as the revision used by the upstream source tree.
          referenceRevision = let lock = builtins.fromJSON (builtins.readFile ./flake.lock);
            in lock.nodes.${lock.nodes.${lock.root}.inputs.nixpkgs}.locked.rev;
        };
        evalLabSettings = import ./lib/eval-lab-settings.nix {
          inherit (nixpkgs) lib;
        };
      };
      checks.${system} = {
        config-schema = assert configSchemaTest; pkgs.runCommand "nixorium-config-schema-test" {} ''
          touch "$out"
        '';
        settings-schema = assert settingsSchemaTest; pkgs.runCommand "nixorium-settings-schema-test" {} ''
          touch "$out"
        '';
        software-schema = assert softwareSchemaTest; pkgs.runCommand "nixorium-software-schema-test" {} ''
          touch "$out"
        '';
        software-preset-schema = assert softwarePresetSchemaTest; pkgs.runCommand "nixorium-software-preset-schema-test" {} ''
          touch "$out"
        '';
        mk-lab = assert mkLabTest; pkgs.runCommand "nixorium-mk-lab-test" {} ''
          touch "$out"
        '';
        client-installer = pkgs.runCommand "nixorium-client-installer-test" {
          nativeBuildInputs = [ pkgs.bash pkgs.coreutils pkgs.gnugrep pkgs.jq ];
        } ''
          NIXORIUM_TEST_REPO_ROOT=${self} bash ${./tests/client-installer.sh}
          NIXORIUM_TEST_REPO_ROOT=${self} bash ${./tests/client-installer-library.sh}
          NIXORIUM_TEST_REPO_ROOT=${self} \
            NIXORIUM_PLAN_VALIDATOR=${defaultLab.packages.${system}.nixorium}/bin/nixorium-remote-validator \
            bash ${./tests/remote-client-installer.sh}
          touch "$out"
        '';
        client-installer-vm = clientInstallerVmTest;
        remote-client-installer-vm = remoteClientInstallerVmTest;
        management-vm = managementVmTest;
      };
      templates.site = {
        path = ./templates/site;
        description = "Private Nixorium deployment repository";
      };
      apps.${system} = defaultLab.apps.${system} // {
        default = defaultLab.apps.${system}.nixorium;
      };
      packages.${system} = defaultLab.packages.${system} // {
        default = defaultLab.packages.${system}.nixorium;
      };
    };
}
