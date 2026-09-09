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
      mkLabTest = import ./tests/mk-lab.nix {
        inherit mkLab;
        deploymentSelf = self;
        labConfig = import ./lab-config.nix;
      };
    in
    defaultLab // {
      lib = {
        inherit mkLab;
        configSchemaVersion = 2;
      };
      checks.${system} = {
        config-schema = assert configSchemaTest; pkgs.runCommand "nixorium-config-schema-test" {} ''
          touch "$out"
        '';
        mk-lab = assert mkLabTest; pkgs.runCommand "nixorium-mk-lab-test" {} ''
          touch "$out"
        '';
      };
      templates.site = {
        path = ./templates/site;
        description = "Private Nixorium deployment repository";
      };
    };
}
