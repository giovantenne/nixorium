{ system ? builtins.currentSystem }:
let
  lock = builtins.fromJSON (builtins.readFile ../flake.lock);
  nixpkgsNode = lock.nodes.${lock.nodes.${lock.root}.inputs.nixpkgs};
  nixpkgs = builtins.fetchTree nixpkgsNode.locked;
  pkgs = import nixpkgs.outPath {
    inherit system;
  };
  configSchemaTest = import ./eval-lab-config.nix {
    inherit (pkgs) lib;
  };
  settingsSchemaTest = import ./eval-lab-settings.nix {
    inherit (pkgs) lib;
  };
  softwareSchemaTest = import ./eval-lab-software.nix {
    inherit (pkgs) lib;
    inherit pkgs;
  };
in
{
  config-schema = assert configSchemaTest; pkgs.runCommand "nixorium-config-schema-test" {} ''
    touch "$out"
  '';
  settings-schema = assert settingsSchemaTest; pkgs.runCommand "nixorium-settings-schema-test" {} ''
    touch "$out"
  '';
  software-schema = assert softwareSchemaTest; pkgs.runCommand "nixorium-software-schema-test" {} ''
    touch "$out"
  '';
  nixorium = pkgs.callPackage ../pkgs/nixorium.nix {};
  go-shell = pkgs.mkShell {
    packages = [ pkgs.go ];
  };
}
