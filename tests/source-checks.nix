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
  documentationGenerator = nixoriumPackage.overrideAttrs (_: {
    pname = "nixorium-docs";
    subPackages = [ "cmd/nixorium-docs" ];
    doCheck = false;
    ldflags = [];
    postFixup = "";
  });
  documentationSource = pkgs.lib.fileset.toSource {
    root = ../.;
    fileset = ../docs/tui-gallery.md;
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
  software-preset-schema = assert softwarePresetSchemaTest; pkgs.runCommand "nixorium-software-preset-schema-test" {} ''
    touch "$out"
  '';
  desktop-profile = import ./desktop-profile.nix { inherit pkgs; };
  nixorium = nixoriumPackage;
  nixorium-runtime = pkgs.runCommand "nixorium-runtime-test" {} ''
    mkdir -p repository "$TMPDIR/home"
    ${pkgs.git}/bin/git -C repository init -q
    ${pkgs.git}/bin/git -C repository config user.name Test
    ${pkgs.git}/bin/git -C repository config user.email test@example.invalid
    echo test > repository/README
    echo '{ outputs = { self }: {}; }' > repository/flake.nix
    ${pkgs.git}/bin/git -C repository add README flake.nix
    ${pkgs.git}/bin/git -C repository commit -qm initial
    ${pkgs.coreutils}/bin/env -i \
      HOME="$TMPDIR/home" \
      PATH=/nonexistent \
      ${nixoriumPackage}/bin/nixorium git review \
        --repo "$PWD/repository" \
        --json > report.json
    ${pkgs.jq}/bin/jq -e \
      '.operation == "git-review" and .state == "clean"' \
      report.json >/dev/null
    touch "$out"
  '';
  documentation-check = pkgs.runCommand "nixorium-documentation-check" {
    nativeBuildInputs = [ documentationGenerator ];
  } ''
    cp -R ${documentationSource} source
    chmod -R u+w source
    nixorium-docs --check --repo source
    touch "$out"
  '';
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
  network-shell = pkgs.mkShell {
    packages = [ pkgs.nftables pkgs.iproute2 pkgs.util-linux pkgs.python3 pkgs.nix ];
  };
  internet-management-vm-tcg = pkgs.testers.runNixOSTest (import ./management-vm.nix {
    inherit nixoriumPackage;
    useKVM = false;
    internetOnly = true;
  });
  go-shell = pkgs.mkShell {
    packages = [ pkgs.go ];
  };
  security-shell = pkgs.mkShell {
    packages = [ pkgs.go pkgs.go-tools pkgs.govulncheck pkgs.actionlint ];
  };
}
