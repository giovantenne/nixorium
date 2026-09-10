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
      mkLabTest = import ./tests/mk-lab.nix {
        inherit mkLab;
        deploymentSelf = self;
        labConfig = import ./lab-config.nix;
      };
      managementVmTest = pkgs.testers.runNixOSTest (import ./tests/management-vm.nix {
        nixoriumPackage = defaultLab.packages.${system}.nixorium;
      });
    in
    defaultLab // {
      lib = {
        inherit mkLab;
        configSchemaVersion = 2;
        settingsSchemaVersion = 1;
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
        mk-lab = assert mkLabTest; pkgs.runCommand "nixorium-mk-lab-test" {} ''
          touch "$out"
        '';
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
