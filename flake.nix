{
  description = "NixOS system configuration";

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
    in
    defaultLab // {
      lib = {
        inherit mkLab;
        configSchemaVersion = 1;
      };
      templates.site = {
        path = ./templates/site;
        description = "Private deployment repository for a NixOS lab";
      };
    };
}
