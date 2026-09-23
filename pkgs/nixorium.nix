{ buildGoModule, lib, makeWrapper, openssh, openssl, whois }:

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
    ];
  };

  vendorHash = "sha256-wtmeoJiq2DEa/ZHY1JUFIs4A87Vi0khXNQEgmT0MkMA=";
  subPackages = [ "cmd/nixorium" ];

  nativeBuildInputs = [ makeWrapper ];

  postFixup = ''
    wrapProgram "$out/bin/nixorium" \
      --prefix PATH : ${lib.makeBinPath [ openssh openssl whois ]}
  '';

  ldflags = [ "-s" "-w" "-X main.nixoriumVersion=${version}" ];

  meta = {
    description = "Terminal management interface for Nixorium laboratories";
    mainProgram = "nixorium";
  };
}
