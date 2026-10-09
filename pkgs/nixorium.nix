{ buildGoModule, callPackage, git, lib, makeWrapper, openssh, openssl, whois }:

let
  go = callPackage ./go.nix { };
  version = builtins.replaceStrings [ "\n" ] [ "" ] (builtins.readFile ../VERSION);
in
(buildGoModule.override { inherit go; }) {
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
      ../templates/site/software-presets.json
      ../templates/site/workspace-profile.example.json
      ../templates/site/workspace-profile.programming.example.json
      ../tests/lab-settings-validation-cases.json
      ../tests/software-preset-validation-cases.json
      ../tests/workspace-validation-cases.json
      ../tests/usb-completed-session.json
    ];
  };

  vendorHash = "sha256-mxFfTQ+K15B25rXhg0E5chEGVosTDPLxhG+eDc2sU9o=";
  subPackages = [ "cmd/nixorium" "cmd/nixorium-classroom-agent" "cmd/nixorium-classroom-worker" "cmd/nixorium-home-reset" "cmd/nixorium-remote-validator" "cmd/nixorium-remote-worker" ];

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

  ldflags = [ "-s" "-w" "-X main.nixoriumVersion=${version}" "-X main.agentVersion=${version}" ];

  passthru.go = go;

  meta = {
    description = "Terminal management interface for Nixorium laboratories";
    mainProgram = "nixorium";
  };
}
