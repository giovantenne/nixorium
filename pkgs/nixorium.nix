{ buildGoModule, bubblewrap, callPackage, git, lib, makeWrapper, openssh, openssl, whois }:

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
      ../docs/assistant-tools-v1.json
      ../tests/lab-settings-validation-cases.json
      ../tests/software-preset-validation-cases.json
      ../tests/workspace-validation-cases.json
      ../tests/usb-completed-session.json
    ];
  };

  vendorHash = "sha256-QSzcOVyA8bAvpE3EQvfSGOCggwPrHg5dOJdShAeW8jU=";
  subPackages = [ "cmd/nixorium" "cmd/nixorium-classroom-agent" "cmd/nixorium-classroom-worker" "cmd/nixorium-home-reset" "cmd/nixorium-host-diagnostics" "cmd/nixorium-npm-resolver" "cmd/nixorium-remote-validator" "cmd/nixorium-remote-worker" ];

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
      --set NIXORIUM_NPM_BWRAP ${bubblewrap}/bin/bwrap \
      --set NIXORIUM_NPM_RESOLVER "$out/bin/nixorium-npm-resolver" \
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
