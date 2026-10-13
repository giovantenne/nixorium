{ pkgs }:
let
  inherit (pkgs) lib;
  profile = {
    schemaVersion = 1;
    directories = [ "Documents/empty" ];
    files = [
      { destination = "Documents/lesson.bin"; source = "imports/0e65a074a920df375483f2139a86aad84b3932e03a5f08f013ef142de1d008a9"; }
      { destination = ".config/ghostty/config"; text = "theme = dark\n"; }
      { destination = "bin/lesson"; text = "#!/bin/sh\nexit 0\n"; executable = true; }
    ];
  };
  home = import ../lib/build-workspace-home.nix { inherit lib pkgs; } {
    resolution = { effective.schemaVersion = 1; extensions = []; };
    labSettings = { studentGitName = "Student"; studentGitEmail = "student@example.invalid"; };
    homeFilesProfile = profile;
    homeFilesRoot = ./fixtures/home-files;
  };
in
pkgs.runCommand "nixorium-home-files-seed-check" { nativeBuildInputs = [ pkgs.jq ]; } ''
  cmp ${./fixtures/home-files/imports/0e65a074a920df375483f2139a86aad84b3932e03a5f08f013ef142de1d008a9} ${home}/home/Documents/lesson.bin
  grep -qx 'theme = dark' ${home}/home/.config/ghostty/config
  test -d ${home}/home/Documents/empty
  test -x ${home}/home/bin/lesson
  test ! -x ${home}/home/Documents/lesson.bin
  jq -e '.homeFiles | length == 3' ${home}/manifest.json
  jq -e '.homeFiles[] | select(.destination == "Documents/lesson.bin") | .sha256 == "0e65a074a920df375483f2139a86aad84b3932e03a5f08f013ef142de1d008a9" and .size == 23 and .executable == false' ${home}/manifest.json
  touch "$out"
''
