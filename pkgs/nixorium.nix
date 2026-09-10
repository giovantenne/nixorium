{ buildGoModule }:

buildGoModule {
  pname = "nixorium";
  version = builtins.replaceStrings [ "\n" ] [ "" ] (builtins.readFile ../VERSION);
  src = ../.;

  vendorHash = "sha256-uwBJAqN4sIepiiJf9lCDumLqfKJEowQO2tOiSWD3Fig=";
  subPackages = [ "cmd/nixorium" ];

  ldflags = [ "-s" "-w" ];

  meta = {
    description = "Terminal management interface for Nixorium laboratories";
    mainProgram = "nixorium";
  };
}
