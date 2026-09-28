{ buildGoModule, git, lib, makeWrapper, openssh, openssl, whois }:

let
  version = builtins.replaceStrings [ "\n" ] [ "" ] (builtins.readFile ../VERSION);
in
buildGoModule {
  pname = "nixorium";
  inherit version;
  src = lib.fileset.toSource {
    root = ../.;
    fileset = lib.fileset.unions [
      ../VERSION
      ../go.mod
      ../go.sum
      ../cmd
      ../internal
      ../templates/site/flake.nix
      ../templates/site/lab-settings.json
      ../tests/lab-settings-validation-cases.json
      ../tests/software-preset-validation-cases.json
      ../tests/workspace-validation-cases.json
      ../tests/usb-completed-session.json
    ];
  };

  vendorHash = "sha256-yeHLh7vZyFssX4AsvMfvxiX0j3/rK5VNpR36iMLoENU=";
  subPackages = [ "cmd/nixorium" "cmd/nixorium-classroom-worker" "cmd/nixorium-remote-validator" "cmd/nixorium-remote-worker" ];

  nativeBuildInputs = [ makeWrapper ];
  nativeCheckInputs = [ git ];

  # subPackages limits installed commands, not the unit-test coverage.
  checkPhase = ''
    runHook preCheck
    export GOFLAGS=''${GOFLAGS//-trimpath/}
    go test ./...
    runHook postCheck
  '';

  postFixup = ''
    wrapProgram "$out/bin/nixorium" \
      --prefix PATH : ${lib.makeBinPath [ openssh openssl whois ]} \
      --suffix PATH : ${lib.makeBinPath [ git ]}
  '';

  ldflags = [ "-s" "-w" "-X main.nixoriumVersion=${version}" ];

  meta = {
    description = "Terminal management interface for Nixorium laboratories";
    mainProgram = "nixorium";
  };
}
