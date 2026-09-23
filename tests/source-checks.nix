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
  softwarePresetSchemaTest = import ./eval-software-presets.nix {
    inherit (pkgs) lib;
  };
  nixoriumPackage = pkgs.callPackage ../pkgs/nixorium.nix {};
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
  software-preset-schema = assert softwarePresetSchemaTest; pkgs.runCommand "nixorium-software-preset-schema-test" {} ''
    touch "$out"
  '';
  nixorium = nixoriumPackage;
  # Compile once from code only; inspect checkout guidance at runtime.
  guidance-check = nixoriumPackage.overrideAttrs {
    pname = "nixorium-guidance-check";
    doCheck = false;
    buildPhase = ''
      runHook preBuild
      go test -tags guidance -c -o guidance-check ./cmd/nixorium
      runHook postBuild
    '';
    installPhase = ''
      install -D -m 0755 guidance-check "$out/bin/guidance-check"
    '';
    postFixup = "";
  };
  go-shell = pkgs.mkShell {
    packages = [ pkgs.go ];
  };
  security-shell = pkgs.mkShell {
    packages = [ pkgs.go pkgs.go-tools pkgs.govulncheck pkgs.actionlint ];
  };
}
