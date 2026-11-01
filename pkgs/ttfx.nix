{ fetchFromGitHub, lib, rustPlatform }:

# Terminal text effects for the lab screensaver: a Rust port of
# terminaltexteffects that uses a fraction of its CPU. Packaged here until
# nixpkgs ships it; then the screensaver uses nixpkgs' ttfx instead.
rustPlatform.buildRustPackage rec {
  pname = "ttfx";
  version = "0.5.0";
  src = fetchFromGitHub {
    owner = "omacom-io";
    repo = "ttfx";
    rev = "v${version}";
    hash = "sha256-ZeWRyo9zturjRcH23SDgFOKoPOSY6nGMFzGeJAoDapk=";
  };
  cargoHash = "sha256-ntoj5bmAa9U2+3K1UX6HL0t6MjfCYQNe5LuiuNJ/CnY=";
  # No binary cache has it, so every controller compiles it, again after each
  # system and packages update. Upstream's release profile (LTO, one codegen
  # unit) and the test build made that about 4 minutes on a 4-processor
  # controller; without them it is under a quarter of that, for a binary
  # 0.5 MB larger that animates text just as well.
  env = {
    CARGO_PROFILE_RELEASE_LTO = "false";
    CARGO_PROFILE_RELEASE_CODEGEN_UNITS = "16";
  };
  doCheck = false;
  meta = {
    description = "Terminal text effects, a Rust port of terminaltexteffects";
    homepage = "https://github.com/omacom-io/ttfx";
    license = lib.licenses.mit;
    mainProgram = "ttfx";
  };
}
