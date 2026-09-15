{ buildGoModule, lib, makeWrapper, openssh, openssl, whois }:

buildGoModule {
  pname = "nixorium";
  version = builtins.replaceStrings [ "\n" ] [ "" ] (builtins.readFile ../VERSION);
  src = ../.;

  vendorHash = "sha256-wtmeoJiq2DEa/ZHY1JUFIs4A87Vi0khXNQEgmT0MkMA=";
  subPackages = [ "cmd/nixorium" ];

  nativeBuildInputs = [ makeWrapper ];

  postFixup = ''
    wrapProgram "$out/bin/nixorium" \
      --prefix PATH : ${lib.makeBinPath [ openssh openssl whois ]}
  '';

  ldflags = [ "-s" "-w" ];

  meta = {
    description = "Terminal management interface for Nixorium laboratories";
    mainProgram = "nixorium";
  };
}
