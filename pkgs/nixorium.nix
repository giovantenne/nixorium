{ buildGoModule, lib, makeWrapper, openssh, openssl, whois }:

buildGoModule {
  pname = "nixorium";
  version = builtins.replaceStrings [ "\n" ] [ "" ] (builtins.readFile ../VERSION);
  src = ../.;

  vendorHash = "sha256-Nu2JZWW1BtqB2grTWJqgxMDpNAJNWomYBi6vKGU8D4w=";
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
